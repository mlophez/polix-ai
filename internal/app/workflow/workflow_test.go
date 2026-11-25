package workflow

import (
	"errors"
	"strings"
	"testing"
)

// The problems of the workflow read from the text of polix.yaml, the example of the repository and the recipes of
// docs/use-cases.md included, are tested in internal/app/workflow/adapters, which decodes it; these tests build the
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

// The types of finished_since_commit, in when, unless, not and any, are observed next to those of finished, sorted
// once.
func TestObservedSubagentsSinceCommit(t *testing.T) {
	block := Action{Block: &Block{Message: "x"}, Keys: []string{VerbBlock}}
	w := Workflow{Version: 1, On: map[string][]Entry{
		PreCommit: {
			{Unless: Cond{"finished_since_commit": "polix:builder"}, Action: block, Line: 4},
			{When: Cond{"any": map[string]any{"not": map[string]any{"finished_since_commit": []any{"acme:docs", "polix:builder"}},
				"branch": "x"}}, Action: block, Line: 6},
			{When: Cond{"not": map[string]any{"finished_since_commit": "acme:ci"}}, Unless: Cond{"finished": "polix:reviewer"},
				Action: block, Line: 8},
		},
	}}
	if err := w.Valid(); err != nil {
		t.Fatal(err)
	}
	if got := w.ObservedSubagents(); strings.Join(got, ",") != "acme:ci,acme:docs,polix:builder,polix:reviewer" {
		t.Fatalf("observed: %q", got)
	}
}

// A workflow whose only use of the log is an agents_are_running nested in an any of an unless observes the starts
// and counts the subagents; one without it does neither.
func TestCountsAgents(t *testing.T) {
	block := Action{Block: &Block{Message: "x"}, Keys: []string{VerbBlock}}
	w := Workflow{Version: 1, On: map[string][]Entry{PreCommit: {{Unless: Cond{"any": map[string]any{
		"branch": "main", "agents_are_running": false}}, Action: block, Keys: []string{"unless", "action"}, Line: 4}}}}
	if err := w.Valid(); err != nil {
		t.Fatal(err)
	}
	if !w.CountsAgents() || !w.ObservesStart() {
		t.Fatalf("counts %v, observes the start %v", w.CountsAgents(), w.ObservesStart())
	}
	if w.InjectsOnce() {
		t.Fatal("the condition does not inject once")
	}
	plain := Workflow{Version: 1, On: map[string][]Entry{PreCommit: {{When: Cond{"not": map[string]any{
		"branch": "main"}}, Action: block, Keys: []string{"when", "action"}, Line: 4}}}}
	if plain.CountsAgents() || plain.ObservesStart() {
		t.Fatal("a workflow without the condition counts the subagents")
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
		{Prompt, Entry{Action: inject(Inject{Rules: "{branch}", Keys: []string{"rules"}})}, true},
		{PreEdit, Entry{Action: inject(Inject{Text: "x", Always: true, Keys: []string{"text", "always"}})}, false},
		{Start, Entry{Action: inject(Inject{Text: "x"})}, false},
		{PostCommit, Entry{Action: inject(Inject{Text: "x"})}, false},
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

// AC-05: every inject of skill acts every time the skill starts, so an inject on skill alone, without always, does
// not make the workflow inject once.
func TestInjectsOnceSkill(t *testing.T) {
	inject := Action{Inject: &Inject{Text: "Skill {skill} started."}, Keys: []string{VerbInject}}
	w := Workflow{Version: 1, On: map[string][]Entry{Skill: {{Action: inject}}}}
	if err := w.Valid(); err != nil {
		t.Fatal(err)
	}
	if w.InjectsOnce() {
		t.Fatal("an inject on skill alone injects once")
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
	w := Workflow{Version: 2, StopRetries: -1}
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

// placeholderEntry builds an entry of one verb with the text s, at line.
func placeholderEntry(verb, s string, line int) Entry {
	a := Action{Keys: []string{verb}}
	switch verb {
	case VerbRun:
		a.Run = &Run{Command: []string{"tool", s}}
	case VerbInject:
		a.Inject = &Inject{Text: s}
	case VerbBlock:
		a.Block = &Block{Message: s}
	default:
		a.Notify = &Notify{Message: s}
	}
	return Entry{Action: a, Keys: []string{"action"}, Line: line}
}

// CUJ-03, AC-03: a built-in placeholder written on an event outside its applicable events is a problem at the line of
// its entry, one per occurrence; on its events, or without an event, it is none.
func TestPlaceholderEvents(t *testing.T) {
	cases := []struct{ ph, ev, verb, want string }{
		{"commit", PreEdit, VerbBlock, "placeholder {commit} never expands on pre_edit (only on post_commit)"},
		{"skill", Start, VerbInject, "placeholder {skill} never expands on start (only on skill)"},
		{"path", Stop, VerbNotify, "placeholder {path} never expands on stop (only on pre_edit, post_edit)"},
		{"command", PostEdit, VerbRun,
			"placeholder {command} never expands on post_edit (only on pre_command, post_command, pre_commit, post_commit)"},
		{"subject", PostEdit, VerbNotify, "placeholder {subject} never expands on post_edit (only on post_commit)"},
	}
	for _, c := range cases {
		w := Workflow{Version: 1, On: map[string][]Entry{c.ev: {placeholderEntry(c.verb, "x {"+c.ph+"}", 7)}}}
		got := w.Problems()
		if len(got) != 1 || got[0] != "polix.yaml:7: "+c.want {
			t.Errorf("{%s} on %s: %q", c.ph, c.ev, got)
		}
	}
	two := Workflow{Version: 1, On: map[string][]Entry{Stop: {placeholderEntry(VerbNotify, "{path} {path}", 3)}}}
	if got := two.Problems(); len(got) != 2 {
		t.Errorf("one problem per occurrence: %q", got)
	}
	for ev, ph := range map[string]string{PostEdit: "path", PreEdit: "path", PostCommand: "command", PreCommit: "command",
		PostCommit: "commit", Skill: "skill", Stop: "branch", Start: "risk_score"} {
		w := Workflow{Version: 1, On: map[string][]Entry{ev: {placeholderEntry(VerbRun, "{"+ph+"}", 4)}}}
		if got := w.Problems(); len(got) != 0 {
			t.Errorf("{%s} on %s: %q", ph, ev, got)
		}
	}
	if err := placeholderEntry(VerbNotify, "{commit} {skill}", 2).Valid(); err != nil {
		t.Errorf("an entry without an event: %v", err)
	}
	w := Workflow{Version: 1, On: map[string][]Entry{Stop: {{
		When: Cond{"contains": "{path}", "file": "a"}, Action: placeholderEntry(VerbBlock, "x", 5).Action, Line: 5}}}}
	if got := w.Problems(); len(got) != 1 || !strings.Contains(got[0], "placeholder {path} never expands on stop") {
		t.Errorf("a placeholder of a condition: %q", got)
	}
}

// AC-03: the entries a shortcut expands into are checked on the event they expand into.
func TestPlaceholderEventsShortcut(t *testing.T) {
	item := func(run string) []GuardrailCheck { return []GuardrailCheck{{Run: []string{"tool", run}, Line: 6}} }
	check := (Workflow{Version: 1}).Guard(Guardrails{CheckOnStop: item("{path}")})
	got := check.Problems()
	want := "polix.yaml:6: placeholder {path} never expands on stop (only on pre_edit, post_edit)"
	if len(got) != 1 || got[0] != want {
		t.Errorf("check_on_stop: %q", got)
	}
	format := (Workflow{Version: 1}).Guard(Guardrails{FormatOnEdit: item("{path}")})
	if got := format.Problems(); len(got) != 0 {
		t.Errorf("format_on_edit: %q", got)
	}
}

// CUJ-03, AC-03: the new placeholders of the event are rejected outside their events, hold on the others, and their
// names are taken from the variables.
func TestPlaceholderEventsNew(t *testing.T) {
	for _, c := range []struct{ ph, ev, verb, want string }{
		{"abs_path", PreCommand, VerbRun, "placeholder {abs_path} never expands on pre_command (only on pre_edit, post_edit)"},
		{"exit_code", Stop, VerbNotify, "placeholder {exit_code} never expands on stop (only on post_command, post_commit)"},
	} {
		w := Workflow{Version: 1, On: map[string][]Entry{c.ev: {placeholderEntry(c.verb, "x {"+c.ph+"}", 7)}}}
		if got := w.Problems(); len(got) != 1 || got[0] != "polix.yaml:7: "+c.want {
			t.Errorf("{%s} on %s: %q", c.ph, c.ev, got)
		}
	}
	for ev, ph := range map[string]string{PostEdit: "abs_path", PreEdit: "abs_path", PostCommand: "exit_code",
		PostCommit: "exit_code", Stop: "root", Start: "agent", Skill: "session", SubagentStart: "actor", Prompt: "event"} {
		w := Workflow{Version: 1, On: map[string][]Entry{ev: {placeholderEntry(VerbRun, "{"+ph+"}", 4)}}}
		if got := w.Problems(); len(got) != 0 {
			t.Errorf("{%s} on %s: %q", ph, ev, got)
		}
	}
	w := Workflow{Version: 1, Vars: map[string]Var{"root": {Branch: "x"}}}
	if got := w.Problems(); len(got) != 1 || !strings.Contains(got[0], "var root: the name is a built-in placeholder") {
		t.Errorf("a variable named root: %q", got)
	}
}
