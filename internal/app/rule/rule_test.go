package rule

import (
	"errors"
	"testing"
)

// complete gives r the fields polix requires that it leaves empty: a kind, a subject and the scope of the whole project.
func complete(r Rule) Rule {
	if r.Kind == "" {
		r.Kind = "unknown"
	}
	if r.Subject == "" {
		r.Subject = "subject"
	}
	if len(r.Scope) == 0 {
		r.Scope = []string{"**"}
	}
	return r
}

func TestRuleValid(t *testing.T) {
	for _, tc := range []struct {
		name  string
		rule  Rule
		valid bool
	}{
		{"complete", complete(Rule{ID: "GO-1", Description: "gofmt", Deterministic: true, Check: "ai/checks/gofmt.sh"}), true},
		{"without check", complete(Rule{ID: "GO-2", Description: "name by intent"}), true},
		{"no id", complete(Rule{Description: "gofmt"}), false},
		{"no description", complete(Rule{ID: "GO-1"}), false},
		{"absolute check", complete(Rule{ID: "GO-1", Description: "d", Deterministic: true, Check: "/tmp/x.sh"}), false},
		{"absolute check on Windows", complete(Rule{ID: "GO-1", Description: "d", Deterministic: true, Check: `C:\x.sh`}),
			false},
		{"check of a rule that is not deterministic", complete(Rule{ID: "GO-1", Description: "gofmt", Check: "x.sh"}),
			false},
	} {
		if err := tc.rule.Valid(); (err == nil) != tc.valid {
			t.Errorf("%s: Valid() = %v, want valid %v", tc.name, err, tc.valid)
		}
	}
}

// Every field of a rule is required (CUJ-02): each case is refused with its problem, in the order of the fields.
func TestValidFields(t *testing.T) {
	const kinds = "layout, dependency, declaration, naming, purity, data, flow, metric, testing, docs, convention or unknown"
	base := Rule{ID: "A-1", Kind: "dependency", Subject: "domain", Description: "d", Scope: []string{"**"},
		Deterministic: true, Check: "c.sh", File: "ai/rules/a.yaml"}
	with := func(edit func(*Rule)) Rule {
		r := base
		edit(&r)
		return r
	}
	for _, tc := range []struct {
		name  string
		rule  Rule
		long  string
		short string
	}{
		{"no kind", with(func(r *Rule) { r.Kind = "" }), "rule A-1 has no kind", "ai/rules/a.yaml A-1: no kind"},
		{"kind outside the list", with(func(r *Rule) { r.Kind = "style" }),
			"rule A-1 has the kind style, not one of " + kinds,
			"ai/rules/a.yaml A-1: kind style is not one of " + kinds},
		{"kind is case sensitive", with(func(r *Rule) { r.Kind = "Layout" }),
			"rule A-1 has the kind Layout, not one of " + kinds,
			"ai/rules/a.yaml A-1: kind Layout is not one of " + kinds},
		{"no subject", with(func(r *Rule) { r.Subject = "" }), "rule A-1 has no subject",
			"ai/rules/a.yaml A-1: no subject"},
		{"blank subject", with(func(r *Rule) { r.Subject = "  " }), "rule A-1 has no subject",
			"ai/rules/a.yaml A-1: no subject"},
		{"no scope", with(func(r *Rule) { r.Scope = nil }), "rule A-1 has no scope", "ai/rules/a.yaml A-1: no scope"},
		{"empty scope", with(func(r *Rule) { r.Scope = []string{} }), "rule A-1 has no scope",
			"ai/rules/a.yaml A-1: no scope"},
		{"no deterministic", with(func(r *Rule) { r.Deterministic, r.DeterministicMissing, r.Check = false, true, "" }),
			"rule A-1 does not say whether it is deterministic", "ai/rules/a.yaml A-1: no deterministic"},
		{"deterministic without check", with(func(r *Rule) { r.Check = "" }),
			"rule A-1 is deterministic but names no check", "ai/rules/a.yaml A-1: deterministic: true needs a check"},
		{"first problem wins", with(func(r *Rule) { r.Kind, r.Subject, r.Scope = "", "", nil }), "rule A-1 has no kind",
			"ai/rules/a.yaml A-1: no kind"},
	} {
		err := tc.rule.Valid()
		if err == nil || err.Error() != tc.long {
			t.Errorf("%s: Valid() = %v, want %q", tc.name, err, tc.long)
		}
		if got := tc.rule.problem(); got != tc.short {
			t.Errorf("%s: problem() = %q, want %q", tc.name, got, tc.short)
		}
	}
	if err := base.Valid(); err != nil {
		t.Errorf("complete rule: %v", err)
	}
}

// kind: unknown is valid for polix (CUJ-03), and every kind of the list is.
func TestValidKindUnknown(t *testing.T) {
	r := Rule{ID: "A-1", Kind: "unknown", Subject: "s", Description: "d", Scope: []string{"**"}}
	if err := r.Valid(); err != nil {
		t.Fatalf("kind unknown: %v", err)
	}
	if len(Kind("").All()) != 12 {
		t.Fatalf("Kinds = %v", Kind("").All())
	}
	for _, k := range Kind("").All() {
		r.Kind = k
		if err := r.Valid(); err != nil {
			t.Errorf("kind %s: %v", k, err)
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
		{"repeated ids among the problems", Loaded{Rules: rules, Exists: true, Problems: []string{"x"},
			Repeated: []string{"x"}}, true},
		{"repeated ids with the error", Loaded{Exists: true, Repeated: []string{"x"}, Err: boom, Strict: boom}, false},
		{"repeated ids out of the problems", Loaded{Rules: rules, Exists: true, Repeated: []string{"x"}}, false},
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
	l := Loaded{Rules: []Rule{{ID: "A-1"}}, Exists: true, Problems: []string{"x"}, Repeated: []string{"x"}}
	failed := l.Fail(boom)
	if failed.Rules != nil || failed.Problems != nil || failed.Repeated != nil || !failed.Exists ||
		!errors.Is(failed.Err, boom) ||
		!errors.Is(failed.Strict, boom) || failed.Valid() != nil {
		t.Fatalf("Fail = %+v", failed)
	}
	if len(l.Rules) != 1 || l.Err != nil {
		t.Fatal("Fail returns a copy")
	}
}
