package answer

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/mlophez/polix-ai/internal/app/agent"
	"github.com/mlophez/polix-ai/internal/app/diff"
	"github.com/mlophez/polix-ai/internal/app/hook"
	"github.com/mlophez/polix-ai/internal/app/rule"
	"github.com/mlophez/polix-ai/internal/app/state"
	"github.com/mlophez/polix-ai/internal/app/workflow"
	"github.com/mlophez/polix-ai/internal/platform/testenv"
)

// fakePort is a fake of Port: a repository at /repo with the workflow of its yaml, the errors
// it is told to give, and the calls it got.
type fakePort struct {
	yaml      string // polix.yaml; empty for none
	openErr   error
	hashErr   error
	rulesErr  error
	stateErr  error
	runErr    error
	appendErr error
	readErr   error
	diffErr   error
	commitErr error // the error Commit gives
	treeErr   error
	restErr   error // the error Restore gives
	state     state.State
	trees     []string        // the paths TreeDiff gives
	removed   []string        // those of trees TreeDiff gives as removed
	refs      string          // what Refs gives
	foreign   map[string]bool // the folders inside another git repository
	absent    map[string]bool // the programs the PATH does not run
	rules     []rule.Rule
	calls     []string
	appended  []state.Entry
}

var _ Port = (*fakePort)(nil)

// TestMain runs the tests without the variables git exports to its hooks (see testenv.ClearGitHookEnv): the githooks
// of polix.yaml run go test, and these tests run git in repositories of their own.
func TestMain(m *testing.M) {
	testenv.ClearGitHookEnv()
	os.Exit(m.Run())
}

func (p *fakePort) Open(_ context.Context, dir string) (string, string, error) {
	p.calls = append(p.calls, "Open "+dir)
	if p.openErr != nil {
		return "", "", p.openErr
	}
	root := filepath.FromSlash("/repo")
	return root, filepath.Join(root, ".git"), nil
}

func (p *fakePort) LoadWorkflow(_ context.Context, running string) (workflow.Workflow, bool, error) {
	if p.yaml == "" {
		return workflow.Workflow{}, false, nil
	}
	wf, err := parseWorkflow(p.yaml, running)
	return wf, true, err
}

func (p *fakePort) Locate(_ context.Context, path, _ string) (string, bool) {
	p.calls = append(p.calls, "Locate "+filepath.ToSlash(path))
	rel, ok := strings.CutPrefix(filepath.ToSlash(path), "/repo/")
	return rel, ok
}

func (p *fakePort) Head(context.Context) (string, string) {
	p.calls = append(p.calls, "Head")
	return "main", "c0ffee"
}

func (p *fakePort) Hash(context.Context) (string, error) {
	p.calls = append(p.calls, "Hash")
	if p.hashErr != nil {
		return "", p.hashErr
	}
	return "h", nil
}

func (p *fakePort) Refs(context.Context) string { return p.refs }

func (p *fakePort) TreeDiff(_ context.Context, from, to string) ([]string, []string, error) {
	p.calls = append(p.calls, "TreeDiff "+from+" "+to)
	return p.trees, p.removed, p.treeErr
}

func (p *fakePort) Changed(context.Context) []string { return []string{"src/a.go"} }

func (p *fakePort) Diff(context.Context) (diff.Diff, error) {
	p.calls = append(p.calls, "Diff")
	if p.diffErr != nil {
		return diff.Diff{}, p.diffErr
	}
	return diff.Diff{Changes: []diff.Change{{Path: "src/a.go", Added: 200, Deleted: 10}}}, nil
}

func (p *fakePort) Commit(context.Context) (hook.Commit, error) {
	p.calls = append(p.calls, "Commit")
	if p.commitErr != nil {
		return hook.Commit{}, p.commitErr
	}
	return hook.Commit{Diff: diff.Diff{Changes: []diff.Change{{Path: "docs/guide.md", Added: 4}}}, Hash: "abc1234",
		Subject: "feat: x"}, nil
}

func (p *fakePort) Files(context.Context) []string { return []string{"ai/plans/p.md"} }

func (p *fakePort) LoadRules(context.Context, string) ([]rule.Rule, error) {
	return p.rules, p.rulesErr
}

func (p *fakePort) LoadState(context.Context) (state.State, error) {
	if p.stateErr != nil {
		return state.State{}, p.stateErr
	}
	return p.state, nil
}

func (p *fakePort) Ignored(context.Context, string) bool { return false }

func (p *fakePort) Real(_ context.Context, paths []string) []string { return paths }

func (p *fakePort) Foreign(_ context.Context, dir string) bool {
	p.calls = append(p.calls, "Foreign "+filepath.ToSlash(dir))
	return p.foreign[dir]
}

func (p *fakePort) Missing(_ context.Context, programs []string) []string {
	p.calls = append(p.calls, "Missing "+strings.Join(programs, " "))
	var missing []string
	for _, prog := range programs {
		if p.absent[prog] {
			missing = append(missing, prog)
		}
	}
	return missing
}

func (p *fakePort) Contains(_ context.Context, _ []string, text string) bool {
	p.calls = append(p.calls, "Contains "+text)
	return text == "Ticket: PLAT-1"
}

func (p *fakePort) Read(_ context.Context, path string) (string, error) {
	p.calls = append(p.calls, "Read "+path)
	return "Ticket: PLAT-1", p.readErr
}

func (p *fakePort) Run(_ context.Context, argv []string) (string, error) {
	p.calls = append(p.calls, "Run "+strings.Join(argv, " "))
	return "salida", p.runErr
}

func (p *fakePort) RunCheck(_ context.Context, run rule.Run) (string, error) {
	var ids []string
	for _, r := range run.Rules {
		ids = append(ids, r.ID)
	}
	p.calls = append(p.calls, "RunCheck "+strings.Join(ids, " "))
	return "salida", p.runErr
}

func (p *fakePort) Restore(_ context.Context, source string, paths []string) error {
	p.calls = append(p.calls, "Restore "+source+" "+strings.Join(paths, " "))
	return p.restErr
}

func (p *fakePort) Append(_ context.Context, e state.Entry) error {
	if p.appendErr != nil {
		return p.appendErr
	}
	p.appended = append(p.appended, e)
	return nil
}

// claudeRequest returns the request of a hook call of Claude Code with input.
func claudeRequest(input string) Request {
	return Request{Agent: "claude", Input: []byte(input), Version: "dev", System: "linux"}
}

// The events of one call are evaluated in turn and their answers merged: a command and its commit give one
// message.
func TestMergesEvents(t *testing.T) {
	port := &fakePort{yaml: "version: 1\non:\n  pre_command:\n    - action: { block: \"no\" }\n" +
		"  pre_commit:\n    - action: { block: \"no\" }\n"}
	res, err := run(context.Background(), port, claudeRequest(
		`{"hook_event_name":"PreToolUse","tool_name":"Bash","cwd":"/repo","tool_input":{"command":"git commit -m x"}}`))
	if err != nil {
		t.Fatal(err)
	}
	if res.Code != 2 || res.Stderr != "no\n" || len(port.appended) != 2 {
		t.Fatalf("response %+v, appended %+v", res, port.appended)
	}
}

// AC9: the shell tells each event how many come after it, so a notice of the pre_command of a commit is written as
// notified only when the commit is not blocked, and shown to the user then.
func TestNotifiedOnceTheCallIsKnown(t *testing.T) {
	const input = `{"hook_event_name":"PreToolUse","tool_name":"Bash","cwd":"/repo","tool_input":{"command":"git commit -m x"}}`
	for _, c := range []struct {
		branch   string
		code     int
		notified int
	}{{"^main$", 2, 0}, {"^dev$", 0, 1}} {
		port := &fakePort{yaml: "version: 1\non:\n  pre_command:\n    - action: { notify: \"Aviso\" }\n" +
			"  pre_commit:\n    - when: { branch: \"" + c.branch + "\" }\n      action: { block: \"no\" }\n"}
		res, err := run(context.Background(), port, claudeRequest(input))
		if err != nil {
			t.Fatal(err)
		}
		notified := 0
		for _, e := range port.appended {
			if e.Outcome == state.OutcomeNotified {
				notified++
			}
		}
		shown := strings.Contains(res.Stdout, "polix: Aviso")
		if res.Code != c.code || notified != c.notified || shown != (c.code == 0) {
			t.Errorf("block on %s: response %+v, %d notified", c.branch, res, notified)
		}
	}
}

// A commit the shell confirms through the port as one of another repository skips the entries of pre_commit; one
// it does not confirm meets them.
func TestCommitInAnotherRepository(t *testing.T) {
	yaml := "version: 1\non:\n  pre_commit:\n    - action: { block: \"no\" }\n"
	other := filepath.FromSlash("/other")
	for foreign, code := range map[bool]int{true: 0, false: 2} {
		port := &fakePort{yaml: yaml, foreign: map[string]bool{other: foreign}}
		res, err := run(context.Background(), port, claudeRequest(
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
func TestLocatesEdits(t *testing.T) {
	port := &fakePort{yaml: "version: 1\nguardrails:\n  protected_files: [\"src/**\"]\n"}
	patch := `*** Update File: src/a.go\n*** Update File: docs/b.md\n`
	res, err := run(context.Background(), port, Request{Agent: "codex", Version: "dev",
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
func TestGuidanceRendered(t *testing.T) {
	for _, agentName := range []string{"claude", "codex"} {
		port := &fakePort{yaml: "version: 1\n"}
		res, err := run(context.Background(), port, Request{Agent: agentName, Version: "dev",
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
		wf, _ := parseWorkflow("version: 1\n", "dev")
		if want := wf.SessionContext(); want == "" || out.HookSpecificOutput.AdditionalContext != want {
			t.Fatalf("%s: additionalContext %q", agentName, out.HookSpecificOutput.AdditionalContext)
		}
	}
}

// Without events the call is not relevant, and outside a repository there is nothing to govern: both let
// everything through; an input that is not JSON is an error.
func TestNothingToGovern(t *testing.T) {
	port := &fakePort{}
	res, err := run(context.Background(), port, claudeRequest(`{"hook_event_name":"PreToolUse","tool_name":"Read"}`))
	if err != nil || res != (Response{}) || len(port.calls) != 0 {
		t.Fatalf("no events: %+v %v %q", res, err, port.calls)
	}
	port = &fakePort{yaml: "version: 1\non:\n  stop:\n    - action: { block: x }\n",
		openErr: errors.New("not a repo")}
	res, err = run(context.Background(), port, claudeRequest(`{"hook_event_name":"Stop","cwd":"/tmp/x"}`))
	if err != nil || res != (Response{}) || !slices.Equal(port.calls, []string{"Open /tmp/x"}) {
		t.Fatalf("outside a repository: %+v %v %q", res, err, port.calls)
	}
	if _, err := run(context.Background(), port, claudeRequest(`not json`)); err == nil ||
		!strings.Contains(err.Error(), "claude hook input") {
		t.Fatalf("an input that is not JSON: %v", err)
	}
	if _, err := run(context.Background(), port, Request{Agent: "other"}); err == nil {
		t.Fatal("an unknown agent is an error")
	}
}

// The dir of the request wins over the cwd of the call.
func TestDir(t *testing.T) {
	port := &fakePort{}
	req := claudeRequest(`{"hook_event_name":"Stop","cwd":"/tmp/x"}`)
	req.Dir = "/repo"
	if _, err := run(context.Background(), port, req); err != nil || port.calls[0] != "Open /repo" {
		t.Fatalf("%v %q", err, port.calls)
	}
}

// Each error of the port that a step meets is data of the answer, as the failure policy says.
func TestPortErrors(t *testing.T) {
	stop := `{"hook_event_name":"Stop","cwd":"/repo"}`
	for _, c := range []struct {
		name  string
		port  *fakePort
		code  int
		wants string
	}{
		{"hash", &fakePort{yaml: "version: 1\non:\n  stop:\n    - unless: { finished: polix:reviewer }\n" +
			"      action: { run: ls }\n", hashErr: errors.New("boom")},
			0, "polix: polix cannot hash the content (boom)"},
		{"rules", &fakePort{yaml: "version: 1\non:\n  stop:\n    - action: { run: { rules: changed } }\n",
			rulesErr: errors.New("boom")}, 2, "The rules cannot be loaded: boom"},
		{"state", &fakePort{yaml: "version: 1\non:\n  stop:\n    - action: { block: x }\n",
			stateErr: errors.New("boom")}, 0, "cannot use its log (boom): x"},
		{"append", &fakePort{yaml: "version: 1\non:\n  stop:\n    - action: { block: x }\n",
			appendErr: errors.New("boom")}, 0, "cannot use its log (boom): x"},
		{"run", &fakePort{yaml: "version: 1\non:\n  stop:\n    - action: { run: [just, test] }\n",
			runErr: errors.New("exit status 1")}, 2, "just test failed: exit status 1"},
		{"check", &fakePort{yaml: "version: 1\non:\n  stop:\n    - action: { run: { rules: changed } }\n",
			rules:  []rule.Rule{{ID: "R-1", Description: "d", Deterministic: true, Check: "c.sh"}},
			runErr: errors.New("exit status 1")}, 2, "- R-1 (c.sh): d"},
	} {
		res, err := run(context.Background(), c.port, claudeRequest(stop))
		said := res.Stderr
		if c.code == 0 {
			said = systemMessage(t, res.Stdout)
		}
		if err != nil || res.Code != c.code || !strings.Contains(said, c.wants) {
			t.Errorf("%s: %+v %v", c.name, res, err)
		}
	}
	// A file that cannot be read gives no context.
	port := &fakePort{yaml: "version: 1\non:\n  prompt:\n    - action: { inject: { file: notes.md } }\n",
		readErr: errors.New("boom")}
	res, err := run(context.Background(), port, claudeRequest(`{"hook_event_name":"UserPromptSubmit","cwd":"/repo"}`))
	if err != nil || res.Stdout != "" || !slices.Contains(port.calls, "Read notes.md") {
		t.Fatalf("unreadable context: %+v %v %q", res, err, port.calls)
	}
	port.readErr = nil
	res, _ = run(context.Background(), port, claudeRequest(`{"hook_event_name":"UserPromptSubmit","cwd":"/repo"}`))
	if !strings.Contains(res.Stdout, `"additionalContext":"Ticket: PLAT-1"`) {
		t.Fatalf("context: %+v", res)
	}
}

// The shell looks the programs of requirements up through the port on a prompt and gives the AI the context of
// the missing ones, without blocking; a start looks nothing up.
func TestRequirementsMissing(t *testing.T) {
	port := &fakePort{yaml: "version: 1\nrequirements: [go, golangci-lint]\n",
		absent: map[string]bool{"golangci-lint": true}}
	res, err := run(context.Background(), port, claudeRequest(`{"hook_event_name":"UserPromptSubmit","cwd":"/repo"}`))
	if err != nil || res.Code != 0 || !slices.Contains(port.calls, "Missing go golangci-lint git") ||
		!strings.Contains(res.Stdout, "are not on the PATH: golangci-lint. Before doing anything else") {
		t.Fatalf("prompt: %+v %v %q", res, err, port.calls)
	}
	port.calls = nil
	res, err = run(context.Background(), port, claudeRequest(`{"hook_event_name":"SessionStart","cwd":"/repo"}`))
	if err != nil || res.Code != 0 || strings.Contains(res.Stdout, "not on the PATH") ||
		slices.ContainsFunc(port.calls, func(c string) bool { return strings.HasPrefix(c, "Missing") }) {
		t.Fatalf("start: %+v %v %q", res, err, port.calls)
	}
}

// A batch runner checks a passing group of rules with one call of the port.
func TestRunsAPassingGroupOnce(t *testing.T) {
	port := &fakePort{yaml: "version: 1\nrules:\n  run:\n    \"*_test.go\": [go, test, \"{dir}\", -run, " +
		"\"^Test({names})$\"]\non:\n  stop:\n    - action: { run: { rules: changed } }\n",
		rules: []rule.Rule{
			{ID: "A-1", Description: "d", Deterministic: true, Check: "t/a1_test.go"},
			{ID: "A-2", Description: "d", Deterministic: true, Check: "t/a2_test.go"},
		}}
	res, err := run(context.Background(), port, claudeRequest(`{"hook_event_name":"Stop","cwd":"/repo"}`))
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
func TestUnusableWorkflow(t *testing.T) {
	port := &fakePort{yaml: "version: 2\n"}
	res, err := run(context.Background(), port, claudeRequest(
		`{"hook_event_name":"PreToolUse","tool_name":"Bash","cwd":"/repo","tool_input":{"command":"ls"}}`))
	if err != nil || res.Code != 2 || !strings.Contains(res.Stderr, "do not remove them: run polix update") {
		t.Fatalf("command: %+v %v", res, err)
	}
	res, err = run(context.Background(), port, claudeRequest(`{"hook_event_name":"Stop","cwd":"/repo"}`))
	if err != nil || res.Code != 0 || res.Stderr != "" ||
		!strings.HasPrefix(systemMessage(t, res.Stdout), "polix: the workflow is not valid") {
		t.Fatalf("stop: %+v %v", res, err)
	}
	// A workflow of the old vocabulary is told its new spelling, never to update polix to keep it.
	port = &fakePort{yaml: "version: 1\non:\n  turn_stop:\n    - require: x\n"}
	res, err = run(context.Background(), port, claudeRequest(
		`{"hook_event_name":"PreToolUse","tool_name":"Bash","cwd":"/repo","tool_input":{"command":"ls"}}`))
	if err != nil || res.Code != 2 || !strings.Contains(res.Stderr, `unknown event "turn_stop": use stop`) ||
		!strings.Contains(res.Stderr, `unknown key "require": use action: { block: <message> }`) ||
		strings.Contains(res.Stderr, "do not remove them") {
		t.Fatalf("old vocabulary: %+v %v", res, err)
	}
}

// The shell answers the comparison of trees through the port: a review of the Go code holds after an edit of the
// docs, and not after an edit of a Go file nor when the trees cannot be compared.
func TestComparesTrees(t *testing.T) {
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
		port := &fakePort{yaml: yaml, state: reviewed, trees: c.trees, treeErr: c.err}
		res, err := run(context.Background(), port, claudeRequest(commit))
		if err != nil || res.Code != c.code || !slices.Contains(port.calls, "TreeDiff old h") {
			t.Errorf("%s: %+v %v %q", name, res, err, port.calls)
		}
	}
}

// snapshotState is a log with the snapshot of the command of the call toolUse, taken on the tree old and the commit
// HEAD of the fake points to.
func snapshotState(toolUse string) state.State {
	return state.State{Entries: []state.Entry{{Kind: state.KindObserved, Outcome: state.OutcomeSnapshot,
		Agent: agent.Claude, Session: "s1", Event: workflow.PreCommand, Actor: agent.ActorSession, Branch: "main",
		Key: toolUse, Hash: "old", Head: "c0ffee"}}}
}

// postCommand is the input of Claude Code after the command line ran, in the call toolUse of the session s1.
func postCommand(line, toolUse string) string {
	return `{"hook_event_name":"PostToolUse","tool_name":"Bash","cwd":"/repo","session_id":"s1","tool_use_id":"` +
		toolUse + `","tool_input":{"command":"` + line + `"},"tool_response":{"stdout":"ok"}}`
}

// The edits a command made, which the output of its post_command follows with, are evaluated in the same call, each
// located through the port, and their contexts merge into the answer of the call.
func TestEvaluatesFollowedEvents(t *testing.T) {
	port := &fakePort{yaml: "version: 1\non:\n  pre_edit:\n    - action: { inject: { text: \"reglas de {path}\", " +
		"always: true } }\n  post_edit:\n    - action: { inject: { text: \"editado {path}\", always: true } }\n",
		state: snapshotState("toolu_1"), trees: []string{"src/a.go", "docs/b.md"}}
	res, err := run(context.Background(), port, claudeRequest(postCommand("python3 gen.py", "toolu_1")))
	if err != nil || res.Code != 0 {
		t.Fatalf("response %+v %v", res, err)
	}
	var out struct {
		HookSpecificOutput struct {
			AdditionalContext string `json:"additionalContext"`
		} `json:"hookSpecificOutput"`
	}
	if err := json.Unmarshal([]byte(res.Stdout), &out); err != nil {
		t.Fatalf("stdout %q: %v", res.Stdout, err)
	}
	want := "reglas de src/a.go\n\neditado src/a.go\n\nreglas de docs/b.md\n\neditado docs/b.md"
	if out.HookSpecificOutput.AdditionalContext != want {
		t.Fatalf("context %q", out.HookSpecificOutput.AdditionalContext)
	}
	if !slices.Contains(port.calls, "TreeDiff old h") || !slices.Contains(port.calls, "Locate /repo/src/a.go") ||
		!slices.Contains(port.calls, "Locate /repo/docs/b.md") {
		t.Fatalf("calls %q", port.calls)
	}
}

// A Restore step reaches the port with the tree of the snapshot and the protected paths, and its error reaches the
// logic, which refuses with it and warns the user.
func TestCarriesTheRestore(t *testing.T) {
	yaml := "version: 1\nguardrails:\n  protected_files: [AGENTS.md]\n"
	port := &fakePort{yaml: yaml, state: snapshotState("toolu_1"), trees: []string{"AGENTS.md"}}
	res, err := run(context.Background(), port, claudeRequest(postCommand("python3 x.py", "toolu_1")))
	if err != nil || res.Code != 2 || !slices.Contains(port.calls, "Restore old AGENTS.md") ||
		!strings.Contains(res.Stderr, "AGENTS.md is protected in polix.yaml: a command changed it and polix put it back.") {
		t.Fatalf("restored: %+v %v %q", res, err, port.calls)
	}

	port = &fakePort{yaml: yaml, state: snapshotState("toolu_1"), trees: []string{"AGENTS.md"},
		restErr: errors.New("boom")}
	res, err = run(context.Background(), port, claudeRequest(postCommand("python3 x.py", "toolu_1")))
	if err != nil || res.Code != 2 || !strings.Contains(res.Stderr, "could not put it back (boom)") {
		t.Fatalf("failed restore: %+v %v", res, err)
	}

	ev := act(context.Background(), port, hook.Evaluation{}, hook.Step{Restore: []string{"AGENTS.md"}, Source: "old"})
	if len(ev.Journal) != 1 || ev.Journal[0].Err == nil || ev.Journal[0].Err.Error() != "boom" {
		t.Fatalf("journal %+v", ev.Journal)
	}
}

// The shell answers the Branch step through the port with the branch and the HEAD commit, learned together.
func TestCarriesTheHead(t *testing.T) {
	port := &fakePort{}
	ev := carry(context.Background(), port, hook.Evaluation{}, hook.Step{Branch: true})
	f := ev.Facts
	if f.Branch != "main" || f.Head != "c0ffee" || !f.HasBranch || !slices.Equal(port.calls, []string{"Head"}) {
		t.Fatalf("carried: %+v %q", f, port.calls)
	}
}

// The shell answers the Diff step through the port; its error makes the measures hold, with a warning.
func TestMeasuresTheDiff(t *testing.T) {
	yaml := "version: 1\non:\n  pre_commit:\n    - when: { changed: \"*.go\", changed_lines: 150 }\n" +
		"      action: { block: \"{changed_lines} lineas\" }\n"
	commit := `{"hook_event_name":"PreToolUse","tool_name":"Bash","cwd":"/repo","tool_input":{"command":"git commit -m x"}}`
	port := &fakePort{yaml: yaml}
	res, err := run(context.Background(), port, claudeRequest(commit))
	if err != nil || res.Code != 2 || !strings.Contains(res.Stderr, "210 lineas") || !slices.Contains(port.calls, "Diff") {
		t.Fatalf("measured: %+v %v %v", res, err, port.calls)
	}
	port = &fakePort{yaml: yaml, diffErr: errors.New("git roto")}
	res, err = run(context.Background(), port, claudeRequest(commit))
	if err != nil || res.Code != 2 || !strings.Contains(res.Stderr, "? lineas") {
		t.Fatalf("not measured: %+v %v", res, err)
	}
}

// Parse and render go through the format of the agent: PostToolUseFailure gives context to Claude Code only.
func TestFormats(t *testing.T) {
	input := []byte(`{"hook_event_name":"PostToolUse","tool_name":"Bash","tool_use_id":"toolu_1","tool_input":{"command":"ls"}}`)
	for name, toolUse := range map[string]string{agent.Claude: "toolu_1", agent.Codex: ""} {
		native, evs, err := formats[name].parse(input)
		if err != nil || native != "PostToolUse" || len(evs) != 1 || evs[0].Agent != name || evs[0].ToolUse != toolUse {
			t.Errorf("%s: %q %+v %v", name, native, evs, err)
		}
	}
	ans := agent.Answer{}.Inform("x")
	if out, _, _ := formats[agent.Claude].render("PostToolUseFailure", ans); out == "" {
		t.Error("Claude Code reads context on PostToolUseFailure")
	}
	if out, _, _ := formats[agent.Codex].render("PostToolUseFailure", ans); out != "" {
		t.Error("Codex does not read context on PostToolUseFailure")
	}
}

// A block goes to stderr with exit 2, its contexts after the reasons; otherwise the messages for the user go to the
// systemMessage and the contexts to the JSON of the events that take it.
func TestContextEventsRender(t *testing.T) {
	stdout, stderr, code := claudeContextEvents.render("PreToolUse", agent.Answer{}.Refuse("a").Refuse("b").Inform("c"))
	if code != agent.ExitBlock || stdout != "" || stderr != "a\n\nb\n\nc\n" {
		t.Fatalf("block: %q %q %d", stdout, stderr, code)
	}
	stdout, stderr, code = claudeContextEvents.render("PreToolUse", agent.Answer{}.Inform("x<y").Inform("z").Warn("w"))
	want := `{"systemMessage":"polix: w","hookSpecificOutput":{"additionalContext":"x\u003cy\n\nz",` +
		`"hookEventName":"PreToolUse"}}` + "\n"
	if code != agent.ExitOK || stdout != want || stderr != "" {
		t.Fatalf("context: %q %q %d", stdout, stderr, code)
	}
	if stdout, _, _ := claudeContextEvents.render("Stop", agent.Answer{}.Inform("x")); stdout != "" {
		t.Fatalf("Stop takes no context: %q", stdout)
	}
}

// A commit is recognised after global options of git and inside a quoted command, so pre_commit cannot be
// skipped by spelling it differently.
func TestGitCommit(t *testing.T) {
	cases := []struct {
		command string
		want    bool
	}{
		{"git commit -m x", true},
		{"cd x && git commit -m x", true},
		{"git -C . commit -m x", true},
		{"git -C ../repo commit", true},
		{"git -c user.name=a commit -m x", true},
		{"git --no-pager commit", true},
		{"git --git-dir .git --work-tree . commit", true},
		{"git --git-dir=.git commit", true},
		{`bash -c "git commit -m x"`, true},
		{"sh -c 'git commit'", true},
		{"git commit-tree HEAD^{tree}", false},
		{"git log --grep commit", false},
		{"git status", false},
	}
	for _, c := range cases {
		if got := gitCommit.MatchString(c.command); got != c.want {
			t.Errorf("gitCommit(%q) = %v, want %v", c.command, got, c.want)
		}
	}
}

// AC7: a pre_command gives a pre_commit when a command of the line, nested included, is normalized to git commit;
// a line the parser rejects is decided by gitCommit.
func TestCommitRecognised(t *testing.T) {
	for command, want := range map[string]bool{
		"git commit -m x": true, "cd x && git commit -m x": true, "git -C ../repo commit": true,
		"git -c user.name=a commit -m x": true, "git --git-dir .git --work-tree . commit": true,
		`bash -c "git commit -m x"`: true, "sh -c 'git commit'": true, "sudo git commit": true,
		"sudo bash -c 'git commit -m x'": true, `env bash -c "git commit"`: true, "time bash -c 'git commit'": true,
		"bash <<EOF\ngit commit -m x\nEOF": true, "cat <<EOF\ngit commit -m x\nEOF": false,
		"bash <<EOF\ngit commit -m \"$MSG\"\nEOF": true, "cat <<'EOF' | bash\ngit commit -m x\nEOF": true,
		`echo "git commit"`: false, "git commit-tree HEAD^{tree}": false, "git log --grep commit": false,
		"git status": false, `git log -S "git commit"`: false,
		`git commit -m "unclosed`: true, `echo "unclosed git commit`: true,
	} {
		evs, err := hookInput{Cwd: "/r"}.commands(agent.Claude, workflow.PreCommand, command, false)
		if err != nil {
			t.Fatal(err)
		}
		if got := len(evs) == 2 && evs[1].Name == workflow.PreCommit; got != want {
			t.Errorf("commands(%q) gives pre_commit %v, want %v", command, got, want)
		}
	}
}

// A relative path (apply_patch) is made absolute from the cwd of the call, so the protections that compare
// absolute paths see it.
func TestEditsRelativePath(t *testing.T) {
	cwd := filepath.Join(t.TempDir(), "repo")
	abs := filepath.Join(t.TempDir(), "file")
	evs, err := hookInput{Cwd: cwd}.edits(agent.Codex, workflow.PreEdit, []string{".git/polix/log.jsonl", abs})
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(cwd, ".git", "polix", "log.jsonl"); evs[0].AbsPath != want {
		t.Errorf("relative path: AbsPath = %q, want %q", evs[0].AbsPath, want)
	}
	if evs[1].AbsPath != abs {
		t.Errorf("absolute path: AbsPath = %q", evs[1].AbsPath)
	}
	if got, _ := (hookInput{}).edits(agent.Codex, workflow.PreEdit, []string{"a.go"}); got[0].AbsPath != "a.go" {
		t.Errorf("without a cwd the path stays as given: %q", got[0].AbsPath)
	}
}

// A command about to run that commits is also a pre_commit; after it ran with exit code 0 it is also a post_commit.
func TestCommands(t *testing.T) {
	command := "git commit -m x"
	evs, err := hookInput{}.commands(agent.Claude, workflow.PreCommand, command, false)
	if err != nil || len(evs) != 2 || evs[1].Name != workflow.PreCommit || evs[1].Command != command {
		t.Fatalf("pre_command: %+v %v", evs, err)
	}
	evs, err = hookInput{}.commands(agent.Claude, workflow.PostCommand, command, false)
	if err != nil || len(evs) != 2 || evs[0].Name != workflow.PostCommand || evs[1].Name != workflow.PostCommit ||
		evs[1].Command != command || evs[1].ExitCode != 0 {
		t.Fatalf("post_command of a commit: %+v %v", evs, err)
	}
	if evs, err := (hookInput{}).commands(agent.Claude, workflow.PostCommand, "go test ./...", false); err != nil ||
		len(evs) != 1 {
		t.Fatalf("post_command: %+v %v", evs, err)
	}
}

// post_commit follows the post_command of a successful commit line: the exit code of the line is 0 and a git commit
// decides it. A failed line, a commit that does not propagate its code, and a text that only mentions git commit give
// none; nested bash -c and a line the parser rejects are read as pre_commit reads them.
func TestPostCommitDerived(t *testing.T) {
	ok, failed := json.RawMessage(`{"stdout":"ok"}`), json.RawMessage(`{"exit_code":1}`)
	for _, c := range []struct {
		command  string
		response json.RawMessage
		failed   bool
		want     bool
	}{
		{`git commit -m "feat: x"`, ok, false, true},
		{`git commit -m "feat: x" && git push`, ok, false, true},
		{`git add -A && git commit -m x`, ok, false, true},
		{`bash -c "git commit -m x"`, ok, false, true},
		{`git commit -m "unclosed`, ok, false, true},
		{`git commit -m "feat: x"`, nil, true, false},
		{`git commit -m "feat: x"`, failed, false, false},
		{`git commit -m "feat: x"; echo done`, ok, false, false},
		{`git commit -m x || true`, ok, false, false},
		{`git commit -m x | tee out`, ok, false, false},
		{`bash -c "git commit -m x; echo done"`, ok, false, false},
		{`echo "git commit"`, ok, false, false},
		{`git status`, ok, false, false},
	} {
		evs, err := hookInput{ToolResponse: c.response}.commands(agent.Claude, workflow.PostCommand, c.command,
			c.failed)
		if err != nil {
			t.Fatal(err)
		}
		if got := len(evs) == 2 && evs[1].Name == workflow.PostCommit; got != c.want {
			t.Errorf("commands(%q, %s) gives post_commit %v, want %v", c.command, c.response, got, c.want)
		}
		if evs[0].Name != workflow.PostCommand {
			t.Errorf("commands(%q): the first event is %s", c.command, evs[0].Name)
		}
	}
}

// The exit code of a command is read from any of the names the agents use, 0 when it says none.
func TestExitCode(t *testing.T) {
	for raw, want := range map[string]int{`{"exit_code":3}`: 3, `{"exitCode":4}`: 4, `{"returncode":5}`: 5,
		`{"stdout":"ok"}`: 0, `not json`: 0} {
		if got := (hookInput{ToolResponse: json.RawMessage(raw)}).exitCode(); got != want {
			t.Errorf("exitCode(%s) = %d, want %d", raw, got, want)
		}
	}
}
