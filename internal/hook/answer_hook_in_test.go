package hook

import (
	"context"
	"errors"
	"path/filepath"
	"slices"
	"strings"
	"testing"

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

func (p *answerHookFakePort) Changed(context.Context) []string { return []string{"src/a.go"} }

func (p *answerHookFakePort) Files(context.Context) []string { return []string{"ai/plans/p.md"} }

func (p *answerHookFakePort) LoadRules(context.Context) ([]rule.Rule, error) {
	return p.rules, p.rulesErr
}

func (p *answerHookFakePort) LoadState(context.Context) (state.State, error) {
	return state.State{}, p.stateErr
}

func (p *answerHookFakePort) Ignored(context.Context, string) bool { return false }

func (p *answerHookFakePort) Real(_ context.Context, paths []string) []string { return paths }

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
	p.calls = append(p.calls, "RunCheck "+run.Rule.ID)
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
	port := &answerHookFakePort{yaml: "version: 1\non:\n  pre_command:\n    - deny: \"no\"\n  pre_commit:\n    - deny: \"no\"\n"}
	res, err := answerHook(context.Background(), port, answerHookClaude(
		`{"hook_event_name":"PreToolUse","tool_name":"Bash","cwd":"/repo","tool_input":{"command":"git commit -m x"}}`))
	if err != nil {
		t.Fatal(err)
	}
	if res.Code != 2 || res.Stderr != "no\n" || len(port.appended) != 2 {
		t.Fatalf("response %+v, appended %+v", res, port.appended)
	}
}

// Every path of an edit is located and evaluated; the answers merge.
func TestAnswerHookLocatesEdits(t *testing.T) {
	port := &answerHookFakePort{yaml: "version: 1\nprotected: [\"src/**\"]\n"}
	patch := `*** Update File: src/a.go\n*** Update File: docs/b.md\n`
	res, err := answerHook(context.Background(), port, AnswerHookRequest{Agent: "codex", Version: "dev",
		Input: []byte(`{"hook_event_name":"PreToolUse","tool_name":"apply_patch","cwd":"/repo","tool_input":{"input":"` +
			patch + `"}}`)})
	if err != nil {
		t.Fatal(err)
	}
	if res.Code != 2 || !strings.Contains(res.Stderr, "src/a.go está protegido") {
		t.Fatalf("response %+v", res)
	}
	if !slices.Contains(port.calls, "Locate /repo/src/a.go") || !slices.Contains(port.calls, "Locate /repo/docs/b.md") {
		t.Fatalf("calls %q", port.calls)
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
	port = &answerHookFakePort{yaml: "version: 1\non:\n  stop:\n    - require: x\n", openErr: errors.New("not a repo")}
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
		{"hash", &answerHookFakePort{yaml: "version: 1\non:\n  stop:\n    - record: x\n", hashErr: errors.New("boom")},
			0, "polix: polix no puede calcular el hash del contenido (boom)"},
		{"rules", &answerHookFakePort{yaml: "version: 1\non:\n  stop:\n    - run: { rules: changed }\n",
			rulesErr: errors.New("boom")}, 2, "No se pueden cargar las rules: boom"},
		{"state", &answerHookFakePort{yaml: "version: 1\non:\n  stop:\n    - require: x\n", stateErr: errors.New("boom")},
			0, "no puede usar su log (boom): x"},
		{"append", &answerHookFakePort{yaml: "version: 1\non:\n  stop:\n    - require: x\n", appendErr: errors.New("boom")},
			0, "no puede usar su log (boom): x"},
		{"run", &answerHookFakePort{yaml: "version: 1\non:\n  stop:\n    - run: { command: [just, test] }\n",
			runErr: errors.New("exit status 1")}, 2, "Falla just test: exit status 1"},
		{"check", &answerHookFakePort{yaml: "version: 1\non:\n  stop:\n    - run: { rules: changed }\n",
			rules:  []rule.Rule{{ID: "R-1", Description: "d", Deterministic: true, Check: "c.sh"}},
			runErr: errors.New("exit status 1")}, 2, "- R-1 (c.sh): d"},
	} {
		res, err := answerHook(context.Background(), c.port, answerHookClaude(stop))
		if err != nil || res.Code != c.code || !strings.Contains(res.Stderr, c.wants) {
			t.Errorf("%s: %+v %v", c.name, res, err)
		}
	}
	// A file that cannot be read gives no context.
	port := &answerHookFakePort{yaml: "version: 1\non:\n  prompt:\n    - context: { file: notes.md }\n",
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
}
