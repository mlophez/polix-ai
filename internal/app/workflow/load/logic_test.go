package load

import (
	"errors"
	"strings"
	"testing"

	"github.com/mlophez/polix-ai/internal/app/workflow"
)

// Without polix.yaml the workflow is the default one: version 1, the default loop guard, every switch of guidance on
// and no entries.
func TestDecideWithoutTheFile(t *testing.T) {
	got := Decide(workflow.Workflow{}, workflow.Guardrails{}, false, nil).Loaded
	w := got.Workflow
	if got.Exists || got.Err != nil || w.Version != 1 || w.MaxStopRetries != workflow.DefaultMaxStopRetries ||
		!w.Guidance.Commit || !w.Guidance.Edit || !w.Guidance.Layout || len(w.On) != 0 {
		t.Fatalf("got %+v", got)
	}
}

// An error that stops the reading keeps no workflow; the problems of a workflow come with it, as they were read.
func TestDecideKeepsTheError(t *testing.T) {
	boom := errors.New("polix.yaml: yaml: did not find expected node content")
	if got := Decide(workflow.Workflow{}, workflow.Guardrails{}, true, boom).Loaded; !got.Exists ||
		!errors.Is(got.Err, boom) || got.Workflow.Version != 0 {
		t.Fatalf("broken: %+v", got)
	}
	old := workflow.TooOldError{Requires: "0.3.0", Running: "0.2.0"}
	if got := Decide(workflow.Workflow{}, workflow.Guardrails{}, true, old).Loaded; !errors.As(got.Err, &old) {
		t.Fatalf("too old: %+v", got)
	}
	wf := workflow.Workflow{Version: 2, MaxStopRetries: 3}
	got := Decide(wf, workflow.Guardrails{}, true, wf.Valid()).Loaded
	if _, ok := errors.AsType[workflow.InvalidError](got.Err); !ok || got.Workflow.Version != 2 ||
		got.Err.Error() != "polix.yaml: version must be 1, got 2" {
		t.Fatalf("invalid: %+v", got)
	}
	valid := workflow.Workflow{Version: 1, MaxStopRetries: 3}
	if got := Decide(valid, workflow.Guardrails{}, true, nil).Loaded; got.Err != nil || got.Workflow.Version != 1 {
		t.Fatalf("valid: %+v", got)
	}
}

// The problems of the values of the shortcuts come with those of the workflow, in the order of the file.
func TestDecideMergesTheProblemsOfTheShortcuts(t *testing.T) {
	wf := workflow.Workflow{Version: 2, MaxStopRetries: 3,
		On: map[string][]workflow.Entry{workflow.PreEdit: {{Line: 6, Keys: []string{"blok"}}}}}
	guardrails := workflow.Guardrails{ProtectedFiles: []workflow.GuardrailItem{{Text: "", Line: 3}}}
	got := Decide(wf, guardrails, true, wf.Valid()).Loaded
	want := "polix.yaml: version must be 1, got 2\npolix.yaml:3: guardrails.protected_files: a path glob cannot be empty\n" +
		"polix.yaml:6: "
	if _, ok := errors.AsType[workflow.InvalidError](got.Err); !ok || !strings.HasPrefix(got.Err.Error(), want) {
		t.Fatalf("got %v", got.Err)
	}
}

// What polix.yaml cannot load is a failed result: an error without the file, or a workflow with problems without
// them.
func TestDecideFails(t *testing.T) {
	boom := errors.New("boom")
	if got := Decide(workflow.Workflow{}, workflow.Guardrails{}, false, boom).Loaded; got.Err == nil ||
		!strings.Contains(got.Err.Error(), "an error loaded without polix.yaml") {
		t.Fatalf("an error without the file: %+v", got)
	}
	got := Decide(workflow.Workflow{Version: 2}, workflow.Guardrails{}, true, nil).Loaded
	if got.Err == nil || got.Workflow.Version != 0 || !strings.Contains(got.Err.Error(), "without its problems") {
		t.Fatalf("problems left out: %+v", got)
	}
}
