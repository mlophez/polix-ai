// Package workflow is the domain of polix.yaml: for each neutral event, an ordered list of entries, each with
// optional `when` and `unless` conditions and an action with one verb (run, inject, block or notify), plus the
// variables, the base branches, the protected files, the forbidden commands, the loop guard and the runners of the
// checks of the rules. The other shortcuts of the section guardrails are expanded into entries when the file is
// loaded.
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
// Reading and decoding the file is the job of the use case LoadWorkflow (internal/app/workflow/load), whose loader
// (Load) every use case that reads polix.yaml gets from cmd/polix; the domain validates what it decodes.
package workflow

import (
	"fmt"
	"maps"
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
	// Requirements are the programs the repository requires on the PATH (requirements), in file order: the hook
	// checks them, with git (Programs), on the first prompt of a session and `polix doctor` warns about each missing
	// one; neither blocks.
	Requirements []string
	// ProtectedFiles are the path globs no agent writes, the main session included (guardrails.protected_files):
	// a protection the hook applies before the entries of on, also to the files git ignores.
	ProtectedFiles []string
	// ForbiddenCommands are the commands no agent runs, wherever they come in a line
	// (guardrails.forbidden_commands): a protection the hook applies before the entries of on, in file order.
	ForbiddenCommands []ForbiddenCommand
	Vars              map[string]Var
	// On holds the entries of each event: those written in `on` and those the shortcuts of guardrails
	// protected_branches, format_on_edit and run_on_stop expand into.
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
	// Githooks holds the entries of each event of githooks (GithookEvents), the gates git runs on every commit,
	// apart from the events of the AI in On and with a vocabulary of their own.
	Githooks map[string][]Entry
	// GithookLines is the line of each event of githooks in the file, for messages.
	GithookLines map[string]int
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

// InvalidError is the error of a value with problems, one per line; the list is the error itself, so a caller gets
// it back with errors.As.
type InvalidError []string

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

// placeholder finds the {name} placeholders of a string.
var placeholder = regexp.MustCompile(`\{([A-Za-z_][A-Za-z0-9_]*)\}`)

func (p InvalidError) Error() string { return strings.Join(p, "\n") }

// Valid returns every problem of the workflow joined in one Invalid error, or nil when it is valid.
func (w Workflow) Valid() error {
	if p := w.Problems(); len(p) > 0 {
		return InvalidError(p)
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

// workflow reports the problems of the workflow w: its settings and guardrails, its variables, its events, its
// githooks, its layout and its weights.
func (v *validator) workflow(w Workflow) {
	v.settings(w)
	for _, name := range slices.Sorted(maps.Keys(w.Vars)) {
		v.variable(name, w.Vars[name])
	}
	v.events(w)
	v.githooks(w)
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

// settings reports the problems of the settings of the workflow w and of its protections: the version, the polix it
// requires, the loop guard, the base branches, the required programs, the protected files and the forbidden commands.
func (v *validator) settings(w Workflow) {
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
	v.requirements(w.Requirements)
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
}

// events reports the problems of the events of the workflow w, in the order of their names: an unknown event, the
// entries of each known one and then their order. The entries of a renamed event are checked as those of its new
// name, so every hint comes at once.
func (v *validator) events(w Workflow) {
	for _, ev := range slices.Sorted(maps.Keys(w.On)) {
		known := ev
		if !slices.Contains(Events, ev) {
			renamedTo, ok := renamedEvents[ev]
			if !ok {
				v.add(w.EventLines[ev], "unknown event %q (events: %s)", ev, strings.Join(Events, ", "))
				continue
			}
			v.add(w.EventLines[ev], "unknown event %q: use %s", ev, renamedTo)
			known = renamedTo
		}
		for _, e := range w.On[ev] {
			v.entry(known, e)
		}
		v.order(known, w.On[ev])
	}
}

// ObservedSubagents returns the subagent types the `finished` conditions of the workflow name, as written, sorted
// and without repeats: the subagents whose end polix writes to its log.
func (w Workflow) ObservedSubagents() []string {
	var out []string
	for _, entries := range w.On {
		for _, e := range entries {
			out = e.When.appendFinished(out)
			out = e.Unless.appendFinished(out)
		}
	}
	sort.Strings(out)
	return slices.Compact(out)
}

// requirements reports the problems of the program names of requirements: an empty name, a name with spaces around
// it or with a path separator, which the lookup would resolve against the working folder instead of the PATH, and a
// repeated name, each once.
func (v *validator) requirements(names []string) {
	seen := map[string]bool{}
	for _, p := range names {
		switch {
		case strings.TrimSpace(p) == "":
			v.add(0, "requirements: a program name cannot be empty")
		case strings.TrimSpace(p) != p:
			v.add(0, "requirements: %q has spaces around it: write the program name alone", p)
		case strings.ContainsAny(p, `/\`):
			v.add(0, "requirements: %q is a path: write the program name alone, as the PATH finds it", p)
		case seen[p]:
			v.add(0, "requirements: %q is repeated", p)
		}
		seen[p] = true
	}
}

// Git is the program polix itself runs to read every fact of the repository, which Programs adds to requirements.
const Git = "git"

// Programs returns the programs the PATH must run for the repository: those of requirements, in file order, followed
// by git (Git) when the list does not name it; nil without requirements, where nothing is looked up.
func (w Workflow) Programs() []string {
	if len(w.Requirements) == 0 {
		return nil
	}
	if slices.Contains(w.Requirements, Git) {
		return slices.Clone(w.Requirements)
	}
	return append(slices.Clone(w.Requirements), Git)
}

// ObservesStart reports whether polix writes the start of a session, or of a subagent, to its log, so what it tells
// once per session is due again: when the workflow injects once (InjectsOnce), or has requirements, whose missing
// programs the hook tells once on the first prompt of a session; or so it knows the subagents still running, when
// the workflow counts them (CountsAgents).
func (w Workflow) ObservesStart() bool {
	return len(w.Requirements) > 0 || w.InjectsOnce() || w.CountsAgents()
}

// CountsAgents reports whether an entry uses agents_are_running, in its when or its unless or in a not or an any
// nested in them, those the shortcuts expand into included: polix then writes the start and the end of every
// subagent to its log, to know which ones are still running.
func (w Workflow) CountsAgents() bool {
	for _, entries := range w.On {
		for _, e := range entries {
			if e.When.countsAgents() || e.Unless.countsAgents() {
				return true
			}
		}
	}
	return false
}

// InjectsOnce reports whether the workflow has a once-only inject: an inject without always on an event other than
// start and subagent_start, which always inject. Its injects are due again when a session or a subagent starts or a
// subagent finishes, so polix then writes those events to its log.
func (w Workflow) InjectsOnce() bool {
	for ev, entries := range w.On {
		if ev == Start || ev == SubagentStart {
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
		return InvalidError(v.problems)
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
		if !slices.Contains(allowed, k) {
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
		if name := m[1]; !slices.Contains(Builtins, name) {
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
	case slices.Contains(Builtins, name):
		v.add(0, "%s: the name is a built-in placeholder (%s)", label, strings.Join(Builtins, ", "))
	case def.Branch == "":
		v.add(0, "%s needs branch", label)
	default:
		if _, err := regexp.Compile(def.Branch); err != nil {
			v.add(0, "%s: %v", label, err)
		}
	}
}
