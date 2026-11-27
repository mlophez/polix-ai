package check

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/mlophez/polix-ai/internal/app/setup"
	"github.com/mlophez/polix-ai/internal/app/workflow"
)

// observed returns the inspection i as polix 0.1.0 observes it, valid.
func observed(t *testing.T, i setup.Inspection) setup.Inspection {
	t.Helper()
	i.Setup = setup.Setup{Version: "0.1.0"}
	if err := i.Valid(); err != nil {
		t.Fatal(err)
	}
	return i
}

func TestDecideValid(t *testing.T) {
	out := Decide(observed(t, setup.Inspection{Found: true}), "")
	if len(out.WorkflowProblems) != 0 || !out.Found || out.File != workflow.File || out.Hint != "" {
		t.Fatalf("%+v", out)
	}
}

func TestDecideProblems(t *testing.T) {
	out := Decide(observed(t, setup.Inspection{Found: true, WorkflowErr: workflow.InvalidError{`unknown key "an_option"`}}), "")
	if len(out.WorkflowProblems) != 1 || out.Hint == "" {
		t.Fatalf("%+v", out)
	}
	out = Decide(observed(t, setup.Inspection{Found: true, WorkflowErr: errors.New("read polix.yaml: broken")}), "")
	if len(out.WorkflowProblems) != 1 || out.Hint != "" {
		t.Fatalf("%+v", out)
	}
}

// AC3: a top-level key renamed by this polix is reported with its new spelling, never as an option of a newer one.
func TestDecideRenamedKey(t *testing.T) {
	for _, key := range []string{"base", "max_stop_blocks", "protected"} {
		err := workflow.UnknownKeyError{Key: key, Line: 2}
		out := Decide(observed(t, setup.Inspection{Found: true, WorkflowErr: err}), "")
		if len(out.WorkflowProblems) != 1 || !strings.Contains(out.WorkflowProblems[0], ": use ") || out.Hint != "" {
			t.Errorf("%q: %+v", key, out)
		}
	}
}

// A workflow file named by the argument must exist, and its problems name it instead of polix.yaml.
func TestDecideFile(t *testing.T) {
	out := Decide(observed(t, setup.Inspection{}), "examples/polix.yaml")
	if len(out.WorkflowProblems) != 1 ||
		out.WorkflowProblems[0] != "examples/polix.yaml not found" || out.File != "examples/polix.yaml" {
		t.Fatalf("missing: %+v", out)
	}
	err := workflow.InvalidError{`polix.yaml:4: unknown key "rule" in run`, `polix.yaml: requirements: "go" is repeated`}
	out = Decide(observed(t, setup.Inspection{Found: true, WorkflowErr: err}), "examples/polix.yaml")
	want := []string{`examples/polix.yaml:4: unknown key "rule" in run`,
		`examples/polix.yaml: requirements: "go" is repeated`}
	if !slices.Equal(out.WorkflowProblems, want) {
		t.Fatalf("got %q, want %q", out.WorkflowProblems, want)
	}
	if out := Decide(observed(t, setup.Inspection{Found: true}), ""); out.File != workflow.File {
		t.Fatalf("polix.yaml: %+v", out)
	}
	tooOld := workflow.TooOldError{Requires: "9.0.0", Running: "0.2.0"}
	out = Decide(observed(t, setup.Inspection{Found: true, WorkflowErr: tooOld}), "x/polix.yaml")
	if len(out.WorkflowProblems) != 1 || !strings.HasPrefix(out.WorkflowProblems[0], "x/polix.yaml requires polix 9.0.0") {
		t.Fatalf("too old: %q", out.WorkflowProblems)
	}
	// The error of a file that cannot be read already names its path: it is not renamed.
	for _, file := range []string{"x/polix.yaml", "my polix.yaml"} {
		unreadable := errors.New("read " + file + ": is a directory")
		out = Decide(observed(t, setup.Inspection{Found: true, WorkflowErr: unreadable}), file)
		if len(out.WorkflowProblems) != 1 || out.WorkflowProblems[0] != "read "+file+": is a directory" {
			t.Fatalf("unreadable %q: %q", file, out.WorkflowProblems)
		}
	}
	// The error of the workflow, which the loader keeps, is left as it was.
	invalid := workflow.InvalidError{"polix.yaml:2: x"}
	Decide(observed(t, setup.Inspection{Found: true, WorkflowErr: invalid}), "x/polix.yaml")
	if invalid[0] != "polix.yaml:2: x" {
		t.Fatalf("the input changed: %q", invalid)
	}
}
