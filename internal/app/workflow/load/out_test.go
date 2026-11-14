package load

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/mlophez/polix-ai/internal/app/workflow"
)

// The adapter is tested against a temporary folder: the real disk, as the port promises. parse reads src as the
// polix.yaml of a temporary root, through the adapter and the logic, as the loader does without its cache.

// parse writes src as polix.yaml of a temporary root and loads it for the polix of version running: the workflow
// and the error of loading it.
func parse(t *testing.T, src, running string) (workflow.Workflow, error) {
	t.Helper()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, workflow.File), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	loaded := Decide(Adapter{}.ReadWorkflow(context.Background(), filepath.Join(root, workflow.File), running)).Loaded
	if !loaded.Exists {
		t.Fatal("polix.yaml not found")
	}
	return loaded.Workflow, loaded.Err
}

// absentWorkflow loads the workflow of a root without polix.yaml, and its error.
func absentWorkflow(t *testing.T) (workflow.Workflow, error) {
	t.Helper()
	loaded := Decide(Adapter{}.ReadWorkflow(context.Background(), filepath.Join(t.TempDir(), workflow.File), "dev")).Loaded
	if loaded.Exists {
		t.Fatal("polix.yaml found")
	}
	return loaded.Workflow, loaded.Err
}

// A polix.yaml is read and decoded with its guardrails as written; without it there is nothing and no error.
func TestAdapterReadWorkflow(t *testing.T) {
	root := t.TempDir()
	if wf, g, exists, err := (Adapter{}).ReadWorkflow(context.Background(), filepath.Join(root, workflow.File), "dev"); exists || err != nil ||
		wf.Version != 0 || g.ProtectedFiles != nil {
		t.Fatalf("no polix.yaml: %+v %+v %v %v", wf, g, exists, err)
	}
	src := "version: 1\nguardrails:\n  protected_files: [.env]\n"
	if err := os.WriteFile(filepath.Join(root, workflow.File), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	wf, g, exists, err := Adapter{}.ReadWorkflow(context.Background(), filepath.Join(root, workflow.File), "dev")
	if !exists || err != nil || wf.Version != 1 || len(wf.ProtectedFiles) != 1 || len(g.ProtectedFiles) != 1 ||
		g.ProtectedFiles[0] != (workflow.GuardrailItem{Text: ".env", Line: 3}) {
		t.Fatalf("got %+v %+v %v %v", wf, g, exists, err)
	}
}

// A polix.yaml that cannot be read exists, with the error and no workflow.
func TestAdapterReadWorkflowUnreadable(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("the permissions of a file do not stop reads on Windows nor for root")
	}
	root := t.TempDir()
	p := filepath.Join(root, workflow.File)
	if err := os.WriteFile(p, []byte("version: 1\n"), 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(p, 0o644) })
	wf, _, exists, err := Adapter{}.ReadWorkflow(context.Background(), filepath.Join(root, workflow.File), "dev")
	if !exists || err == nil || wf.Version != 0 {
		t.Fatalf("got %+v %v %v", wf, exists, err)
	}
}

// requires is read before the rest of the file, so a polix older than the workflow asks to be updated instead of
// reporting the options it does not know; a development build is never too old.
func TestDecodeRequires(t *testing.T) {
	src := "version: 1\nrequires: 0.3.0\nan_option_of_0_3: true\n"
	_, err := parse(t, src, "0.2.1")
	var old workflow.TooOldError
	if !errors.As(err, &old) || old.Requires != "0.3.0" || old.Running != "0.2.1" {
		t.Fatalf("an older polix: %v", err)
	}
	if !strings.Contains(err.Error(), "polix.yaml requires polix 0.3.0 or newer and this is 0.2.1: run polix update") {
		t.Fatalf("message: %v", err)
	}
	if _, err := parse(t, src, "dev"); err == nil || errors.As(err, &old) {
		t.Fatalf("a dev build parses the file and reports what it does not know: %v", err)
	}
	if _, err := parse(t, src, "0.3.0"); err == nil || errors.As(err, &old) {
		t.Fatalf("the required version parses the file: %v", err)
	}

	w, err := parse(t, "version: 1\nrequires: v0.2.0\n", "0.2.0")
	if err != nil || w.Requires != "v0.2.0" || len(w.Problems()) != 0 {
		t.Fatalf("a polix as new as required: %v %+v %v", err, w, w.Problems())
	}
	w, err = parse(t, "version: 1\nrequires: 0.2\n", "0.2.0")
	var invalid workflow.InvalidError
	if !errors.As(err, &invalid) || len(w.Problems()) != 1 {
		t.Fatalf("requires must be a full version: %v", err)
	}
}

// An unknown top level key and broken YAML stop the parse, with the file in the message.
func TestDecodeErrors(t *testing.T) {
	var invalid workflow.InvalidError
	if _, err := parse(t, "version: 1\nonn: {}\n", "dev"); err == nil || errors.As(err, &invalid) ||
		err.Error() != `polix.yaml:2: unknown key "onn"` {
		t.Fatalf("unknown key: %v", err)
	}
	if _, err := parse(t, "version: [\n", "dev"); err == nil || !strings.HasPrefix(err.Error(), "polix.yaml: ") {
		t.Fatalf("broken yaml: %v", err)
	}
	if _, err := parse(t, "version: 1\nrules:\n  run: [go]\n", "dev"); err == nil || errors.As(err, &invalid) {
		t.Fatalf("rules.run as a list: %v", err)
	}
}

// AC2, AC3: base_branch, max_stop_retries and guardrails.protected_files give the values base, max_stop_blocks and
// protected gave; each old name fails at its line with its new spelling, a problem Renamed tells from an option of
// a newer polix.
func TestDecodeRenamedTopKeys(t *testing.T) {
	w, err := parse(t, `version: 1
base_branch: [dev, main]
max_stop_retries: 5
guardrails:
  protected_files: [.env, "ai/rules/**"]
`, "dev")
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
		_, err := parse(t, src, "0.1.0")
		var invalid workflow.InvalidError
		if err == nil || errors.As(err, &invalid) || err.Error() != want {
			t.Errorf("%q: got %v, want %s", src, err, want)
			continue
		}
		if !workflow.Problem(err.Error()).Renamed() {
			t.Errorf("not renamed: %v", err)
		}
	}
	if workflow.Problem(`polix.yaml:2: unknown key "protected_paths": use guardrails.protected_files`).Renamed() {
		t.Error("a name this polix never had is not renamed")
	}
}

// guardrails is a map of the five shortcuts: any other key or value stops the parse, and an empty one expands into
// nothing.
func TestDecodeGuardrails(t *testing.T) {
	var invalid workflow.InvalidError
	five := "protected_files, forbidden_commands, protected_branches, format_on_edit, run_on_stop"
	for src, want := range map[string]string{
		"version: 1\nguardrails:\n  protected_file: [.env]\n": `polix.yaml: line 3: unknown key "protected_file" in guardrails (` + five + ")",
		"version: 1\nguardrails: [.env]\n":                    "polix.yaml: line 2: guardrails must be a map with " + five,
	} {
		_, err := parse(t, src, "dev")
		if err == nil || errors.As(err, &invalid) || err.Error() != want {
			t.Errorf("%q: got %v, want %s", src, err, want)
		}
	}
	w, err := parse(t, "version: 1\nguardrails: {}\n", "dev")
	if err != nil || len(w.On) != 0 {
		t.Fatalf("empty guardrails: %v %+v", err, w)
	}
}

// No file is the default workflow; an empty file is a workflow without version, as before.
func TestDecodeDefaults(t *testing.T) {
	w, err := absentWorkflow(t)
	if err != nil || w.Version != 1 || w.MaxStopRetries != workflow.DefaultMaxStopRetries || len(w.On) != 0 {
		t.Fatalf("default: %v %+v", err, w)
	}
	w, err = parse(t, "version: 1\n", "dev")
	if err != nil || w.MaxStopRetries != workflow.DefaultMaxStopRetries {
		t.Fatalf("max_stop_retries default: %v %+v", err, w)
	}
	if _, err := parse(t, "", "dev"); err == nil || !strings.Contains(err.Error(), "version must be 1, got 0") {
		t.Fatalf("empty file: %v", err)
	}
}

// The values of the file reach the domain: lines, keys, commands split on spaces and the runners in file order.
func TestDecodeValues(t *testing.T) {
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
	w, err := parse(t, src, "dev")
	if err != nil {
		t.Fatal(err)
	}
	e := w.On[workflow.Stop][0]
	r := e.Action.Run
	if e.Line != 5 || r == nil || strings.Join(r.Command, ",") != "just,test" || r.Message != "Arregla los tests" ||
		!r.Always || e.When["changed"] != "src/**" || e.Unless["finished"] != "polix:reviewer" ||
		strings.Join(e.Keys, ",") != "when,unless,action" || strings.Join(e.Action.Keys, ",") != "run" ||
		strings.Join(r.Keys, ",") != "command,message,always" || w.MaxStopRetries != 5 ||
		w.EventLines[workflow.Stop] != 4 {
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
func TestDecodeVerbForms(t *testing.T) {
	src := `version: 1
on:
  pre_commit:
    - action: { run: "go vet  ./..." }
    - action: { run: [go, test, "./..."] }
    - action: { run: { rules: changed, message: "m" } }
    - action: { inject: "texto" }
    - action: { inject: { file: AGENTS.md, always: true } }
    - action: { block: { message: "recuerda", once: true } }
    - action: { block: "mensaje" }
`
	w, err := parse(t, src, "dev")
	if err != nil {
		t.Fatal(err)
	}
	es := w.On[workflow.PreCommit]
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
	if a := es[5].Action.Block; a == nil || a.Message != "recuerda" || !a.Once {
		t.Fatalf("block map: %+v", a)
	}
	if a := es[6].Action.Block; a == nil || a.Message != "mensaje" || a.Once {
		t.Fatalf("block message: %+v", a)
	}
	for i, e := range es {
		if e.Action.Verb() == "" || e.Line != 4+i {
			t.Fatalf("entry %d: %+v", i, e)
		}
	}
}

// AC1: githooks loads the entries of each of its events with their lines, apart from on, and the short and long
// forms of block and run; its problems come with those of the workflow, and a file without it loads as before.
func TestDecodeGithooks(t *testing.T) {
	src := `version: 1
vars:
  key: { branch: "^([A-Z][A-Z0-9]+-[0-9]+)" }
on:
  pre_commit:
    - action: { block: "IA" }
githooks:
  pre_commit:
    - when: { branch: "^main$" }
      action: { block: "No se commitea en {branch}" }
    - when: { changed: ["src/**", "*.go"] }
      action: { run: go test ./... }
    - action: { run: [go, vet, "./..."] }

  commit_msg:
    - when: { branch: "^[A-Z]+-[0-9]+" }
      unless: { message: "^{key}: " }
      action:
        block: { message: "{message} needs {key}" }
    - action:
        run: { command: just lint, message: "Arregla el lint" }
`
	w, err := parse(t, src, "dev")
	if err != nil {
		t.Fatal(err)
	}
	if len(w.On[workflow.PreCommit]) != 1 || len(w.On[workflow.CommitMsg]) != 0 {
		t.Fatalf("on: %+v", w.On)
	}
	pre, msg := w.Githooks[workflow.PreCommit], w.Githooks[workflow.CommitMsg]
	if len(pre) != 3 || len(msg) != 2 || w.GithookLines[workflow.PreCommit] != 8 ||
		w.GithookLines[workflow.CommitMsg] != 15 {
		t.Fatalf("githooks: %+v %v", w.Githooks, w.GithookLines)
	}
	if b := pre[0].Action.Block; b == nil || b.Message != "No se commitea en {branch}" || pre[0].Line != 9 ||
		pre[0].When["branch"] != "^main$" || strings.Join(pre[0].Keys, ",") != "when,action" {
		t.Fatalf("block short: %+v", pre[0])
	}
	if r := pre[1].Action.Run; r == nil || strings.Join(r.Command, ",") != "go,test,./..." || pre[1].Line != 11 {
		t.Fatalf("run string: %+v", pre[1])
	}
	if r := pre[2].Action.Run; r == nil || strings.Join(r.Command, ",") != "go,vet,./..." {
		t.Fatalf("run list: %+v", pre[2])
	}
	if b := msg[0].Action.Block; b == nil || b.Message != "{message} needs {key}" || msg[0].Line != 16 ||
		msg[0].Unless["message"] != "^{key}: " || strings.Join(b.Keys, ",") != "message" {
		t.Fatalf("block long: %+v", msg[0])
	}
	if r := msg[1].Action.Run; r == nil || strings.Join(r.Command, ",") != "just,lint" || r.Message != "Arregla el lint" ||
		strings.Join(r.Keys, ",") != "command,message" || msg[1].Line != 20 {
		t.Fatalf("run long: %+v", msg[1])
	}

	_, err = parse(t, "version: 1\ngithooks:\n  pre_push:\n    - action: { block: x }\n  pre_commit:\n"+
		"    - action: { inject: x }\n", "dev")
	var invalid workflow.InvalidError
	if !errors.As(err, &invalid) || len(invalid) != 2 ||
		invalid[0] != `polix.yaml:3: unknown githooks event "pre_push" (githooks events: pre_commit, commit_msg)` ||
		invalid[1] != `polix.yaml:6: githooks.pre_commit: verb "inject" is not allowed (githooks allow run, block)` {
		t.Fatalf("problems: %v", err)
	}

	w, err = parse(t, "version: 1\non:\n  stop:\n    - action: { block: x }\n", "dev")
	if err != nil || w.Githooks != nil || w.GithookLines != nil || len(w.On[workflow.Stop]) != 1 {
		t.Fatalf("without githooks: %v %+v", err, w)
	}
}

// AC22: a polix.yaml with a githooks entry loads with the entry that installs the hooks of git first in start,
// before the start entries written in on and those of the shortcuts; one without githooks, or with only empty
// githooks events, loads its start as before.
func TestDecodeGithooksInstall(t *testing.T) {
	const on = "version: 1\non:\n  start:\n    - action: { inject: hola }\n"
	w, err := parse(t, on+"githooks:\n  pre_commit: []\n  commit_msg:\n    - action: { block: x }\n", "dev")
	if err != nil {
		t.Fatal(err)
	}
	start := w.On[workflow.Start]
	if len(start) != 2 || start[0].From != "githooks" || start[0].Line != 7 || start[0].Action.Run == nil ||
		strings.Join(start[0].Action.Run.Command, " ") != "polix install --githooks" || !start[0].Action.Run.Always ||
		start[1].Action.Inject == nil || start[1].Line != 4 {
		t.Fatalf("start: %+v", start)
	}
	for _, src := range []string{on, on + "githooks:\n  pre_commit: []\n  commit_msg: []\n"} {
		w, err := parse(t, src, "dev")
		if start := w.On[workflow.Start]; err != nil || len(start) != 1 || start[0].Action.Inject == nil {
			t.Fatalf("%q: %v %+v", src, err, start)
		}
	}
}

// A verb, an action, once or always of the wrong kind of node stop the parse with the key and its line.
func TestDecodeVerbErrors(t *testing.T) {
	cases := map[string]string{
		"version: 1\non:\n  pre_commit:\n    - action: [block]\n":                                   "polix.yaml: line 4: action must be a map with one of run, inject, block",
		"version: 1\non:\n  pre_commit:\n    - action: { block: [a] }\n":                            "polix.yaml: line 4: block takes a message or a map with message and once",
		"version: 1\non:\n  pre_commit:\n    - action: { inject: [a] }\n":                           "polix.yaml: line 4: inject takes a text or a map with one of file, text or rules",
		"version: 1\non:\n  pre_commit:\n    - action:\n        run: { command: [ls, { a: b }] }\n": "polix.yaml: yaml: unmarshal errors:",
		"version: 1\non:\n  pre_commit:\n    - action: { block: { message: x, once: maybe } }\n":    "polix.yaml: line 4: block.once must be true or false",
		"version: 1\non:\n  pre_commit:\n    - action: { run: { command: ls, always: 1 } }\n":       "polix.yaml: line 4: run.always must be true or false",
		"version: 1\non:\n  pre_commit:\n    - action: { inject: { text: x, always: [a] } }\n":      "polix.yaml: line 4: inject.always must be true or false",
	}
	var invalid workflow.InvalidError
	for src, want := range cases {
		if _, err := parse(t, src, "dev"); err == nil || errors.As(err, &invalid) || !strings.HasPrefix(err.Error(), want) {
			t.Errorf("%q: got %v", src, err)
		}
	}
}

// The errors of yaml name the domain types, as the messages always did.
func TestDecodeErrorsNameDomainTypes(t *testing.T) {
	cases := map[string]string{
		"- a\n":                           "polix.yaml: yaml: unmarshal errors:\n  line 1: cannot unmarshal !!seq into workflow.Workflow",
		"version: 1\non: { stop: foo }\n": "polix.yaml: yaml: unmarshal errors:\n  line 2: cannot unmarshal !!str `foo` into []workflow.Entry",
		"version: 1\non: []\n":            "polix.yaml: yaml: unmarshal errors:\n  line 2: cannot unmarshal !!seq into map[string][]workflow.Entry",
		"version: 1\nvars: []\n":          "polix.yaml: yaml: unmarshal errors:\n  line 2: cannot unmarshal !!seq into map[string]workflow.Var",
	}
	for src, want := range cases {
		if _, err := parse(t, src, "dev"); err == nil || err.Error() != want {
			t.Errorf("%q: got %v", src, err)
		}
	}
	// A value that is not the map its type decodes says what it needs, never the local type of the decoding: a
	// rules file passed as polix.yaml, an entry, a variable or a folder that is not a map.
	cases = map[string]string{
		"rules:\n  - id: GO-1\n":            "polix.yaml: line 2: rules must be a map with run",
		"version: 1\non: { stop: [foo] }\n": "polix.yaml: line 2: an entry must be a map with when, unless, action",
		"version: 1\nvars: { key: x }\n":    "polix.yaml: line 2: a variable of vars must be a map with branch",
	}
	for src, want := range cases {
		if _, err := parse(t, src, "dev"); err == nil || err.Error() != want {
			t.Errorf("%q: got %v", src, err)
		}
	}
}

// CUJ-07, AC-04: guidance, in any form, stops the reading at its line with the hint of its migration, a name this
// polix replaced, never an option of a newer polix; without polix.yaml there is nothing to migrate.
func TestDecodeRemovedGuidance(t *testing.T) {
	const want = `polix.yaml:2: unknown key "guidance": remove it: polix always injects the rules on commits and the folders below path on start`
	for _, src := range []string{
		"version: 1\nguidance: { commit: true }\n",
		"version: 1\nguidance: { layout: false, edit: maybe }\n",
		"version: 1\nguidance: true\n",
		"version: 1\nguidance:\n",
	} {
		_, err := parse(t, src, "dev")
		var invalid workflow.InvalidError
		if err == nil || errors.As(err, &invalid) || err.Error() != want {
			t.Errorf("%q: got %v, want %s", src, err, want)
			continue
		}
		if !workflow.Problem(err.Error()).Renamed() {
			t.Errorf("not renamed: %v", err)
		}
	}
	if _, err := absentWorkflow(t); err != nil {
		t.Fatalf("without polix.yaml: %v", err)
	}
}

// A glob of rules.run is a string, given once.
func TestDecodeRunnerKeys(t *testing.T) {
	_, err := parse(t, "version: 1\nrules:\n  run:\n    \"*.sh\": a\n    \"*.sh\": b\n", "dev")
	if err == nil || !strings.Contains(err.Error(), `line 5: mapping key "*.sh" already defined at line 4`) {
		t.Fatalf("repeated glob: %v", err)
	}
	if _, err := parse(t, "version: 1\nrules:\n  run:\n    [a]: b\n", "dev"); err == nil ||
		!strings.Contains(err.Error(), "a glob of rules.run must be a string") {
		t.Fatalf("non-scalar glob: %v", err)
	}
	_, err = parse(t, "version: 1\nrules:\n  run:\n    \"\": b\n", "dev")
	var invalid workflow.InvalidError
	if !errors.As(err, &invalid) || !strings.Contains(err.Error(), "polix.yaml:4: rules.run: a glob cannot be empty") {
		t.Fatalf("empty glob: %v", err)
	}
}

// risk.weights keeps the order of the file; a section or a value of the wrong shape stops the parse.
func TestDecodeRisk(t *testing.T) {
	w, err := parse(t, "version: 1\nrisk:\n  weights:\n    \"*_test.go\": 0.5\n    \"internal/**\": 3\n    \"docs/**\": 0\n", "dev")
	if err != nil {
		t.Fatal(err)
	}
	if len(w.Weights) != 3 || w.Weights[0].Glob != "*_test.go" || w.Weights[0].Factor != 0.5 ||
		w.Weights[1].Glob != "internal/**" || w.Weights[1].Factor != 3 || w.Weights[2].Factor != 0 {
		t.Fatalf("weights = %+v", w.Weights)
	}
	var invalid workflow.InvalidError
	for want, src := range map[string]string{
		"risk must be a map":                 "version: 1\nrisk: [a]\n",
		`unknown key "weight" in risk`:       "version: 1\nrisk:\n  weight: {}\n",
		"risk.weights must map a glob":       "version: 1\nrisk:\n  weights: [a]\n",
		`the weight of "a" must be a number`: "version: 1\nrisk:\n  weights:\n    a: high\n",
	} {
		if _, err := parse(t, src, "dev"); err == nil || errors.As(err, &invalid) || !strings.Contains(err.Error(), want) {
			t.Errorf("%s: %v", want, err)
		}
	}
}

// AC16: an entry has no key guard: on any event and with any value, it is an unknown key at the line of its entry,
// listed with the keys an entry takes.
func TestDecodeEntryNoGuard(t *testing.T) {
	entry := "version: 1\non:\n  %s:\n    - guard: %s\n      action: { block: \"no\" }\n"
	for _, ev := range []string{workflow.PreEdit, workflow.PreCommand, workflow.PreCommit, workflow.Stop} {
		for _, value := range []string{"true", "yes please"} {
			_, err := parse(t, fmt.Sprintf(entry, ev, value), "dev")
			want := `polix.yaml:4: unknown key "guard" (when, unless, action)`
			if err == nil || err.Error() != want {
				t.Errorf("%s with guard: %s: got %v", ev, value, err)
			}
		}
	}
}

// CUJ-03, CUJ-05, AC-01, AC-03: the top-level key path places the folder of the AI, cleaned, with the folder of
// rules below it; a bad one is a problem of the file by its key, never an option of a newer polix, and a value that
// is not a string stops the parse with the error of yaml.
func TestDecodePath(t *testing.T) {
	for data, want := range map[string]string{
		"version: 1\n":                   "ai",
		"version: 1\npath: docs/ai\n":    "docs/ai",
		"version: 1\npath: ./docs/ai/\n": "docs/ai",
		"version: 1\npath: \"  \"\n":     "ai",
		"version: 1\npath:\n":            "ai",
		"version: 1\npath: 'docs\\ai'\n": "docs/ai",
	} {
		w, err := parse(t, data, "dev")
		if err != nil || w.PolixPath() != want || w.RulesPath() != want+"/rules" {
			t.Errorf("%q: %v, %q %q, want %q", data, err, w.PolixPath(), w.RulesPath(), want)
		}
	}
	for data, want := range map[string]string{
		"version: 1\npath: ../x\n":     `polix.yaml: path "../x" must be below the root`,
		"version: 1\npath: /etc\n":     `polix.yaml: path "/etc" must be relative to the root, not absolute`,
		"version: 1\npath: .\n":        `polix.yaml: path "." cannot be the root itself`,
		"version: 1\npath: \"ai/*\"\n": `polix.yaml: path "ai/*" must be a folder, not a glob`,
	} {
		_, err := parse(t, data, "dev")
		var invalid workflow.InvalidError
		if !errors.As(err, &invalid) || err.Error() != want {
			t.Errorf("%q: got %v, want %s", data, err, want)
			continue
		}
		if workflow.Problem(err.Error()).Renamed() {
			t.Errorf("%q: a bad path is no name of the old vocabulary", data)
		}
	}
	for _, data := range []string{"version: 1\npath: [docs]\n", "version: 1\npath: { x: y }\n"} {
		_, err := parse(t, data, "dev")
		var invalid workflow.InvalidError
		if err == nil || errors.As(err, &invalid) || !strings.Contains(err.Error(), "cannot unmarshal") {
			t.Errorf("%q: got %v, want the error of yaml", data, err)
		}
	}
}

// CUJ-06, CUJ-08, AC-04, AC-05: plans and reports stop the reading at their line with the hint of their migration,
// and path in rules is a problem of the file with its hint, next to the other problems; each is a name this polix
// replaced, never an option of a newer polix.
func TestDecodeRemovedKeys(t *testing.T) {
	for data, want := range map[string]string{
		"version: 1\nplans: { path: ai/plans }\n": `polix.yaml:2: unknown key "plans": remove it: the plans go to plans/ below path (ai/plans by default)`,
		"version: 1\n\nreports: { path: out }\n":  `polix.yaml:3: unknown key "reports": remove it: the reports go to reports/ below path (ai/reports by default)`,
		"version: 1\nreports: x\nplans: [a]\n":    `polix.yaml:2: unknown key "reports": remove it: the reports go to reports/ below path (ai/reports by default)`,
	} {
		_, err := parse(t, data, "dev")
		var invalid workflow.InvalidError
		if err == nil || errors.As(err, &invalid) || err.Error() != want {
			t.Errorf("%q: got %v, want %s", data, err, want)
			continue
		}
		if !workflow.Problem(err.Error()).Renamed() {
			t.Errorf("not renamed: %v", err)
		}
	}
	_, err := parse(t, "version: 1\nrules: { path: docs/rules }\n", "dev")
	want := `polix.yaml: unknown key "path" in rules (run): move it to the top-level key path, the folder that holds rules/ (ai by default)`
	var invalid workflow.InvalidError
	if !errors.As(err, &invalid) || err.Error() != want {
		t.Fatalf("rules.path: got %v, want %s", err, want)
	}
	if !workflow.Problem(err.Error()).Renamed() {
		t.Errorf("not renamed: %v", err)
	}
	_, err = parse(t, "version: 2\nrules: { path: docs/rules, folder: x }\n", "dev")
	for _, want := range []string{"version must be 1", want, `unknown key "folder" in rules (run)`} {
		if !errors.As(err, &invalid) || !strings.Contains(err.Error(), want) {
			t.Errorf("the other problems: %v lacks %s", err, want)
		}
	}
}

// base_branch takes one branch or a list, and main is the default.
func TestDecodeBaseBranches(t *testing.T) {
	for src, want := range map[string]string{
		"version: 1\n":                           "main",
		"version: 1\nbase_branch: dev\n":         "dev",
		"version: 1\nbase_branch: [dev, main]\n": "dev,main",
	} {
		w, err := parse(t, src, "dev")
		if err != nil {
			t.Fatal(err)
		}
		if got := strings.Join(w.BaseBranches(), ","); got != want {
			t.Errorf("%q: BaseBranches = %q, want %q", src, got, want)
		}
	}
	w, err := parse(t, "version: 1\nguardrails:\n  protected_files: [.env, \"ai/rules/**\"]\n", "dev")
	if err != nil || len(w.ProtectedFiles) != 2 || len(w.On) != 0 {
		t.Fatalf("protected_files: %v %+v", err, w)
	}
}
