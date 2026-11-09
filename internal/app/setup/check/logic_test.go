package check

import (
	"errors"
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
		BaseErr: errors.New("no main")}))
	if len(out.Problems) != 0 || out.Rules != 1 || !out.Found || out.Warning == "" || out.RulesPath != "ai/rules" {
		t.Fatalf("%+v", out)
	}
}

func TestDecideProblems(t *testing.T) {
	out := Decide(observed(t, setup.Inspection{Found: true, WorkflowErr: workflow.InvalidError{`unknown key "an_option"`},
		Invalid: []string{"ai/rules/a.yaml A-1: no description"}, BaseErr: errors.New("no main")}))
	if len(out.Problems) != 2 || out.Hint == "" || out.Warning != "" {
		t.Fatalf("%+v", out)
	}
	out = Decide(observed(t, setup.Inspection{RulesErr: errors.New("ai/rules/x.yaml: yaml: broken")}))
	if len(out.Problems) != 1 || out.Hint != "" {
		t.Fatalf("%+v", out)
	}
}

// AC3: a top-level key renamed by this polix is reported with its new spelling, never as an option of a newer one.
func TestDecideRenamedKey(t *testing.T) {
	for _, key := range []string{"base", "max_stop_blocks", "protected"} {
		err := workflow.UnknownKeyError{Key: key, Line: 2}
		out := Decide(observed(t, setup.Inspection{Found: true, WorkflowErr: err}))
		if len(out.Problems) != 1 || !strings.Contains(out.Problems[0], ": use ") || out.Hint != "" {
			t.Errorf("%q: %+v", key, out)
		}
	}
}
