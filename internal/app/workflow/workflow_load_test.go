package workflow

import (
	"errors"
	"strings"
	"testing"
)

// What polix.yaml loads is valid: no error without the file, no workflow with an error that stops the reading, and
// the problems of a workflow with it.
func TestLoadedValid(t *testing.T) {
	boom := errors.New("boom")
	valid := Workflow{Version: 1, MaxStopRetries: 3}
	bad := Workflow{Version: 2}
	cases := []struct {
		name   string
		loaded Loaded
		ok     bool
	}{
		{"the default workflow", Loaded{Workflow: valid}, true},
		{"a valid workflow", Loaded{Workflow: valid, Exists: true}, true},
		{"a file that cannot be read", Loaded{Exists: true, Err: boom}, true},
		{"a workflow with its problems", Loaded{Workflow: bad, Exists: true, Err: bad.Valid()}, true},
		{"an error without the file", Loaded{Err: boom}, false},
		{"a workflow with an error that stops the reading", Loaded{Workflow: valid, Exists: true, Err: boom}, false},
		{"a workflow without its problems", Loaded{Workflow: bad, Exists: true}, false},
	}
	for _, c := range cases {
		if err := c.loaded.Valid(); (err == nil) != c.ok {
			t.Errorf("%s: %v", c.name, err)
		}
	}
}

// A failed result keeps no workflow, and its error stops every use of it.
func TestLoadedFail(t *testing.T) {
	boom := errors.New("boom")
	got := (Loaded{Workflow: Workflow{Version: 2}, Exists: true}).Fail(boom)
	if got.Workflow.Version != 0 || !got.Exists || !errors.Is(got.Err, boom) || got.Valid() != nil {
		t.Fatalf("got %+v", got)
	}
}

// TooOld names both versions and asks for an update.
func TestTooOld(t *testing.T) {
	old := TooOld{Requires: "0.3.0", Running: "0.2.1"}
	if err := old.Valid(); err != nil {
		t.Fatal(err)
	}
	if old.Error() != "polix.yaml requires polix 0.3.0 or newer and this is 0.2.1: run polix update" {
		t.Fatalf("message: %v", old)
	}
	if (TooOld{Requires: "0.3.0"}).Valid() == nil {
		t.Fatal("a TooOld without the running version is valid")
	}
}

// An unknown top level key is reported at its line, with the new spelling of a key of the old vocabulary, which
// Renamed tells from an option of a newer polix.
func TestUnknownKey(t *testing.T) {
	for key, want := range map[string]string{
		"onn":       `polix.yaml:2: unknown key "onn"`,
		"protected": `polix.yaml:2: unknown key "protected": use guardrails.protected_files`,
		"":          `polix.yaml:2: unknown key ""`,
	} {
		e := UnknownKey{Key: key, Line: 2}
		if err := e.Valid(); err != nil || e.Error() != want {
			t.Errorf("%q: %v %q", key, err, e.Error())
		}
		if renamed := strings.Contains(want, ": use "); Problem(e.Error()).Renamed() != renamed {
			t.Errorf("%q: renamed %v", key, !renamed)
		}
	}
	if (UnknownKey{Key: "x"}).Valid() == nil {
		t.Fatal("an unknown key without its line is valid")
	}
}
