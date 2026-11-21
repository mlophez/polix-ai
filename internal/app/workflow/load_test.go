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
	valid := Workflow{Version: 1, StopRetries: 3}
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

// TooOldError names both versions and asks for an update.
func TestTooOld(t *testing.T) {
	old := TooOldError{Requires: "0.3.0", Running: "0.2.1"}
	if err := old.Valid(); err != nil {
		t.Fatal(err)
	}
	if old.Error() != "polix.yaml requires polix 0.3.0 or newer and this is 0.2.1: run polix update" {
		t.Fatalf("message: %v", old)
	}
	if (TooOldError{Requires: "0.3.0"}).Valid() == nil {
		t.Fatal("a TooOldError without the running version is valid")
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
		e := UnknownKeyError{Key: key, Line: 2}
		if err := e.Valid(); err != nil || e.Error() != want {
			t.Errorf("%q: %v %q", key, err, e.Error())
		}
		if renamed := strings.Contains(want, ": use "); Problem(e.Error()).Renamed() != renamed {
			t.Errorf("%q: renamed %v", key, !renamed)
		}
	}
	if (UnknownKeyError{Key: "x"}).Valid() == nil {
		t.Fatal("an unknown key without its line is valid")
	}
}

// TestWorkflowLoaded tests what polix.yaml loads: the default workflow without the file, the error that stops the
// reading, the problems of the workflow and of its shortcuts together, and a failed result for what cannot load.
func TestWorkflowLoaded(t *testing.T) {
	// Without polix.yaml the workflow is the default one: version 1, the default loop guard, the default folder of
	// the AI and no entries.
	t.Run("without the file", func(t *testing.T) {
		got := Workflow{}.Loaded(Guardrails{}, false, nil)
		w := got.Workflow
		if got.Exists || got.Err != nil || w.Version != 1 || w.StopRetries != DefaultStopRetries ||
			w.PolixPath() != DefaultPolixPath || len(w.On) != 0 {
			t.Fatalf("got %+v", got)
		}
	})

	// An error that stops the reading keeps no workflow; the problems of a workflow come with it, as they were read.
	t.Run("keeps the error", func(t *testing.T) {
		boom := errors.New("polix.yaml: yaml: did not find expected node content")
		if got := (Workflow{}).Loaded(Guardrails{}, true, boom); !got.Exists || !errors.Is(got.Err, boom) ||
			got.Workflow.Version != 0 {
			t.Fatalf("broken: %+v", got)
		}
		old := TooOldError{Requires: "0.3.0", Running: "0.2.0"}
		if got := (Workflow{}).Loaded(Guardrails{}, true, old); !errors.As(got.Err, &old) {
			t.Fatalf("too old: %+v", got)
		}
		wf := Workflow{Version: 2, StopRetries: 3}
		got := wf.Loaded(Guardrails{}, true, wf.Valid())
		if _, ok := errors.AsType[InvalidError](got.Err); !ok || got.Workflow.Version != 2 ||
			got.Err.Error() != "polix.yaml: version must be 1, got 2" {
			t.Fatalf("invalid: %+v", got)
		}
		valid := Workflow{Version: 1, StopRetries: 3}
		if got := valid.Loaded(Guardrails{}, true, nil); got.Err != nil || got.Workflow.Version != 1 {
			t.Fatalf("valid: %+v", got)
		}
	})

	// The problems of the values of the shortcuts come with those of the workflow, in the order of the file.
	t.Run("merges the problems of the shortcuts", func(t *testing.T) {
		wf := Workflow{Version: 2, StopRetries: 3,
			On: map[string][]Entry{PreEdit: {{Line: 6, Keys: []string{"blok"}}}}}
		guardrails := Guardrails{ProtectedFiles: []GuardrailItem{{Text: "", Line: 3}}}
		got := wf.Loaded(guardrails, true, wf.Valid())
		want := "polix.yaml: version must be 1, got 2\npolix.yaml:3: guardrails.protected_files: a path glob cannot be empty\n" +
			"polix.yaml:6: "
		if _, ok := errors.AsType[InvalidError](got.Err); !ok || !strings.HasPrefix(got.Err.Error(), want) {
			t.Fatalf("got %v", got.Err)
		}
	})

	// What polix.yaml cannot load is a failed result: an error without the file, or a workflow with problems without
	// them.
	t.Run("fails", func(t *testing.T) {
		boom := errors.New("boom")
		if got := (Workflow{}).Loaded(Guardrails{}, false, boom); got.Err == nil ||
			!strings.Contains(got.Err.Error(), "an error loaded without polix.yaml") {
			t.Fatalf("an error without the file: %+v", got)
		}
		got := Workflow{Version: 2}.Loaded(Guardrails{}, true, nil)
		if got.Err == nil || got.Workflow.Version != 0 || !strings.Contains(got.Err.Error(), "without its problems") {
			t.Fatalf("problems left out: %+v", got)
		}
	})
}
