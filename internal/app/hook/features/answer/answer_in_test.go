package answer

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/mlophez/polix-ai/internal/app/agent"
	"github.com/mlophez/polix-ai/internal/app/diff"
	"github.com/mlophez/polix-ai/internal/app/hook"
	"github.com/mlophez/polix-ai/internal/app/rule"
	"github.com/mlophez/polix-ai/internal/app/state"
	"github.com/mlophez/polix-ai/internal/app/workflow"
	"github.com/mlophez/polix-ai/internal/platform/glob"
	"github.com/mlophez/polix-ai/internal/platform/testenv"
)

// fakePort is a fake of Port: a repository at /repo with the workflow of its yaml, the errors
// it is told to give, and the calls it got.
type fakePort struct {
	yaml      string // polix.yaml; empty for none
	openErr   error
	root      string // the root Open gives, as git writes it; /repo when empty
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
	appended  []state.Record
	clock     func() time.Time // the time Now gives; the time of the machine when nil
	delay     time.Duration    // how long Run and RunCheck take
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
	if p.root != "" {
		return p.root, filepath.Join(p.root, ".git"), nil
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

func (p *fakePort) Exists(_ context.Context, patterns []string) bool {
	return glob.AnyPath(patterns, "ai/plans/p.md")
}

func (p *fakePort) LoadRules(context.Context, string) ([]rule.Rule, error) {
	return p.rules, p.rulesErr
}

// LoadState gives the state of the fake for any session, and, as the adapter, the error of an empty session.
func (p *fakePort) LoadState(_ context.Context, session state.Session) (state.State, error) {
	p.calls = append(p.calls, "LoadState "+string(session))
	if _, err := session.File(); err != nil {
		return state.State{}, err
	}
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
	time.Sleep(p.delay)
	return "salida", p.runErr
}

func (p *fakePort) RunCheck(_ context.Context, run rule.Run) (string, error) {
	var ids []string
	for _, r := range run.Rules {
		ids = append(ids, r.ID)
	}
	p.calls = append(p.calls, "RunCheck "+strings.Join(ids, " "))
	time.Sleep(p.delay)
	return "salida", p.runErr
}

func (p *fakePort) Restore(_ context.Context, source string, paths []string) error {
	p.calls = append(p.calls, "Restore "+source+" "+strings.Join(paths, " "))
	return p.restErr
}

// Append appends r unless it is not valid, as the adapter, or the fake is told to fail.
func (p *fakePort) Append(_ context.Context, r state.Record) error {
	p.calls = append(p.calls, "Append "+r.Event)
	if err := r.Valid(); err != nil {
		return err
	}
	if p.appendErr != nil {
		return p.appendErr
	}
	p.appended = append(p.appended, r)
	return nil
}

// Now gives the time of the clock of the fake, or the time of the machine without one.
func (p *fakePort) Now(context.Context) string {
	if p.clock == nil {
		return time.Now().Format(time.RFC3339)
	}
	return p.clock().Format(time.RFC3339)
}

// entries returns the entries of the records the fake appended, in order.
func (p *fakePort) entries() []entry {
	var out []entry
	for _, r := range p.appended {
		out = append(out, entriesOf(r)...)
	}
	return out
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
		`{"hook_event_name":"PreToolUse","tool_name":"Bash","cwd":"/repo","session_id":"s1",`+
			`"tool_input":{"command":"git commit -m x"}}`))
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
	const input = `{"hook_event_name":"PreToolUse","tool_name":"Bash","cwd":"/repo","session_id":"s1",` +
		`"tool_input":{"command":"git commit -m x"}}`
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
		for _, e := range port.entries() {
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

// CUJ-01, AC-01: git gives the root with forward slashes, even on Windows; {root} reaches the message in the native,
// cleaned form of the system.
func TestRootNativeForm(t *testing.T) {
	const given = "/my repo/./sub//"
	port := &fakePort{root: given, yaml: "version: 1\non:\n  stop:\n    - action: { notify: \"[{root}]\" }\n"}
	res, err := run(context.Background(), port, claudeRequest(`{"hook_event_name":"Stop","cwd":"/my repo"}`))
	if err != nil {
		t.Fatal(err)
	}
	if want := "[" + filepath.Clean(filepath.FromSlash(given)) + "]"; !strings.Contains(systemMessage(t, res.Stdout), want) {
		t.Fatalf("want %q in %+v", want, res)
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

// CUJ-01, AC-07: the start context, with the root of the event and the folders below path, reaches the session of
// Claude Code and of Codex as the additionalContext of SessionStart.
func TestGuidanceRendered(t *testing.T) {
	const yaml = "version: 1\npath: docs/ai\n"
	for _, agentName := range []string{"claude", "codex"} {
		port := &fakePort{yaml: yaml}
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
		wf, _ := parseWorkflow(yaml, "dev")
		got := out.HookSpecificOutput.AdditionalContext
		if want := wf.SessionContext(filepath.FromSlash("/repo")); got != want ||
			!strings.Contains(got, "\n- The root of the project is /repo.\n") ||
			!strings.HasSuffix(got, "\n  - docs/ai/tasks/: the backlog of tasks, one per file.") {
			t.Fatalf("%s: additionalContext %q", agentName, got)
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

// AC5: the shell reads the state of the session of the event. An event without a session reads no state, even one
// that would spare its verbs: the run acts, the block with once blocks, the hook warns as when the log cannot be
// read, and nothing is appended.
func TestStateOfTheSession(t *testing.T) {
	const yaml = "version: 1\non:\n  pre_command:\n    - action: { run: [just, test] }\n" +
		"    - action: { block: { message: x, once: true } }\n"
	st := stateOf(
		entry{Kind: kindObserved, Session: "s1", Actor: agent.ActorSession, Branch: "main", Head: "c0ffee", Hash: "h",
			Key: "just test", Line: 4, Outcome: state.OutcomePassed},
		entry{Kind: kindObserved, Session: "s1", Actor: agent.ActorSession, Branch: "main", Head: "c0ffee", Hash: "h",
			Line: 5, Outcome: state.OutcomeBlocked},
	)
	const command = `"hook_event_name":"PreToolUse","tool_name":"Bash","cwd":"/repo","tool_input":{"command":"ls"}`
	port := &fakePort{yaml: yaml, state: st}
	res, err := run(context.Background(), port, claudeRequest(`{`+command+`,"session_id":"s1"}`))
	if err != nil || res.Code != 0 || !slices.Contains(port.calls, "LoadState s1") ||
		slices.Contains(port.calls, "Run just test") {
		t.Fatalf("s1: %+v %v %q", res, err, port.calls)
	}
	port = &fakePort{yaml: yaml, state: st}
	res, err = run(context.Background(), port, claudeRequest(`{`+command+`}`))
	if err != nil || res.Code != 2 || !slices.Contains(port.calls, "LoadState ") ||
		!slices.Contains(port.calls, "Run just test") || len(port.appended) != 0 {
		t.Fatalf("no session: %+v %v %q %+v", res, err, port.calls, port.appended)
	}
	// A block shows the user no message (rule 15), so the warning is seen on a call that does not block.
	port = &fakePort{yaml: "version: 1\non:\n  pre_command:\n    - action: { run: [just, test] }\n", state: st}
	res, err = run(context.Background(), port, claudeRequest(`{`+command+`}`))
	if err != nil || res.Code != 0 || !slices.Contains(port.calls, "Run just test") || len(port.appended) != 0 ||
		!strings.Contains(systemMessage(t, res.Stdout), "polix cannot use its log (a session needs an id") {
		t.Fatalf("no session warns: %+v %v %q %+v", res, err, port.calls, port.appended)
	}
	// An event that never reads the state warns too: the entries it would write are rejected.
	port = &fakePort{yaml: "version: 1\non:\n  pre_command:\n    - action: { run: { command: [just, test], " +
		"always: true } }\n", state: st}
	res, err = run(context.Background(), port, claudeRequest(`{`+command+`}`))
	if err != nil || res.Code != 0 || slices.Contains(port.calls, "LoadState ") ||
		!slices.Contains(port.calls, "Run just test") || len(port.appended) != 0 ||
		strings.Count(systemMessage(t, res.Stdout), "polix cannot use its log (a session needs an id") != 1 {
		t.Fatalf("no session, no state, warns: %+v %v %q %+v", res, err, port.calls, port.appended)
	}
}

// Each error of the port that a step meets is data of the answer, as the failure policy says.
func TestPortErrors(t *testing.T) {
	stop := `{"hook_event_name":"Stop","cwd":"/repo","session_id":"s1"}`
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
	res, err := run(context.Background(), port, claudeRequest(
		`{"hook_event_name":"UserPromptSubmit","cwd":"/repo","session_id":"s1"}`))
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

// CUJ-07, AC-07: an event where nothing acts writes nothing: a prompt no entry matches asks no Append step and leaves
// the log as it was, and so does an event of a workflow without entries.
func TestNothingActsWritesNothing(t *testing.T) {
	port := &fakePort{yaml: "version: 1\non:\n  prompt:\n    - when: { branch: \"^never$\" }\n" +
		"      action: { inject: \"Remember\" }\n  pre_edit:\n    - action: { block: \"no\" }\n",
		state: stateOf(entry{Kind: kindObserved, Outcome: state.OutcomeStarted, Session: "s1", Event: workflow.Start,
			Actor: agent.ActorSession, Key: "startup"})}
	res, err := run(context.Background(), port, claudeRequest(promptCall(t, "hola")))
	if err != nil || res.Code != 0 || additionalContext(t, res.Stdout) != "" || len(port.appended) != 0 {
		t.Fatalf("a prompt nothing acts on: %+v %v %+v", res, err, port.appended)
	}
	if len(port.state.Records) != 1 {
		t.Fatalf("the log changed: %+v", port.state)
	}
	bare := &fakePort{yaml: "version: 1\n"}
	res, err = run(context.Background(), bare, claudeRequest(promptCall(t, "hola")))
	if err != nil || res.Code != 0 || len(bare.appended) != 0 {
		t.Fatalf("a workflow without entries: %+v %v %+v", res, err, bare.appended)
	}
}

// eventNames returns the event of each record, in order.
func eventNames(records []state.Record) []string {
	var out []string
	for _, r := range records {
		out = append(out, r.Event)
	}
	return out
}

// CUJ-18, AC-06, AC-19: the events of one call each get their own record, one Append per event that acted, in the
// order of the events and all of them at the end of the call: a command whose watch derives two edits of a.txt gives
// the records of the post_command and of each edit; a block of a later event of the call drops the notified
// observations of the earlier ones.
func TestOneRecordPerEventOfTheCall(t *testing.T) {
	const yaml = "version: 1\non:\n  post_command:\n    - action: { notify: \"Aviso\" }\n  pre_edit:\n" +
		"    - action: { block: \"No editar\" }\n  post_edit:\n    - action: { notify: \"Editado\" }\n"
	snapshot := stateOf(entry{Kind: kindObserved, Outcome: state.OutcomeSnapshot, Session: "s1",
		Event: workflow.PreCommand, Actor: agent.ActorSession, Branch: "main", Head: "c0ffee", Key: "toolu_1", Hash: "old"})
	port := &fakePort{yaml: yaml, state: snapshot, trees: []string{"a.txt"}}
	const post = `{"hook_event_name":"PostToolUse","tool_name":"Bash","cwd":"/repo","session_id":"s1",` +
		`"tool_use_id":"toolu_1","tool_input":{"command":"echo x > a.txt"},"tool_response":{}}`
	res, err := run(context.Background(), port, claudeRequest(post))
	if err != nil || res.Code != 0 {
		t.Fatalf("the call: %+v %v", res, err)
	}
	if got, want := eventNames(port.appended), []string{"post_command", "pre_edit", "post_edit"}; !slices.Equal(got, want) {
		t.Fatalf("records %v, want %v", got, want)
	}
	last := -1
	for i, c := range port.calls {
		if !strings.HasPrefix(c, "Append") {
			last = i
		}
	}
	if appends := port.calls[last+1:]; len(appends) != 3 {
		t.Fatalf("every record is appended after the last event: %v", port.calls)
	}
	post0, edit := port.appended[0], port.appended[1]
	if len(post0.Observations) != 2 || post0.Observations[0].Outcome != state.OutcomeWatched ||
		post0.Observations[1].Outcome != state.OutcomeNotified || post0.Observations[1].Key != "Aviso" ||
		len(edit.Actions) != 1 || edit.Actions[0].Outcome != "inform" || edit.Input.Path != "a.txt" ||
		edit.Actor != agent.ActorSession || len(edit.Observations) != 0 {
		t.Fatalf("the records: %+v %+v", post0, edit)
	}

	// A commit whose pre_commit blocks: the notice of the pre_command is not shown, so it is never noted.
	blocked := &fakePort{yaml: "version: 1\non:\n  pre_command:\n    - action: { notify: \"Aviso\" }\n  pre_commit:\n" +
		"    - action: { block: \"no\" }\n"}
	res, err = run(context.Background(), blocked, claudeRequest(
		`{"hook_event_name":"PreToolUse","tool_name":"Bash","cwd":"/repo","session_id":"s1",`+
			`"tool_input":{"command":"git commit -m x"}}`))
	if err != nil || res.Code != 2 {
		t.Fatalf("the blocked call: %+v %v", res, err)
	}
	if got, want := eventNames(blocked.appended), []string{"pre_command", "pre_commit"}; !slices.Equal(got, want) {
		t.Fatalf("records %v, want %v", got, want)
	}
	for _, r := range blocked.appended {
		if slices.ContainsFunc(r.Observations, func(o state.Observation) bool { return o.Outcome == state.OutcomeNotified }) {
			t.Fatalf("a notice of a blocked call is noted: %+v", r)
		}
	}
	if len(blocked.appended[0].Actions) != 1 || blocked.appended[0].Actions[0].Outcome != "notify" {
		t.Fatalf("the record of the pre_command keeps its action: %+v", blocked.appended[0])
	}
}

// CUJ-18, AC-19: each record carries the time the shell gave the evaluation of its event, not the time it is
// appended: two events of a call get the two times of the clock, in order.
func TestRecordAtIsStartOfEvent(t *testing.T) {
	port := &fakePort{yaml: "version: 1\non:\n  pre_command:\n    - action: { notify: \"Aviso\" }\n  pre_commit:\n" +
		"    - action: { block: \"no\" }\n"}
	tick := time.Date(2026, 11, 22, 13, 50, 0, 0, time.FixedZone("CET", 3600))
	req := claudeRequest(`{"hook_event_name":"PreToolUse","tool_name":"Bash","cwd":"/repo","session_id":"s1",` +
		`"tool_input":{"command":"git commit -m x"}}`)
	port.clock = func() time.Time {
		tick = tick.Add(5 * time.Second)
		return tick
	}
	if _, err := run(context.Background(), port, req); err != nil {
		t.Fatal(err)
	}
	if len(port.appended) != 2 || port.appended[0].At != "2026-11-22T13:50:05+01:00" ||
		port.appended[1].At != "2026-11-22T13:50:10+01:00" {
		t.Fatalf("records: %+v", port.appended)
	}
}

// CUJ-11, AC-11: a stop the loop guard counts is let through when its record cannot be appended at the end of the
// call, with the warning of a log polix cannot use, and never answered with a block the guard could not count.
func TestLoopGuardFailsOpenOnFinalWrite(t *testing.T) {
	port := &fakePort{yaml: "version: 1\non:\n  stop:\n    - action: { block: \"revisa\" }\n",
		appendErr: errors.New("permission denied")}
	res, err := run(context.Background(), port, claudeRequest(`{"hook_event_name":"Stop","cwd":"/repo","session_id":"s1"}`))
	if err != nil || res.Code != 0 || res.Stderr != "" ||
		!strings.Contains(systemMessage(t, res.Stdout), "cannot use its log (permission denied): revisa") {
		t.Fatalf("a stop whose record cannot be appended: %+v %v", res, err)
	}
	port.appendErr = nil
	res, err = run(context.Background(), port, claudeRequest(`{"hook_event_name":"Stop","cwd":"/repo","session_id":"s1"}`))
	if err != nil || res.Code != 2 || len(port.appended) != 1 {
		t.Fatalf("a stop whose record is appended: %+v %v %+v", res, err, port.appended)
	}
}

// CUJ-12, AC-12: a legacy line of a run that passed on the current content does not skip the run, since the memory of
// the session ignores legacy lines; the run runs and its record has the passed observation.
func TestLegacyLinesIgnoredByState(t *testing.T) {
	legacy := state.Record{At: "t0", Session: "s1", Event: workflow.PreCommit, Actor: agent.ActorSession, Branch: "main",
		Head: "c0ffee", Legacy: state.LegacyObserved, Observations: []state.Observation{{Line: 4,
			Outcome: state.OutcomePassed, Key: "just test", Hash: "h"}}}
	decision := state.Record{At: "t1", Session: "s1", Event: workflow.Stop, Actor: agent.ActorSession, Legacy: state.LegacyDecision,
		Actions: []state.Action{{Line: 5, Action: "block", Outcome: "block"}}}
	port := &fakePort{yaml: "version: 1\non:\n  pre_commit:\n    - action: { run: just test }\n",
		state: state.State{Records: []state.Record{legacy, decision}}}
	res, err := run(context.Background(), port, claudeRequest(
		`{"hook_event_name":"PreToolUse","tool_name":"Bash","cwd":"/repo","session_id":"s1",`+
			`"tool_input":{"command":"git commit -m x"}}`))
	if err != nil || res.Code != 0 || res.Stderr != "" || !slices.Contains(port.calls, "Run just test") {
		t.Fatalf("the run runs again: %+v %v %q", res, err, port.calls)
	}
	if len(port.appended) != 1 || port.appended[0].Event != workflow.PreCommit ||
		!slices.ContainsFunc(port.appended[0].Observations, func(o state.Observation) bool {
			return o.Outcome == state.OutcomePassed && o.Key == "just test" && o.Hash == "h"
		}) {
		t.Fatalf("the new record: %+v", port.appended)
	}
	// The same record as an event line is what the memory of the session reads: the run is skipped.
	event := legacy
	event.Legacy = ""
	again := &fakePort{yaml: port.yaml, state: state.State{Records: []state.Record{event}}}
	if _, err := run(context.Background(), again, claudeRequest(
		`{"hook_event_name":"PreToolUse","tool_name":"Bash","cwd":"/repo","session_id":"s1",`+
			`"tool_input":{"command":"git commit -m x"}}`)); err != nil || slices.Contains(again.calls, "Run just test") {
		t.Fatalf("the event line skips the run: %v %q", err, again.calls)
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
	commit := `{"hook_event_name":"PreToolUse","tool_name":"Bash","cwd":"/repo","session_id":"s1",` +
		`"tool_input":{"command":"git commit -m x"}}`
	reviewed := stateOf(entry{Kind: kindObserved, Outcome: state.OutcomeFinished,
		Session: "s1", Key: "polix:reviewer", Branch: "main", Hash: "old"})
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
	return stateOf(entry{Kind: kindObserved, Outcome: state.OutcomeSnapshot,
		Agent: agent.Claude, Session: "s1", Event: workflow.PreCommand, Actor: agent.ActorSession, Branch: "main",
		Key: toolUse, Hash: "old", Head: "c0ffee"})
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

// skillCall returns the input of a PreToolUse of the tool Skill of Claude Code in the session s1 with tool_input
// input, from the subagent of type actor, or from the main session when actor is empty.
func skillCall(input, actor string) string {
	by := ""
	if actor != "" {
		by = `"agent_id":"a1","agent_type":"` + actor + `",`
	}
	return `{"hook_event_name":"PreToolUse","tool_name":"Skill","cwd":"/repo","session_id":"s1",` + by +
		`"tool_input":` + input + `}`
}

// additionalContext returns the additionalContext of the JSON stdout of a hook, "" when stdout is empty or holds
// none.
func additionalContext(t *testing.T, stdout string) string {
	t.Helper()
	if stdout == "" {
		return ""
	}
	var out struct {
		HookSpecificOutput struct {
			AdditionalContext string `json:"additionalContext"`
		} `json:"hookSpecificOutput"`
	}
	if err := json.Unmarshal([]byte(stdout), &out); err != nil {
		t.Fatalf("stdout is not one JSON object: %q: %v", stdout, err)
	}
	return out.HookSpecificOutput.AdditionalContext
}

// skillInject is a polix.yaml whose entry of skill injects the name of the skill when the condition when holds.
func skillInject(when string) string {
	return "version: 1\non:\n  skill:\n    - when: " + when + "\n      action: { inject: \"Skill {skill} started.\" }\n"
}

// CUJ-01 and AC-01: the call of the tool Skill by the agent gives the agent the context of the entry of skill that
// names it, with {skill} expanded, on exit 0; a call of another skill gives none, and a call without a name or a
// PostToolUse of the tool Skill gives no event at all.
func TestSkillEventAgentCall(t *testing.T) {
	port := &fakePort{yaml: skillInject(`{ skill: "polix:spec" }`)}
	res, err := run(context.Background(), port, claudeRequest(skillCall(`{"skill":"polix:spec","args":"add a login"}`, "")))
	if err != nil || res.Code != 0 || additionalContext(t, res.Stdout) != "Skill polix:spec started." {
		t.Fatalf("polix:spec: %+v %v", res, err)
	}
	res, err = run(context.Background(), port, claudeRequest(skillCall(`{"skill":"polix:build"}`, "")))
	if err != nil || res.Code != 0 || res.Stdout != "" {
		t.Fatalf("polix:build: %+v %v", res, err)
	}
	for _, input := range []string{
		skillCall(`{}`, ""),
		skillCall(`{"skill":" "}`, ""),
		`{"hook_event_name":"PostToolUse","tool_name":"Skill","cwd":"/repo","session_id":"s1",` +
			`"tool_input":{"skill":"polix:spec"}}`,
	} {
		port := &fakePort{yaml: "version: 1\non:\n  skill:\n    - action: { inject: \"Skill {skill} started.\" }\n"}
		res, err := run(context.Background(), port, claudeRequest(input))
		if err != nil || res != (Response{}) || len(port.calls) != 0 {
			t.Errorf("%s: %+v %v %q", input, res, err, port.calls)
		}
	}
}

// CUJ-12 and AC-01: a subagent that calls the tool Skill is the actor of the event, so an entry for its type applies
// to its call and not to the same call of the main session.
func TestSkillEventActor(t *testing.T) {
	port := &fakePort{yaml: skillInject("{ actor: polix:builder, skill: spec }")}
	res, err := run(context.Background(), port, claudeRequest(skillCall(`{"skill":"polix:spec"}`, "polix:builder")))
	if err != nil || res.Code != 0 || additionalContext(t, res.Stdout) != "Skill polix:spec started." {
		t.Fatalf("polix:builder: %+v %v", res, err)
	}
	res, err = run(context.Background(), port, claudeRequest(skillCall(`{"skill":"polix:spec"}`, "")))
	if err != nil || res.Code != 0 || res.Stdout != "" {
		t.Fatalf("main session: %+v %v", res, err)
	}
}

// CUJ-03 and AC-03: through the tool Skill, the condition skill matches the name with or without its prefix, and any
// name of a list; never a name that only starts with it.
func TestSkillEventMatchesBareName(t *testing.T) {
	cases := []struct {
		when, skill, want string
	}{
		{"{ skill: spec }", "polix:spec", "Skill polix:spec started."},
		{`{ skill: "polix:spec" }`, "polix:spec", "Skill polix:spec started."},
		{"{ skill: [polix:build, spec] }", "polix:spec", "Skill polix:spec started."},
		{"{ skill: spec }", "polix:spectre", ""},
	}
	for _, c := range cases {
		port := &fakePort{yaml: skillInject(c.when)}
		res, err := run(context.Background(), port, claudeRequest(skillCall(`{"skill":"`+c.skill+`"}`, "")))
		if err != nil || res.Code != 0 || additionalContext(t, res.Stdout) != c.want {
			t.Errorf("%s on %s: %+v %v", c.when, c.skill, res, err)
		}
	}
}

// CUJ-05 and AC-04: a block of an entry of skill refuses the call of the tool Skill with exit 2 and its message on
// stderr, which the agent reads; the call of another skill goes through.
func TestSkillEventBlock(t *testing.T) {
	port := &fakePort{yaml: "version: 1\non:\n  skill:\n    - when: { skill: \"polix:spec\" }\n" +
		"      action: { block: \"Run polix:spec on a plan branch.\" }\n"}
	res, err := run(context.Background(), port, claudeRequest(skillCall(`{"skill":"polix:spec"}`, "")))
	if err != nil || res.Code != 2 || res.Stderr != "Run polix:spec on a plan branch.\n" || res.Stdout != "" {
		t.Fatalf("polix:spec: %+v %v", res, err)
	}
	res, err = run(context.Background(), port, claudeRequest(skillCall(`{"skill":"polix:build"}`, "")))
	if err != nil || res.Code != 0 || res.Stderr != "" {
		t.Fatalf("polix:build: %+v %v", res, err)
	}
}

// CUJ-06 and AC-05: every inject of skill, without always, acts on every start of the skill in the session, while the
// inject of prompt still acts once.
func TestSkillEventInjectsEveryTime(t *testing.T) {
	port := &fakePort{yaml: skillInject(`{ skill: "polix:spec" }`) +
		"  prompt:\n    - action: { inject: \"Prompt received.\" }\n"}
	for i := range 2 {
		port.state = state.State{Records: port.appended}
		res, err := run(context.Background(), port, claudeRequest(skillCall(`{"skill":"polix:spec"}`, "")))
		if err != nil || res.Code != 0 || additionalContext(t, res.Stdout) != "Skill polix:spec started." {
			t.Fatalf("call %d: %+v %v", i+1, res, err)
		}
	}
	var contexts []string
	for range 2 {
		port.state = state.State{Records: port.appended}
		res, err := run(context.Background(), port, claudeRequest(
			`{"hook_event_name":"UserPromptSubmit","cwd":"/repo","session_id":"s1","prompt":"hola"}`))
		if err != nil || res.Code != 0 {
			t.Fatalf("prompt: %+v %v", res, err)
		}
		contexts = append(contexts, additionalContext(t, res.Stdout))
	}
	if !slices.Equal(contexts, []string{"Prompt received.", ""}) {
		t.Fatalf("prompts: %q", contexts)
	}
}

// CUJ-11 and AC-06: an entry of skill runs its command with the name of the skill, and another notifies the user with
// {skill} expanded, in the systemMessage and never in the context. The run comes first, since a notice before an
// entry that may block its event is not valid.
func TestSkillEventNotifyAndRun(t *testing.T) {
	port := &fakePort{yaml: "version: 1\non:\n  skill:\n    - action: { run: \"echo {skill}\" }\n" +
		"    - action: { notify: \"Skill {skill} started.\" }\n"}
	res, err := run(context.Background(), port, claudeRequest(skillCall(`{"skill":"polix:spec"}`, "")))
	if err != nil || res.Code != 0 || systemMessage(t, res.Stdout) != "polix: Skill polix:spec started." ||
		strings.Contains(additionalContext(t, res.Stdout), "Skill polix:spec started.") {
		t.Fatalf("notify: %+v %v", res, err)
	}
	if !slices.ContainsFunc(port.calls, func(c string) bool {
		return strings.HasPrefix(c, "Run ") && strings.HasSuffix(c, "echo polix:spec")
	}) {
		t.Fatalf("run: %q", port.calls)
	}
}

// startedOf returns the keys of the observations started of the event skill that the records hold.
func startedOf(records []state.Record) []string {
	var keys []string
	for _, rec := range records {
		if rec.Event != workflow.Skill {
			continue
		}
		for _, o := range rec.Observations {
			if o.Outcome == state.OutcomeStarted {
				keys = append(keys, o.Key)
			}
		}
	}
	return keys
}

// CUJ-01, CUJ-02 and AC-01: every skill start, typed or invoked, leaves an observation started of the event skill
// keyed by its name, with no entry on skill, and its answer is unchanged.
func TestSkillStartRecorded(t *testing.T) {
	yaml := "version: 1\non:\n  pre_commit:\n    - action: { block: \"no\" }\n"
	for name, input := range map[string]string{
		"typed":   promptCall(t, "/polix:spec add a login"),
		"invoked": skillCall(`{"skill":"polix:spec"}`, ""),
	} {
		port := &fakePort{yaml: yaml}
		res, err := run(context.Background(), port, claudeRequest(input))
		if err != nil || res.Code != 0 || res.Stdout != "" || res.Stderr != "" {
			t.Fatalf("%s: %+v %v", name, res, err)
		}
		if got := startedOf(port.appended); !slices.Equal(got, []string{"polix:spec"}) {
			t.Errorf("%s: started %q in %+v", name, got, port.appended)
		}
	}
}

// CUJ-05 and AC-02: a skill start blocked by an entry of skill leaves no observation started.
func TestSkillStartBlockedNotRecorded(t *testing.T) {
	port := &fakePort{yaml: "version: 1\non:\n  skill:\n    - when: { skill: \"polix:build\" }\n" +
		"      action: { block: \"Do not build.\" }\n"}
	res, err := run(context.Background(), port, claudeRequest(skillCall(`{"skill":"polix:build"}`, "")))
	if err != nil || res.Code != 2 {
		t.Fatalf("blocked: %+v %v", res, err)
	}
	if got := startedOf(port.appended); len(got) != 0 {
		t.Fatalf("started %q after a block", got)
	}
}

// promptCall returns the input of a UserPromptSubmit of Claude Code in the session s1 with the prompt text.
func promptCall(t *testing.T, text string) string {
	t.Helper()
	raw, err := json.Marshal(map[string]string{"hook_event_name": "UserPromptSubmit", "cwd": "/repo",
		"session_id": "s1", "prompt": text})
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

// CUJ-02 and AC-02: a typed skill gives the context of its entry of skill once, and the text of the entry of prompt
// once, on exit 0.
func TestSkillEventTypedPrompt(t *testing.T) {
	port := &fakePort{yaml: skillInject(`{ skill: "polix:spec" }`) +
		"  prompt:\n    - action: { inject: \"Prompt received.\" }\n"}
	res, err := run(context.Background(), port, claudeRequest(promptCall(t, "/polix:spec add a login")))
	if err != nil || res.Code != 0 {
		t.Fatalf("typed: %+v %v", res, err)
	}
	got := additionalContext(t, res.Stdout)
	if strings.Count(got, "Skill polix:spec started.") != 1 || strings.Count(got, "Prompt received.") != 1 {
		t.Fatalf("context: %q", got)
	}
}

// CUJ-04 and AC-02: a prompt without the shape of a typed skill gives only the text of the entry of prompt; the
// prompt /tmp is full has the shape, so it also gives the context of the skill tmp, the known limit of the detection.
func TestSkillEventNotASkill(t *testing.T) {
	yaml := "version: 1\non:\n  skill:\n    - action: { inject: \"Skill {skill} started.\" }\n" +
		"  prompt:\n    - action: { inject: \"Prompt received.\" }\n"
	for _, prompt := range []string{"please run /polix:spec", "/tmp/foo is full", "/", " /polix:spec", "hola"} {
		port := &fakePort{yaml: yaml}
		res, err := run(context.Background(), port, claudeRequest(promptCall(t, prompt)))
		if err != nil || res.Code != 0 || additionalContext(t, res.Stdout) != "Prompt received." {
			t.Errorf("%q: %+v %v", prompt, res, err)
		}
	}
	port := &fakePort{yaml: yaml}
	res, err := run(context.Background(), port, claudeRequest(promptCall(t, "/tmp is full")))
	got := additionalContext(t, res.Stdout)
	if err != nil || res.Code != 0 || !strings.Contains(got, "Skill tmp started.") ||
		!strings.Contains(got, "Prompt received.") {
		t.Fatalf("/tmp is full: %+v %v", res, err)
	}
}

// CUJ-03 and AC-03: through a typed prompt, the condition skill matches the name as typed, and a name with a prefix
// does not match the bare name typed.
func TestSkillEventTypedMatchesBareName(t *testing.T) {
	port := &fakePort{yaml: skillInject("{ skill: spec }")}
	res, err := run(context.Background(), port, claudeRequest(promptCall(t, "/spec")))
	if err != nil || res.Code != 0 || additionalContext(t, res.Stdout) != "Skill spec started." {
		t.Fatalf("spec: %+v %v", res, err)
	}
	port = &fakePort{yaml: skillInject(`{ skill: "polix:spec" }`)}
	res, err = run(context.Background(), port, claudeRequest(promptCall(t, "/spec")))
	if err != nil || res.Code != 0 || res.Stdout != "" {
		t.Fatalf("polix:spec: %+v %v", res, err)
	}
}

// CUJ-05 and AC-04 and DEC-13: a block of skill rejects the typed prompt with exit 2 and its message on stderr, and
// another skill goes through; a block of prompt rejects it too, and the run of a skill entry still runs.
func TestSkillEventTypedBlock(t *testing.T) {
	port := &fakePort{yaml: "version: 1\non:\n  skill:\n    - when: { skill: \"polix:spec\" }\n" +
		"      action: { block: \"Run polix:spec on a plan branch.\" }\n"}
	res, err := run(context.Background(), port, claudeRequest(promptCall(t, "/polix:spec add a login")))
	if err != nil || res.Code != 2 || res.Stderr != "Run polix:spec on a plan branch.\n" || res.Stdout != "" {
		t.Fatalf("polix:spec: %+v %v", res, err)
	}
	res, err = run(context.Background(), port, claudeRequest(promptCall(t, "/polix:build add a login")))
	if err != nil || res.Code != 0 || res.Stderr != "" {
		t.Fatalf("polix:build: %+v %v", res, err)
	}
	port = &fakePort{yaml: "version: 1\non:\n  prompt:\n    - action: { block: \"No prompts.\" }\n" +
		"  skill:\n    - action: { run: \"echo {skill}\" }\n"}
	res, err = run(context.Background(), port, claudeRequest(promptCall(t, "/polix:spec add a login")))
	if err != nil || res.Code != 2 || !strings.Contains(res.Stderr, "No prompts.") {
		t.Fatalf("block on prompt: %+v %v", res, err)
	}
	if !slices.ContainsFunc(port.calls, func(c string) bool {
		return strings.HasPrefix(c, "Run ") && strings.HasSuffix(c, "echo polix:spec")
	}) {
		t.Fatalf("run: %q", port.calls)
	}
}

// The exit code of a command is read from any of the names the agents use, 0 when it says none.
func TestExitCode(t *testing.T) {
	for raw, want := range map[string]int{`{"exit_code":3}`: 3, `{"exitCode":4}`: 4, `{"returncode":5}`: 5,
		`{"stdout":"ok"}`: 0, `not json`: 0} {
		got, given := (hookInput{ToolResponse: json.RawMessage(raw)}).exitCode()
		if got != want || given != (want != 0) {
			t.Errorf("exitCode(%s) = %d, %v, want %d", raw, got, given, want)
		}
	}
	if _, given := (hookInput{ToolResponse: json.RawMessage(`{"exit_code":0}`)}).exitCode(); !given {
		t.Error("a zero exit code the agent gave is given")
	}
}

// CUJ-06, AC-06: a pre_commit with an inject that always applies, a run that passes and a block gets one event line,
// the one of LIT-05, with the inject injected, the run passed with its key and its duration, and the block.
func TestPreCommitEventLine(t *testing.T) {
	at := time.Date(2026, 11, 22, 13, 50, 5, 0, time.FixedZone("CET", 3600))
	port := &fakePort{yaml: "version: 1\non:\n  pre_commit:\n" +
		"    - action: { inject: { file: ai/rules/go.yaml, always: true } }\n" +
		"    - action: { run: just test }\n" +
		"    - action: { block: \"Subagents never commit.\" }\n", clock: func() time.Time { return at }}
	res, err := run(context.Background(), port, claudeRequest(
		`{"hook_event_name":"PreToolUse","tool_name":"Bash","cwd":"/repo","session_id":"s1","tool_use_id":"toolu_1",`+
			`"tool_input":{"command":"git commit -m \"wip\""}}`))
	if err != nil {
		t.Fatal(err)
	}
	if res.Code != 2 || !strings.Contains(res.Stderr, "Subagents never commit.") || len(port.appended) != 1 {
		t.Fatalf("response %+v, appended %+v", res, port.appended)
	}
	rec := port.appended[0]
	if len(rec.Actions) != 3 || rec.Actions[1].DurationMS == nil {
		t.Fatalf("actions %+v", rec.Actions)
	}
	want := state.Record{At: "2026-11-22T13:50:05+01:00", Agent: "claude", Session: "s1", Event: "pre_commit",
		Actor: "session", Branch: "main", Head: "c0ffee", Hash: "h",
		Input: state.Input{Command: `git commit -m "wip"`, ToolUse: "toolu_1"},
		Actions: []state.Action{
			{Line: 4, Action: "inject", Outcome: "injected", Key: "ai/rules/go.yaml"},
			{Line: 5, Action: "run", Outcome: "passed", Key: "just test", DurationMS: rec.Actions[1].DurationMS},
			{Line: 6, Action: "block", Outcome: "block", Reason: "Subagents never commit."}},
		Observations: []state.Observation{{Line: 5, Outcome: state.OutcomePassed, Key: "just test", Hash: "h"}}}
	if !reflect.DeepEqual(rec, want) {
		t.Fatalf("record\n got %+v\nwant %+v", rec, want)
	}
	// The adapter encodes this record as the event line of LIT-05 (TestAppendWritesOneEventLine).
}

// AC-09: the shell times each Run and Check step into the outcome of the evaluation, whatever the port gives back,
// and the duration of the action is its whole milliseconds, truncated.
func TestShellMeasuresRunDuration(t *testing.T) {
	const delay = 25 * time.Millisecond
	port := &fakePort{delay: delay}
	ev := hook.Evaluation{}
	for _, s := range []hook.Step{{Run: []string{"just", "test"}}, {Check: &rule.Run{Rules: []rule.Rule{{ID: "GO-1"}}}},
		{Restore: []string{"a"}, Source: "t"}} {
		ev = act(context.Background(), port, ev, s)
	}
	if len(ev.Journal) != 3 {
		t.Fatalf("journal %+v", ev.Journal)
	}
	for i, took := range []bool{true, true, false} {
		if got := ev.Journal[i].Took; took != (got >= delay) || !took && got != 0 {
			t.Errorf("outcome %d took %v", i, got)
		}
	}
	if got := millis(1999 * time.Microsecond); got == nil || *got != 1 {
		t.Fatalf("1999us is 1 ms, truncated: %v", got)
	}
	if got := millis(0); got == nil || *got != 0 {
		t.Fatalf("a run that took nothing still has 0: %v", got)
	}
}

const startedGate = "version: 1\non:\n  pre_edit:\n    - when: { not: { started: \"polix:build\" } }\n" +
	"      action: { block: \"Start polix:build before editing the code.\" }\n"

// editCall returns the input of a PreToolUse of the tool Edit on a file of the repository in the session s.
func editCall(s string) string {
	return `{"hook_event_name":"PreToolUse","tool_name":"Edit","cwd":"/repo","session_id":"` + s + `",` +
		`"tool_input":{"file_path":"/repo/internal/app/x.go","old_string":"a","new_string":"b"}}`
}

// editInS1 runs an edit in the session s1 on port, with the log of the records it appended so far.
func editInS1(t *testing.T, port *fakePort) Response {
	t.Helper()
	port.state = state.State{Records: port.appended}
	res, err := run(context.Background(), port, claudeRequest(editCall("s1")))
	if err != nil {
		t.Fatal(err)
	}
	return res
}

// CUJ-03, CUJ-04 and AC-03: the gate of started refuses the edit before a start of polix:build and lets it through
// after it, even once HEAD moved; the start in progress never counts for its own event (DEC-08), the bare name
// matches, and started holds on another branch inside an any.
func TestStartedCondition(t *testing.T) {
	port := &fakePort{yaml: startedGate}
	if res := editInS1(t, port); res.Code != 2 || res.Stderr != "Start polix:build before editing the code.\n" {
		t.Fatalf("before the start: %+v", res)
	}
	if _, err := run(context.Background(), port, claudeRequest(skillCall(`{"skill":"polix:build"}`, ""))); err != nil {
		t.Fatal(err)
	}
	if res := editInS1(t, port); res.Code != 0 || res.Stderr != "" || res.Stdout != "" {
		t.Fatalf("after the start: %+v", res)
	}
	for i := range port.appended {
		port.appended[i].Head = "deadbeef"
		port.appended[i].Branch = "feature"
	}
	if res := editInS1(t, port); res.Code != 0 || res.Stderr != "" {
		t.Fatalf("after a commit and another branch: %+v", res)
	}

	t.Run("the current start does not count", func(t *testing.T) {
		port := &fakePort{yaml: "version: 1\non:\n  skill:\n    - when: { started: \"polix:build\" }\n" +
			"      action: { block: \"second\" }\n"}
		for i, code := range []int{0, 2} {
			port.state = state.State{Records: port.appended}
			res, err := run(context.Background(), port, claudeRequest(skillCall(`{"skill":"polix:build"}`, "")))
			if err != nil || res.Code != code {
				t.Fatalf("start %d: %+v %v", i+1, res, err)
			}
		}
	})
	t.Run("the bare name matches", func(t *testing.T) {
		port := &fakePort{yaml: "version: 1\non:\n  pre_edit:\n    - when: { not: { started: build } }\n" +
			"      action: { block: \"no\" }\n"}
		if _, err := run(context.Background(), port, claudeRequest(skillCall(`{"skill":"polix:build"}`, ""))); err != nil {
			t.Fatal(err)
		}
		if res := editInS1(t, port); res.Code != 0 {
			t.Fatalf("bare name: %+v", res)
		}
	})
	t.Run("any holds after the start on another branch", func(t *testing.T) {
		port := &fakePort{yaml: "version: 1\non:\n  pre_edit:\n    - unless: { any: { started: \"polix:build\", " +
			"branch: \"^feature$\" } }\n      action: { block: \"no\" }\n"}
		if res := editInS1(t, port); res.Code != 2 {
			t.Fatalf("before: %+v", res)
		}
		if _, err := run(context.Background(), port, claudeRequest(skillCall(`{"skill":"polix:build"}`, ""))); err != nil {
			t.Fatal(err)
		}
		if res := editInS1(t, port); res.Code != 0 {
			t.Fatalf("after: %+v", res)
		}
	})
}

// CUJ-05 and AC-04: a start in another session, or one a block refused, never opens the gate.
func TestStartedOtherSessionOrBlocked(t *testing.T) {
	other := &fakePort{yaml: startedGate}
	if _, err := run(context.Background(), other, claudeRequest(strings.Replace(
		skillCall(`{"skill":"polix:build"}`, ""), `"s1"`, `"s0"`, 1))); err != nil {
		t.Fatal(err)
	}
	if got := startedOf(other.appended); !slices.Equal(got, []string{"polix:build"}) {
		t.Fatalf("log of s0: %q", got)
	}
	// The log of s1 holds the records of s0, as a shared file would: the query must filter by session.
	port := &fakePort{yaml: startedGate}
	port.appended = other.appended
	if res := editInS1(t, port); res.Code != 2 || res.Stderr != "Start polix:build before editing the code.\n" {
		t.Fatalf("other session: %+v", res)
	}

	blocked := &fakePort{yaml: startedGate + "  skill:\n    - when: { branch: \"^main$\" }\n" +
		"      action: { block: \"Do not build on main.\" }\n"}
	res, err := run(context.Background(), blocked, claudeRequest(skillCall(`{"skill":"polix:build"}`, "")))
	if err != nil || res.Code != 2 {
		t.Fatalf("blocked start: %+v %v", res, err)
	}
	if res := editInS1(t, blocked); res.Code != 2 || res.Stderr != "Start polix:build before editing the code.\n" {
		t.Fatalf("after a blocked start: %+v", res)
	}
	if got := startedOf(blocked.appended); len(got) != 0 {
		t.Fatalf("started %q after a block", got)
	}
}

// FND-05: a prompt blocked by an entry of prompt followed by the skill it types leaves no observation started.
func TestSkillStartBlockedPromptNotRecorded(t *testing.T) {
	port := &fakePort{yaml: "version: 1\non:\n  prompt:\n    - action: { block: \"No prompts.\" }\n"}
	res, err := run(context.Background(), port, claudeRequest(promptCall(t, "/polix:build go")))
	if err != nil || res.Code != 2 {
		t.Fatalf("blocked prompt: %+v %v", res, err)
	}
	if got := startedOf(port.appended); len(got) != 0 {
		t.Fatalf("started %q after a blocked prompt", got)
	}
}
