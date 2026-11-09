package check

import (
	"errors"
	"strings"
	"testing"

	"github.com/mlophez/polix-ai/internal/app/rule"
	"github.com/mlophez/polix-ai/internal/app/workflow"
)

func TestDecideValid(t *testing.T) {
	rules := []rule.Rule{{ID: "S-1", Description: "d", Deterministic: true, Check: "checks/s.sh", File: "ai/rules/s.yaml"}}
	out := Decide("0.1.0", true, nil, "ai/rules", rules, nil, nil, map[string]bool{}, errors.New("no main"))
	if len(out.Problems) != 0 || out.Rules != 1 || !out.Found || out.Warning == "" || out.RulesPath != "ai/rules" {
		t.Fatalf("%+v", out)
	}
}

func TestDecideProblems(t *testing.T) {
	out := Decide("0.1.0", true, workflow.Invalid{`unknown key "an_option"`}, "ai/rules", nil,
		[]string{"ai/rules/a.yaml A-1: no description"}, nil, nil, errors.New("no main"))
	if len(out.Problems) != 2 || out.Hint == "" || out.Warning != "" {
		t.Fatalf("%+v", out)
	}
	out = Decide("0.1.0", false, nil, "ai/rules", nil, nil, errors.New("ai/rules/x.yaml: yaml: broken"), nil, nil)
	if len(out.Problems) != 1 || out.Hint != "" {
		t.Fatalf("%+v", out)
	}
	if out := Decide("", true, nil, "ai/rules", nil, nil, nil, nil, nil); len(out.Problems) != 1 {
		t.Fatalf("without the running version: %+v", out)
	}
}

// AC3: a top-level key renamed by this polix is reported with its new spelling, never as an option of a newer one.
func TestDecideRenamedKey(t *testing.T) {
	for _, key := range []string{"base", "max_stop_blocks", "protected"} {
		err := workflow.UnknownKey{Key: key, Line: 2}
		out := Decide("0.1.0", true, err, "ai/rules", nil, nil, nil, nil, nil)
		if len(out.Problems) != 1 || !strings.Contains(out.Problems[0], ": use ") || out.Hint != "" {
			t.Errorf("%q: %+v", key, out)
		}
	}
}
