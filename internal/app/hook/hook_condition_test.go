package hook

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
	vars := Vars(wf, "PLAT-7-orders")
	if vars["key"] != "PLAT-7" || vars["all"] != "" || vars["miss"] != "" || vars["bad"] != "" {
		t.Fatalf("%q", vars)
	}
	if Vars(wf, "feature/x")["all"] != "feature/x" {
		t.Fatal("the whole match")
	}
}

// Expand replaces the placeholders of the event, the branch and the variables.
func TestExpand(t *testing.T) {
	ev := agent.Event{Path: "src/a.go", Command: "ls"}
	got := Expand("{path} {command} {branch} {key} {other} {risk_score}", ev, "main", map[string]string{"key": "K-1"}, nil)
	if got != "src/a.go ls main K-1 {other} {risk_score}" {
		t.Fatalf("%q", got)
	}
	if Expand("plain", ev, "", nil, nil) != "plain" {
		t.Fatal("plain")
	}
}

// The measures expand as integers, or ? when the diff could not be measured.
func TestExpandMeasures(t *testing.T) {
	d := diff.Diff{Changes: []diff.Change{{Path: "a.go", Added: 3}, {Path: "b.png", Binary: true}}}
	s := "{changed_lines} {changed_files} {risk_score}"
	if got := Expand(s, agent.Event{}, "", nil, Measures(d, []workflow.Weight{{Glob: "a.go", Factor: 2}}, true)); got != "3 2 4" {
		t.Fatalf("known: %q", got)
	}
	if got := Expand(s, agent.Event{}, "", nil, Measures(d, nil, false)); got != "? ? ?" {
		t.Fatalf("unknown: %q", got)
	}
	if !UsesMeasures("riesgo {risk_score}") || UsesMeasures("{branch}") {
		t.Fatal("UsesMeasures")
	}
}

// A pattern matches each command of a line, and its exit code is trusted only when its failure propagates.
func TestMatchingCommands(t *testing.T) {
	if segs := MatchingCommands([]string{"just test*"}, "cd x && just test | tail -3"); len(segs) != 1 || Propagates(segs) {
		t.Fatalf("piped: %+v", segs)
	}
	if segs := MatchingCommands([]string{"just test*"}, "cd x && just test -k a"); len(segs) != 1 || !Propagates(segs) {
		t.Fatalf("chained: %+v", segs)
	}
	if len(MatchingCommands([]string{"git push*"}, `echo "git push"`)) != 0 {
		t.Fatal("the text of a command inside quotes")
	}
}

// branch holds on a match of its expression, never on one that does not compile.
func TestBranch(t *testing.T) {
	if !Branch("^main$", "main") || Branch("^main$", "dev") || Branch("(", "(") {
		t.Fatal("Branch")
	}
}

// Indent indents every line, and nothing for an empty text.
func TestIndent(t *testing.T) {
	if Indent("a\nb\n") != "  a\n  b" || Indent("") != "" {
		t.Fatal("Indent")
	}
}

// Only the commands that check the workflow or update polix pass an invalid workflow.
func TestOnlyPolix(t *testing.T) {
	for command, want := range map[string]bool{"polix config check": true, "cd . && polix config check --dir .": true,
		"polix update --version 0.2.0": true, "polix version": true, "polix doctor": true, "ls": false,
		"polix config check && ls": false, "polix check": false, "polix config fmt --check": true, "polix config fmt": false, "polix config fmt --check=false": false,
		"polix config fmt --check --check=false": false, "": false,
		// A substitution or a redirection could run or write anything behind an allowed command.
		"polix config check $(polix config fmt)": false, "polix config check `polix config fmt`": false,
		"polix config check > polix.yaml": false, "polix doctor >> polix.yaml": false, "polix version < x": false,
		"polix config check 2>&1": true, "polix doctor 1>&2 | tail -3": false, "polix doctor 2>&1; polix version": true} {
		if got := OnlyPolix(command); got != want {
			t.Errorf("OnlyPolix(%q) = %v", command, got)
		}
	}
}
