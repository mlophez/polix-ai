package workflow

import (
	"fmt"
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

// AC16: an entry has no key guard: on any event and with any value, it is an unknown key at the line of its entry,
// listed with the keys an entry takes.
func TestEntryNoGuard(t *testing.T) {
	entry := "version: 1\non:\n  %s:\n    - guard: %s\n      action: { block: \"no\" }\n"
	for _, ev := range []string{PreEdit, PreCommand, PreCommit, Stop} {
		for _, value := range []string{"true", "yes please"} {
			_, err := Parse([]byte(fmt.Sprintf(entry, ev, value)), "dev")
			want := `polix.yaml:4: unknown key "guard" (when, unless, action)`
			if err == nil || err.Error() != want {
				t.Errorf("%s with guard: %s: got %v", ev, value, err)
			}
		}
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

// AC10: a measure condition is a non-negative integer; a map of the old vocabulary is reported with the number to
// write instead.
func TestCondMeasured(t *testing.T) {
	m, err := Measured(150)
	if err != nil || m.AtLeast != 150 {
		t.Fatalf("Measured = %+v, %v", m, err)
	}
	if m, err := Measured(0); err != nil || m.AtLeast != 0 {
		t.Fatalf("Measured of 0 = %+v, %v", m, err)
	}
	for _, v := range []any{-1, "150", 2.5, map[string]any{}, Cond{"at_most": 2}} {
		if _, err := Measured(v); err == nil || !strings.Contains(err.Error(), "needs a non-negative integer") {
			t.Errorf("Measured(%v): %v", v, err)
		}
	}
	_, err = Measured(map[string]any{"at_least": 150})
	if err == nil || !strings.Contains(err.Error(), "use 150 instead of { at_least: 150 }") {
		t.Fatalf("the old map: %v", err)
	}
	if (Measure{AtLeast: -1}).Valid() == nil || (Measure{AtLeast: 3}).Valid() != nil {
		t.Fatal("Measure.Valid")
	}
}

func TestNestedAndStrings(t *testing.T) {
	if c, ok := Nested(map[string]any{"actor": "session"}); !ok || c["actor"] != "session" {
		t.Fatal("a map is a Cond")
	}
	if _, ok := Nested("x"); ok {
		t.Fatal("a string is not a Cond")
	}
	if l, err := Strings([]any{"a", "b"}); err != nil || len(l) != 2 {
		t.Fatal(l, err)
	}
	if _, err := Strings([]any{"a", 1}); err == nil {
		t.Fatal("a number in the list")
	}
	if _, err := Strings(3); err == nil {
		t.Fatal("a number")
	}
}
