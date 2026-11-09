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

// The problems of the folders of a workflow name their key: rules.path, plans.path and reports.path.
func TestWorkflowLayoutProblems(t *testing.T) {
	cases := map[string]Workflow{
		`rules.path "/etc" must be relative`:        {Version: 1, Rules: Rules{Path: "/etc"}},
		`plans.path "../x" must be below the root`:  {Version: 1, Plans: Folder{Path: "../x"}},
		`reports.path "*" must be a folder`:         {Version: 1, Reports: Folder{Path: "*"}},
		`unknown key "dir" in plans (path)`:         {Version: 1, Plans: Folder{Keys: []string{"dir"}}},
		`unknown key "folder" in rules (path, run)`: {Version: 1, Rules: Rules{Keys: []string{"folder"}}},
	}
	for want, w := range cases {
		if err := w.Valid(); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%s: got %v", want, err)
		}
	}
	w := Workflow{Version: 1, Rules: Rules{Path: "docs/rules"}, Plans: Folder{Path: "docs/plans"},
		Reports: Folder{Path: "docs/reports"}}
	if err := w.Valid(); err != nil {
		t.Fatal(err)
	}
}
