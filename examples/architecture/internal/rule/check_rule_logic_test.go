package rule

import "testing"

// The logic is pure: its tests pass domain values and compare the output, without fakes.

var someRules = []Rule{
	{ID: "go", Scope: []string{"**/*.go"}, Deterministic: true, Check: "ai/checks/gofmt.sh"},
	{ID: "sh", Scope: []string{"**/*.sh"}, Deterministic: true, Check: "ai/checks/shfmt.sh"},
	{ID: "prose", Scope: []string{"**/*.go"}}, // no check
}

func TestCheckRuleAll(t *testing.T) {
	out := CheckRule(someRules, false, nil)
	if len(out.Run) != 2 || out.Run[0].ID != "go" || out.Run[1].ID != "sh" {
		t.Fatalf("got %+v", out)
	}
}

func TestCheckRuleOnlyChanged(t *testing.T) {
	out := CheckRule(someRules, true, []string{"internal/rule/rule.go"})
	if len(out.Run) != 1 || out.Run[0].ID != "go" {
		t.Fatalf("got %+v", out)
	}
}

func TestCheckRuleNothingChanged(t *testing.T) {
	if out := CheckRule(someRules, true, nil); len(out.Run) != 0 {
		t.Fatalf("got %+v", out)
	}
}
