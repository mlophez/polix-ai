package workflow

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// Entry is one rule of an event: when it applies (When and Unless) and what it does (Action).
type Entry struct {
	When   Cond
	Unless Cond
	Action Action
	// From is the path of the shortcut the entry was expanded from (guardrails.format_on_edit), empty for an entry
	// written in on.
	From string
	Line int      // line in the file, for messages
	Keys []string // the keys the file used, so Valid rejects unknown ones
	// OldIf holds the conditions under the key if of the old vocabulary. It is never evaluated: Valid reports the key
	// with its new spelling and the problems of these conditions with it, so every hint comes in one pass.
	OldIf Cond
}

// Action is what an entry does: exactly one of its verbs is set.
type Action struct {
	Run    *Run
	Inject *Inject
	Block  *Block
	Keys   []string // the keys the file used, so Valid rejects unknown ones
}

// Run executes a command, or the check of the rules that apply to the changed files (`rules: changed`) or to
// a path (`rules: "{path}"`). A failure blocks the event, or reaches the AI as context on the events that already
// happened (Informed), with Message before the output.
type Run struct {
	Command []string // never run through a shell; a string in the file is split on spaces
	Rules   string
	Message string
	Always  bool // run on every event, not only until it passed on the content
	Keys    []string
}

// Inject injects text into the session: a file, a text or the rules that apply to a path.
type Inject struct {
	File   string
	Text   string
	Rules  string
	Always bool // inject on every event, not once per session
	Keys   []string
}

// Block blocks the event with a message for the AI, every time its entry applies; Once makes it a reminder that
// blocks the first attempt of an actor on a content.
type Block struct {
	Message string
	Once    bool
	Keys    []string
}

// Cond is a set of conditions that must all hold.
type Cond map[string]any

// Measure is the value of a measure condition (changed_lines, changed_files, risk_score): it holds when the
// measure is at least AtLeast.
type Measure struct {
	AtLeast int
}

// entryKeys are the keys of an entry.
var entryKeys = []string{"when", "unless", "action"}

// runKeys, injectKeys and blockKeys are the keys of the long form of each verb.
var (
	runKeys    = []string{"command", "rules", "message", "always"}
	injectKeys = []string{"file", "text", "rules", "always"}
	blockKeys  = []string{"message", "once"}
)

// Valid tells why the measure is not valid: a negative minimum.
func (m Measure) Valid() error {
	if m.AtLeast < 0 {
		return fmt.Errorf("needs a non-negative integer, got %d", m.AtLeast)
	}
	return nil
}

// Measured reads the value of a measure condition, a non-negative integer: the measure holds from that number up.
// The map of the old vocabulary, { at_least: N }, is reported with the number to write instead.
func Measured(v any) (Measure, error) {
	if m, ok := Nested(v); ok {
		if n, ok := m["at_least"]; ok {
			return Measure{}, fmt.Errorf("needs a non-negative integer, got a map: use %v instead of { at_least: %v }",
				n, n)
		}
		return Measure{}, fmt.Errorf("needs a non-negative integer, got %v", v)
	}
	n, ok := v.(int)
	if !ok {
		return Measure{}, fmt.Errorf("needs a non-negative integer, got %v", v)
	}
	measure := Measure{AtLeast: n}
	if err := measure.Valid(); err != nil {
		return Measure{}, err
	}
	return measure, nil
}

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

// Valid tells why the entry is not valid on its own: unknown keys (the old ones with their new spelling), no verb
// or several, the shape of its conditions and of its action. What depends on the event or on the vars is checked
// by Workflow.Valid.
func (e Entry) Valid() error {
	v := &validator{}
	v.entry("", e)
	if len(v.problems) > 0 {
		return Invalid(v.problems)
	}
	return nil
}

// Valid tells why the action is not valid on its own: unknown keys, not exactly one verb, the shape of the verb.
func (a Action) Valid() error {
	v := &validator{}
	v.action("", 0, a, false)
	if len(v.problems) > 0 {
		return Invalid(v.problems)
	}
	return nil
}

// Verb names the verb the action holds (VerbRun, VerbInject or VerbBlock), the first one when it holds several, or
// "" when it holds none.
func (a Action) Verb() string {
	for _, verb := range a.verbs() {
		return verb
	}
	return ""
}

// verbs lists the verbs the action holds, in the order of Actions.
func (a Action) verbs() []string {
	var out []string
	if a.Run != nil {
		out = append(out, VerbRun)
	}
	if a.Inject != nil {
		out = append(out, VerbInject)
	}
	if a.Block != nil {
		out = append(out, VerbBlock)
	}
	return out
}

// Valid tells why the run is not valid: an unknown key, not exactly one of command or rules.
func (r Run) Valid() error {
	v := &validator{}
	v.run(0, r)
	if len(v.problems) > 0 {
		return Invalid(v.problems)
	}
	return nil
}

// Valid tells why the inject is not valid: an unknown key or not exactly one of file, text or rules.
func (c Inject) Valid() error {
	v := &validator{}
	v.inject("", 0, c)
	if len(v.problems) > 0 {
		return Invalid(v.problems)
	}
	return nil
}

// Valid tells why the block is not valid: an unknown key or no message.
func (b Block) Valid() error {
	v := &validator{}
	v.block("", 0, b)
	if len(v.problems) > 0 {
		return Invalid(v.problems)
	}
	return nil
}

// entry reports the problems of the entry e of event ev; an empty ev skips what depends on the event.
func (v *validator) entry(ev string, e Entry) {
	old := false
	for _, k := range e.Keys {
		if contains(entryKeys, k) {
			continue
		}
		if h, ok := renamed["key"][k]; ok {
			v.add(e.Line, "unknown key %q: %s", k, h)
			old = old || contains(oldActions, k)
			if k == "if" {
				v.cond(ev, e.OldIf, e.Line, false)
			}
			continue
		}
		v.add(e.Line, "unknown key %q (%s)", k, strings.Join(entryKeys, ", "))
	}
	v.cond(ev, e.When, e.Line, false)
	v.cond(ev, e.Unless, e.Line, false)
	v.action(ev, e.Line, e.Action, old)
}

// action reports the problems of the action a of event ev at line; old tells that the entry has an action of the
// old vocabulary, already reported with its new spelling, so a missing verb is not reported again. Neither is it for
// an action that only holds verbs of the old vocabulary, each reported with its new spelling.
func (v *validator) action(ev string, line int, a Action, old bool) {
	oldVerbs := len(a.Keys) > 0
	for _, k := range a.Keys {
		h, renamedVerb := renamed["action"][k]
		oldVerbs = oldVerbs && renamedVerb
		if contains(Actions, k) {
			continue
		}
		if renamedVerb {
			v.add(line, "unknown key %q in action: %s", k, h)
			continue
		}
		v.add(line, "unknown key %q in action (%s)", k, strings.Join(Actions, ", "))
	}
	old = old || oldVerbs
	verbs := a.verbs()
	empty := false
	for _, k := range a.Keys {
		if contains(Actions, k) && !contains(verbs, k) {
			v.add(line, "%s needs a value", k)
			empty = true
		}
	}
	switch {
	case len(verbs) == 0 && !old && !empty:
		v.add(line, "the entry needs an action with one of %s", strings.Join(Actions, ", "))
	case len(verbs) > 1:
		v.add(line, "the action has several verbs (%s): use one entry per verb", strings.Join(verbs, ", "))
	}
	if a.Run != nil {
		v.run(line, *a.Run)
	}
	if a.Inject != nil {
		v.inject(ev, line, *a.Inject)
	}
	if a.Block != nil {
		v.block(ev, line, *a.Block)
	}
}

// run reports the problems of the run r at line.
func (v *validator) run(line int, r Run) {
	v.unknownKeys(line, "run", r.Keys, runKeys)
	if (len(r.Command) == 0) == (r.Rules == "") {
		v.add(line, "run needs exactly one of command or rules")
	}
	for _, a := range r.Command {
		v.placeholders(line, a)
	}
	v.placeholders(line, r.Rules)
	v.placeholders(line, r.Message)
}

// inject reports the problems of c on event ev, in the order polix config check reports them (its keys, then the
// event); an empty ev skips the event.
func (v *validator) inject(ev string, line int, c Inject) {
	v.unknownKeys(line, "inject", c.Keys, injectKeys)
	if ev == Stop || ev == SubagentStop {
		v.add(line, "inject does not reach the agent on %s, which cannot receive context", ev)
	}
	n := 0
	for _, s := range []string{c.File, c.Text, c.Rules} {
		if s != "" {
			n++
		}
		v.placeholders(line, s)
	}
	if n != 1 {
		v.add(line, "inject needs exactly one of file, text or rules")
	}
}

// block reports the problems of b on event ev, its keys first; an empty ev skips the event.
func (v *validator) block(ev string, line int, b Block) {
	v.unknownKeys(line, "block", b.Keys, blockKeys)
	if contains(Informed, ev) {
		v.add(line, "block cannot stop %s, which already happened: use inject, or a run whose failure reaches the AI",
			ev)
	}
	if strings.TrimSpace(b.Message) == "" {
		v.add(line, "block needs a message")
	}
	v.placeholders(line, b.Message)
}

// cond reports the problems of the conditions c on event ev; an empty ev skips the events they hold on. inAny
// tells that c is the map of an `any`, where contains is rejected.
func (v *validator) cond(ev string, c Cond, line int, inAny bool) {
	keys := make([]string, 0, len(c))
	for k := range c {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		val := c[k]
		if !contains(Conditions, k) {
			if h, ok := renamed["condition"][k]; ok {
				v.add(line, "unknown condition %q: %s", k, h)
			} else {
				v.add(line, "unknown condition %q (conditions: %s)", k, strings.Join(Conditions, ", "))
			}
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
			v.cond(ev, nested, line, false)
		case "any":
			nested, ok := Nested(val)
			if !ok {
				v.add(line, "any needs a map of conditions")
				continue
			}
			if len(nested) == 0 {
				v.add(line, "any needs at least one condition")
			}
			v.cond(ev, nested, line, true)
		case "changed_lines", "changed_files", "risk_score":
			if _, err := Measured(val); err != nil {
				v.add(line, "%s: %v", k, err)
			}
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
			if inAny {
				v.add(line, "contains cannot go inside any, where it holds on its own: put file and contains "+
					"next to the any, or in a not inside it")
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
				if k == "finished" && strings.TrimSpace(s) == "" {
					v.add(line, "finished needs the type of a subagent")
				}
				if k == "var" && v.w != nil {
					if _, ok := v.w.Vars[s]; !ok {
						v.add(line, "var %s is not defined in vars", s)
					}
				}
			}
		}
	}
}
