package rule

import "testing"

func TestRuleValid(t *testing.T) {
	for _, tc := range []struct {
		name  string
		rule  Rule
		valid bool
	}{
		{"complete", Rule{ID: "GO-1", Description: "gofmt", Deterministic: true, Check: "ai/checks/gofmt.sh"}, true},
		{"without check", Rule{ID: "GO-2", Description: "name by intent"}, true},
		{"no id", Rule{Description: "gofmt"}, false},
		{"no description", Rule{ID: "GO-1"}, false},
		{"check of a rule that is not deterministic", Rule{ID: "GO-1", Description: "gofmt", Check: "x.sh"}, false},
	} {
		if err := tc.rule.Valid(); (err == nil) != tc.valid {
			t.Errorf("%s: Valid() = %v, want valid %v", tc.name, err, tc.valid)
		}
	}
}

func TestMatching(t *testing.T) {
	all := []Rule{
		{ID: "go", Description: "gofmt", Scope: []string{"**/*.go"}},
		{ID: "all", Description: "the whole project"},
	}
	if got := Matching(all, []string{"README.md"}); len(got) != 1 || got[0].ID != "all" {
		t.Fatalf("a file out of every scope: got %+v", got)
	}
	if got := Matching(all, []string{"internal/rule/rule.go"}); len(got) != 2 {
		t.Fatalf("a Go file: got %+v", got)
	}
	if got := Matching(all, nil); len(got) != 0 {
		t.Fatalf("nothing changed: got %+v", got)
	}
}
