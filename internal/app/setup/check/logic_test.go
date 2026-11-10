package check

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/mlophez/polix-ai/internal/app/rule"
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
	rules := []rule.Rule{{ID: "S-1", Description: "d", Deterministic: true, Check: "checks/s.sh", File: "ai/rules/s.yaml"}}
	out := Decide(observed(t, setup.Inspection{Found: true, Rules: rules, Missing: map[string]bool{},
		BaseErr: errors.New("no main")}), "")
	if len(out.Problems) != 0 || out.Rules != 1 || !out.Found || out.Warning == "" || out.RulesPath != "ai/rules" {
		t.Fatalf("%+v", out)
	}
}

func TestDecideProblems(t *testing.T) {
	out := Decide(observed(t, setup.Inspection{Found: true, WorkflowErr: workflow.InvalidError{`unknown key "an_option"`},
		Invalid: []string{"ai/rules/a.yaml A-1: no description"}, BaseErr: errors.New("no main")}), "")
	if len(out.Problems) != 2 || out.Hint == "" || out.Warning != "" {
		t.Fatalf("%+v", out)
	}
	out = Decide(observed(t, setup.Inspection{RulesErr: errors.New("ai/rules/x.yaml: yaml: broken")}), "")
	if len(out.Problems) != 1 || out.Hint != "" {
		t.Fatalf("%+v", out)
	}
}

// AC3: a top-level key renamed by this polix is reported with its new spelling, never as an option of a newer one.
func TestDecideRenamedKey(t *testing.T) {
	for _, key := range []string{"base", "max_stop_blocks", "protected"} {
		err := workflow.UnknownKeyError{Key: key, Line: 2}
		out := Decide(observed(t, setup.Inspection{Found: true, WorkflowErr: err}), "")
		if len(out.Problems) != 1 || !strings.Contains(out.Problems[0], ": use ") || out.Hint != "" {
			t.Errorf("%q: %+v", key, out)
		}
	}
}

// A workflow file named by the argument must exist, and its problems name it instead of polix.yaml; the problems of
// the rules keep their files.
func TestDecideFile(t *testing.T) {
	out := Decide(observed(t, setup.Inspection{}), "examples/polix.yaml")
	if len(out.Problems) != 1 || out.Problems[0] != "examples/polix.yaml not found" || out.File != "examples/polix.yaml" {
		t.Fatalf("missing: %+v", out)
	}
	err := workflow.InvalidError{`polix.yaml:4: unknown key "rule" in run`, `polix.yaml: requirements: "go" is repeated`}
	out = Decide(observed(t, setup.Inspection{Found: true, WorkflowErr: err,
		Invalid: []string{"ai/rules/a.yaml A-1: no description"}}), "examples/polix.yaml")
	want := []string{`examples/polix.yaml:4: unknown key "rule" in run`,
		`examples/polix.yaml: requirements: "go" is repeated`, "ai/rules/a.yaml A-1: no description"}
	if !slices.Equal(out.Problems, want) {
		t.Fatalf("got %q, want %q", out.Problems, want)
	}
	if out := Decide(observed(t, setup.Inspection{Found: true}), ""); out.File != workflow.File {
		t.Fatalf("polix.yaml: %+v", out)
	}
	tooOld := workflow.TooOldError{Requires: "9.0.0", Running: "0.2.0"}
	out = Decide(observed(t, setup.Inspection{Found: true, WorkflowErr: tooOld}), "x/polix.yaml")
	if len(out.Problems) != 1 || !strings.HasPrefix(out.Problems[0], "x/polix.yaml requires polix 9.0.0") {
		t.Fatalf("too old: %q", out.Problems)
	}
	rules := []rule.Rule{{ID: "S-1", Description: "d", Deterministic: true, Check: "c.sh", File: "ai/rules/s.yaml"}}
	out = Decide(observed(t, setup.Inspection{Found: true, Rules: rules, Missing: map[string]bool{},
		BaseErr: errors.New("no main")}), "x/polix.yaml")
	if !strings.Contains(out.Warning, "set base_branch in x/polix.yaml or pass --base") {
		t.Fatalf("warning: %q", out.Warning)
	}
	// A path that ends in polix.yaml, in the error of a file that cannot be read, is not renamed.
	unreadable := errors.New("read x/polix.yaml: is a directory")
	out = Decide(observed(t, setup.Inspection{Found: true, WorkflowErr: unreadable}), "x/polix.yaml")
	if len(out.Problems) != 1 || out.Problems[0] != "read x/polix.yaml: is a directory" {
		t.Fatalf("unreadable: %q", out.Problems)
	}
	// The error of the workflow, which the loader keeps, is left as it was.
	invalid := workflow.InvalidError{"polix.yaml:2: x"}
	Decide(observed(t, setup.Inspection{Found: true, WorkflowErr: invalid}), "x/polix.yaml")
	if invalid[0] != "polix.yaml:2: x" {
		t.Fatalf("the input changed: %q", invalid)
	}
}
