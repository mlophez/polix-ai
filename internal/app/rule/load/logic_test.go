package load

import (
	"errors"
	"strings"
	"testing"

	"github.com/mlophez/polix-ai/internal/app/rule"
)

// Every rule is kept, invalid ones included, ordered by id, with the problem of each invalid one in the words of
// polix config check, in the order of the files; a strict reading stops at the first invalid one, with its file.
func TestDecideKeepsEveryRule(t *testing.T) {
	rules := []rule.Rule{
		{ID: "B-1", Description: "ok", File: "ai/rules/a.yaml"},
		{ID: "A-2", File: "ai/rules/a.yaml"},
		{Description: "no id", File: "ai/rules/a.yaml"},
		{ID: "A-3", Description: "d", Check: "checks/a.sh", File: "ai/rules/a.yaml"},
		{ID: "A-4", Description: "d", Deterministic: true, Check: "/abs/a.sh", File: "ai/rules/a.yaml"},
		{ID: "A-1", Description: "d", File: "ai/rules/b.yaml"},
	}
	got := Decide(rules, true, nil).Loaded
	want := []string{
		"ai/rules/a.yaml A-2: no description",
		"ai/rules/a.yaml: a rule has no id",
		"ai/rules/a.yaml A-3: check checks/a.sh needs deterministic: true",
		"ai/rules/a.yaml A-4: check /abs/a.sh is an absolute path, not relative to the root",
	}
	if strings.Join(got.Problems, "\n") != strings.Join(want, "\n") {
		t.Fatalf("problems:\n%s", strings.Join(got.Problems, "\n"))
	}
	ids := make([]string, len(got.Rules))
	for i, r := range got.Rules {
		ids[i] = r.ID
	}
	if strings.Join(ids, ",") != ",A-1,A-2,A-3,A-4,B-1" || !got.Exists || got.Err != nil {
		t.Fatalf("rules %v, exists %v, err %v", ids, got.Exists, got.Err)
	}
	if got.Strict == nil || got.Strict.Error() != "ai/rules/a.yaml: rule A-2 has no description" {
		t.Fatalf("strict: %v", got.Strict)
	}
}

// Valid rules leave nothing to stop at; no rules, without the folder, are none.
func TestDecideValidRules(t *testing.T) {
	rules := []rule.Rule{{ID: "GO-1", Description: "gofmt", Deterministic: true, Check: "ai/checks/gofmt.sh",
		Scope: []string{"**/*.go"}, File: "ai/rules/go.yaml"}}
	got := Decide(rules, true, nil).Loaded
	if len(got.Rules) != 1 || got.Rules[0].ID != "GO-1" || got.Problems != nil || got.Err != nil || got.Strict != nil {
		t.Fatalf("got %+v", got)
	}
	if got := Decide(nil, false, nil).Loaded; got.Rules != nil || got.Exists || got.Err != nil || got.Strict != nil {
		t.Fatalf("no folder: %+v", got)
	}
}

// A file that cannot be read or is not yaml stops the reading: no rules nor problems. A strict reading stops at an
// invalid rule of a file before it, else at the error.
func TestDecideStopsAtAFileThatCannotBeRead(t *testing.T) {
	broken := errors.New("ai/rules/c.yaml: yaml: did not find expected node content")
	got := Decide(nil, true, broken).Loaded
	if got.Rules != nil || got.Problems != nil || !errors.Is(got.Err, broken) || !errors.Is(got.Strict, broken) ||
		!got.Exists {
		t.Fatalf("got %+v", got)
	}
	before := []rule.Rule{{ID: "A-1", Description: "d", File: "ai/rules/a.yaml"}, {ID: "B-1", File: "ai/rules/b.yaml"}}
	got = Decide(before, true, broken).Loaded
	if got.Rules != nil || !errors.Is(got.Err, broken) || got.Strict == nil ||
		got.Strict.Error() != "ai/rules/b.yaml: rule B-1 has no description" {
		t.Fatalf("an invalid rule before: %+v", got)
	}
}
