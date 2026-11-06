// Package workflow is the domain of polix.yaml: for each neutral event, an ordered list of entries, each with
// optional `if` and `unless` conditions and exactly one action, plus the variables, the base branches, the
// protected paths, the loop guard and the runners of the checks of the rules.
//
//	version: 1
//	vars:
//	  key: { branch: "^([A-Z][A-Z0-9]+-[0-9]+)" }
//	on:
//	  stop:
//	    - if: { changed: "src/**" }
//	      unless: { done: reviewed }
//	      require: "Lanza el subagente reviewer sobre el diff actual"
//
// The vocabulary is closed: Valid rejects unknown events, conditions, actions and keys, so a typo never turns
// into a rule that silently stops applying. Reading the file is the job of the adapters: Parse takes its bytes.
package workflow

import (
	"fmt"
	"regexp"
	"slices"
	"sort"
	"strings"

	"github.com/mlophez/polix-ai/pkg/semver"
)

// Workflow is polix.yaml.
type Workflow struct {
	Version int
	// Requires is the oldest polix that knows every option the file uses (`requires: 0.2.0`). An older polix
	// reads it before anything else and asks to be updated instead of reporting the options it does not know.
	Requires string
	// Base names the branches a change may merge into, for `polix rules check --changed`; the hooks always
	// compare `changed` with HEAD.
	Base          []string
	MaxStopBlocks int // loop guard of every block on stop and subagent_stop
	// Protected are the path globs no agent writes, the main session included: only a person changes them.
	Protected []string
	Vars      map[string]Var
	On        map[string][]Entry
	Rules     Rules    // the folder of the rules and how their checks run
	Plans     Folder   // the folder of the plans
	Reports   Folder   // the folder of the reports
	Guidance  Guidance // the built-in instructions injected at session start
}

// Var extracts a variable from the branch name: the first group of the regular expression, or the whole match.
type Var struct {
	Branch string
	Keys   []string // the keys the file used, so Valid rejects unknown ones
}

// Invalid is the error of a value with problems, one per line; the list is the error itself, so a caller gets
// it back with errors.As.
type Invalid []string

// validator collects the problems of one workflow, or of one of its values when w is nil (no vars nor event
// known: the problems that depend on them are left to the workflow).
type validator struct {
	w        *Workflow
	problems []string
}

// File is the workflow, relative to the repository root.
const File = "polix.yaml"

// DefaultMaxStopBlocks is the loop guard of stop and subagent_stop when the workflow does not say.
const DefaultMaxStopBlocks = 3

// Neutral events, the same for every agent.
const (
	SessionStart = "session_start"
	Prompt       = "prompt"
	PreEdit      = "pre_edit"
	PostEdit     = "post_edit"
	PreCommand   = "pre_command"
	PostCommand  = "post_command"
	PreCommit    = "pre_commit"
	SubagentStop = "subagent_stop"
	Stop         = "stop"
)

// Modes of an entry: enforce applies it, warn turns a block into a warning in the context, audit only logs.
const (
	ModeEnforce = "enforce"
	ModeWarn    = "warn"
	ModeAudit   = "audit"
)

// Events lists every neutral event in lifecycle order.
var Events = []string{SessionStart, Prompt, PreEdit, PostEdit, PreCommand, PostCommand, PreCommit, SubagentStop, Stop}

// Actions an entry can take; each entry has exactly one, and record may also go next to any other one, as the
// fact that action records once it applied.
var Actions = []string{"deny", "allow", "require", "run", "context", "record"}

// Conditions an `if` or `unless` can use.
var Conditions = []string{"branch", "var", "path", "changed", "file", "contains", "command", "writes", "exit", "actor", "done", "not"}

// Builtins are the placeholders every string may use; a variable cannot take their names.
var Builtins = []string{"path", "command", "branch"}

// conditionEvents limits the conditions that only make sense on some events; the rest hold on any event.
var conditionEvents = map[string][]string{
	"path":    {PreEdit, PostEdit},
	"command": {PreCommand, PostCommand, PreCommit},
	"writes":  {PreCommand, PostCommand, PreCommit},
	"exit":    {PostCommand},
}

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

// Problems returns every problem of the workflow, one per line as `polix check` prints them; an empty list means
// it is valid.
func (w Workflow) Problems() []string {
	v := &validator{w: &w}
	if w.Version != 1 {
		v.add(0, "version must be 1, got %d", w.Version)
	}
	if w.Requires != "" && !semver.Valid(w.Requires) {
		v.add(0, "requires must be a version of polix like 0.2.0, got %q", w.Requires)
	}
	if w.MaxStopBlocks < 0 {
		v.add(0, "max_stop_blocks must be positive, got %d", w.MaxStopBlocks)
	}
	for _, b := range w.Base {
		if strings.TrimSpace(b) == "" {
			v.add(0, "base: a branch name cannot be empty")
		}
	}
	for _, g := range w.Protected {
		if strings.TrimSpace(g) == "" {
			v.add(0, "protected: a path glob cannot be empty")
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
		if !contains(Events, ev) {
			v.add(0, "unknown event %q (events: %s)", ev, strings.Join(Events, ", "))
			continue
		}
		for _, e := range w.On[ev] {
			v.entry(ev, e)
		}
	}
	v.rules(w.Rules)
	v.folder("plans", w.Plans)
	v.folder("reports", w.Reports)
	v.guidance(w.Guidance)
	return v.problems
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

func (v *validator) add(line int, format string, args ...any) {
	where := File
	if line > 0 {
		where = fmt.Sprintf("%s:%d", File, line)
	}
	v.problems = append(v.problems, where+": "+fmt.Sprintf(format, args...))
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

func contains(list []string, s string) bool { return slices.Contains(list, s) }
