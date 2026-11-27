package rule

import (
	"errors"
	"slices"
	"strings"
	"testing"
)

func TestMatching(t *testing.T) {
	all := []Rule{
		{ID: "ARCH-1", Scope: []string{"src/domain/**"}},
		{ID: "ARCH-2"}, // no scope: the whole project
		{ID: "STYLE-1", Scope: []string{"**/*.tf"}},
	}
	got := Rules(all).Matching([]string{"src/domain/order.go"})
	if len(got) != 2 || got[0].ID != "ARCH-1" || got[1].ID != "ARCH-2" {
		t.Fatalf("Matching = %v", got)
	}
	if got := Rules(all).Matching([]string{"README.md"}); len(got) != 1 || got[0].ID != "ARCH-2" {
		t.Fatalf("a file out of every scope: %v", got)
	}
	if got := Rules(all).Matching(nil); len(got) != 0 {
		t.Fatalf("no paths, no rules: %v", got)
	}
}

func TestRepeated(t *testing.T) {
	all := []Rule{
		{ID: "A-1", File: "ai/rules/a.yaml"},
		{ID: "A-1", File: "ai/rules/b.yaml"},
		{ID: "B-1", File: "ai/rules/b.yaml"},
		{File: "ai/rules/c.yaml"},
	}
	want := []string{"ai/rules/b.yaml A-1: id repeated, also in ai/rules/a.yaml"}
	if got := Rules(all).Repeated(); !slices.Equal(got, want) {
		t.Fatalf("Repeated = %q, want %q", got, want)
	}
	if got := Rules(all[2:]).Repeated(); len(got) != 0 {
		t.Fatalf("no repeated id: %q", got)
	}
}

// A deleted rule frees its id: an id is repeated only among the rules that exist.
func TestRepeatedFreesDeletedIDs(t *testing.T) {
	before := Rules{
		{ID: "A-1", File: "ai/rules/a.yaml"},
		{ID: "A-2", File: "ai/rules/a.yaml"},
	}
	if got := before.Repeated(); len(got) != 0 {
		t.Fatalf("no repeat: %q", got)
	}
	after := Rules{{ID: "A-2", File: "ai/rules/a.yaml"}, {ID: "A-1", File: "ai/rules/a.yaml"}}
	if got := after.Repeated(); len(got) != 0 {
		t.Fatalf("a freed id: %q", got)
	}
	twice := append(after, Rule{ID: "A-1", File: "ai/rules/b.yaml"})
	want := []string{"ai/rules/b.yaml A-1: id repeated, also in ai/rules/a.yaml"}
	if got := twice.Repeated(); !slices.Equal(got, want) {
		t.Fatalf("Repeated = %q, want %q", got, want)
	}
}

func TestSelection(t *testing.T) {
	rs := Rules{{ID: "A-1", Scope: []string{"*.go"}}, {ID: "B-1", Scope: []string{"docs/**"}}}
	every := Selection{}
	if err := every.Valid(); err != nil || len(every.Narrow(rs)) != 2 {
		t.Fatalf("every rule: %v %v", err, every.Narrow(rs))
	}
	changed := Selection{OnlyChanged: true, Changed: []string{"main.go"}}
	if got := changed.Narrow(rs); changed.Valid() != nil || len(got) != 1 || got[0].ID != "A-1" {
		t.Fatalf("only the rules of the changed files: %v", got)
	}
	if none := (Selection{OnlyChanged: true}).Narrow(rs); len(none) != 0 {
		t.Fatalf("nothing changed selects no rule: %v", none)
	}
	if (Selection{Changed: []string{"main.go"}}).Valid() == nil {
		t.Fatal("changed files without OnlyChanged are valid")
	}
}

// TestRulesLoaded tests what a folder of rules loads: every rule, with the problems of the invalid ones, where a
// strict reading stops, and the ids repeated among the rules or taken by the retired ones.
func TestRulesLoaded(t *testing.T) {
	// Every rule is kept, invalid ones included, ordered by id, with the problem of each invalid one in the words of
	// polix config check, in the order of the files; a strict reading stops at the first invalid one, with its file.
	t.Run("keeps every rule", func(t *testing.T) {
		rules := Rules{
			{ID: "B-1", Description: "ok", File: "ai/rules/a.yaml"},
			{ID: "A-2", File: "ai/rules/a.yaml"},
			{Description: "no id", File: "ai/rules/a.yaml"},
			{ID: "A-3", Description: "d", Check: "checks/a.sh", File: "ai/rules/a.yaml"},
			{ID: "A-4", Description: "d", Deterministic: true, Check: "/abs/a.sh", File: "ai/rules/a.yaml"},
			{ID: "A-1", Description: "d", File: "ai/rules/b.yaml"},
		}
		got := rules.Loaded("", true, nil)
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
	})

	// Valid rules leave nothing to stop at; no rules, without the folder, are none.
	t.Run("valid rules", func(t *testing.T) {
		rules := Rules{{ID: "GO-1", Description: "gofmt", Deterministic: true, Check: "ai/checks/gofmt.sh",
			Scope: []string{"**/*.go"}, File: "ai/rules/go.yaml"}}
		got := rules.Loaded("", true, nil)
		if len(got.Rules) != 1 || got.Rules[0].ID != "GO-1" || got.Problems != nil || got.Repeated != nil ||
			got.Err != nil || got.Strict != nil {
			t.Fatalf("got %+v", got)
		}
		if got := Rules(nil).Loaded("", false, nil); got.Rules != nil || got.Exists || got.Err != nil || got.Strict != nil {
			t.Fatalf("no folder: %+v", got)
		}
	})

	// A file that cannot be read or is not yaml stops the reading: no rules nor problems. A strict reading stops at
	// an invalid rule of a file before it, else at the error.
	t.Run("stops at a file that cannot be read", func(t *testing.T) {
		broken := errors.New("ai/rules/c.yaml: yaml: did not find expected node content")
		got := Rules(nil).Loaded("", true, broken)
		if got.Rules != nil || got.Problems != nil || !errors.Is(got.Err, broken) || !errors.Is(got.Strict, broken) ||
			!got.Exists {
			t.Fatalf("got %+v", got)
		}
		before := Rules{{ID: "A-1", Description: "d", File: "ai/rules/a.yaml"}, {ID: "B-1", File: "ai/rules/b.yaml"}}
		got = before.Loaded("", true, broken)
		if got.Rules != nil || !errors.Is(got.Err, broken) || got.Strict == nil ||
			got.Strict.Error() != "ai/rules/b.yaml: rule B-1 has no description" {
			t.Fatalf("an invalid rule before: %+v", got)
		}
		repeated := Rules{{ID: "A-1", Description: "d", File: "ai/rules/a.yaml"},
			{ID: "A-1", Description: "d", File: "ai/rules/b.yaml"}}
		got = repeated.Loaded("", true, broken)
		if got.Rules != nil || got.Problems != nil || got.Repeated != nil || !errors.Is(got.Err, broken) ||
			got.Strict == nil || got.Strict.Error() != "ai/rules/b.yaml A-1: id repeated, also in ai/rules/a.yaml" {
			t.Fatalf("a repeated id before: %+v", got)
		}
	})

	// Two rules with one id: the repeat is a problem after those of the invalid rules, it is in Repeated too, and a
	// strict reading stops at it when no rule is invalid; an invalid rule of any file still wins.
	t.Run("repeated ids", func(t *testing.T) {
		rules := Rules{
			{ID: "A-1", Description: "d", File: "ai/rules/a.yaml"},
			{ID: "B-1", Description: "d", File: "ai/rules/a.yaml"},
			{ID: "A-1", Description: "d", File: "ai/rules/b.yaml"},
		}
		repeat := "ai/rules/b.yaml A-1: id repeated, also in ai/rules/a.yaml"
		got := rules.Loaded("", true, nil)
		if strings.Join(got.Problems, "\n") != repeat || strings.Join(got.Repeated, "\n") != repeat ||
			got.Strict == nil || got.Strict.Error() != repeat || len(got.Rules) != 3 || got.Err != nil {
			t.Fatalf("got %+v", got)
		}
		invalid := append(append(Rules(nil), rules...), Rule{ID: "C-1", File: "ai/rules/c.yaml"})
		got = invalid.Loaded("", true, nil)
		want := []string{"ai/rules/c.yaml C-1: no description", repeat}
		if strings.Join(got.Problems, "\n") != strings.Join(want, "\n") || strings.Join(got.Repeated, "\n") != repeat ||
			got.Strict == nil || got.Strict.Error() != "ai/rules/c.yaml: rule C-1 has no description" {
			t.Fatalf("an invalid rule after the repeat: %+v", got)
		}
	})

	// A folder that holds retired.yaml has the problem of the file first and a strict reading that stops at it, before
	// any invalid rule; the rules of the file never count, and the rules of the other files are kept.
	t.Run("retired file", func(t *testing.T) {
		const path = "ai/rules/retired.yaml"
		want := path + " is no longer read: delete it, its ids are free"
		rules := Rules{{ID: "A-1", File: "ai/rules/a.yaml"}}
		got := rules.Loaded(path, true, nil)
		if len(got.Problems) == 0 || got.Problems[0] != want || got.Strict == nil || got.Strict.Error() != want ||
			len(got.Rules) != 1 || got.Err != nil {
			t.Fatalf("got %+v", got)
		}
		valid := Rules{{ID: "A-1", Description: "d", File: "ai/rules/a.yaml"}}
		if got := valid.Loaded(path, true, nil); got.Strict == nil || got.Strict.Error() != want || got.Err != nil {
			t.Fatalf("valid rules: %+v", got)
		}
	})
}

// TestLoadedRetiredFile is the folder that holds retired.yaml (CUJ-05): its problem LIT-08 is among the problems of
// the folder and stops a strict reading, and none of its rules loads.
func TestLoadedRetiredFile(t *testing.T) {
	const path = "ai/rules/retired.yaml"
	want := path + " is no longer read: delete it, its ids are free"
	got := Rules{{ID: "GOARCH-1", Description: "d", File: "ai/rules/a.yaml"}}.Loaded(path, true, nil)
	if !slices.Contains(got.Problems, want) || got.Strict == nil || !strings.Contains(got.Strict.Error(), want) ||
		got.Err != nil || len(got.Rules) != 1 {
		t.Fatalf("got %+v", got)
	}
	// With a file that cannot be read, no problems: the strict reading still names the retired file first.
	broken := errors.New("ai/rules/c.yaml: yaml: did not find expected node content")
	got = Rules(nil).Loaded(path, true, broken)
	if got.Problems != nil || !errors.Is(got.Err, broken) || got.Strict == nil || got.Strict.Error() != want {
		t.Fatalf("a broken file: %+v", got)
	}
}
