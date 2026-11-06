package setup

import (
	"errors"
	"testing"

	"github.com/mlophez/polix-ai/internal/rule"
	"github.com/mlophez/polix-ai/internal/workflow"
)

func TestCheckSetupValid(t *testing.T) {
	rules := []rule.Rule{{ID: "S-1", Description: "d", Deterministic: true, Check: "checks/s.sh", File: "ai/rules/s.yaml"}}
	out := CheckSetup("0.1.0", true, nil, "ai/rules", rules, nil, nil, map[string]bool{}, errors.New("no main"))
	if len(out.Problems) != 0 || out.Rules != 1 || !out.Found || out.Warning == "" || out.RulesPath != "ai/rules" {
		t.Fatalf("%+v", out)
	}
}

func TestCheckSetupProblems(t *testing.T) {
	out := CheckSetup("0.1.0", true, workflow.Invalid{`unknown key "an_option"`}, "ai/rules", nil,
		[]string{"ai/rules/a.yaml A-1: no description"}, nil, nil, errors.New("no main"))
	if len(out.Problems) != 2 || out.Hint == "" || out.Warning != "" {
		t.Fatalf("%+v", out)
	}
	out = CheckSetup("0.1.0", false, nil, "ai/rules", nil, nil, errors.New("ai/rules/x.yaml: yaml: broken"), nil, nil)
	if len(out.Problems) != 1 || out.Hint != "" {
		t.Fatalf("%+v", out)
	}
}
