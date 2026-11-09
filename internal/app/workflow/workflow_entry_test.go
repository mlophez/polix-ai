package workflow

import (
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
		Action{Inject: &Inject{Rules: "{path}"}},
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

// The verb of an action is the one it holds, the first one when it holds several, or none.
func TestActionVerb(t *testing.T) {
	cases := map[string]Action{
		VerbRun:    {Run: &Run{Command: []string{"ls"}}},
		VerbInject: {Inject: &Inject{Text: "x"}},
		VerbBlock:  {Block: &Block{Message: "x"}},
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
