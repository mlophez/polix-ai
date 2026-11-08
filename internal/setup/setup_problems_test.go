package setup

import (
	"errors"
	"strings"
	"testing"

	"github.com/mlophez/polix-ai/internal/rule"
	"github.com/mlophez/polix-ai/internal/workflow"
)

func TestWorkflowProblems(t *testing.T) {
	if p, newer := WorkflowProblems(nil); p != nil || newer {
		t.Fatal("no error, no problem")
	}
	p, newer := WorkflowProblems(workflow.Invalid{`polix.yaml:3: unknown key "foo"`, "x"})
	if len(p) != 2 || !newer {
		t.Fatalf("each problem, and unknown options hint a newer polix: %v %v", p, newer)
	}
	if p, newer := WorkflowProblems(workflow.TooOld{Requires: "0.2.0", Running: "0.1.0"}); len(p) != 1 || newer ||
		!strings.Contains(p[0], "requires polix 0.2.0") {
		t.Fatalf("too old: %v %v", p, newer)
	}
	if _, newer := WorkflowProblems(errors.New("polix.yaml: yaml: broken")); newer {
		t.Fatal("broken yaml is no newer polix")
	}
}

var problemRules = []rule.Rule{
	{ID: "A-1", Description: "d", Deterministic: true, Check: "checks/a.sh", File: "ai/rules/a.yaml"},
	{ID: "A-1", Description: "d", File: "ai/rules/b.yaml"},
	{ID: "B-1", Description: "d", Deterministic: true, Check: "tests/b_test.go", File: "ai/rules/b.yaml"},
}

func TestRuleProblems(t *testing.T) {
	got := RuleProblems(problemRules, []string{"ai/rules/c.yaml C-1: no description"},
		map[string]bool{"checks/a.sh": true})
	want := []string{"ai/rules/c.yaml C-1: no description", "ai/rules/b.yaml A-1: id repeated, also in ai/rules/a.yaml",
		"ai/rules/a.yaml A-1: check checks/a.sh does not exist"}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("got\n%s", strings.Join(got, "\n"))
	}
}

// A check that a runner runs is never reported as not executable; the same error is reported once.
func TestRunProblems(t *testing.T) {
	wf := workflow.Workflow{Rules: workflow.Rules{Run: []workflow.Runner{{Glob: "*_test.go", Argv: []string{"go", "test"}}}}}
	runnable := map[string]error{"checks/a.sh": errors.New("check checks/a.sh is not executable (chmod +x)"),
		"tests/b_test.go": errors.New("check tests/b_test.go is not executable (chmod +x)")}
	got := RunProblems(problemRules, wf, runnable)
	if len(got) != 1 || got[0] != "ai/rules/a.yaml A-1: check checks/a.sh is not executable (chmod +x)" {
		t.Fatalf("%v", got)
	}
	if got := RunProblems(problemRules, workflow.Workflow{}, runnable); len(got) != 2 {
		t.Fatalf("without runner: %v", got)
	}
}

func TestBaseWarning(t *testing.T) {
	err := errors.New("no base branch")
	if got := BaseWarning(problemRules, err); got != "polix rules check --changed would fail: no base branch; set base_branch in polix.yaml or pass --base" {
		t.Fatal(got)
	}
	if BaseWarning(problemRules, nil) != "" || BaseWarning(problemRules[1:2], err) != "" {
		t.Fatal("only without base and with a check")
	}
	if !strings.Contains(NewerHint("0.1.0"), "(0.1.0): run polix update") {
		t.Fatal("hint")
	}
}
