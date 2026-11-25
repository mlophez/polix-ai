package answer

import (
	"slices"
	"testing"

	"github.com/mlophez/polix-ai/internal/app/agent"
	"github.com/mlophez/polix-ai/internal/app/diff"
	"github.com/mlophez/polix-ai/internal/app/hook"
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
	got := expandPlaceholders("{path} {command} {branch} {key} {other} {risk_score}", ev, "", "main",
		map[string]string{"key": "K-1"}, nil, hook.Commit{})
	if got != "src/a.go ls main K-1 {other} {risk_score}" {
		t.Fatalf("%q", got)
	}
	if expandPlaceholders("plain", ev, "", "", nil, nil, hook.Commit{}) != "plain" {
		t.Fatal("plain")
	}
	commit := hook.Commit{Hash: "abc1234", Subject: "feat: login form"}
	if got := expandPlaceholders("{commit} ({subject})", ev, "", "main", nil, nil, commit); got != "abc1234 (feat: login form)" {
		t.Fatalf("commit: %q", got)
	}
	if got := expandPlaceholders("[{commit}] [{subject}]", ev, "", "main", nil, nil, hook.Commit{}); got != "[] []" {
		t.Fatalf("no commit: %q", got)
	}
}

// CUJ-07: {commit} and {subject} give the hash and the subject of the commit just made on post_commit, read only
// when a string holds one of them; on any other event the validator rejects them (placeholderEvents).
func TestExpandCommitByEvent(t *testing.T) {
	const entry = "    - action: { inject: \"Comment commit {commit} ({subject}) on {branch}.\" }\n"
	commit := hook.Commit{Hash: "abc1234", Subject: "feat: login form"}
	after := eval(t, "version: 1\non:\n  post_commit:\n"+entry, workflow.PostCommit)
	out, steps := walk(t, after, facts{branch: "main", commit: commit})
	if !slices.Equal(out.Answer.Contexts, []string{"Comment commit abc1234 (feat: login form) on main."}) {
		t.Fatalf("post_commit: %+v", out.Answer)
	}
	if n := len(asked(steps, func(s hook.Step) bool { return s.Commit })); n != 1 {
		t.Fatalf("post_commit asked for the commit %d times", n)
	}
	plain := eval(t, "version: 1\non:\n  post_commit:\n    - action: { inject: \"Done on {branch}\" }\n", workflow.PostCommit)
	if _, steps := walk(t, plain, facts{branch: "main"}); asked(steps, func(s hook.Step) bool { return s.Commit }) != nil {
		t.Fatal("a string without {commit} or {subject} asked for the commit")
	}
}

// AC-06: {skill} expands to the name of the skill as it came, its prefix included, on the event skill, and to an
// empty text when the event holds none.
func TestExpandSkill(t *testing.T) {
	ev := agent.Event{Name: workflow.Skill, Skill: "polix:spec"}
	if got := expandPlaceholders("Skill {skill} started.", ev, "", "main", nil, nil, hook.Commit{}); got != "Skill polix:spec started." {
		t.Fatalf("skill: %q", got)
	}
	prompt := agent.Event{Name: workflow.Prompt, Prompt: "/polix:spec add a login"}
	if got := expandPlaceholders("[{skill}]", prompt, "", "main", nil, nil, hook.Commit{}); got != "[]" {
		t.Fatalf("prompt: %q", got)
	}
	const entry = "    - action: { inject: \"Skill {skill} started.\" }\n"
	out, _ := walk(t, skillEval(t, "version: 1\non:\n  skill:\n"+entry, "polix:spec"), facts{branch: "main"})
	if !slices.Equal(out.Answer.Contexts, []string{"Skill polix:spec started."}) {
		t.Fatalf("skill: %+v", out.Answer)
	}
}

// The measures expand as integers, or ? when the diff could not be measured.
func TestExpandMeasures(t *testing.T) {
	d := diff.Diff{Changes: []diff.Change{{Path: "a.go", Added: 3}, {Path: "b.png", Binary: true}}}
	s := "{changed_lines} {changed_files} {risk_score}"
	known := measureValues(d, []workflow.Weight{{Glob: "a.go", Factor: 2}}, true)
	if got := expandPlaceholders(s, agent.Event{}, "", "", nil, known, hook.Commit{}); got != "3 2 4" {
		t.Fatalf("known: %q", got)
	}
	if got := expandPlaceholders(s, agent.Event{}, "", "", nil, measureValues(d, nil, false), hook.Commit{}); got != "? ? ?" {
		t.Fatalf("unknown: %q", got)
	}
	if !usesMeasures("riesgo {risk_score}") || usesMeasures("{branch}") {
		t.Fatal("usesMeasures")
	}
}
