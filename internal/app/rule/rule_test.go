package rule

import (
	"errors"
	"testing"
)

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

func TestLoadedValid(t *testing.T) {
	boom := errors.New("boom")
	rules := []Rule{{ID: "A-1"}}
	for _, tc := range []struct {
		name   string
		loaded Loaded
		valid  bool
	}{
		{"no folder", Loaded{}, true},
		{"rules of the folder", Loaded{Rules: rules, Exists: true, Problems: []string{"x"}}, true},
		{"a folder that cannot be read", Loaded{Exists: true, Err: boom, Strict: boom}, true},
		{"rules with the error", Loaded{Rules: rules, Exists: true, Err: boom, Strict: boom}, false},
		{"problems with the error", Loaded{Exists: true, Problems: []string{"x"}, Err: boom, Strict: boom}, false},
		{"an error without a strict one", Loaded{Exists: true, Err: boom}, false},
		{"rules without the folder", Loaded{Rules: rules}, false},
	} {
		if err := tc.loaded.Valid(); (err == nil) != tc.valid {
			t.Errorf("%s: Valid() = %v, want valid %v", tc.name, err, tc.valid)
		}
	}
}

// Fail drops what was read and stops every reading at its error, on a copy.
func TestLoadedFail(t *testing.T) {
	boom := errors.New("boom")
	l := Loaded{Rules: []Rule{{ID: "A-1"}}, Exists: true, Problems: []string{"x"}}
	failed := l.Fail(boom)
	if failed.Rules != nil || failed.Problems != nil || !failed.Exists || !errors.Is(failed.Err, boom) ||
		!errors.Is(failed.Strict, boom) || failed.Valid() != nil {
		t.Fatalf("Fail = %+v", failed)
	}
	if len(l.Rules) != 1 || l.Err != nil {
		t.Fatal("Fail returns a copy")
	}
}
