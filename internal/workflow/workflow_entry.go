package workflow

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// Entry is one rule of an event.
type Entry struct {
	If      Cond
	Unless  Cond
	Mode    string
	Why     string
	Deny    *string
	Allow   *bool
	Require *string
	Run     *Run
	Context *Context
	Record  *string
	Line    int      // line in the file, for messages
	Keys    []string // the keys the file used, so Valid rejects unknown ones
}

// Run executes a command, or the check of the rules that apply to the changed files (`rules: changed`) or to
// a path (`rules: "{path}"`). A failure blocks the event.
type Run struct {
	Command []string // never run through a shell; a string in the file is split on spaces
	Rules   string
	// Record names a fact recorded on the current content when the run passes.
	Record string
	Keys   []string // the keys the file used, so Valid rejects unknown ones
}

// Context injects text into the session: a file, a text or the rules that apply to a path.
type Context struct {
	File  string
	Text  string
	Rules string
	Keys  []string // the keys the file used, so Valid rejects unknown ones
}

// Cond is a set of conditions that must all hold.
type Cond map[string]any

// Nested reads the value of a `not` condition as a Cond; a nested mapping may come as a Cond or as a
// map[string]any, and both are accepted.
func Nested(v any) (Cond, bool) {
	switch t := v.(type) {
	case Cond:
		return t, true
	case map[string]any:
		return Cond(t), true
	}
	return nil, false
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
	case []string:
		return t, nil
	default:
		return nil, fmt.Errorf("expected a string or a list of strings, got %v", v)
	}
}

// Valid tells why the entry is not valid on its own: unknown keys, no action or several, an unknown mode, the
// shape of its conditions and of its action. What depends on the event or on the vars is checked by
// Workflow.Valid.
func (e Entry) Valid() error {
	v := &validator{}
	v.entry("", e)
	if len(v.problems) > 0 {
		return Invalid(v.problems)
	}
	return nil
}

// EffectiveMode returns the mode of the entry, enforce by default.
func (e Entry) EffectiveMode() string {
	if e.Mode == "" {
		return ModeEnforce
	}
	return e.Mode
}

// Action returns the name of the action of the entry, or "" when it has none.
func (e Entry) Action() string {
	for _, a := range e.actions() {
		return a
	}
	return ""
}

func (e Entry) actions() []string {
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

// Valid tells why the run is not valid: an unknown key, not exactly one of command or rules, an empty record.
func (r Run) Valid() error {
	v := &validator{}
	v.run(0, r)
	if len(v.problems) > 0 {
		return Invalid(v.problems)
	}
	return nil
}

// Valid tells why the context is not valid: an unknown key or not exactly one of file, text or rules.
func (c Context) Valid() error {
	v := &validator{}
	v.context("", 0, c)
	if len(v.problems) > 0 {
		return Invalid(v.problems)
	}
	return nil
}

// entry reports the problems of the entry e of event ev; an empty ev skips what depends on the event.
func (v *validator) entry(ev string, e Entry) {
	for _, k := range e.Keys {
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
	v.action(ev, e)
}

func (v *validator) action(ev string, e Entry) {
	switch e.Action() {
	case "require":
		if ev != "" && ev != Stop && ev != SubagentStop {
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
		v.run(e.Line, *e.Run)
	case "context":
		v.context(ev, e.Line, *e.Context)
	case "record":
		if *e.Record == "" {
			v.add(e.Line, "record needs the name of the fact")
		}
		v.placeholders(e.Line, *e.Record)
	}
}

func (v *validator) run(line int, r Run) {
	v.unknownKeys(line, "run", r.Keys, []string{"command", "rules", "record"})
	if (len(r.Command) == 0) == (r.Rules == "") {
		v.add(line, "run needs exactly one of command or rules")
	}
	if contains(r.Keys, "record") && r.Record == "" {
		v.add(line, "record in run needs the name of the fact")
	}
	v.placeholders(line, r.Record)
	for _, a := range r.Command {
		v.placeholders(line, a)
	}
	v.placeholders(line, r.Rules)
}

// context reports the problems of c on event ev, in the order polix check reports them (its keys, then the event); an empty ev skips the
// event.
func (v *validator) context(ev string, line int, c Context) {
	v.unknownKeys(line, "context", c.Keys, []string{"file", "text", "rules"})
	if ev == Stop || ev == SubagentStop {
		v.add(line, "context does not reach the agent on %s, which cannot receive context", ev)
	}
	n := 0
	for _, s := range []string{c.File, c.Text, c.Rules} {
		if s != "" {
			n++
		}
		v.placeholders(line, s)
	}
	if n != 1 {
		v.add(line, "context needs exactly one of file, text or rules")
	}
}

// cond reports the problems of the conditions c on event ev; an empty ev skips the events they hold on.
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
		if only, ok := conditionEvents[k]; ok && ev != "" && !contains(only, ev) {
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
				if k == "var" && v.w != nil {
					if _, ok := v.w.Vars[s]; !ok {
						v.add(line, "var %s is not defined in vars", s)
					}
				}
			}
		}
	}
}
