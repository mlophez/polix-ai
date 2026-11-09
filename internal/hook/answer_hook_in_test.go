package hook

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/mlophez/polix-ai/internal/diff"
	"github.com/mlophez/polix-ai/internal/rule"
	"github.com/mlophez/polix-ai/internal/state"
	"github.com/mlophez/polix-ai/internal/workflow"
)

// answerHookFakePort is a fake of AnswerHookPort: a repository at /repo with the workflow of its yaml, the errors
// it is told to give, and the calls it got.
type answerHookFakePort struct {
	yaml      string // polix.yaml; empty for none
	openErr   error
	hashErr   error
	rulesErr  error
	stateErr  error
	runErr    error
	appendErr error
	readErr   error
	diffErr   error
	treeErr   error
	state     state.State
	trees     []string        // the paths TreeDiff gives
	foreign   map[string]bool // the folders inside another git repository
	rules     []rule.Rule
	calls     []string
	appended  []state.Entry
}

var _ AnswerHookPort = (*answerHookFakePort)(nil)

func (p *answerHookFakePort) Open(_ context.Context, dir string) (string, string, error) {
	p.calls = append(p.calls, "Open "+dir)
	if p.openErr != nil {
		return "", "", p.openErr
	}
	root := filepath.FromSlash("/repo")
	return root, filepath.Join(root, ".git"), nil
}

func (p *answerHookFakePort) LoadWorkflow(_ context.Context, running string) (workflow.Workflow, bool, error) {
	if p.yaml == "" {
		return workflow.Workflow{}, false, nil
	}
	wf, err := workflow.Parse([]byte(p.yaml), running)
	return wf, true, err
}

func (p *answerHookFakePort) Locate(_ context.Context, path, _ string) (string, bool) {
	p.calls = append(p.calls, "Locate "+filepath.ToSlash(path))
	rel, ok := strings.CutPrefix(filepath.ToSlash(path), "/repo/")
	return rel, ok
}

func (p *answerHookFakePort) Branch(context.Context) string { return "main" }

func (p *answerHookFakePort) Hash(context.Context) (string, error) {
	p.calls = append(p.calls, "Hash")
	if p.hashErr != nil {
		return "", p.hashErr
	}
	return "h", nil
}

func (p *answerHookFakePort) TreeDiff(_ context.Context, from, to string) ([]string, error) {
	p.calls = append(p.calls, "TreeDiff "+from+" "+to)
	return p.trees, p.treeErr
}

func (p *answerHookFakePort) Changed(context.Context) []string { return []string{"src/a.go"} }

func (p *answerHookFakePort) Diff(context.Context) (diff.Diff, error) {
	p.calls = append(p.calls, "Diff")
	if p.diffErr != nil {
		return diff.Diff{}, p.diffErr
	}
	return diff.Diff{Changes: []diff.Change{{Path: "src/a.go", Added: 200, Deleted: 10}}}, nil
}

func (p *answerHookFakePort) Files(context.Context) []string { return []string{"ai/plans/p.md"} }

func (p *answerHookFakePort) LoadRules(context.Context, string) ([]rule.Rule, error) {
	return p.rules, p.rulesErr
}

func (p *answerHookFakePort) LoadState(context.Context) (state.State, error) {
	if p.stateErr != nil {
		return state.State{}, p.stateErr
	}
	return p.state, nil
}

func (p *answerHookFakePort) Ignored(context.Context, string) bool { return false }

func (p *answerHookFakePort) Real(_ context.Context, paths []string) []string { return paths }

func (p *answerHookFakePort) Foreign(_ context.Context, dir string) bool {
	p.calls = append(p.calls, "Foreign "+filepath.ToSlash(dir))
	return p.foreign[dir]
}

func (p *answerHookFakePort) Contains(_ context.Context, _ []string, text string) bool {
	p.calls = append(p.calls, "Contains "+text)
	return text == "Ticket: PLAT-1"
}

func (p *answerHookFakePort) Read(_ context.Context, path string) (string, error) {
	p.calls = append(p.calls, "Read "+path)
	return "Ticket: PLAT-1", p.readErr
}

func (p *answerHookFakePort) Run(_ context.Context, argv []string) (string, error) {
	p.calls = append(p.calls, "Run "+strings.Join(argv, " "))
	return "salida", p.runErr
}

func (p *answerHookFakePort) RunCheck(_ context.Context, run rule.Run) (string, error) {
	var ids []string
	for _, r := range run.Rules {
		ids = append(ids, r.ID)
	}
	p.calls = append(p.calls, "RunCheck "+strings.Join(ids, " "))
	return "salida", p.runErr
}

func (p *answerHookFakePort) Append(_ context.Context, e state.Entry) error {
	if p.appendErr != nil {
		return p.appendErr
	}
	p.appended = append(p.appended, e)
	return nil
}

// answerHookClaude returns the request of a hook call of Claude Code with input.
func answerHookClaude(input string) AnswerHookRequest {
	return AnswerHookRequest{Agent: "claude", Input: []byte(input), Version: "dev", System: "linux"}
}

// The events of one call are evaluated in turn and their answers merged: a command and its commit give one
// message.
func TestAnswerHookMergesEvents(t *testing.T) {
	port := &answerHookFakePort{yaml: "version: 1\non:\n  pre_command:\n    - action: { block: \"no\" }\n" +
		"  pre_commit:\n    - action: { block: \"no\" }\n"}
	res, err := answerHook(context.Background(), port, answerHookClaude(
		`{"hook_event_name":"PreToolUse","tool_name":"Bash","cwd":"/repo","tool_input":{"command":"git commit -m x"}}`))
	if err != nil {
		t.Fatal(err)
	}
	if res.Code != 2 || res.Stderr != "no\n" || len(port.appended) != 2 {
		t.Fatalf("response %+v, appended %+v", res, port.appended)
	}
}

// A commit the shell confirms through the port as one of another repository skips the entries of pre_commit; one
// it does not confirm meets them.
func TestAnswerHookCommitInAnotherRepository(t *testing.T) {
	yaml := "version: 1\non:\n  pre_commit:\n    - action: { block: \"no\" }\n"
	other := filepath.FromSlash("/other")
	for foreign, code := range map[bool]int{true: 0, false: 2} {
		port := &answerHookFakePort{yaml: yaml, foreign: map[string]bool{other: foreign}}
		res, err := answerHook(context.Background(), port, answerHookClaude(
			`{"hook_event_name":"PreToolUse","tool_name":"Bash","cwd":"/repo","tool_input":{"command":"git -C /other commit -m x"}}`))
		if err != nil {
			t.Fatal(err)
		}
		if res.Code != code || !slices.Contains(port.calls, "Foreign /other") {
			t.Errorf("foreign %v: response %+v, calls %q", foreign, res, port.calls)
		}
	}
}

// Every path of an edit is located and evaluated; the answers merge.
func TestAnswerHookLocatesEdits(t *testing.T) {
	port := &answerHookFakePort{yaml: "version: 1\nguardrails:\n  protected_files: [\"src/**\"]\n"}
	patch := `*** Update File: src/a.go\n*** Update File: docs/b.md\n`
	res, err := answerHook(context.Background(), port, AnswerHookRequest{Agent: "codex", Version: "dev",
		Input: []byte(`{"hook_event_name":"PreToolUse","tool_name":"apply_patch","cwd":"/repo","tool_input":{"input":"` +
			patch + `"}}`)})
	if err != nil {
		t.Fatal(err)
	}
	if res.Code != 2 || !strings.Contains(res.Stderr, "src/a.go is protected in polix.yaml: only a person changes it.") {
		t.Fatalf("response %+v", res)
	}
	if !slices.Contains(port.calls, "Locate /repo/src/a.go") || !slices.Contains(port.calls, "Locate /repo/docs/b.md") {
		t.Fatalf("calls %q", port.calls)
	}
}

// The guidance reaches the session of Claude Code and of Codex as the additionalContext of SessionStart.
func TestAnswerHookGuidanceRendered(t *testing.T) {
	for _, agentName := range []string{"claude", "codex"} {
		port := &answerHookFakePort{yaml: "version: 1\n"}
		res, err := answerHook(context.Background(), port, AnswerHookRequest{Agent: agentName, Version: "dev",
			System: "linux", Input: []byte(`{"hook_event_name":"SessionStart","cwd":"/repo"}`)})
		if err != nil || res.Code != 0 {
			t.Fatalf("%s: %+v %v", agentName, res, err)
		}
		var out struct {
			HookSpecificOutput struct {
				AdditionalContext string `json:"additionalContext"`
			} `json:"hookSpecificOutput"`
		}
		if err := json.Unmarshal([]byte(res.Stdout), &out); err != nil {
			t.Fatalf("%s: stdout %q: %v", agentName, res.Stdout, err)
		}
		wf, _ := workflow.Parse([]byte("version: 1\n"), "dev")
		if want := wf.SessionContext(); want == "" || out.HookSpecificOutput.AdditionalContext != want {
			t.Fatalf("%s: additionalContext %q", agentName, out.HookSpecificOutput.AdditionalContext)
		}
	}
}

// Without events the call is not relevant, and outside a repository there is nothing to govern: both let
// everything through; an input that is not JSON is an error.
func TestAnswerHookNothingToGovern(t *testing.T) {
	port := &answerHookFakePort{}
	res, err := answerHook(context.Background(), port, answerHookClaude(`{"hook_event_name":"PreToolUse","tool_name":"Read"}`))
	if err != nil || res != (AnswerHookResponse{}) || len(port.calls) != 0 {
		t.Fatalf("no events: %+v %v %q", res, err, port.calls)
	}
	port = &answerHookFakePort{yaml: "version: 1\non:\n  stop:\n    - action: { block: x }\n",
		openErr: errors.New("not a repo")}
	res, err = answerHook(context.Background(), port, answerHookClaude(`{"hook_event_name":"Stop","cwd":"/tmp/x"}`))
	if err != nil || res != (AnswerHookResponse{}) || !slices.Equal(port.calls, []string{"Open /tmp/x"}) {
		t.Fatalf("outside a repository: %+v %v %q", res, err, port.calls)
	}
	if _, err := answerHook(context.Background(), port, answerHookClaude(`not json`)); err == nil ||
		!strings.Contains(err.Error(), "claude hook input") {
		t.Fatalf("an input that is not JSON: %v", err)
	}
	if _, err := answerHook(context.Background(), port, AnswerHookRequest{Agent: "other"}); err == nil {
		t.Fatal("an unknown agent is an error")
	}
}

// The dir of the request wins over the cwd of the call.
func TestAnswerHookDir(t *testing.T) {
	port := &answerHookFakePort{}
	req := answerHookClaude(`{"hook_event_name":"Stop","cwd":"/tmp/x"}`)
	req.Dir = "/repo"
	if _, err := answerHook(context.Background(), port, req); err != nil || port.calls[0] != "Open /repo" {
		t.Fatalf("%v %q", err, port.calls)
	}
}

// Each error of the port that a step meets is data of the answer, as the failure policy says.
func TestAnswerHookPortErrors(t *testing.T) {
	stop := `{"hook_event_name":"Stop","cwd":"/repo"}`
	for _, c := range []struct {
		name  string
		port  *answerHookFakePort
		code  int
		wants string
	}{
		{"hash", &answerHookFakePort{yaml: "version: 1\non:\n  stop:\n    - unless: { finished: polix:reviewer }\n" +
			"      action: { run: ls }\n", hashErr: errors.New("boom")},
			0, "polix: polix no puede calcular el hash del contenido (boom)"},
		{"rules", &answerHookFakePort{yaml: "version: 1\non:\n  stop:\n    - action: { run: { rules: changed } }\n",
			rulesErr: errors.New("boom")}, 2, "No se pueden cargar las rules: boom"},
		{"state", &answerHookFakePort{yaml: "version: 1\non:\n  stop:\n    - action: { block: x }\n",
			stateErr: errors.New("boom")}, 0, "no puede usar su log (boom): x"},
		{"append", &answerHookFakePort{yaml: "version: 1\non:\n  stop:\n    - action: { block: x }\n",
			appendErr: errors.New("boom")}, 0, "no puede usar su log (boom): x"},
		{"run", &answerHookFakePort{yaml: "version: 1\non:\n  stop:\n    - action: { run: [just, test] }\n",
			runErr: errors.New("exit status 1")}, 2, "Falla just test: exit status 1"},
		{"check", &answerHookFakePort{yaml: "version: 1\non:\n  stop:\n    - action: { run: { rules: changed } }\n",
			rules:  []rule.Rule{{ID: "R-1", Description: "d", Deterministic: true, Check: "c.sh"}},
			runErr: errors.New("exit status 1")}, 2, "- R-1 (c.sh): d"},
	} {
		res, err := answerHook(context.Background(), c.port, answerHookClaude(stop))
		if err != nil || res.Code != c.code || !strings.Contains(res.Stderr, c.wants) {
			t.Errorf("%s: %+v %v", c.name, res, err)
		}
	}
	// A file that cannot be read gives no context.
	port := &answerHookFakePort{yaml: "version: 1\non:\n  prompt:\n    - action: { inject: { file: notes.md } }\n",
		readErr: errors.New("boom")}
	res, err := answerHook(context.Background(), port, answerHookClaude(`{"hook_event_name":"UserPromptSubmit","cwd":"/repo"}`))
	if err != nil || res.Stdout != "" || !slices.Contains(port.calls, "Read notes.md") {
		t.Fatalf("unreadable context: %+v %v %q", res, err, port.calls)
	}
	port.readErr = nil
	res, _ = answerHook(context.Background(), port, answerHookClaude(`{"hook_event_name":"UserPromptSubmit","cwd":"/repo"}`))
	if !strings.Contains(res.Stdout, `"additionalContext":"Ticket: PLAT-1"`) {
		t.Fatalf("context: %+v", res)
	}
}

// A batch runner checks a passing group of rules with one call of the port.
func TestAnswerHookRunsAPassingGroupOnce(t *testing.T) {
	port := &answerHookFakePort{yaml: "version: 1\nrules:\n  run:\n    \"*_test.go\": [go, test, \"{dir}\", -run, " +
		"\"^Test({names})$\"]\non:\n  stop:\n    - action: { run: { rules: changed } }\n",
		rules: []rule.Rule{
			{ID: "A-1", Description: "d", Deterministic: true, Check: "t/a1_test.go"},
			{ID: "A-2", Description: "d", Deterministic: true, Check: "t/a2_test.go"},
		}}
	res, err := answerHook(context.Background(), port, answerHookClaude(`{"hook_event_name":"Stop","cwd":"/repo"}`))
	if err != nil || res.Code != 0 {
		t.Fatalf("got %+v %v", res, err)
	}
	var checks []string
	for _, c := range port.calls {
		if strings.HasPrefix(c, "RunCheck") {
			checks = append(checks, c)
		}
	}
	if !slices.Equal(checks, []string{"RunCheck A-1 A-2"}) {
		t.Fatalf("checks: %v", checks)
	}
}

// A workflow that cannot be used fails closed on edits and commands, with the hint of a newer polix when its
// problems may come from one, and only warns on a stop.
func TestAnswerHookUnusableWorkflow(t *testing.T) {
	port := &answerHookFakePort{yaml: "version: 2\n"}
	res, err := answerHook(context.Background(), port, answerHookClaude(
		`{"hook_event_name":"PreToolUse","tool_name":"Bash","cwd":"/repo","tool_input":{"command":"ls"}}`))
	if err != nil || res.Code != 2 || !strings.Contains(res.Stderr, "no las quites: ejecuta polix update") {
		t.Fatalf("command: %+v %v", res, err)
	}
	res, err = answerHook(context.Background(), port, answerHookClaude(`{"hook_event_name":"Stop","cwd":"/repo"}`))
	if err != nil || res.Code != 0 || !strings.HasPrefix(res.Stderr, "polix: el workflow no es válido") {
		t.Fatalf("stop: %+v %v", res, err)
	}
	// A workflow of the old vocabulary is told its new spelling, never to update polix to keep it.
	port = &answerHookFakePort{yaml: "version: 1\non:\n  turn_stop:\n    - require: x\n"}
	res, err = answerHook(context.Background(), port, answerHookClaude(
		`{"hook_event_name":"PreToolUse","tool_name":"Bash","cwd":"/repo","tool_input":{"command":"ls"}}`))
	if err != nil || res.Code != 2 || !strings.Contains(res.Stderr, `unknown event "turn_stop": use stop`) ||
		!strings.Contains(res.Stderr, `unknown key "require": use action: { block: <message> }`) ||
		strings.Contains(res.Stderr, "no las quites") {
		t.Fatalf("old vocabulary: %+v %v", res, err)
	}
}

// The shell answers the comparison of trees through the port: a review of the Go code holds after an edit of the
// docs, and not after an edit of a Go file nor when the trees cannot be compared.
func TestAnswerHookComparesTrees(t *testing.T) {
	yaml := "version: 1\non:\n  pre_commit:\n    - when: { changed: \"*.go\" }\n" +
		"      unless: { finished: polix:reviewer }\n      action: { block: \"revisa\" }\n"
	commit := `{"hook_event_name":"PreToolUse","tool_name":"Bash","cwd":"/repo","tool_input":{"command":"git commit -m x"}}`
	reviewed := state.State{Entries: []state.Entry{{Kind: state.KindObserved, Outcome: state.OutcomeFinished,
		Key: "polix:reviewer", Branch: "main", Hash: "old"}}}
	for name, c := range map[string]struct {
		trees []string
		err   error
		code  int
	}{
		"docs":    {[]string{"README.md"}, nil, 0},
		"go":      {[]string{"src/a.go"}, nil, 2},
		"missing": {nil, errors.New("fatal: bad object old"), 2},
	} {
		port := &answerHookFakePort{yaml: yaml, state: reviewed, trees: c.trees, treeErr: c.err}
		res, err := answerHook(context.Background(), port, answerHookClaude(commit))
		if err != nil || res.Code != c.code || !slices.Contains(port.calls, "TreeDiff old h") {
			t.Errorf("%s: %+v %v %q", name, res, err, port.calls)
		}
	}
}

// The shell answers the Diff step through the port; its error makes the measures hold, with a warning.
func TestAnswerHookMeasuresTheDiff(t *testing.T) {
	yaml := "version: 1\non:\n  pre_commit:\n    - when: { changed: \"*.go\", changed_lines: 150 }\n" +
		"      action: { block: \"{changed_lines} lineas\" }\n"
	commit := `{"hook_event_name":"PreToolUse","tool_name":"Bash","cwd":"/repo","tool_input":{"command":"git commit -m x"}}`
	port := &answerHookFakePort{yaml: yaml}
	res, err := answerHook(context.Background(), port, answerHookClaude(commit))
	if err != nil || res.Code != 2 || !strings.Contains(res.Stderr, "210 lineas") || !slices.Contains(port.calls, "Diff") {
		t.Fatalf("measured: %+v %v %v", res, err, port.calls)
	}
	port = &answerHookFakePort{yaml: yaml, diffErr: errors.New("git roto")}
	res, err = answerHook(context.Background(), port, answerHookClaude(commit))
	if err != nil || res.Code != 2 || !strings.Contains(res.Stderr, "? lineas") {
		t.Fatalf("not measured: %+v %v", res, err)
	}
}
