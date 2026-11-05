package rule

import (
	"slices"
	"testing"

	"github.com/mlophez/polix-ai/internal/workflow"
)

// The logic is pure: its tests pass domain values and compare the output, without fakes.

// someRules are valid rules, as the logic only ever receives valid ones.
var someRules = []Rule{
	{ID: "go", Description: "gofmt", Scope: []string{"**/*.go"}, Deterministic: true, Check: "ai/checks/gofmt.sh"},
	{ID: "sh", Description: "shfmt", Scope: []string{"**/*.sh"}, Deterministic: true, Check: "ai/checks/shfmt.sh"},
	{ID: "prose", Description: "no check", Scope: []string{"**/*.go"}},
}

// ids are the ids of the rules of the runs.
func ids(runs []Run) []string {
	var out []string
	for _, r := range runs {
		out = append(out, r.Rule.ID)
	}
	return out
}

func TestCheckRuleNoCheck(t *testing.T) {
	if out := CheckRule(someRules[2:], workflow.Workflow{}, false, nil); len(out.Run) != 0 {
		t.Fatalf("a rule without check runs: %+v", out)
	}
}

func TestCheckRuleAll(t *testing.T) {
	out := CheckRule(someRules, workflow.Workflow{}, false, nil)
	if got := ids(out.Run); !slices.Equal(got, []string{"go", "sh"}) {
		t.Fatalf("got %v", got)
	}
}

func TestCheckRuleOnlyChanged(t *testing.T) {
	out := CheckRule(someRules, workflow.Workflow{}, true, []string{"internal/rule/rule.go"})
	if got := ids(out.Run); !slices.Equal(got, []string{"go"}) {
		t.Fatalf("got %v", got)
	}
}

func TestCheckRuleChangedOutOfEveryScope(t *testing.T) {
	if out := CheckRule(someRules, workflow.Workflow{}, true, []string{"README.md"}); len(out.Run) != 0 {
		t.Fatalf("got %+v", out)
	}
}

func TestCheckRuleNothingChanged(t *testing.T) {
	if out := CheckRule(someRules, workflow.Workflow{}, true, nil); len(out.Run) != 0 {
		t.Fatalf("got %+v", out)
	}
}

func TestCheckRuleWithoutScopeRunsWhenSomethingChanged(t *testing.T) {
	all := []Rule{{ID: "all", Description: "d", Deterministic: true, Check: "c.sh"}}
	if out := CheckRule(all, workflow.Workflow{}, true, []string{"README.md"}); len(out.Run) != 1 {
		t.Fatalf("got %+v", out)
	}
}

func TestCheckRuleRunner(t *testing.T) {
	wf := workflow.Workflow{Rules: workflow.Rules{Run: []workflow.Runner{
		{Glob: "*_test.go", Argv: []string{"go", "test", "{dir}", "-run", "^Test{name}$"}},
	}}}
	all := []Rule{
		{ID: "GOARCH-4", Description: "d", Deterministic: true, Check: "tests/architecture/goarch04_test.go"},
		{ID: "SH-1", Description: "d", Deterministic: true, Check: "checks/a.sh"},
	}
	out := CheckRule(all, wf, false, nil)
	if len(out.Run) != 2 {
		t.Fatalf("got %+v", out)
	}
	want := []string{"go", "test", "./tests/architecture", "-run", "^TestGOARCH4$"}
	if !slices.Equal(out.Run[0].Argv, want) {
		t.Fatalf("the argv of the runner is %q, want %q", out.Run[0].Argv, want)
	}
	if out.Run[1].Argv != nil {
		t.Fatalf("a check no runner matches runs as a program, got %q", out.Run[1].Argv)
	}
}
