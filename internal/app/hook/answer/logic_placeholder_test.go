package answer

import (
	"testing"

	"github.com/mlophez/polix-ai/internal/app/agent"
	"github.com/mlophez/polix-ai/internal/app/diff"
	"github.com/mlophez/polix-ai/internal/app/workflow"
)

// A variable is the first group of its expression on the branch, or the whole match; "" when it does not match.
func TestVars(t *testing.T) {
	wf := workflow.Workflow{Vars: map[string]workflow.Var{
		"key":  {Branch: "^([A-Z]+-[0-9]+)"},
		"all":  {Branch: "^feature/.*"},
		"bad":  {Branch: "("},
		"miss": {Branch: "^nope"},
	}}
	vars := branchVars(wf, "PLAT-7-orders")
	if vars["key"] != "PLAT-7" || vars["all"] != "" || vars["miss"] != "" || vars["bad"] != "" {
		t.Fatalf("%q", vars)
	}
	if branchVars(wf, "feature/x")["all"] != "feature/x" {
		t.Fatal("the whole match")
	}
}

// expandPlaceholders replaces the placeholders of the event, the branch and the variables.
func TestExpand(t *testing.T) {
	ev := agent.Event{Path: "src/a.go", Command: "ls"}
	got := expandPlaceholders("{path} {command} {branch} {key} {other} {risk_score}", ev, "main",
		map[string]string{"key": "K-1"}, nil)
	if got != "src/a.go ls main K-1 {other} {risk_score}" {
		t.Fatalf("%q", got)
	}
	if expandPlaceholders("plain", ev, "", nil, nil) != "plain" {
		t.Fatal("plain")
	}
}

// The measures expand as integers, or ? when the diff could not be measured.
func TestExpandMeasures(t *testing.T) {
	d := diff.Diff{Changes: []diff.Change{{Path: "a.go", Added: 3}, {Path: "b.png", Binary: true}}}
	s := "{changed_lines} {changed_files} {risk_score}"
	known := measureValues(d, []workflow.Weight{{Glob: "a.go", Factor: 2}}, true)
	if got := expandPlaceholders(s, agent.Event{}, "", nil, known); got != "3 2 4" {
		t.Fatalf("known: %q", got)
	}
	if got := expandPlaceholders(s, agent.Event{}, "", nil, measureValues(d, nil, false)); got != "? ? ?" {
		t.Fatalf("unknown: %q", got)
	}
	if !usesMeasures("riesgo {risk_score}") || usesMeasures("{branch}") {
		t.Fatal("usesMeasures")
	}
}
