package workflow

import (
	"slices"
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

// EntryKeys are the keys of an entry.
var EntryKeys = []string{"when", "unless", "action"}

// RunKeys, InjectKeys and BlockKeys are the keys of the long form of each verb.
var (
	RunKeys    = []string{"command", "rules", "message", "always"}
	InjectKeys = []string{"file", "text", "rules", "always"}
	BlockKeys  = []string{"message", "once"}
)

// Valid tells why the entry is not valid on its own: unknown keys (the old ones with their new spelling), no verb
// or several, the shape of its conditions and of its action. What depends on the event or on the vars is checked
// by Workflow.Valid.
func (e Entry) Valid() error {
	v := &validator{}
	v.entry("", e)
	if len(v.problems) > 0 {
		return InvalidError(v.problems)
	}
	return nil
}

// Valid tells why the action is not valid on its own: unknown keys, not exactly one verb, the shape of the verb.
func (a Action) Valid() error {
	v := &validator{}
	v.action("", 0, a, false)
	if len(v.problems) > 0 {
		return InvalidError(v.problems)
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
		return InvalidError(v.problems)
	}
	return nil
}

// Valid tells why the inject is not valid: an unknown key or not exactly one of file, text or rules.
func (c Inject) Valid() error {
	v := &validator{}
	v.inject("", 0, c)
	if len(v.problems) > 0 {
		return InvalidError(v.problems)
	}
	return nil
}

// Valid tells why the block is not valid: an unknown key or no message.
func (b Block) Valid() error {
	v := &validator{}
	v.block("", 0, b)
	if len(v.problems) > 0 {
		return InvalidError(v.problems)
	}
	return nil
}

// entry reports the problems of the entry e of event ev; an empty ev skips what depends on the event.
func (v *validator) entry(ev string, e Entry) {
	old := false
	for _, k := range e.Keys {
		if slices.Contains(EntryKeys, k) {
			continue
		}
		if h, ok := renamed["key"][k]; ok {
			v.add(e.Line, "unknown key %q: %s", k, h)
			old = old || slices.Contains(oldActions, k)
			if k == "if" {
				v.cond(ev, e.OldIf, e.Line, false)
			}
			continue
		}
		v.add(e.Line, "unknown key %q (%s)", k, strings.Join(EntryKeys, ", "))
	}
	v.cond(ev, e.When, e.Line, false)
	v.cond(ev, e.Unless, e.Line, false)
	v.action(ev, e.Line, e.Action, old)
}

// action reports the problems of the action a of event ev at line; old tells that the entry has an action of the
// old vocabulary, already reported with its new spelling, so a missing verb is not reported again. Neither is it for
// an action that only holds verbs of the old vocabulary, each reported with its new spelling.
func (v *validator) action(ev string, line int, a Action, old bool) {
	old = v.actionKeys(line, a) || old
	verbs := a.verbs()
	empty := false
	for _, k := range a.Keys {
		if slices.Contains(Actions, k) && !slices.Contains(verbs, k) {
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

// actionKeys reports the keys of the action a at line that are not verbs, those of the old vocabulary with their new
// spelling, and tells whether the action holds keys and all of them are verbs of the old vocabulary.
func (v *validator) actionKeys(line int, a Action) bool {
	oldVerbs := len(a.Keys) > 0
	for _, k := range a.Keys {
		h, renamedVerb := renamed["action"][k]
		oldVerbs = oldVerbs && renamedVerb
		if slices.Contains(Actions, k) {
			continue
		}
		if renamedVerb {
			v.add(line, "unknown key %q in action: %s", k, h)
			continue
		}
		v.add(line, "unknown key %q in action (%s)", k, strings.Join(Actions, ", "))
	}
	return oldVerbs
}

// run reports the problems of the run r at line.
func (v *validator) run(line int, r Run) {
	v.unknownKeys(line, "run", r.Keys, RunKeys)
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
	v.unknownKeys(line, "inject", c.Keys, InjectKeys)
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
	v.unknownKeys(line, "block", b.Keys, BlockKeys)
	if slices.Contains(Informed, ev) {
		v.add(line, "block cannot stop %s, which already happened: use inject, or a run whose failure reaches the AI",
			ev)
	}
	if strings.TrimSpace(b.Message) == "" {
		v.add(line, "block needs a message")
	}
	v.placeholders(line, b.Message)
}
