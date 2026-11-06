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

// No data is the default workflow; an empty file is a workflow without version, as before.
func TestParseDefaults(t *testing.T) {
	w, err := Parse(nil, "dev")
	if err != nil || w.Version != 1 || w.MaxStopBlocks != DefaultMaxStopBlocks || len(w.On) != 0 {
		t.Fatalf("default: %v %+v", err, w)
	}
	w, err = Parse([]byte("version: 1\n"), "dev")
	if err != nil || w.MaxStopBlocks != DefaultMaxStopBlocks {
		t.Fatalf("max_stop_blocks default: %v %+v", err, w)
	}
	if _, err := Parse([]byte{}, "dev"); err == nil || !strings.Contains(err.Error(), "version must be 1, got 0") {
		t.Fatalf("empty file: %v", err)
	}
}

// The values of the file reach the domain: lines, keys, commands split on spaces and the runners in file order.
func TestParseValues(t *testing.T) {
	src := `version: 1
max_stop_blocks: 5
on:
  stop:
    - if: { changed: "src/**" }
      run: { command: just test }
      record: tested
      mode: warn
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
	if e.Line != 5 || e.Run == nil || strings.Join(e.Run.Command, ",") != "just,test" || e.Record == nil || *e.Record != "tested" ||
		e.EffectiveMode() != ModeWarn || w.MaxStopBlocks != 5 {
		t.Fatalf("entry: %+v %+v", e, e.Run)
	}
	r := w.Rules.Run
	if len(r) != 2 || r[0].Glob != "*_test.go" || r[0].Line != 11 || strings.Join(r[1].Argv, " ") != "bash {file}" {
		t.Fatalf("runners: %+v", r)
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
