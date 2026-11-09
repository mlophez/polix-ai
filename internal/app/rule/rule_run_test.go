package rule

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/mlophez/polix-ai/internal/app/workflow"
)

func flow(argv ...string) workflow.Workflow {
	return workflow.Workflow{Version: 1, Rules: workflow.Rules{Run: []workflow.Runner{{Glob: "*_test.go", Argv: argv}}}}
}

func archRules(n int) []Rule {
	var out []Rule
	for i := 1; i <= n; i++ {
		out = append(out, Rule{ID: fmt.Sprintf("A-%d", i), Description: "d", Deterministic: true,
			Check: fmt.Sprintf("tests/architecture/a%02d_test.go", i)})
	}
	return out
}

func ruleIDs(rules []Rule) string {
	var s []string
	for _, r := range rules {
		s = append(s, r.ID)
	}
	return strings.Join(s, ",")
}

func TestRunValid(t *testing.T) {
	if err := (Run{Rules: []Rule{goRule}, Argv: []string{"go", "test"}}).Valid(); err != nil {
		t.Fatal(err)
	}
	if err := (Run{Rules: []Rule{goRule}}).Valid(); err != nil {
		t.Fatalf("a run without argv runs the check as a program: %v", err)
	}
	if err := (Run{Rules: []Rule{goRule, goRule}, Argv: []string{"x"}}).Valid(); err != nil {
		t.Fatal(err)
	}
	invalid := map[string]Run{
		"no rules":                     {Argv: []string{"x"}},
		"a rule without check":         {Rules: []Rule{{ID: "prose", Description: "no check"}}},
		"several rules without a argv": {Rules: []Rule{goRule, goRule}},
	}
	for name, r := range invalid {
		if r.Valid() == nil {
			t.Errorf("%s is valid", name)
		}
	}
}

func TestPlan(t *testing.T) {
	rules := append(archRules(33), Rule{ID: "P", Description: "prose"}, Rule{ID: "S", Description: "d",
		Deterministic: true, Check: "scripts/s.sh"})
	runs := Plan(rules, flow("go", "test", "{dir}", "-run", "^Test({names})$"))
	if len(runs) != 2 || len(runs[0].Rules) != 33 || ruleIDs(runs[0].Rules) != ruleIDs(archRules(33)) {
		t.Fatalf("got %d runs: %+v", len(runs), runs)
	}
	if !strings.HasPrefix(strings.Join(runs[0].Argv, " "), "go test ./tests/architecture -run ^Test(A1|A2|") {
		t.Fatalf("argv %q", runs[0].Argv)
	}
	if ruleIDs(runs[1].Rules) != "S" || runs[1].Argv != nil {
		t.Fatalf("a check without runner runs alone as a program: %+v", runs[1])
	}
}

func TestRunSettle(t *testing.T) {
	batch := flow("go", "test", "{dir}", "-run", "^Test({names})$")
	rules := archRules(2)
	shared := Plan(rules, batch)[0]
	fail := Outcome{Output: "boom", Err: errors.New("exit status 1")}

	results, reruns := shared.Settle(Outcome{Output: "ok"}, batch)
	if len(results) != 2 || len(reruns) != 0 || !results[0].Passed() || results[1].Output != "ok" {
		t.Fatalf("a passed shared run: %+v %+v", results, reruns)
	}

	results, reruns = Plan(rules[:1], batch)[0].Settle(fail, batch)
	if len(results) != 1 || results[0].Passed() || len(reruns) != 0 {
		t.Fatalf("a failed single run: %+v %+v", results, reruns)
	}

	results, reruns = shared.Settle(fail, batch)
	if len(results) != 0 || len(reruns) != 2 || ruleIDs(reruns[1].Rules) != "A-2" ||
		strings.Join(reruns[1].Argv, " ") != "go test ./tests/architecture -run ^Test(A2)$" {
		t.Fatalf("a failed shared run splits: %+v %+v", results, reruns)
	}

	all := flow("go", "test", "./...")
	results, reruns = Plan(rules, all)[0].Settle(fail, all)
	if len(results) != 2 || len(reruns) != 0 || results[0].Passed() || results[1].Output != "boom" {
		t.Fatalf("a failed shared run without placeholders: %+v %+v", results, reruns)
	}
}
