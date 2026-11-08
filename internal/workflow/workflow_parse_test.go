package workflow

import (
	"errors"
	"strings"
	"testing"
)

// requires is read before the rest of the file, so a polix older than the workflow asks to be updated instead of
// reporting the options it does not know; a development build is never too old.
func TestParseRequires(t *testing.T) {
	src := []byte("version: 1\nrequires: 0.3.0\nan_option_of_0_3: true\n")
	_, err := Parse(src, "0.2.1")
	var old TooOld
	if !errors.As(err, &old) || old.Requires != "0.3.0" || old.Running != "0.2.1" {
		t.Fatalf("an older polix: %v", err)
	}
	if !strings.Contains(err.Error(), "polix.yaml requires polix 0.3.0 or newer and this is 0.2.1: run polix update") {
		t.Fatalf("message: %v", err)
	}
	if _, err := Parse(src, "dev"); err == nil || errors.As(err, &old) {
		t.Fatalf("a dev build parses the file and reports what it does not know: %v", err)
	}
	if _, err := Parse(src, "0.3.0"); err == nil || errors.As(err, &old) {
		t.Fatalf("the required version parses the file: %v", err)
	}

	w, err := Parse([]byte("version: 1\nrequires: v0.2.0\n"), "0.2.0")
	if err != nil || w.Requires != "v0.2.0" || len(w.Problems()) != 0 {
		t.Fatalf("a polix as new as required: %v %+v %v", err, w, w.Problems())
	}
	w, err = Parse([]byte("version: 1\nrequires: 0.2\n"), "0.2.0")
	var invalid Invalid
	if !errors.As(err, &invalid) || len(w.Problems()) != 1 {
		t.Fatalf("requires must be a full version: %v", err)
	}
}

// An unknown top level key and broken YAML stop the parse, with the file in the message.
func TestParseErrors(t *testing.T) {
	var invalid Invalid
	if _, err := Parse([]byte("version: 1\nonn: {}\n"), "dev"); err == nil || errors.As(err, &invalid) ||
		err.Error() != `polix.yaml:2: unknown key "onn"` {
		t.Fatalf("unknown key: %v", err)
	}
	if _, err := Parse([]byte("version: [\n"), "dev"); err == nil || !strings.HasPrefix(err.Error(), "polix.yaml: ") {
		t.Fatalf("broken yaml: %v", err)
	}
	if _, err := Parse([]byte("version: 1\nrules:\n  run: [go]\n"), "dev"); err == nil || errors.As(err, &invalid) {
		t.Fatalf("rules.run as a list: %v", err)
	}
}

// AC2, AC3: base_branch, max_stop_retries and guardrails.protected_files give the values base, max_stop_blocks and
// protected gave; each old name fails at its line with its new spelling, a problem Renamed tells from an option of
// a newer polix.
func TestParseRenamedTopKeys(t *testing.T) {
	w, err := Parse([]byte(`version: 1
base_branch: [dev, main]
max_stop_retries: 5
guardrails:
  protected_files: [.env, "ai/rules/**"]
`), "dev")
	if err != nil || strings.Join(w.Base, ",") != "dev,main" || w.MaxStopRetries != 5 ||
		strings.Join(w.ProtectedFiles, " ") != ".env ai/rules/**" {
		t.Fatalf("new names: %v %+v", err, w)
	}
	for src, want := range map[string]string{
		"version: 1\nprotected: [.env]\n":             `polix.yaml:2: unknown key "protected": use guardrails.protected_files`,
		"version: 1\n\nbase: dev\n":                   `polix.yaml:3: unknown key "base": use base_branch`,
		"version: 1\nmax_stop_blocks: 5\n":            `polix.yaml:2: unknown key "max_stop_blocks": use max_stop_retries`,
		"version: 1\nrequires: 0.1.0\nbase: [main]\n": `polix.yaml:3: unknown key "base": use base_branch`,
	} {
		_, err := Parse([]byte(src), "0.1.0")
		var invalid Invalid
		if err == nil || errors.As(err, &invalid) || err.Error() != want {
			t.Errorf("%q: got %v, want %s", src, err, want)
			continue
		}
		if !Renamed(err.Error()) {
			t.Errorf("not renamed: %v", err)
		}
	}
	if Renamed(`polix.yaml:2: unknown key "protected_paths": use guardrails.protected_files`) {
		t.Error("a name this polix never had is not renamed")
	}
}

// guardrails is a map of the five shortcuts: any other key or value stops the parse, and an empty one expands into
// nothing.
func TestParseGuardrails(t *testing.T) {
	var invalid Invalid
	five := "protected_files, forbidden_commands, protected_branches, format_on_edit, run_before_done"
	for src, want := range map[string]string{
		"version: 1\nguardrails:\n  protected_file: [.env]\n": `polix.yaml: line 3: unknown key "protected_file" in guardrails (` + five + ")",
		"version: 1\nguardrails: [.env]\n":                    "polix.yaml: line 2: guardrails must be a map with " + five,
	} {
		_, err := Parse([]byte(src), "dev")
		if err == nil || errors.As(err, &invalid) || err.Error() != want {
			t.Errorf("%q: got %v, want %s", src, err, want)
		}
	}
	w, err := Parse([]byte("version: 1\nguardrails: {}\n"), "dev")
	if err != nil || len(w.On) != 0 {
		t.Fatalf("empty guardrails: %v %+v", err, w)
	}
}

// No data is the default workflow; an empty file is a workflow without version, as before.
func TestParseDefaults(t *testing.T) {
	w, err := Parse(nil, "dev")
	if err != nil || w.Version != 1 || w.MaxStopRetries != DefaultMaxStopRetries || len(w.On) != 0 {
		t.Fatalf("default: %v %+v", err, w)
	}
	w, err = Parse([]byte("version: 1\n"), "dev")
	if err != nil || w.MaxStopRetries != DefaultMaxStopRetries {
		t.Fatalf("max_stop_retries default: %v %+v", err, w)
	}
	if _, err := Parse([]byte{}, "dev"); err == nil || !strings.Contains(err.Error(), "version must be 1, got 0") {
		t.Fatalf("empty file: %v", err)
	}
}

// The values of the file reach the domain: lines, keys, commands split on spaces and the runners in file order.
func TestParseValues(t *testing.T) {
	src := `version: 1
max_stop_retries: 5
on:
  stop:
    - when: { changed: "src/**" }
      unless: { finished: polix:reviewer }
      action:
        run: { command: just test, message: "Arregla los tests", always: true }
rules:
  run:
    "*_test.go": [go, test, "{dir}"]
    "*.sh": bash {file}
`
	w, err := Parse([]byte(src), "dev")
	if err != nil {
		t.Fatal(err)
	}
	e := w.On[Stop][0]
	r := e.Action.Run
	if e.Line != 5 || r == nil || strings.Join(r.Command, ",") != "just,test" || r.Message != "Arregla los tests" ||
		!r.Always || e.When["changed"] != "src/**" || e.Unless["finished"] != "polix:reviewer" ||
		strings.Join(e.Keys, ",") != "when,unless,action" || strings.Join(e.Action.Keys, ",") != "run" ||
		strings.Join(r.Keys, ",") != "command,message,always" || w.MaxStopRetries != 5 || w.EventLines[Stop] != 4 {
		t.Fatalf("entry: %+v %+v", e, r)
	}
	runners := w.Rules.Run
	if len(runners) != 2 || runners[0].Glob != "*_test.go" || runners[0].Line != 11 ||
		strings.Join(runners[1].Argv, " ") != "bash {file}" {
		t.Fatalf("runners: %+v", runners)
	}
}

// Each verb takes a short form (a run string split on spaces or a list, an inject text, a block message) and a
// long form, a map with once and always where they apply.
func TestParseVerbForms(t *testing.T) {
	src := `version: 1
on:
  pre_commit:
    - action: { run: "go vet  ./..." }
    - action: { run: [go, test, "./..."] }
    - action: { run: { rules: changed, message: "m" } }
    - action: { inject: "texto" }
    - action: { inject: { file: AGENTS.md, always: true } }
    - action: { block: "mensaje" }
    - action: { block: { message: "recuerda", once: true } }
`
	w, err := Parse([]byte(src), "dev")
	if err != nil {
		t.Fatal(err)
	}
	es := w.On[PreCommit]
	if a := es[0].Action.Run; a == nil || strings.Join(a.Command, ",") != "go,vet,./..." || a.Always || a.Keys != nil {
		t.Fatalf("run string: %+v", a)
	}
	if a := es[1].Action.Run; a == nil || strings.Join(a.Command, ",") != "go,test,./..." {
		t.Fatalf("run list: %+v", a)
	}
	if a := es[2].Action.Run; a == nil || a.Rules != "changed" || a.Message != "m" || len(a.Command) != 0 {
		t.Fatalf("run map: %+v", a)
	}
	if a := es[3].Action.Inject; a == nil || a.Text != "texto" || a.Always {
		t.Fatalf("inject text: %+v", a)
	}
	if a := es[4].Action.Inject; a == nil || a.File != "AGENTS.md" || !a.Always {
		t.Fatalf("inject map: %+v", a)
	}
	if a := es[5].Action.Block; a == nil || a.Message != "mensaje" || a.Once {
		t.Fatalf("block message: %+v", a)
	}
	if a := es[6].Action.Block; a == nil || a.Message != "recuerda" || !a.Once {
		t.Fatalf("block map: %+v", a)
	}
	for i, e := range es {
		if e.Action.Verb() == "" || e.Line != 4+i {
			t.Fatalf("entry %d: %+v", i, e)
		}
	}
}

// A verb, an action, once or always of the wrong kind of node stop the parse with the key and its line.
func TestParseVerbErrors(t *testing.T) {
	cases := map[string]string{
		"version: 1\non:\n  pre_commit:\n    - action: [block]\n":                                   "polix.yaml: line 4: action must be a map with one of run, inject, block",
		"version: 1\non:\n  pre_commit:\n    - action: { block: [a] }\n":                            "polix.yaml: line 4: block takes a message or a map with message and once",
		"version: 1\non:\n  pre_commit:\n    - action: { inject: [a] }\n":                           "polix.yaml: line 4: inject takes a text or a map with one of file, text or rules",
		"version: 1\non:\n  pre_commit:\n    - action:\n        run: { command: [ls, { a: b }] }\n": "polix.yaml: yaml: unmarshal errors:",
		"version: 1\non:\n  pre_commit:\n    - action: { block: { message: x, once: maybe } }\n":    "polix.yaml: line 4: block.once must be true or false",
		"version: 1\non:\n  pre_commit:\n    - action: { run: { command: ls, always: 1 } }\n":       "polix.yaml: line 4: run.always must be true or false",
		"version: 1\non:\n  pre_commit:\n    - action: { inject: { text: x, always: [a] } }\n":      "polix.yaml: line 4: inject.always must be true or false",
	}
	var invalid Invalid
	for src, want := range cases {
		if _, err := Parse([]byte(src), "dev"); err == nil || errors.As(err, &invalid) || !strings.HasPrefix(err.Error(), want) {
			t.Errorf("%q: got %v", src, err)
		}
	}
}

// The errors of yaml name the domain types, as the messages always did.
func TestParseDecodeErrorsNameDomainTypes(t *testing.T) {
	cases := map[string]string{
		"- a\n":                           "polix.yaml: yaml: unmarshal errors:\n  line 1: cannot unmarshal !!seq into workflow.Workflow",
		"version: 1\non: { stop: foo }\n": "polix.yaml: yaml: unmarshal errors:\n  line 2: cannot unmarshal !!str `foo` into []workflow.Entry",
		"version: 1\non: []\n":            "polix.yaml: yaml: unmarshal errors:\n  line 2: cannot unmarshal !!seq into map[string][]workflow.Entry",
		"version: 1\nvars: []\n":          "polix.yaml: yaml: unmarshal errors:\n  line 2: cannot unmarshal !!seq into map[string]workflow.Var",
	}
	for src, want := range cases {
		if _, err := Parse([]byte(src), "dev"); err == nil || err.Error() != want {
			t.Errorf("%q: got %v", src, err)
		}
	}
	// A value that is not the map its type decodes says what it needs, never the local type of the decoding: a
	// rules file passed as polix.yaml, an entry, a variable or a folder that is not a map.
	cases = map[string]string{
		"rules:\n  - id: GO-1\n":            "polix.yaml: line 2: rules must be a map with path and run",
		"version: 1\non: { stop: [foo] }\n": "polix.yaml: line 2: an entry must be a map with when, unless, action",
		"version: 1\nvars: { key: x }\n":    "polix.yaml: line 2: a variable of vars must be a map with branch",
		"version: 1\nplans: x\n":            "polix.yaml: line 2: plans and reports must be a map with path",
	}
	for src, want := range cases {
		if _, err := Parse([]byte(src), "dev"); err == nil || err.Error() != want {
			t.Errorf("%q: got %v", src, err)
		}
	}
}

// A switch of guidance left out is on, with or without polix.yaml; false turns it off.
func TestParseGuidance(t *testing.T) {
	cases := []struct {
		name, src    string
		commit, edit bool
	}{
		{"omitted", "version: 1\n", true, true},
		{"empty", "version: 1\nguidance: {}\n", true, true},
		{"null", "version: 1\nguidance:\n", true, true},
		{"commit off", "version: 1\nguidance: { commit: false }\n", false, true},
		{"edit off", "version: 1\nguidance: { edit: false }\n", true, false},
		{"both off", "version: 1\nguidance: { commit: false, edit: false }\n", false, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			w, err := Parse([]byte(c.src), "dev")
			if err != nil || w.Guidance.Commit != c.commit || w.Guidance.Edit != c.edit {
				t.Fatalf("got %v %+v", err, w.Guidance)
			}
		})
	}
	if w, err := Parse(nil, "dev"); err != nil || !w.Guidance.Commit || !w.Guidance.Edit {
		t.Fatalf("without polix.yaml: %v %+v", err, w.Guidance)
	}
}

// An unknown key of guidance is a problem; a value that is not a boolean, or a guidance that is not a map, stops
// the parse with the file in the message.
func TestParseGuidanceErrors(t *testing.T) {
	_, err := Parse([]byte("version: 1\nguidance: { commit: true, delegation: false }\n"), "dev")
	var invalid Invalid
	if !errors.As(err, &invalid) || !strings.Contains(err.Error(), `unknown key "delegation" in guidance (commit, edit, layout)`) {
		t.Fatalf("unknown key: %v", err)
	}
	cases := map[string]string{
		"version: 1\nguidance: { commit: maybe }\n": "polix.yaml: yaml: unmarshal errors:\n  line 2: cannot unmarshal !!str `maybe` into bool",
		"version: 1\nguidance: true\n":              "polix.yaml: line 2: guidance must map commit and edit to true or false",
		"version: 1\nguidance: [commit]\n":          "polix.yaml: line 2: guidance must map commit and edit to true or false",
	}
	for src, want := range cases {
		if _, err := Parse([]byte(src), "dev"); err == nil || errors.As(err, &invalid) || err.Error() != want {
			t.Errorf("%q: got %v", src, err)
		}
	}
}

// A glob of rules.run is a string, given once.
func TestParseRunnerKeys(t *testing.T) {
	_, err := Parse([]byte("version: 1\nrules:\n  run:\n    \"*.sh\": a\n    \"*.sh\": b\n"), "dev")
	if err == nil || !strings.Contains(err.Error(), `line 5: mapping key "*.sh" already defined at line 4`) {
		t.Fatalf("repeated glob: %v", err)
	}
	if _, err := Parse([]byte("version: 1\nrules:\n  run:\n    [a]: b\n"), "dev"); err == nil ||
		!strings.Contains(err.Error(), "a glob of rules.run must be a string") {
		t.Fatalf("non-scalar glob: %v", err)
	}
	_, err = Parse([]byte("version: 1\nrules:\n  run:\n    \"\": b\n"), "dev")
	var invalid Invalid
	if !errors.As(err, &invalid) || !strings.Contains(err.Error(), "polix.yaml:4: rules.run: a glob cannot be empty") {
		t.Fatalf("empty glob: %v", err)
	}
}

// risk.weights keeps the order of the file; a section or a value of the wrong shape stops the parse.
func TestParseRisk(t *testing.T) {
	w, err := Parse([]byte("version: 1\nrisk:\n  weights:\n    \"*_test.go\": 0.5\n    \"internal/**\": 3\n    \"docs/**\": 0\n"), "dev")
	if err != nil {
		t.Fatal(err)
	}
	if len(w.Weights) != 3 || w.Weights[0].Glob != "*_test.go" || w.Weights[0].Factor != 0.5 ||
		w.Weights[1].Glob != "internal/**" || w.Weights[1].Factor != 3 || w.Weights[2].Factor != 0 {
		t.Fatalf("weights = %+v", w.Weights)
	}
	var invalid Invalid
	for want, src := range map[string]string{
		"risk must be a map":                 "version: 1\nrisk: [a]\n",
		`unknown key "weight" in risk`:       "version: 1\nrisk:\n  weight: {}\n",
		"risk.weights must map a glob":       "version: 1\nrisk:\n  weights: [a]\n",
		`the weight of "a" must be a number`: "version: 1\nrisk:\n  weights:\n    a: high\n",
	} {
		if _, err := Parse([]byte(src), "dev"); err == nil || errors.As(err, &invalid) || !strings.Contains(err.Error(), want) {
			t.Errorf("%s: %v", want, err)
		}
	}
}
