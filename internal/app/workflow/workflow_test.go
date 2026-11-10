package workflow

import (
	"errors"
	"strings"
	"testing"
)

// The problems of the workflow read from the text of polix.yaml, the example of the repository and the recipes of
// docs/use-cases.md included, are tested in internal/app/workflow/load, which decodes it; these tests build the
// workflow.

// The workflow names the subagent types its finished conditions observe, nested ones included, once each.
func TestObservedSubagents(t *testing.T) {
	block := Action{Block: &Block{Message: "x"}, Keys: []string{VerbBlock}}
	w := Workflow{Version: 1, On: map[string][]Entry{
		PreCommit: {
			{Unless: Cond{"finished": "polix:reviewer"}, Action: block, Line: 4},
			{When: Cond{"any": map[string]any{"not": map[string]any{"finished": []any{"acme:docs", "polix:reviewer"}},
				"branch": "x"}}, Action: block, Line: 6},
		},
		Stop: {{Action: Action{Run: &Run{Command: []string{"ls"}}, Keys: []string{VerbRun}}, Line: 9}},
	}}
	if err := w.Valid(); err != nil {
		t.Fatal(err)
	}
	if got := w.ObservedSubagents(); strings.Join(got, ",") != "acme:docs,polix:reviewer" {
		t.Fatalf("observed: %q", got)
	}
	if got := (Workflow{Version: 1}).ObservedSubagents(); len(got) != 0 {
		t.Fatalf("nothing observed: %q", got)
	}
}

// A workflow injects once when an inject without always is on an event other than start.
func TestInjectsOnce(t *testing.T) {
	inject := func(c Inject) Action { return Action{Inject: &c, Keys: []string{VerbInject}} }
	cases := []struct {
		event string
		entry Entry
		once  bool
	}{
		{PreEdit, Entry{Action: inject(Inject{Text: "x"})}, true},
		{Prompt, Entry{Action: inject(Inject{Rules: "{path}", Keys: []string{"rules"}})}, true},
		{PreEdit, Entry{Action: inject(Inject{Text: "x", Always: true, Keys: []string{"text", "always"}})}, false},
		{Start, Entry{Action: inject(Inject{Text: "x"})}, false},
		{PreCommit, Entry{Action: Action{Block: &Block{Message: "x"}, Keys: []string{VerbBlock}}}, false},
	}
	for _, c := range cases {
		w := Workflow{Version: 1, On: map[string][]Entry{c.event: {c.entry}}}
		if err := w.Valid(); err != nil {
			t.Fatal(err)
		}
		if got := w.InjectsOnce(); got != c.once {
			t.Errorf("%s %+v: %v, want %v", c.event, c.entry.Action, got, c.once)
		}
	}
}

// The requirements inject once without any inject of on: the context of the missing programs on the first prompt.
func TestObservesStartRequirements(t *testing.T) {
	w := Workflow{Version: 1, Requirements: []string{"go"}}
	if err := w.Valid(); err != nil {
		t.Fatal(err)
	}
	if !w.ObservesStart() {
		t.Fatal("requirements without injects: start not observed")
	}
	// The context of the requirements never goes to a subagent, so it does not make the workflow inject once, which
	// would write the end of every subagent.
	if w.InjectsOnce() {
		t.Fatal("requirements without injects: once")
	}
	if (Workflow{Version: 1, Requirements: []string{}}).ObservesStart() {
		t.Fatal("an empty list of requirements: start observed")
	}
	once := Workflow{Version: 1, On: map[string][]Entry{Prompt: {{Action: Action{Inject: &Inject{Text: "x"}}}}}}
	if !once.ObservesStart() {
		t.Fatal("a once-only inject: start not observed")
	}
}

// The programs of the repository are those of requirements followed by git, which a list that names it keeps once in
// its place; without requirements there are none.
func TestPrograms(t *testing.T) {
	cases := []struct {
		reqs []string
		want string
	}{
		{nil, ""},
		{[]string{}, ""},
		{[]string{"go", "golangci-lint"}, "go,golangci-lint,git"},
		{[]string{"git", "go"}, "git,go"},
	}
	for _, c := range cases {
		w := Workflow{Version: 1, Requirements: c.reqs}
		if got := strings.Join(w.Programs(), ","); got != c.want {
			t.Errorf("%q: %q, want %q", c.reqs, got, c.want)
		}
		if len(c.reqs) > 0 && strings.Join(w.Requirements, ",") != strings.Join(c.reqs, ",") {
			t.Errorf("%q: Programs changed the requirements to %q", c.reqs, w.Requirements)
		}
	}
}

// A program name of requirements that is empty, has spaces around it, is a path or is repeated is a problem of the
// whole file, once per name.
func TestRequirementsValid(t *testing.T) {
	if err := (Workflow{Version: 1, Requirements: []string{"go", "golangci-lint"}}).Valid(); err != nil {
		t.Fatal(err)
	}
	w := Workflow{Version: 1, Requirements: []string{"go", "", " ", " go", "bin/tool", `bin\tool`, "go", "go"}}
	want := "polix.yaml: requirements: a program name cannot be empty\n" +
		"polix.yaml: requirements: a program name cannot be empty\n" +
		"polix.yaml: requirements: \" go\" has spaces around it: write the program name alone\n" +
		"polix.yaml: requirements: \"bin/tool\" is a path: write the program name alone, as the PATH finds it\n" +
		"polix.yaml: requirements: \"bin\\\\tool\" is a path: write the program name alone, as the PATH finds it\n" +
		"polix.yaml: requirements: \"go\" is repeated\n" +
		"polix.yaml: requirements: \"go\" is repeated"
	if got := strings.Join(w.Problems(), "\n"); got != want {
		t.Fatalf("problems:\n%s\nwant:\n%s", got, want)
	}
}

// Valid joins every problem in one Invalid error, whose list is that of Problems.
func TestValidJoinsProblems(t *testing.T) {
	w := Workflow{Version: 2, MaxStopRetries: -1}
	err := w.Valid()
	var invalid InvalidError
	if !errors.As(err, &invalid) || len(invalid) != 2 || err.Error() != strings.Join(w.Problems(), "\n") {
		t.Fatalf("Valid: %v", err)
	}
	if err := (Workflow{Version: 1}).Valid(); err != nil {
		t.Fatalf("a minimal workflow: %v", err)
	}
}

// A variable outside a workflow reports its own problems, without a name.
func TestVarValid(t *testing.T) {
	cases := map[string]Var{
		"var needs branch":                {},
		"var: error parsing regexp":       {Branch: "("},
		`unknown key "x" in var (branch)`: {Branch: "x", Keys: []string{"branch", "x"}},
	}
	for want, v := range cases {
		if err := v.Valid(); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%+v: want %q, got %v", v, want, err)
		}
	}
	if err := (Var{Branch: "^main$"}).Valid(); err != nil {
		t.Fatal(err)
	}
}

// AC16: a forbidden command needs a pattern, and the problems of the protections of a workflow built by hand name
// the line of a forbidden command.
func TestForbiddenCommandValid(t *testing.T) {
	if err := (ForbiddenCommand{Pattern: "rm -rf /*"}).Valid(); err != nil {
		t.Fatal(err)
	}
	if err := (ForbiddenCommand{Pattern: " ", Line: 4}).Valid(); err == nil {
		t.Fatal("an empty pattern is valid")
	}
	w := Workflow{Version: 1, ProtectedFiles: []string{""}, ForbiddenCommands: []ForbiddenCommand{{Line: 4}}}
	want := "polix.yaml: guardrails.protected_files: a path glob cannot be empty\n" +
		"polix.yaml:4: guardrails.forbidden_commands: an item needs a pattern"
	if got := strings.Join(w.Problems(), "\n"); got != want {
		t.Fatalf("problems: %q", got)
	}
}
