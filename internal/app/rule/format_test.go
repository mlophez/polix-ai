package rule

import (
	"slices"
	"strings"
	"testing"
)

// rules goes first, the keys of a rule follow the schema with the unknown ones after (reason among them), and the rules never move; a
// comment above a key moves with it, a flow scope on two lines ends on one and a folded description keeps its text.
func TestFormat(t *testing.T) {
	src := `x-owner: platform
rules:
  - check: tests/a_test.go
    scope: ["a/**",
      "b/**"]
    x-note: kept last
    reason: why
    # the id of the rule
    id: B-1
    deterministic: true
    description: >-
      folded
      text
  - id: A-1
    description: d
`
	want := `rules:
  # the id of the rule
  - id: B-1
    description: >-
      folded
      text
    scope: ["a/**", "b/**"]
    deterministic: true
    check: tests/a_test.go
    x-note: kept last
    reason: why

  - id: A-1
    description: d
x-owner: platform
`
	got, err := Source(src).Format()
	if err != nil || string(got) != want {
		t.Fatalf("got:\n%s\nwant:\n%s\n%v", got, want, err)
	}
}

// Three rules written without blank lines come out with one blank line between each two, above the comments of a
// rule, and none after rules:; the scope of a rule in block style gets no blank line.
func TestFormatSpacesTheRules(t *testing.T) {
	src := `rules:
  - id: A-1
    description: a
    scope:
      - "a/**"
      - "b/**"
  # about A-2
  - id: A-2
    description: b
  - id: A-3
    description: c
`
	want := `rules:
  - id: A-1
    description: a
    scope:
      - "a/**"
      - "b/**"

  # about A-2
  - id: A-2
    description: b

  - id: A-3
    description: c
`
	got, err := Source(src).Format()
	if err != nil || string(got) != want {
		t.Fatalf("got:\n%s\nwant:\n%s\n%v", got, want, err)
	}
	if twice, err := Source(got).Format(); err != nil || string(twice) != want {
		t.Fatalf("not idempotent:\n%s\n%v", twice, err)
	}
}

// kind and subject go right after id, then the fields of rule.Rule in order; a comment stays with its key.
func TestOrderKindSubject(t *testing.T) {
	src := `rules:
  - check: tests/a_test.go
    deterministic: true
    scope: ["**"]
    description: d
    # what it applies to
    subject: the code
    kind: layout
    id: A-1
`
	want := `rules:
  - id: A-1
    kind: layout
    # what it applies to
    subject: the code
    description: d
    scope: ["**"]
    deterministic: true
    check: tests/a_test.go
`
	got, err := Source(src).Format()
	if err != nil || string(got) != want {
		t.Fatalf("got:\n%s\nwant:\n%s\n%v", got, want, err)
	}
}

// A folder is valid with a relative path with slashes, its rules ordered by id and valid, and its files named without
// a folder; Format splits the files it would change, with their data, from those it cannot format.
func TestFolder(t *testing.T) {
	ok := Source("rules:\n  - id: A-1\n    description: d\n")
	f := Folder{Dir: "ai/rules", Files: map[string]Source{"b.yaml": ok, "a.yaml": Source("rules:\n- id: A-1\n"),
		"c.yaml": Source("rules:\n  - id: !!str C-1\n")}}
	if err := f.Valid(); err != nil {
		t.Fatal(err)
	}
	got := f.Format()
	if err := got.Valid(); err != nil || !slices.Equal(got.Changed, []string{"ai/rules/a.yaml"}) ||
		string(got.Data["a.yaml"]) != "rules:\n  - id: A-1\n" || len(got.Problems) != 1 ||
		!strings.HasPrefix(got.Problems[0], "ai/rules/c.yaml: ") {
		t.Fatalf("Format = %+v, %v", got, err)
	}
	prefix := Folder{Dir: "d", Files: map[string]Source{"a.yaml": Source("x: !!str 1\n"), "a.yaml-old.yaml": Source("x: !!str 1\n")}}
	if got := prefix.Format(); len(got.Problems) != 2 {
		t.Fatalf("two files whose names share a prefix keep their problems: %q", got.Problems)
	}
	valid := complete(Rule{ID: "A-1", Description: "d"})
	for name, bad := range map[string]Folder{
		"no path":        {},
		"absolute":       {Dir: "/ai/rules"},
		"backslash":      {Dir: `ai\rules`},
		"drive":          {Dir: "C:/ai/rules"},
		"unordered":      {Dir: "ai/rules", Rules: []Rule{complete(Rule{ID: "B-1", Description: "d"}), valid}},
		"invalid rule":   {Dir: "ai/rules", Rules: []Rule{{ID: "A-1"}}},
		"file in folder": {Dir: "ai/rules", Files: map[string]Source{"sub/a.yaml": ok}},
	} {
		if bad.Valid() == nil {
			t.Errorf("%s: valid", name)
		}
	}
	for name, bad := range map[string]Formatted{
		"no data":    {Changed: []string{"ai/rules/a.yaml"}},
		"other data": {Changed: []string{"ai/rules/a.yaml"}, Data: map[string][]byte{"b.yaml": nil}},
		"unsorted":   {Changed: []string{"d/b.yaml", "d/a.yaml"}, Data: map[string][]byte{"a.yaml": nil, "b.yaml": nil}},
		"problems":   {Problems: []string{"d/b.yaml: x", "d/a.yaml: x"}},
	} {
		if bad.Valid() == nil {
			t.Errorf("%s: valid", name)
		}
	}
	if failed := (Formatted{Changed: []string{"a"}}).Fail("boom"); failed.Changed != nil || !slices.Equal(failed.Problems, []string{"boom"}) {
		t.Fatalf("Fail = %+v", failed)
	}
}
