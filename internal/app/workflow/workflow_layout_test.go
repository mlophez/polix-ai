package workflow

import (
	"errors"
	"strings"
	"testing"
)

// AC-01, AC-02: the folder of the AI defaults to ai, and its folder of rules to ai/rules; a configured path comes
// cleaned, with slashes and without spaces, ./ nor a trailing slash, and holds the folder of rules.
func TestWorkflowPolixPath(t *testing.T) {
	var w Workflow
	if w.PolixPath() != DefaultPolixPath || DefaultPolixPath != "ai" || w.RulesPath() != "ai/rules" ||
		w.RulesPath() != DefaultRulesPath {
		t.Fatalf("defaults: %q %q", w.PolixPath(), w.RulesPath())
	}
	for p, want := range map[string]string{
		"":              "ai",
		"   ":           "ai",
		"docs/ai":       "docs/ai",
		"./docs/ai/":    "docs/ai",
		` docs\ai `:     "docs/ai",
		"docs//ai/x/..": "docs/ai",
	} {
		w := Workflow{Path: p}
		if w.PolixPath() != want || w.RulesPath() != want+"/rules" {
			t.Errorf("%q: %q %q, want %q", p, w.PolixPath(), w.RulesPath(), want)
		}
	}
}

// AC-03: a path that is absolute, the root itself, outside the root or a glob is a problem of the workflow, which
// ValidPath tells alone, in the same words; a good one, or none, is no problem.
func TestWorkflowPathProblems(t *testing.T) {
	broken := Workflow{Version: 1, Rules: Rules{Run: []Runner{{Glob: "*"}}}}
	for _, p := range []string{"", "ai", "docs/ai", "docs/ai/", "./ai", `docs\ai`} {
		w := broken
		w.Path = p
		if err := w.ValidPath(); err != nil {
			t.Errorf("%q: %v", p, err)
		}
		if err := (Workflow{Version: 1, Path: p}).Valid(); err != nil {
			t.Errorf("%q: %v", p, err)
		}
	}
	for p, want := range map[string]string{
		"/etc":      `polix.yaml: path "/etc" must be relative to the root, not absolute`,
		`C:\x`:      `polix.yaml: path "C:\\x" must be relative to the root, not absolute`,
		".":         `polix.yaml: path "." cannot be the root itself`,
		"./":        `polix.yaml: path "./" cannot be the root itself`,
		"..":        `polix.yaml: path ".." must be below the root`,
		"../x":      `polix.yaml: path "../x" must be below the root`,
		"a/../../b": `polix.yaml: path "a/../../b" must be below the root`,
		"ai/*":      `polix.yaml: path "ai/*" must be a folder, not a glob`,
		"ai/{a}":    `polix.yaml: path "ai/{a}" must be a folder, not a glob`,
		"ai/[ab]":   `polix.yaml: path "ai/[ab]" must be a folder, not a glob`,
	} {
		w := Workflow{Version: 1, Path: p}
		err := w.ValidPath()
		if _, invalid := errors.AsType[InvalidError](err); !invalid || err.Error() != want {
			t.Errorf("%q: got %v, want %s", p, err, want)
		}
		if got := strings.Join(w.Problems(), "\n"); got != want {
			t.Errorf("%q: the workflow says it otherwise: %s", p, got)
		}
		if Problem(want).Renamed() {
			t.Errorf("%q: a bad path is no name of the old vocabulary", p)
		}
	}
}

// The keys of rules are only run: any other is a problem that lists it.
func TestWorkflowLayoutProblems(t *testing.T) {
	w := Workflow{Version: 1, Rules: Rules{Keys: []string{"folder"}}}
	if err := w.Valid(); err == nil || !strings.Contains(err.Error(), `unknown key "folder" in rules (run)`) {
		t.Fatalf("got %v", err)
	}
	if err := (Workflow{Version: 1, Path: "docs/ai", Rules: Rules{Keys: []string{"run"}}}).Valid(); err != nil {
		t.Fatal(err)
	}
}
