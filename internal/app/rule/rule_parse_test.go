package rule

import (
	"strings"
	"testing"
)

func TestParse(t *testing.T) {
	got, err := Parse("ai/rules/go.yaml", []byte(`rules:
  - id: GO-1
    description: >-
      gofmt
    scope: ["**/*.go"]
    deterministic: true
    check: ai/checks/gofmt.sh
`))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].ID != "GO-1" || got[0].Description != "gofmt" || got[0].File != "ai/rules/go.yaml" ||
		!got[0].HasCheck() || got[0].Scope[0] != "**/*.go" {
		t.Fatalf("got %+v", got)
	}
}

func TestParseRejectsAnInvalidRule(t *testing.T) {
	_, err := Parse("ai/rules/go.yaml", []byte("rules:\n  - id: GO-1\n"))
	if err == nil || !strings.Contains(err.Error(), "ai/rules/go.yaml") {
		t.Fatalf("got %v, want the error of Valid with the file", err)
	}
}

func TestParseRejectsBrokenYAML(t *testing.T) {
	_, err := Parse("ai/rules/bad.yaml", []byte("rules: [\n"))
	if err == nil || !strings.Contains(err.Error(), "ai/rules/bad.yaml") {
		t.Fatalf("got %v, want the error of the yaml with the file", err)
	}
}

// ParseAll keeps every rule and lists the problem of each invalid one, in the words of polix config check.
func TestParseAll(t *testing.T) {
	got, problems, err := ParseAll("ai/rules/a.yaml", []byte(`rules:
  - id: A-1
    description: ok
  - id: A-2
  - description: no id
  - id: A-3
    description: d
    check: checks/a.sh
  - id: A-4
    description: d
    deterministic: true
    check: /abs/a.sh
`))
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"ai/rules/a.yaml A-2: no description",
		"ai/rules/a.yaml: a rule has no id",
		"ai/rules/a.yaml A-3: check checks/a.sh needs deterministic: true",
		"ai/rules/a.yaml A-4: check /abs/a.sh is an absolute path, not relative to the root",
	}
	if len(got) != 5 || strings.Join(problems, "\n") != strings.Join(want, "\n") {
		t.Fatalf("got %d rules and\n%s", len(got), strings.Join(problems, "\n"))
	}
	if _, _, err := ParseAll("ai/rules/bad.yaml", []byte("rules: [\n")); err == nil ||
		!strings.Contains(err.Error(), "ai/rules/bad.yaml") {
		t.Fatalf("broken yaml: %v", err)
	}
}

// The files of a folder of rules, named by their path below it: retired.yaml and other files left out, every rule ordered by id; strict or lenient.
func TestParseFiles(t *testing.T) {
	files := map[string][]byte{
		"b.yaml":       []byte("rules:\n  - id: B-1\n    description: d\n  - id: A-2\n"),
		"a.yaml":       []byte("rules:\n  - id: A-1\n    description: d\n"),
		"retired.yaml": []byte("rules:\n  - id: R-1\n"),
		"notes.md":     []byte("x"),
	}
	rules, problems, err := ParseFilesAll("ai/rules", files)
	if err != nil || len(rules) != 3 || rules[0].ID != "A-1" || rules[1].ID != "A-2" || len(problems) != 1 {
		t.Fatal(rules, problems, err)
	}
	if rules[0].File != "ai/rules/a.yaml" {
		t.Fatalf("file %q, want below the folder given", rules[0].File)
	}
	if _, err := ParseFiles("docs/rules", files); err == nil || !strings.Contains(err.Error(), "docs/rules/b.yaml") {
		t.Fatalf("strict: %v", err)
	}
	delete(files, "b.yaml")
	if rules, err := ParseFiles("ai/rules", files); err != nil || len(rules) != 1 {
		t.Fatal(rules, err)
	}
	files["c.yaml"] = []byte("rules: [\n")
	if _, _, err := ParseFilesAll("ai/rules", files); err == nil {
		t.Fatal("broken yaml")
	}
}
