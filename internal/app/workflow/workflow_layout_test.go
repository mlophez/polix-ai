package workflow

import (
	"strings"
	"testing"
)

// The folders default to ai/ and come cleaned, with slashes and without ./ nor a trailing slash.
func TestWorkflowLayoutPaths(t *testing.T) {
	var w Workflow
	if w.RulesPath() != DefaultRulesPath || w.PlansPath() != DefaultPlansPath || w.ReportsPath() != DefaultReportsPath {
		t.Fatalf("defaults: %q %q %q", w.RulesPath(), w.PlansPath(), w.ReportsPath())
	}
	w = Workflow{Rules: Rules{Path: "./docs/rules/"}, Plans: Folder{Path: `work\plans`}, Reports: Folder{Path: " out "}}
	if w.RulesPath() != "docs/rules" || w.PlansPath() != "work/plans" || w.ReportsPath() != "out" {
		t.Fatalf("configured: %q %q %q", w.RulesPath(), w.PlansPath(), w.ReportsPath())
	}
}

// A folder is relative, below the root and not a glob; an unknown key is a problem.
func TestFolderValid(t *testing.T) {
	for _, p := range []string{"", "ai/plans", "docs/plans/", "./plans", `docs\plans`} {
		if err := (Folder{Path: p}).Valid(); err != nil {
			t.Errorf("%q: %v", p, err)
		}
	}
	bad := map[string]string{
		"/abs":       "must be relative",
		`C:\plans`:   "must be relative",
		".":          "cannot be the root",
		"./":         "cannot be the root",
		"..":         "must be below the root",
		"../plans":   "must be below the root",
		"a/../../b":  "must be below the root",
		"plans/*":    "not a glob",
		"plans/{a}":  "not a glob",
		"plans/[ab]": "not a glob",
	}
	for p, want := range bad {
		if err := (Folder{Path: p}).Valid(); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%q: got %v, want %q", p, err, want)
		}
	}
	if err := (Folder{Keys: []string{"path", "dir"}}).Valid(); err == nil || !strings.Contains(err.Error(), `unknown key "dir"`) {
		t.Fatalf("unknown key: %v", err)
	}
}

// polix.yaml places the folders with rules.path, plans.path and reports.path, and reports a bad one by its key.
func TestParseLayout(t *testing.T) {
	data := "version: 1\nrules:\n  path: docs/rules\nplans: { path: docs/plans }\nreports: { path: docs/reports }\n"
	w, err := Parse([]byte(data), "dev")
	if err != nil {
		t.Fatal(err)
	}
	if w.RulesPath() != "docs/rules" || w.PlansPath() != "docs/plans" || w.ReportsPath() != "docs/reports" {
		t.Fatalf("%q %q %q", w.RulesPath(), w.PlansPath(), w.ReportsPath())
	}
	if !w.Guidance.Layout {
		t.Fatal("the layout is injected unless guidance.layout is false")
	}
	cases := map[string]string{
		"version: 1\nrules: { path: /etc }\n":       `rules.path "/etc" must be relative`,
		"version: 1\nplans: { path: ../x }\n":       `plans.path "../x" must be below the root`,
		"version: 1\nreports: { path: \"*\" }\n":    `reports.path "*" must be a folder`,
		"version: 1\nplans: { dir: x }\n":           `unknown key "dir" in plans (path)`,
		"version: 1\nrules: { folder: x }\n":        `unknown key "folder" in rules (path, run)`,
		"version: 1\nguidance: { layout: false }\n": "",
	}
	for data, want := range cases {
		w, err := Parse([]byte(data), "dev")
		switch {
		case want == "" && (err != nil || w.Guidance.Layout):
			t.Errorf("%q: %v, layout %v", data, err, w.Guidance.Layout)
		case want != "" && (err == nil || !strings.Contains(err.Error(), want)):
			t.Errorf("%q: got %v, want %q", data, err, want)
		}
	}
}
