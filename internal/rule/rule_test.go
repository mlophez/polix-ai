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
		{"absolute check", Rule{ID: "GO-1", Description: "d", Deterministic: true, Check: "/tmp/x.sh"}, false},
		{"absolute check on Windows", Rule{ID: "GO-1", Description: "d", Deterministic: true, Check: `C:\x.sh`}, false},
		{"check of a rule that is not deterministic", Rule{ID: "GO-1", Description: "gofmt", Check: "x.sh"}, false},
	} {
		if err := tc.rule.Valid(); (err == nil) != tc.valid {
			t.Errorf("%s: Valid() = %v, want valid %v", tc.name, err, tc.valid)
		}
	}
}

func TestRuleHasCheck(t *testing.T) {
	if !(Rule{Deterministic: true, Check: "c.sh"}).HasCheck() {
		t.Fatal("a deterministic rule with a check has no check")
	}
	if (Rule{Deterministic: true}).HasCheck() || (Rule{Check: "c.sh"}).HasCheck() {
		t.Fatal("a rule without check or not deterministic has a check")
	}
}
