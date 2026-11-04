// Package workflow loads and validates polix.yaml: for each neutral event, an ordered list of entries,
// each with optional `if` and `unless` conditions and exactly one action.
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
// The vocabulary is closed: Validate rejects unknown events, conditions, actions and keys, so a typo never turns
// into a rule that silently stops applying.
package workflow

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// File is the workflow, relative to the repository root.
const File = "polix.yaml"

// DefaultBase is the branch a change merges into when the workflow does not say.
const DefaultBase = "main"

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

// Events lists every neutral event in lifecycle order.
var Events = []string{SessionStart, Prompt, PreEdit, PostEdit, PreCommand, PostCommand, PreCommit, SubagentStop, Stop}

// Actions an entry can take; each entry has exactly one.
var Actions = []string{"deny", "allow", "require", "run", "context", "record"}

// Conditions an `if` or `unless` can use.
var Conditions = []string{"branch", "var", "path", "changed", "file", "contains", "command", "exit", "actor", "done", "not"}

// Modes of an entry: enforce applies it, warn turns a block into a warning in the context, audit only logs.
const (
	ModeEnforce = "enforce"
	ModeWarn    = "warn"
	ModeAudit   = "audit"
)

// Workflow is polix.yaml.
type Workflow struct {
	Version       int                `yaml:"version"`
	Base          string             `yaml:"base"`            // branch a change merges into, for `changed`
	MaxStopBlocks int                `yaml:"max_stop_blocks"` // loop guard of every block on stop and subagent_stop
	Vars          map[string]Var     `yaml:"vars"`
	On            map[string][]Entry `yaml:"on"`
}

// Var extracts a variable from the branch name: the first group of the regular expression, or the whole match.
type Var struct {
	Branch string `yaml:"branch"`
	keys   []string
}

// Builtins are the placeholders every string may use; a variable cannot take their names.
var Builtins = []string{"path", "command", "branch"}

// Entry is one rule of an event.
type Entry struct {
	If      Cond     `yaml:"if"`
	Unless  Cond     `yaml:"unless"`
	Mode    string   `yaml:"mode"`
	Why     string   `yaml:"why"`
	Deny    *string  `yaml:"deny"`
	Allow   *bool    `yaml:"allow"`
	Require *string  `yaml:"require"`
	Run     *Run     `yaml:"run"`
	Context *Context `yaml:"context"`
	Record  *string  `yaml:"record"`
	Line    int      `yaml:"-"` // line in the file, for messages
	keys    []string
}

// Cond is a set of conditions that must all hold.
type Cond map[string]any

// Nested reads the value of a `not` condition as a Cond. yaml.v3 decodes a nested mapping with the type of the
// map that holds it, so the value is a Cond, not a map[string]any; both are accepted.
func Nested(v any) (Cond, bool) {
	switch t := v.(type) {
	case Cond:
		return t, true
	case map[string]any:
		return Cond(t), true
	}
	return nil, false
}

// Run executes a command, or the check of the rules that apply to the changed files (`rules: changed`) or to
// a path (`rules: "{path}"`). A failure blocks the event.
type Run struct {
	Command Args   `yaml:"command"`
	Rules   string `yaml:"rules"`
	// Record names a fact recorded on the current content when the run passes.
	Record string `yaml:"record"`
	keys   []string
}

// Context injects text into the session: a file, a text or the rules that apply to a path.
type Context struct {
	File  string `yaml:"file"`
	Text  string `yaml:"text"`
	Rules string `yaml:"rules"`
	keys  []string
}

// Args is a command as a list of arguments; a string is split on spaces. Commands never run through a shell.
type Args []string

// UnmarshalYAML accepts a string or a list of strings.
func (a *Args) UnmarshalYAML(n *yaml.Node) error {
	if n.Kind == yaml.ScalarNode {
		*a = strings.Fields(n.Value)
		return nil
	}
	var list []string
	if err := n.Decode(&list); err != nil {
		return err
	}
	*a = list
	return nil
}

// UnmarshalYAML keeps the line of the entry and the keys it uses, so Validate can reject unknown ones.
func (e *Entry) UnmarshalYAML(n *yaml.Node) error {
	type plain Entry
	var p plain
	if err := n.Decode(&p); err != nil {
		return err
	}
	*e = Entry(p)
	e.Line = n.Line
	e.keys = mapKeys(n)
	return nil
}

// UnmarshalYAML keeps the keys of the variable, so Validate can reject unknown ones.
func (v *Var) UnmarshalYAML(n *yaml.Node) error {
	type plain Var
	var p plain
	if err := n.Decode(&p); err != nil {
		return err
	}
	*v = Var(p)
	v.keys = mapKeys(n)
	return nil
}

// UnmarshalYAML keeps the keys of the run, so Validate can reject unknown ones.
func (r *Run) UnmarshalYAML(n *yaml.Node) error {
	type plain Run
	var p plain
	if err := n.Decode(&p); err != nil {
		return err
	}
	*r = Run(p)
	r.keys = mapKeys(n)
	return nil
}

// UnmarshalYAML keeps the keys of the context, so Validate can reject unknown ones.
func (c *Context) UnmarshalYAML(n *yaml.Node) error {
	type plain Context
	var p plain
	if err := n.Decode(&p); err != nil {
		return err
	}
	*c = Context(p)
	c.keys = mapKeys(n)
	return nil
}

// mapKeys returns the keys of a mapping node, in file order.
func mapKeys(n *yaml.Node) []string {
	var keys []string
	if n.Kind == yaml.MappingNode {
		for i := 0; i < len(n.Content); i += 2 {
			keys = append(keys, n.Content[i].Value)
		}
	}
	return keys
}

// BaseBranch returns the branch a change merges into.
func (w *Workflow) BaseBranch() string {
	if w == nil || w.Base == "" {
		return DefaultBase
	}
	return w.Base
}

// EffectiveMode returns the mode of the entry, enforce by default.
func (e *Entry) EffectiveMode() string {
	if e.Mode == "" {
		return ModeEnforce
	}
	return e.Mode
}

// Action returns the name of the action of the entry, or "" when it has none.
func (e *Entry) Action() string {
	for _, a := range e.actions() {
		return a
	}
	return ""
}

func (e *Entry) actions() []string {
	var out []string
	if e.Deny != nil {
		out = append(out, "deny")
	}
	if e.Allow != nil {
		out = append(out, "allow")
	}
	if e.Require != nil {
		out = append(out, "require")
	}
	if e.Run != nil {
		out = append(out, "run")
	}
	if e.Context != nil {
		out = append(out, "context")
	}
	if e.Record != nil {
		out = append(out, "record")
	}
	return out
}

// Load reads the workflow of root. It returns nil without error when the repository has none.
func Load(root string) (*Workflow, error) {
	data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(File)))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return Parse(data)
}

// Parse decodes a workflow, rejecting unknown top level keys.
func Parse(data []byte) (*Workflow, error) {
	var root yaml.Node
	if err := yaml.Unmarshal(data, &root); err != nil {
		return nil, fmt.Errorf("%s: %w", File, err)
	}
	if len(root.Content) == 1 && root.Content[0].Kind == yaml.MappingNode {
		top := root.Content[0]
		for i := 0; i < len(top.Content); i += 2 {
			switch k := top.Content[i].Value; k {
			case "version", "base", "max_stop_blocks", "vars", "on":
			default:
				return nil, fmt.Errorf("%s:%d: unknown key %q", File, top.Content[i].Line, k)
			}
		}
	}
	var w Workflow
	if err := root.Decode(&w); err != nil {
		return nil, fmt.Errorf("%s: %w", File, err)
	}
	if w.MaxStopBlocks == 0 {
		w.MaxStopBlocks = DefaultMaxStopBlocks
	}
	return &w, nil
}

// conditionEvents limits the conditions that only make sense on some events; the rest hold on any event.
var conditionEvents = map[string][]string{
	"path":    {PreEdit, PostEdit},
	"command": {PreCommand, PostCommand, PreCommit},
	"exit":    {PostCommand},
}

// placeholder finds the {name} placeholders of a string.
var placeholder = regexp.MustCompile(`\{([A-Za-z_][A-Za-z0-9_]*)\}`)

// validator collects the problems of one workflow.
type validator struct {
	w        *Workflow
	problems []string
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
// stay in the text as is.
func (v *validator) placeholders(line int, s string) {
	for _, m := range placeholder.FindAllStringSubmatch(s, -1) {
		if name := m[1]; !contains(Builtins, name) {
			if _, ok := v.w.Vars[name]; !ok {
				v.add(line, "unknown placeholder {%s} (built in: %s; or a variable of vars)", name, strings.Join(Builtins, ", "))
			}
		}
	}
}

// Validate returns every problem of the workflow; an empty list means it is valid.
func Validate(w *Workflow) []string {
	v := &validator{w: w}
	if w.Version != 1 {
		v.add(0, "version must be 1, got %d", w.Version)
	}
	if w.MaxStopBlocks < 0 {
		v.add(0, "max_stop_blocks must be positive, got %d", w.MaxStopBlocks)
	}
	names := make([]string, 0, len(w.Vars))
	for name := range w.Vars {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		def := w.Vars[name]
		v.unknownKeys(0, "var "+name, def.keys, []string{"branch"})
		switch {
		case contains(Builtins, name):
			v.add(0, "var %s: the name is a built-in placeholder (%s)", name, strings.Join(Builtins, ", "))
		case def.Branch == "":
			v.add(0, "var %s needs branch", name)
		default:
			if _, err := regexp.Compile(def.Branch); err != nil {
				v.add(0, "var %s: %v", name, err)
			}
		}
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
			for _, k := range e.keys {
				if !contains(Actions, k) && k != "if" && k != "unless" && k != "mode" && k != "why" {
					v.add(e.Line, "unknown key %q", k)
				}
			}
			acts := e.actions()
			switch {
			case len(acts) == 0:
				v.add(e.Line, "the entry has no action (%s)", strings.Join(Actions, ", "))
			case len(acts) > 1:
				v.add(e.Line, "the entry has several actions (%s): use one entry per action", strings.Join(acts, ", "))
			}
			if m := e.Mode; m != "" && m != ModeEnforce && m != ModeWarn && m != ModeAudit {
				v.add(e.Line, "unknown mode %q (enforce, warn, audit)", m)
			}
			v.cond(ev, e.If, e.Line)
			v.cond(ev, e.Unless, e.Line)
			v.action(ev, &e)
		}
	}
	return v.problems
}

func (v *validator) action(ev string, e *Entry) {
	switch e.Action() {
	case "require":
		if ev != Stop && ev != SubagentStop {
			v.add(e.Line, "require only works on stop and subagent_stop; use deny on %s", ev)
		}
		v.placeholders(e.Line, *e.Require)
	case "deny":
		if ev == SessionStart || ev == PostEdit || ev == PostCommand {
			v.add(e.Line, "deny cannot block %s, which already happened: use run or context", ev)
		}
		v.placeholders(e.Line, *e.Deny)
	case "allow":
		if ev == SessionStart || ev == PostEdit || ev == PostCommand {
			v.add(e.Line, "allow cannot let %s through, which already happened", ev)
		}
		if !*e.Allow {
			v.add(e.Line, "allow only takes true: allow: false would still stop the evaluation; remove the entry")
		}
	case "run":
		r := e.Run
		v.unknownKeys(e.Line, "run", r.keys, []string{"command", "rules", "record"})
		if (len(r.Command) == 0) == (r.Rules == "") {
			v.add(e.Line, "run needs exactly one of command or rules")
		}
		if contains(r.keys, "record") && r.Record == "" {
			v.add(e.Line, "record in run needs the name of the fact")
		}
		v.placeholders(e.Line, r.Record)
		for _, a := range r.Command {
			v.placeholders(e.Line, a)
		}
		v.placeholders(e.Line, r.Rules)
	case "context":
		c := e.Context
		v.unknownKeys(e.Line, "context", c.keys, []string{"file", "text", "rules"})
		if ev == Stop || ev == SubagentStop {
			v.add(e.Line, "context does not reach the agent on %s, which cannot receive context", ev)
		}
		n := 0
		for _, s := range []string{c.File, c.Text, c.Rules} {
			if s != "" {
				n++
			}
			v.placeholders(e.Line, s)
		}
		if n != 1 {
			v.add(e.Line, "context needs exactly one of file, text or rules")
		}
	case "record":
		if *e.Record == "" {
			v.add(e.Line, "record needs the name of the fact")
		}
		v.placeholders(e.Line, *e.Record)
	}
}

func (v *validator) cond(ev string, c Cond, line int) {
	keys := make([]string, 0, len(c))
	for k := range c {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		val := c[k]
		if !contains(Conditions, k) {
			v.add(line, "unknown condition %q (conditions: %s)", k, strings.Join(Conditions, ", "))
			continue
		}
		if only, ok := conditionEvents[k]; ok && !contains(only, ev) {
			v.add(line, "%s never holds on %s (only on %s)", k, ev, strings.Join(only, ", "))
		}
		switch k {
		case "not":
			nested, ok := Nested(val)
			if !ok {
				v.add(line, "not needs a map of conditions")
				continue
			}
			v.cond(ev, nested, line)
		case "branch":
			s, ok := val.(string)
			if !ok {
				v.add(line, "branch needs a regular expression")
			} else if _, err := regexp.Compile(s); err != nil {
				v.add(line, "branch: %v", err)
			}
		case "exit":
			if _, ok := val.(int); !ok {
				v.add(line, "exit needs a number")
			}
		case "contains":
			s, ok := val.(string)
			if !ok {
				v.add(line, "contains needs a text")
			}
			if _, ok := c["file"]; !ok {
				v.add(line, "contains only works next to file")
			}
			v.placeholders(line, s)
		default:
			list, err := Strings(val)
			if err != nil {
				v.add(line, "%s: %v", k, err)
				continue
			}
			for _, s := range list {
				v.placeholders(line, s)
				if k == "var" {
					if _, ok := v.w.Vars[s]; !ok {
						v.add(line, "var %s is not defined in vars", s)
					}
				}
			}
		}
	}
}

// Strings reads a condition value that may be a string or a list of strings.
func Strings(v any) ([]string, error) {
	switch t := v.(type) {
	case string:
		return []string{t}, nil
	case []any:
		out := make([]string, 0, len(t))
		for _, x := range t {
			s, ok := x.(string)
			if !ok {
				return nil, fmt.Errorf("expected strings, got %v", x)
			}
			out = append(out, s)
		}
		return out, nil
	default:
		return nil, fmt.Errorf("expected a string or a list of strings, got %v", v)
	}
}

func contains(list []string, s string) bool { return slices.Contains(list, s) }
