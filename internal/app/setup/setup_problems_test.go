package setup

import (
	"errors"
	"strings"
	"testing"

	"github.com/mlophez/polix-ai/internal/app/rule"
	"github.com/mlophez/polix-ai/internal/app/workflow"
)

func TestWorkflowProblems(t *testing.T) {
	if p, newer := (Inspection{}).WorkflowProblems(); p != nil || newer {
		t.Fatal("no error, no problem")
	}
	p, newer := Inspection{WorkflowErr: workflow.InvalidError{`polix.yaml:3: unknown key "foo"`, "x"}}.WorkflowProblems()
	if len(p) != 2 || !newer {
		t.Fatalf("each problem, and unknown options hint a newer polix: %v %v", p, newer)
	}
	if p, newer := (Inspection{WorkflowErr: workflow.TooOldError{Requires: "0.2.0", Running: "0.1.0"}}).WorkflowProblems(); len(p) != 1 || newer ||
		!strings.Contains(p[0], "requires polix 0.2.0") {
		t.Fatalf("too old: %v %v", p, newer)
	}
	if _, newer := (Inspection{WorkflowErr: errors.New("polix.yaml: yaml: broken")}).WorkflowProblems(); newer {
		t.Fatal("broken yaml is no newer polix")
	}
}

var problemRules = []rule.Rule{
	{ID: "A-1", Description: "d", Deterministic: true, Check: "checks/a.sh", File: "ai/rules/a.yaml"},
	{ID: "A-1", Description: "d", File: "ai/rules/b.yaml"},
	{ID: "B-1", Description: "d", Deterministic: true, Check: "tests/b_test.go", File: "ai/rules/b.yaml"},
}

func TestRuleProblems(t *testing.T) {
	i := Inspection{Rules: problemRules, Invalid: []string{"ai/rules/c.yaml C-1: no description"},
		Missing: map[string]bool{"checks/a.sh": true}}
	got := i.RuleProblems()
	want := []string{"ai/rules/c.yaml C-1: no description", "ai/rules/b.yaml A-1: id repeated, also in ai/rules/a.yaml",
		"ai/rules/a.yaml A-1: check checks/a.sh does not exist"}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("got\n%s", strings.Join(got, "\n"))
	}
}

func TestBaseWarning(t *testing.T) {
	err := errors.New("no base branch")
	if got := (Inspection{Rules: problemRules, BaseErr: err}).BaseWarning(); got != "polix rules check --changed would fail: no base branch; set base_branch in polix.yaml or pass --base" {
		t.Fatal(got)
	}
	if (Inspection{Rules: problemRules}).BaseWarning() != "" || (Inspection{Rules: problemRules[1:2], BaseErr: err}).BaseWarning() != "" {
		t.Fatal("only without base and with a check")
	}
	if !strings.Contains((Inspection{Setup: Setup{Version: "0.1.0"}}).NewerHint(), "(0.1.0): run polix update") {
		t.Fatal("hint")
	}
}
