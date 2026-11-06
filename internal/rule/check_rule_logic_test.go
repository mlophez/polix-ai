package rule

import (
	"errors"
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

// walk drives the logic as the shell does, answering each run with outcome, and returns the runs it asked for
// and the final output.
func walk(rules []Rule, wf workflow.Workflow, onlyChanged bool, changed []string,
	outcome func(Run) Outcome) ([]Run, CheckRuleOutput) {
	var runs []Run
	var outcomes []Outcome
	for {
		out := CheckRule(rules, wf, onlyChanged, changed, outcomes)
		if out.Next == nil {
			return runs, out
		}
		runs = append(runs, *out.Next)
		outcomes = append(outcomes, outcome(*out.Next))
	}
}

func passAll(Run) Outcome { return Outcome{Output: "ok"} }

// resultIDs are the ids of the rules of the results.
func resultIDs(results []Result) []string {
	var out []string
	for _, r := range results {
		out = append(out, r.Rule.ID)
	}
	return out
}

func TestCheckRuleNoCheck(t *testing.T) {
	if runs, out := walk(someRules[2:], workflow.Workflow{}, false, nil, passAll); len(runs)+len(out.Results) != 0 {
		t.Fatalf("a rule without check runs: %+v", out)
	}
}

func TestCheckRuleAll(t *testing.T) {
	_, out := walk(someRules, workflow.Workflow{}, false, nil, passAll)
	if got := resultIDs(out.Results); !slices.Equal(got, []string{"go", "sh"}) {
		t.Fatalf("got %v", got)
	}
}

func TestCheckRuleOnlyChanged(t *testing.T) {
	_, out := walk(someRules, workflow.Workflow{}, true, []string{"internal/rule/rule.go"}, passAll)
	if got := resultIDs(out.Results); !slices.Equal(got, []string{"go"}) {
		t.Fatalf("got %v", got)
	}
}

func TestCheckRuleChangedOutOfEveryScope(t *testing.T) {
	if runs, _ := walk(someRules, workflow.Workflow{}, true, []string{"README.md"}, passAll); len(runs) != 0 {
		t.Fatalf("got %+v", runs)
	}
}

func TestCheckRuleNothingChanged(t *testing.T) {
	if runs, _ := walk(someRules, workflow.Workflow{}, true, nil, passAll); len(runs) != 0 {
		t.Fatalf("got %+v", runs)
	}
}

func TestCheckRuleWithoutScopeRunsWhenSomethingChanged(t *testing.T) {
	all := []Rule{{ID: "all", Description: "d", Deterministic: true, Check: "c.sh"}}
	if runs, _ := walk(all, workflow.Workflow{}, true, []string{"README.md"}, passAll); len(runs) != 1 {
		t.Fatalf("got %+v", runs)
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
	runs, _ := walk(all, wf, false, nil, passAll)
	if len(runs) != 2 {
		t.Fatalf("got %+v", runs)
	}
	want := []string{"go", "test", "./tests/architecture", "-run", "^TestGOARCH4$"}
	if !slices.Equal(runs[0].Argv, want) {
		t.Fatalf("the argv of the runner is %q, want %q", runs[0].Argv, want)
	}
	if runs[1].Argv != nil {
		t.Fatalf("a check no runner matches runs as a program, got %q", runs[1].Argv)
	}
}

// batchFlow runs the Go tests of a folder together, selected by their names.
var batchFlow = workflow.Workflow{Rules: workflow.Rules{Run: []workflow.Runner{
	{Glob: "*_test.go", Argv: []string{"go", "test", "{dir}", "-run", "^Test({names})$"}},
}}}

func TestCheckRuleBatchAsksForTheSharedRun(t *testing.T) {
	out := CheckRule(archRules(3), batchFlow, false, nil, nil)
	if out.Next == nil || len(out.Next.Rules) != 3 || out.Results != nil {
		t.Fatalf("got %+v", out)
	}
	out = CheckRule(archRules(3), batchFlow, false, nil, []Outcome{{Output: "ok"}})
	if out.Next != nil || len(out.Results) != 3 || len(Failed(out.Results)) != 0 {
		t.Fatalf("a passing shared run passes every rule: %+v", out)
	}
}

func TestCheckRuleBatchRerunsAFailedGroup(t *testing.T) {
	fail := Outcome{Output: "boom", Err: errors.New("exit status 1")}
	out := CheckRule(archRules(3), batchFlow, false, nil, []Outcome{fail})
	if out.Next == nil || len(out.Next.Rules) != 1 || out.Next.Rules[0].ID != "A-1" {
		t.Fatalf("a failed shared run asks for the first rerun: %+v", out)
	}
	out = CheckRule(archRules(3), batchFlow, false, nil, []Outcome{fail, {Output: "ok"}, fail, {Output: "ok"}})
	if out.Next != nil || len(out.Results) != 3 {
		t.Fatalf("got %+v", out)
	}
	if got := resultIDs(Failed(out.Results)); !slices.Equal(got, []string{"A-2"}) {
		t.Fatalf("only the rules whose rerun fails fail: %v", got)
	}
}

func TestCheckRuleBatchWithoutPlaceholdersFailsEveryRule(t *testing.T) {
	wf := workflow.Workflow{Rules: workflow.Rules{Run: []workflow.Runner{
		{Glob: "*_test.go", Argv: []string{"go", "test", "./..."}},
	}}}
	fail := func(Run) Outcome { return Outcome{Output: "boom", Err: errors.New("exit status 1")} }
	runs, out := walk(archRules(3), wf, false, nil, fail)
	if len(runs) != 1 || len(Failed(out.Results)) != 3 {
		t.Fatalf("got %d runs and %+v", len(runs), out)
	}
}

func TestCheckRuleBatchOnlyChanged(t *testing.T) {
	rules := archRules(3)
	rules[1].Scope = []string{"docs/**"}
	runs, out := walk(rules, batchFlow, true, []string{"docs/a.md"}, passAll)
	if len(runs) != 1 || len(out.Results) != 3 {
		t.Fatalf("got %+v %+v", runs, out)
	}
	rules[0].Scope, rules[2].Scope = []string{"src/**"}, []string{"src/**"}
	runs, out = walk(rules, batchFlow, true, []string{"docs/a.md"}, passAll)
	if len(runs) != 1 || !slices.Equal(resultIDs(out.Results), []string{"A-2"}) {
		t.Fatalf("--changed narrows the group: %+v", out)
	}
}
