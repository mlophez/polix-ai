package workflow

import (
	"slices"
	"strings"
	"testing"
)

// An entry reports on its own what does not depend on the event nor the vars.
func TestEntryValid(t *testing.T) {
	block := Action{Block: &Block{Message: "x"}}
	cases := map[string]Entry{
		"the entry needs an action":         {Line: 3},
		"the action has several verbs":      {Action: Action{Block: &Block{Message: "x"}, Run: &Run{Command: []string{"ls"}}}},
		`unknown key "dney" (when, unless`:  {Action: block, Keys: []string{"action", "dney"}},
		`unknown key "deny": use action`:    {Keys: []string{"deny"}},
		"exit needs a number":               {Action: block, When: Cond{"exit": "0"}},
		"run needs exactly one":             {Action: Action{Run: &Run{}}},
		"inject needs exactly one":          {Action: Action{Inject: &Inject{}}},
		"block needs a message":             {Action: Action{Block: &Block{Once: true}}},
		"notify needs a message":            {Action: Action{Notify: &Notify{Always: true}}},
		`unknown condition "done": use`:     {Action: block, Unless: Cond{"done": "tested"}},
		"contains cannot go inside any":     {Action: block, When: Cond{"any": Cond{"file": "x", "contains": "y"}}},
		"needs a non-negative integer, got": {Action: block, When: Cond{"risk_score": -2}},
	}
	for want, e := range cases {
		if err := e.Valid(); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%s: got %v", want, err)
		}
	}
	// What depends on the event or the vars is left to the workflow.
	ok := Entry{Action: Action{Block: &Block{Message: "{kye}"}}, When: Cond{"path": "{kye}"}}
	if err := ok.Valid(); err != nil {
		t.Fatal(err)
	}
	// An entry of the old vocabulary is told how to rewrite its action, not that it has none.
	if err := (Entry{Keys: []string{"require"}}).Valid(); err == nil || strings.Contains(err.Error(), "needs an action") {
		t.Fatalf("old action: %v", err)
	}
}

// Each verb reports its own problems, with the keys of its long form.
func TestVerbsValid(t *testing.T) {
	valid := []interface{ Valid() error }{
		Run{Command: []string{"ls"}},
		Run{Rules: "changed", Message: "m", Always: true, Keys: []string{"rules", "message", "always"}},
		Inject{Text: "x"},
		Inject{File: "f", Always: true, Keys: []string{"file", "always"}},
		Block{Message: "x"},
		Block{Message: "x", Once: true, Keys: []string{"message", "once"}},
		Notify{Message: "x"},
		Notify{Message: "{branch}", Always: true, Keys: []string{"message", "always"}},
		Action{Inject: &Inject{Rules: "{path}"}},
		Action{Notify: &Notify{Message: "x"}},
	}
	for _, v := range valid {
		if err := v.Valid(); err != nil {
			t.Errorf("%+v: %v", v, err)
		}
	}
	invalid := map[string]interface{ Valid() error }{
		`unknown key "once" in run`:      Run{Command: []string{"ls"}, Keys: []string{"command", "once"}},
		"run needs exactly one":          Run{Command: []string{"ls"}, Rules: "changed"},
		`unknown key "once" in inject`:   Inject{Text: "x", Keys: []string{"text", "once"}},
		"inject needs exactly one":       Inject{Text: "x", File: "y"},
		`unknown key "always" in block`:  Block{Message: "x", Keys: []string{"message", "always"}},
		"block needs a message":          Block{Message: "  "},
		`unknown key "once" in notify`:   Notify{Message: "x", Keys: []string{"message", "once"}},
		"notify needs a message":         Notify{Message: " ", Keys: []string{"always"}},
		"the entry needs an action":      Action{},
		`unknown key "context" in actio`: Action{Keys: []string{"context"}},
		"block needs a value":            Action{Keys: []string{"block"}},
	}
	for want, v := range invalid {
		if err := v.Valid(); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%s: got %v", want, err)
		}
	}
}

// AC2: a verb the event does not allow is one problem at the line of its entry, naming the event, the verb, why and
// the verbs the event allows; every verb the table allows on an event is valid there.
func TestEventVerbsValid(t *testing.T) {
	actions := map[string]Action{
		VerbRun:    {Run: &Run{Command: []string{"ls"}}, Keys: []string{VerbRun}},
		VerbInject: {Inject: &Inject{Text: "x"}, Keys: []string{VerbInject}},
		VerbBlock:  {Block: &Block{Message: "x"}, Keys: []string{VerbBlock}},
		VerbNotify: {Notify: &Notify{Message: "x"}, Keys: []string{VerbNotify}},
	}
	refused := map[string]string{
		Start + " " + VerbBlock: "polix.yaml:7: block cannot stop start, which already happened: use inject, or a run " +
			"whose failure reaches the AI (start allows run, inject, notify)",
		SubagentStart + " " + VerbBlock: "polix.yaml:7: block cannot stop subagent_start, which already happened: " +
			"use inject, or a run whose failure reaches the AI (subagent_start allows run, inject, notify)",
		PostEdit + " " + VerbBlock: "polix.yaml:7: block cannot stop post_edit, which already happened: use inject, " +
			"or a run whose failure reaches the AI (post_edit allows run, inject, notify)",
		PostCommand + " " + VerbBlock: "polix.yaml:7: block cannot stop post_command, which already happened: use " +
			"inject, or a run whose failure reaches the AI (post_command allows run, inject, notify)",
		PostCommit + " " + VerbBlock: "polix.yaml:7: block cannot stop post_commit, which already happened: use " +
			"inject, or a run whose failure reaches the AI (post_commit allows run, inject, notify)",
		Stop + " " + VerbInject: "polix.yaml:7: inject does not reach the agent on stop, which cannot receive " +
			"context (stop allows run, block, notify)",
		SubagentStop + " " + VerbInject: "polix.yaml:7: inject does not reach the agent on subagent_stop, which " +
			"cannot receive context (subagent_stop allows run, block, notify)",
	}
	for _, ev := range Events {
		for _, verb := range Actions {
			w := Workflow{Version: 1, On: map[string][]Entry{ev: {{Action: actions[verb], Keys: []string{"action"},
				Line: 7}}}}
			got := w.Problems()
			want, bad := refused[ev+" "+verb]
			switch {
			case bad && (len(got) != 1 || got[0] != want):
				t.Errorf("%s on %s: %q, want %q", verb, ev, got, want)
			case !bad && len(got) > 0:
				t.Errorf("%s on %s: %q", verb, ev, got)
			}
			if allowed := !bad; allowed != slices.Contains(EventVerbs[ev], verb) {
				t.Errorf("%s on %s: the cases disagree with EventVerbs", verb, ev)
			}
		}
	}
	// On its own, without an event, an action is valid with any verb.
	if err := actions[VerbBlock].Valid(); err != nil {
		t.Fatal(err)
	}
}

// AC9: notify is valid on every event in both forms, its message with placeholders; without a message or with a key
// of another verb it is a problem at the line of its entry. An always that is not a boolean never reaches the domain:
// the decoding of polix.yaml rejects it (see the use case LoadWorkflow).
func TestNotifyOnEveryEvent(t *testing.T) {
	forms := []Notify{
		{Message: "Rama {branch}"},
		{Message: "Aviso", Always: true, Keys: []string{"always", "message"}},
	}
	for _, ev := range Events {
		for _, n := range forms {
			w := Workflow{Version: 1, On: map[string][]Entry{ev: {{Action: Action{Notify: &n,
				Keys: []string{VerbNotify}}, Keys: []string{"action"}, Line: 4}}}}
			if p := w.Problems(); len(p) > 0 {
				t.Errorf("%s: %+v: %q", ev, n, p)
			}
		}
	}
	for want, n := range map[string]Notify{
		"polix.yaml:4: notify needs a message":                                 {Keys: []string{"always"}, Always: true},
		`polix.yaml:4: unknown key "once" in notify (always, message)`:         {Message: "x", Keys: []string{"once"}},
		"polix.yaml:4: unknown placeholder {kye} (built in: path, command, br": {Message: "falta {kye}"},
	} {
		w := Workflow{Version: 1, On: map[string][]Entry{Prompt: {{Action: Action{Notify: &n,
			Keys: []string{VerbNotify}}, Keys: []string{"action"}, Line: 4}}}}
		if got := strings.Join(w.Problems(), "\n"); !strings.HasPrefix(got, want) {
			t.Errorf("%+v: %q, want %q", n, got, want)
		}
	}
}

// The verb of an action is the one it holds, the first one when it holds several, or none.
func TestActionVerb(t *testing.T) {
	cases := map[string]Action{
		VerbRun:    {Run: &Run{Command: []string{"ls"}}},
		VerbInject: {Inject: &Inject{Text: "x"}},
		VerbBlock:  {Block: &Block{Message: "x"}},
		VerbNotify: {Notify: &Notify{Message: "x"}},
		"":         {},
	}
	for want, a := range cases {
		if got := a.Verb(); got != want {
			t.Errorf("%+v: got %q, want %q", a, got, want)
		}
	}
	if got := (Action{Block: &Block{Message: "x"}, Inject: &Inject{Text: "x"}}).Verb(); got != VerbInject {
		t.Fatalf("several verbs: %q", got)
	}
}
