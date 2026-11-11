package workflow

import (
	"reflect"
	"strings"
	"testing"
)

// githooksWorkflow is a valid workflow whose githooks hold the entries of each event, with the variable key.
func githooksWorkflow(githooks map[string][]Entry) Workflow {
	lines := map[string]int{}
	for ev := range githooks {
		lines[ev] = 4
	}
	return Workflow{Version: 1, MaxStopRetries: DefaultMaxStopRetries,
		Vars:     map[string]Var{"key": {Branch: "^([A-Z][A-Z0-9]+-[0-9]+)", Keys: []string{"branch"}}},
		Githooks: githooks, GithookLines: lines}
}

// githookBlock is the action of a githooks entry that blocks with message.
func githookBlock(message string) Action {
	return Action{Block: &Block{Message: message}, Keys: []string{VerbBlock}}
}

// AC1: entries of both githooks events that use branch, changed, message on commit_msg, block, run with command and
// message, {branch}, {message} on commit_msg and a variable of vars have no problems.
func TestGithooksValid(t *testing.T) {
	w := githooksWorkflow(map[string][]Entry{
		PreCommit: {
			{When: Cond{"branch": "^main$"}, Action: githookBlock("No se commitea en {branch}"),
				Keys: []string{"when", "action"}, Line: 5},
			{When: Cond{"changed": []any{"src/**", "docs/{key}/**"}}, Unless: Cond{"branch": "^release/"},
				Action: Action{Run: &Run{Command: []string{"just", "test", "{branch}"}, Message: "Arregla {key}",
					Keys: []string{"command", "message"}}, Keys: []string{VerbRun}},
				Keys: []string{"when", "unless", "action"}, Line: 8},
			{Action: Action{Run: &Run{Command: []string{"go", "vet"}}, Keys: []string{VerbRun}},
				Keys: []string{"action"}, Line: 12},
		},
		CommitMsg: {
			{When: Cond{"branch": "^[A-Z]+-[0-9]+"}, Unless: Cond{"message": "^{key}: "},
				Action: githookBlock("{message} needs the key {key} of {branch}"),
				Keys:   []string{"when", "unless", "action"}, Line: 16},
			{When: Cond{"changed": "*.go", "message": []any{"^fix", "^feat"}},
				Action: Action{Block: &Block{Message: "m", Keys: []string{"message"}}, Keys: []string{VerbBlock}},
				Keys:   []string{"when", "action"}, Line: 20},
		},
	})
	if p := w.Problems(); len(p) > 0 {
		t.Fatalf("problems: %q", p)
	}
}

// AC2: each problem of githooks is reported once, on the line of its entry (of its event for an unknown one), with a
// message that names the key and what githooks allow.
func TestGithooksProblems(t *testing.T) {
	entry := func(when Cond, action Action) Entry {
		return Entry{When: when, Action: action, Keys: []string{"when", "action"}, Line: 5}
	}
	ok := githookBlock("x")
	run := func(r Run) Action { return Action{Run: &r, Keys: []string{VerbRun}} }
	cases := []struct {
		name  string
		event string
		entry Entry
		want  string
	}{
		{"unknown event", "pre_push", entry(nil, ok),
			`polix.yaml:4: unknown githooks event "pre_push" (githooks events: pre_commit, commit_msg)`},
		{"inject", PreCommit, entry(nil, Action{Inject: &Inject{Text: "x"}, Keys: []string{VerbInject}}),
			`polix.yaml:5: githooks.pre_commit: verb "inject" is not allowed (githooks allow run, block)`},
		{"notify", CommitMsg, entry(nil, Action{Notify: &Notify{Message: "x"}, Keys: []string{VerbNotify}}),
			`polix.yaml:5: githooks.commit_msg: verb "notify" is not allowed (githooks allow run, block)`},
		{"unknown action key", PreCommit, entry(nil, Action{Keys: []string{"deny"}}),
			`polix.yaml:5: githooks.pre_commit: unknown key "deny" in action (githooks allow run, block)`},
		{"no action", PreCommit, entry(Cond{"branch": "^main$"}, Action{}),
			`polix.yaml:5: githooks.pre_commit: the entry needs an action with one of run, block`},
		{"unknown entry key", PreCommit, Entry{Action: ok, Keys: []string{"if", "action"}, Line: 5},
			`polix.yaml:5: githooks.pre_commit: unknown key "if" in an entry (when, unless, action)`},
		{"block once", PreCommit, entry(nil, Action{Block: &Block{Message: "x", Once: true,
			Keys: []string{"once", "message"}}, Keys: []string{VerbBlock}}),
			`polix.yaml:5: githooks.pre_commit: key "once" is not allowed in block (githooks allow message)`},
		{"block without message", PreCommit, entry(nil, githookBlock(" ")),
			`polix.yaml:5: githooks.pre_commit: block needs a message`},
		{"run rules", PreCommit, entry(nil, run(Run{Command: []string{"ls"}, Rules: "changed",
			Keys: []string{"command", "rules"}})),
			`polix.yaml:5: githooks.pre_commit: key "rules" is not allowed in run (githooks allow command, message)`},
		{"run always", CommitMsg, entry(nil, run(Run{Command: []string{"ls"}, Always: true,
			Keys: []string{"command", "always"}})),
			`polix.yaml:5: githooks.commit_msg: key "always" is not allowed in run (githooks allow command, message)`},
		{"run without command", PreCommit, entry(nil, run(Run{Message: "m", Keys: []string{"message"}})),
			`polix.yaml:5: githooks.pre_commit: run needs a command`},
		{"actor", PreCommit, entry(Cond{"actor": "session"}, ok),
			`polix.yaml:5: githooks.pre_commit: condition "actor" is not allowed (githooks allow branch, changed, message)`},
		{"any", CommitMsg, entry(Cond{"any": map[string]any{"branch": "^main$"}}, ok),
			`polix.yaml:5: githooks.commit_msg: condition "any" is not allowed (githooks allow branch, changed, message)`},
		{"not", PreCommit, entry(Cond{"not": map[string]any{"branch": "^main$"}}, ok),
			`polix.yaml:5: githooks.pre_commit: condition "not" is not allowed (githooks allow branch, changed, message)`},
		{"message on pre_commit", PreCommit, entry(Cond{"message": "^fix"}, ok),
			`polix.yaml:5: githooks.pre_commit: condition "message" only holds on commit_msg`},
		{"{message} on pre_commit", PreCommit, entry(nil, githookBlock("{message}")),
			`polix.yaml:5: githooks.pre_commit: placeholder {message} only expands on commit_msg`},
		{"{path} on pre_commit", PreCommit, entry(Cond{"changed": "{path}"}, ok),
			`polix.yaml:5: githooks.pre_commit: unknown placeholder {path} (githooks allow {branch}, or a variable of vars)`},
		{"{path} on commit_msg", CommitMsg, entry(nil, run(Run{Command: []string{"echo", "{path}"}})),
			`polix.yaml:5: githooks.commit_msg: unknown placeholder {path} (githooks allow {branch}, {message}, or a variable of vars)`},
		{"empty changed", PreCommit, entry(Cond{"changed": ""}, ok),
			`polix.yaml:5: githooks.pre_commit: changed: a pattern cannot be empty`},
		{"empty message list", CommitMsg, entry(Cond{"message": []any{}}, ok),
			`polix.yaml:5: githooks.commit_msg: message needs a pattern`},
		{"empty branch", PreCommit, entry(Cond{"branch": ""}, ok),
			`polix.yaml:5: githooks.pre_commit: branch: a pattern cannot be empty`},
		{"branch not a text", PreCommit, entry(Cond{"branch": []any{"a"}}, ok),
			`polix.yaml:5: githooks.pre_commit: branch needs a regular expression`},
		{"changed not texts", PreCommit, entry(Cond{"changed": 3}, ok),
			`polix.yaml:5: githooks.pre_commit: changed: expected a string or a list of strings, got 3`},
		{"branch does not compile", PreCommit, entry(Cond{"branch": "(main"}, ok),
			"polix.yaml:5: githooks.pre_commit: branch: error parsing regexp: missing closing ): `(main`"},
		{"message does not compile", CommitMsg, entry(Cond{"message": []any{"^fix", "^{key}["}}, ok),
			"polix.yaml:5: githooks.commit_msg: message: error parsing regexp: missing closing ]: `[`"},
	}
	for _, c := range cases {
		w := githooksWorkflow(map[string][]Entry{c.event: {c.entry}})
		if got := w.Problems(); len(got) != 1 || got[0] != c.want {
			t.Errorf("%s: got %q, want %q", c.name, got, c.want)
		}
	}
}

// With entries in githooks, a variable cannot take the name of the built-in {message} of githooks, which would
// shadow it on commit_msg, and {message} stays unknown in on.
func TestGithooksReservedVarName(t *testing.T) {
	w := githooksWorkflow(map[string][]Entry{PreCommit: {{Action: githookBlock("b"), Keys: []string{"action"}, Line: 5}}})
	w.Vars["message"] = Var{Branch: "^(.*)$", Keys: []string{"branch"}}
	want := "polix.yaml: var message: the name is a built-in placeholder of githooks (branch, message)"
	if got := w.Problems(); len(got) != 1 || got[0] != want {
		t.Fatalf("got %q, want %q", got, want)
	}
	w = githooksWorkflow(nil)
	w.On = map[string][]Entry{PreCommit: {{Action: githookBlock("{message}"), Keys: []string{"action"}, Line: 3}}}
	if got := w.Problems(); len(got) != 1 || !strings.HasPrefix(got[0], "polix.yaml:3: unknown placeholder {message}") {
		t.Fatalf("{message} in on: %q", got)
	}
}

// Without entries in githooks, a variable named message is valid and expands in on, as before githooks existed.
func TestGithooksVarMessageWithoutGithooks(t *testing.T) {
	for name, githooks := range map[string]map[string][]Entry{"no githooks": nil, "empty event": {CommitMsg: {}}} {
		w := githooksWorkflow(githooks)
		w.Vars["message"] = Var{Branch: "^(.*)$", Keys: []string{"branch"}}
		w.On = map[string][]Entry{Stop: {{Action: githookBlock("msg {message}"), Keys: []string{"action"}, Line: 3}}}
		if got := w.Problems(); len(got) != 0 {
			t.Errorf("%s: got %q, want no problems", name, got)
		}
	}
}

// AC17: on.pre_commit keeps the vocabulary of on, apart from that of githooks: message is no condition of on, and
// actor and inject still are; githooks never reach the entries of on.
func TestGithooksApartFromOn(t *testing.T) {
	w := githooksWorkflow(nil)
	w.On = map[string][]Entry{PreCommit: {
		{When: Cond{"actor": "session"}, Action: Action{Inject: &Inject{Text: "x"}, Keys: []string{VerbInject}},
			Keys: []string{"when", "action"}, Line: 3},
		{When: Cond{"message": "^fix"}, Action: githookBlock("x"), Keys: []string{"when", "action"}, Line: 6},
	}}
	got := w.Problems()
	if len(got) != 1 || !strings.HasPrefix(got[0], `polix.yaml:6: unknown condition "message"`) {
		t.Fatalf("got %q", got)
	}
	w.Githooks = map[string][]Entry{PreCommit: {{When: Cond{"agents_are_running": true}, Unless: Cond{"finished": "x"},
		Action: Action{Inject: &Inject{Text: "x"}, Keys: []string{VerbInject}}, Line: 9}}}
	w.On[PreCommit] = w.On[PreCommit][:1]
	w.On[PreCommit][0].Action = githookBlock("x")
	if w.CountsAgents() || w.InjectsOnce() || len(w.ObservedSubagents()) != 0 {
		t.Fatal("githooks reach the entries of on")
	}
}

// AC22: githooks with an entry expand on with one entry first in start, before the start entries written in on,
// from githooks at the line of the first event with entries, that runs polix install --githooks every time with
// the fixed message; the other events of on stay. Without githooks, or with only empty events, on stays the same.
func TestExpandGithooks(t *testing.T) {
	written := Entry{Action: Action{Inject: &Inject{Text: "hola"}, Keys: []string{VerbInject}}, Keys: []string{"action"},
		Line: 3}
	stop := Entry{Action: githookBlock("x"), Keys: []string{"action"}, Line: 6}
	w := githooksWorkflow(map[string][]Entry{PreCommit: {}, CommitMsg: {{Action: githookBlock("x"), Line: 12}}})
	w.GithookLines = map[string]int{PreCommit: 9, CommitMsg: 11}
	w.On = map[string][]Entry{Start: {written}, Stop: {stop}}
	got := w.ExpandGithooks()
	start := got.On[Start]
	if len(start) != 2 || !reflect.DeepEqual(start[1], written) || !reflect.DeepEqual(got.On[Stop], []Entry{stop}) {
		t.Fatalf("on: %+v", got.On)
	}
	e := start[0]
	if r := e.Action.Run; r == nil || strings.Join(r.Command, " ") != "polix install --githooks" || !r.Always ||
		r.Message != "The hooks of git of githooks are not installed, so the gates of githooks do not apply to the commits:" ||
		e.From != "githooks" || e.Line != 11 || e.When != nil || e.Unless != nil {
		t.Fatalf("entry: %+v %+v", e, e.Action.Run)
	}
	if len(w.On[Start]) != 1 {
		t.Fatalf("the workflow expanded changed: %+v", w.On)
	}
	if p := got.Problems(); len(p) > 0 {
		t.Fatalf("problems: %q", p)
	}

	for name, githooks := range map[string]map[string][]Entry{"without githooks": nil,
		"empty events": {PreCommit: {}, CommitMsg: nil}} {
		w := githooksWorkflow(githooks)
		w.On = map[string][]Entry{Start: {written}}
		if got := w.ExpandGithooks(); !reflect.DeepEqual(got.On, w.On) {
			t.Fatalf("%s: %+v", name, got.On)
		}
	}
}
