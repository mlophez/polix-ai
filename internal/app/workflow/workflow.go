// Package workflow is the domain of polix.yaml: for each neutral event, an ordered list of entries, each with
// optional `when` and `unless` conditions and an action with one verb (run, inject or block), plus the variables,
// the base branches, the protected files, the forbidden commands, the loop guard and the runners of the checks of
// the rules. The other shortcuts of the section guardrails are expanded into entries when the file is parsed.
//
//	version: 1
//	vars:
//	  key: { branch: "^([A-Z][A-Z0-9]+-[0-9]+)" }
//	on:
//	  pre_commit:
//	    - when: { changed: "src/**" }
//	      unless: { finished: polix:reviewer }
//	      action:
//	        block: "Lanza el subagente polix:reviewer sobre el diff actual"
//
// The vocabulary is closed: Valid rejects unknown events, conditions, verbs and keys, so a typo never turns into
// a rule that silently stops applying, and a name of the old vocabulary is reported with its new spelling.
// Reading the file is the job of the adapters: Parse takes its bytes.
package workflow

import (
	"fmt"
	"regexp"
	"slices"
	"sort"
	"strings"

	"github.com/mlophez/polix-ai/internal/platform/semver"
)

// Workflow is polix.yaml.
type Workflow struct {
	Version int
	// Requires is the oldest polix that knows every option the file uses (`requires: 0.2.0`). An older polix
	// reads it before anything else and asks to be updated instead of reporting the options it does not know.
	Requires string
	// Base names the branches a change may merge into (base_branch), for `polix rules check --changed`; the hooks
	// always compare `changed` with HEAD.
	Base           []string
	MaxStopRetries int // loop guard of every block on stop and subagent_stop (max_stop_retries)
	// ProtectedFiles are the path globs no agent writes, the main session included (guardrails.protected_files):
	// a protection the hook applies before the entries of on, also to the files git ignores.
	ProtectedFiles []string
	// ForbiddenCommands are the commands no agent runs, wherever they come in a line
	// (guardrails.forbidden_commands): a protection the hook applies before the entries of on, in file order.
	ForbiddenCommands []ForbiddenCommand
	Vars              map[string]Var
	// On holds the entries of each event: those written in `on` and those the shortcuts of guardrails
	// protected_branches, format_on_edit and run_before_done expand into.
	On       map[string][]Entry
	Rules    Rules    // the folder of the rules and how their checks run
	Plans    Folder   // the folder of the plans
	Reports  Folder   // the folder of the reports
	Guidance Guidance // the built-in instructions injected at session start
	// Weights are the factors of the risk score per path glob (risk.weights), in file order: the first glob that
	// matches a file gives its weight, and a file no glob matches weighs 1.
	Weights []Weight
	// EventLines is the line of each event of `on` in the file, for messages.
	EventLines map[string]int
}

// Var extracts a variable from the branch name: the first group of the regular expression, or the whole match.
type Var struct {
	Branch string
	Keys   []string // the keys the file used, so Valid rejects unknown ones
}

// ForbiddenCommand is an item of guardrails.forbidden_commands: a command no agent runs.
type ForbiddenCommand struct {
	Pattern string // the glob of a command, matched against each command of a line
	Reason  string // the message of the denial; empty for the default one
	Line    int    // the line of the item in the file, for messages and the log
}

// Invalid is the error of a value with problems, one per line; the list is the error itself, so a caller gets
// it back with errors.As.
type Invalid []string

// validator collects the problems of one workflow, or of one of its values when w is nil (no vars nor event
// known: the problems that depend on them are left to the workflow).
type validator struct {
	w        *Workflow
	problems []string
	lines    []int // the line of each problem, 0 for the whole file
}

// File is the workflow, relative to the repository root.
const File = "polix.yaml"

// DefaultMaxStopRetries is the loop guard of stop and subagent_stop when the workflow does not say.
const DefaultMaxStopRetries = 3

// Neutral events, the same for every agent.
const (
	Start        = "start"
	Prompt       = "prompt"
	PreEdit      = "pre_edit"
	PostEdit     = "post_edit"
	PreCommand   = "pre_command"
	PostCommand  = "post_command"
	PreCommit    = "pre_commit"
	SubagentStop = "subagent_stop"
	Stop         = "stop" // the main session ends its turn; a subagent ends with subagent_stop
)

// Verbs of an action: the action of an entry holds exactly one.
const (
	VerbRun    = "run"
	VerbInject = "inject"
	VerbBlock  = "block"
)

// Events lists every neutral event in lifecycle order.
var Events = []string{Start, Prompt, PreEdit, PostEdit, PreCommand, PostCommand, PreCommit, SubagentStop, Stop}

// Actions are the verbs an action can hold, exactly one per entry.
var Actions = []string{VerbRun, VerbInject, VerbBlock}

// Conditions a `when` or `unless` can use.
var Conditions = []string{"branch", "var", "path", "changed", "file", "contains", "command", "writes", "exit", "actor",
	"finished", "not", "any", "changed_lines", "changed_files", "risk_score"}

// Measures are the conditions that measure the uncommitted diff, over the files of the nearest `changed`.
var Measures = []string{"changed_lines", "changed_files", "risk_score"}

// Builtins are the placeholders every string may use; a variable cannot take their names.
var Builtins = []string{"path", "command", "branch", "changed_lines", "changed_files", "risk_score"}

// conditionEvents limits the conditions that only make sense on some events; the rest hold on any event.
var conditionEvents = map[string][]string{
	"path":    {PreEdit, PostEdit},
	"command": {PreCommand, PostCommand, PreCommit},
	"writes":  {PreCommand, PostCommand, PreCommit},
	"exit":    {PostCommand},
}

// Informed are the events that already happened when polix hears of them: a block cannot stop them, and a failing
// run only reaches the AI as context.
var Informed = []string{Start, PostEdit, PostCommand}

// renamedEvents maps the events of the old vocabulary to their new name.
var renamedEvents = map[string]string{"session_start": Start, "turn_stop": Stop}

// renamed holds, per place of the file, the names of the old vocabulary and the hint of their new spelling, which
// the validator, or Parse for a top-level key (place "top"), appends to the problem of the unknown name.
var renamed = map[string]map[string]string{
	"top": {
		"protected":       "use guardrails.protected_files",
		"base":            "use base_branch",
		"max_stop_blocks": "use max_stop_retries",
	},
	"key": {
		"if":      "use when",
		"why":     "remove it, or keep the text as a YAML comment",
		"mode":    "remove it: a warning is an action: { inject: ... }, a failing run on post_edit or post_command already reaches the AI as context, and audit has no replacement",
		"deny":    "use action: { block: <message> }",
		"require": "use action: { block: <message> }",
		"allow":   "write the exception as a not in when, or in unless",
		"run":     "move it into action: { run: ... }",
		"context": "use action: { inject: ... }",
		"record":  "remove it: polix observes the end of a subagent by itself, so use finished: <subagent> in when or unless; next to a run, a run that passed is not run again on the same content",
	},
	"action":    {"deny": "use block", "require": "use block", "context": "use inject"},
	"condition": {"done": "use finished: <subagent>, which holds once that subagent ended on the same files; for the fact of a run, remove it: a run that passed is not run again on the same content"},
}

// oldActions are the keys of the old vocabulary that were the action of an entry: an entry that has one is told
// how to rewrite it, not that it has no action.
var oldActions = []string{"deny", "require", "allow", "run", "context", "record"}

// renamedProblem finds, in a problem of the validator or of Parse, the unknown name it reports with its new
// spelling: the place (key, event or condition), the name and, for a key of the action, " in action". A top-level
// key is reported as a key.
var renamedProblem = regexp.MustCompile(`unknown (key|event|condition) "([^"]*)"( in action)?: `)

// placeholder finds the {name} placeholders of a string.
var placeholder = regexp.MustCompile(`\{([A-Za-z_][A-Za-z0-9_]*)\}`)

func (p Invalid) Error() string { return strings.Join(p, "\n") }

// Valid returns every problem of the workflow joined in one Invalid error, or nil when it is valid.
func (w Workflow) Valid() error {
	if p := w.Problems(); len(p) > 0 {
		return Invalid(p)
	}
	return nil
}

// Problems returns every problem of the workflow, one per line as `polix config check` prints them, in the order of the
// file (see byLine); an empty list means it is valid.
func (w Workflow) Problems() []string {
	v := &validator{w: &w}
	v.workflow(w)
	return v.byLine()
}

// workflow reports the problems of the workflow w.
func (v *validator) workflow(w Workflow) {
	if w.Version != 1 {
		v.add(0, "version must be 1, got %d", w.Version)
	}
	if w.Requires != "" && !semver.Valid(w.Requires) {
		v.add(0, "requires must be a version of polix like 0.2.0, got %q", w.Requires)
	}
	if w.MaxStopRetries < 0 {
		v.add(0, "max_stop_retries must be positive, got %d", w.MaxStopRetries)
	}
	for _, b := range w.Base {
		if strings.TrimSpace(b) == "" {
			v.add(0, "base_branch: a branch name cannot be empty")
		}
	}
	for _, g := range w.ProtectedFiles {
		if strings.TrimSpace(g) == "" {
			v.add(0, "guardrails.%s: a path glob cannot be empty", ShortcutProtectedFiles)
		}
	}
	for _, f := range w.ForbiddenCommands {
		if err := f.Valid(); err != nil {
			v.add(f.Line, "%v", err)
		}
	}
	names := make([]string, 0, len(w.Vars))
	for name := range w.Vars {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		v.variable(name, w.Vars[name])
	}
	events := make([]string, 0, len(w.On))
	for ev := range w.On {
		events = append(events, ev)
	}
	sort.Strings(events)
	for _, ev := range events {
		known := ev
		if !contains(Events, ev) {
			renamedTo, ok := renamedEvents[ev]
			if !ok {
				v.add(w.EventLines[ev], "unknown event %q (events: %s)", ev, strings.Join(Events, ", "))
				continue
			}
			// The entries of a renamed event are checked as those of its new name, so every hint comes at once.
			v.add(w.EventLines[ev], "unknown event %q: use %s", ev, renamedTo)
			known = renamedTo
		}
		for _, e := range w.On[ev] {
			v.entry(known, e)
		}
	}
	v.rules(w.Rules)
	v.folder("plans", w.Plans)
	v.folder("reports", w.Reports)
	v.guidance(w.Guidance)
	for _, wt := range w.Weights {
		if err := wt.Valid(); err != nil {
			v.add(0, "risk.weights: %v", err)
		}
	}
}

// ObservedSubagents returns the subagent types the `finished` conditions of the workflow name, as written, sorted
// and without repeats: the subagents whose end polix writes to its log.
func (w Workflow) ObservedSubagents() []string {
	var out []string
	for _, entries := range w.On {
		for _, e := range entries {
			out = appendFinished(out, e.When)
			out = appendFinished(out, e.Unless)
		}
	}
	sort.Strings(out)
	return slices.Compact(out)
}

// InjectsOnce reports whether the workflow has a once-only inject: an inject without always on an event other than
// start, which always injects. Its injects are due again when a session starts or a subagent finishes, so polix
// then writes those events to its log.
func (w Workflow) InjectsOnce() bool {
	for ev, entries := range w.On {
		if ev == Start {
			continue
		}
		for _, e := range entries {
			if e.Action.Inject != nil && !e.Action.Inject.Always {
				return true
			}
		}
	}
	return false
}

// Valid tells why the variable is not valid: an unknown key, no branch or a branch that does not compile.
func (d Var) Valid() error {
	v := &validator{}
	v.variable("", d)
	if len(v.problems) > 0 {
		return Invalid(v.problems)
	}
	return nil
}

// Valid tells why the forbidden command is not valid: it has no pattern.
func (f ForbiddenCommand) Valid() error {
	if strings.TrimSpace(f.Pattern) == "" {
		return fmt.Errorf("guardrails.%s: an item needs a pattern", ShortcutForbiddenCommands)
	}
	return nil
}

func (v *validator) add(line int, format string, args ...any) {
	where := File
	if line > 0 {
		where = fmt.Sprintf("%s:%d", File, line)
	}
	v.problems = append(v.problems, where+": "+fmt.Sprintf(format, args...))
	v.lines = append(v.lines, line)
}

// byLine returns the problems sorted by their line, so the report follows the file: those of the whole file (line
// 0) first, and those of one line in the order they were found.
func (v *validator) byLine() []string {
	order := make([]int, len(v.problems))
	for i := range order {
		order[i] = i
	}
	sort.SliceStable(order, func(a, b int) bool { return v.lines[order[a]] < v.lines[order[b]] })
	out := make([]string, len(order))
	for i, j := range order {
		out[i] = v.problems[j]
	}
	return out
}

// unknownKeys reports the keys of a map that are not in allowed.
func (v *validator) unknownKeys(line int, what string, keys, allowed []string) {
	for _, k := range keys {
		if !contains(allowed, k) {
			v.add(line, "unknown key %q in %s (%s)", k, what, strings.Join(allowed, ", "))
		}
	}
}

// placeholders reports the placeholders of s that are neither built in nor a variable, which would otherwise
// stay in the text as is. Without a workflow the variables are unknown and nothing is reported.
func (v *validator) placeholders(line int, s string) {
	if v.w == nil {
		return
	}
	for _, m := range placeholder.FindAllStringSubmatch(s, -1) {
		if name := m[1]; !contains(Builtins, name) {
			if _, ok := v.w.Vars[name]; !ok {
				v.add(line, "unknown placeholder {%s} (built in: %s; or a variable of vars)", name, strings.Join(Builtins, ", "))
			}
		}
	}
}

// variable reports the problems of the variable name; an empty name is a variable outside a workflow.
func (v *validator) variable(name string, def Var) {
	label := "var"
	if name != "" {
		label += " " + name
	}
	v.unknownKeys(0, label, def.Keys, []string{"branch"})
	switch {
	case contains(Builtins, name):
		v.add(0, "%s: the name is a built-in placeholder (%s)", label, strings.Join(Builtins, ", "))
	case def.Branch == "":
		v.add(0, "%s needs branch", label)
	default:
		if _, err := regexp.Compile(def.Branch); err != nil {
			v.add(0, "%s: %v", label, err)
		}
	}
}

// Renamed reports whether a problem of the workflow (one line of Problems) is about a name of the old vocabulary,
// reported with its new spelling: it is a name this polix knows it replaced, never an option of a newer polix.
func Renamed(problem string) bool {
	m := renamedProblem.FindStringSubmatch(problem)
	if m == nil {
		return false
	}
	place, name, inAction := m[1], m[2], m[3] != ""
	switch {
	case place == "event":
		_, ok := renamedEvents[name]
		return ok
	case place == "key" && inAction:
		_, ok := renamed["action"][name]
		return ok
	case place == "key":
		_, entry := renamed["key"][name]
		_, top := renamed["top"][name]
		return entry || top
	default:
		_, ok := renamed[place][name]
		return ok
	}
}

func contains(list []string, s string) bool { return slices.Contains(list, s) }

// appendFinished appends to out the subagent types the `finished` of c names, in c and the maps nested in it.
func appendFinished(out []string, c Cond) []string {
	for k, val := range c {
		switch k {
		case "finished":
			names, _ := Strings(val)
			out = append(out, names...)
		case "not", "any":
			if nested, ok := Nested(val); ok {
				out = appendFinished(out, nested)
			}
		}
	}
	return out
}
