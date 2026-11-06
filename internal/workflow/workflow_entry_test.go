package workflow

import (
	"strings"
	"testing"
)

// An entry reports on its own what does not depend on the event nor the vars.
func TestEntryValid(t *testing.T) {
	deny, no := "x", false
	cases := map[string]Entry{
		"the entry has no action":       {Line: 3},
		"the entry has several actions": {Deny: &deny, Allow: &no},
		"allow only takes true":         {Allow: &no},
		`unknown mode "loud"`:           {Deny: &deny, Mode: "loud"},
		`unknown key "dney"`:            {Deny: &deny, Keys: []string{"deny", "dney"}},
		"exit needs a number":           {Deny: &deny, If: Cond{"exit": "0"}},
		"run needs exactly one":         {Run: &Run{}},
		"context needs exactly one":     {Context: &Context{}},
	}
	for want, e := range cases {
		if err := e.Valid(); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%s: got %v", want, err)
		}
	}
	// What depends on the event or the vars is left to the workflow.
	ok := Entry{Deny: &deny, If: Cond{"path": "{kye}"}}
	if err := ok.Valid(); err != nil {
		t.Fatal(err)
	}
}

func TestRunAndContextValid(t *testing.T) {
	if err := (Run{Command: []string{"ls"}}).Valid(); err != nil {
		t.Fatal(err)
	}
	err := (Run{Command: []string{"ls"}, Keys: []string{"command", "record"}}).Valid()
	if err == nil || !strings.Contains(err.Error(), "record goes next to run") {
		t.Fatalf("record inside run is a problem that names the fix: %v", err)
	}
	if err := (Context{Text: "x"}).Valid(); err != nil {
		t.Fatal(err)
	}
	if err := (Context{Text: "x", File: "y"}).Valid(); err == nil {
		t.Fatal("two sources of context are a problem")
	}
}

func TestEntryActionAndMode(t *testing.T) {
	rec := "tested"
	e := Entry{Record: &rec}
	if e.Action() != "record" || e.EffectiveMode() != ModeEnforce {
		t.Fatalf("%q %q", e.Action(), e.EffectiveMode())
	}
	if (Entry{}).Action() != "" {
		t.Fatal("an entry without action")
	}
}

// record next to another action is the fact of that action, not a second action; empty, it is a problem.
func TestEntryRecordNextToAction(t *testing.T) {
	rec, require, empty := "asked", "review", ""
	e := Entry{Require: &require, Record: &rec}
	if e.Action() != "require" {
		t.Fatalf("the action is require, got %q", e.Action())
	}
	if err := e.Valid(); err != nil {
		t.Fatalf("require with record is valid: %v", err)
	}
	cmd := Entry{Run: &Run{Command: []string{"ls"}}, Record: &rec}
	if cmd.Action() != "run" || cmd.Valid() != nil {
		t.Fatalf("run with record is valid and its action is run: %q %v", cmd.Action(), cmd.Valid())
	}
	err := (Entry{Require: &require, Record: &empty}).Valid()
	if err == nil || !strings.Contains(err.Error(), "record needs the name of the fact") {
		t.Fatalf("an empty record next to an action is a problem: %v", err)
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
