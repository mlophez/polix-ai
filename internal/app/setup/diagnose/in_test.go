package diagnose

import (
	"context"
	"errors"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/mlophez/polix-ai/internal/app/rule"
	"github.com/mlophez/polix-ai/internal/app/setup"
	"github.com/mlophez/polix-ai/internal/app/version"
	"github.com/mlophez/polix-ai/internal/app/workflow"
	"github.com/mlophez/polix-ai/internal/platform/testenv"
)

// fakePort answers the port of DiagnoseSetup from memory and records the calls.
type fakePort struct {
	calls   []string
	saved   version.Cache
	repoErr error
	wf      workflow.Workflow // the workflow LoadWorkflow returns
	absent  map[string]bool   // the programs LookPath does not find
	looked  []string          // the programs LookPath was asked for
	// gitHooks are the roots GitHooks was asked for, apart from calls; GitHooks lists every hook of polix.
	gitHooks []string
}

// TestMain runs the tests without the variables git exports to its hooks (see testenv.ClearGitHookEnv): the githooks
// of polix.yaml run go test, and these tests run git in repositories of their own.
func TestMain(m *testing.M) {
	testenv.ClearGitHookEnv()
	os.Exit(m.Run())
}

func (f *fakePort) note(c string) { f.calls = append(f.calls, c) }

func (f *fakePort) Home(_ context.Context) string { return "/h" }
func (f *fakePort) Executable(_ context.Context) (string, error) {
	return "/bin/polix", nil
}
func (f *fakePort) LoadCache(_ context.Context) version.Cache {
	return version.Cache{Latest: "0.1.0"}
}
func (f *fakePort) SaveCache(_ context.Context, c version.Cache) error {
	f.saved = c
	return nil
}
func (f *fakePort) Latest(_ context.Context) (string, error) {
	f.note("Latest")
	return "0.3.0", nil
}
func (f *fakePort) LookPath(_ context.Context, name string) (string, error) {
	f.looked = append(f.looked, name)
	if f.absent[name] {
		return "", errors.New("executable file not found in $PATH")
	}
	return "/bin/" + name, nil
}
func (f *fakePort) SameFile(_ context.Context, a, b string) bool { return a == b }
func (f *fakePort) OpenRepo(_ context.Context, _ string) (string, string, error) {
	return "/r", "/r/gitdir", f.repoErr
}
func (f *fakePort) LoadWorkflow(_ context.Context, _, _ string) (workflow.Workflow, bool, error, error) {
	f.note("LoadWorkflow")
	return f.wf, true, nil, nil
}
func (f *fakePort) LoadRules(_ context.Context, _, _ string) (rule.Loaded, map[string]bool, map[string]error) {
	loaded := rule.Loaded{Rules: []rule.Rule{{ID: "A-1", Description: "d", Deterministic: true, Check: "a.sh", File: "f"}}}
	return loaded, map[string]bool{"a.sh": false},
		map[string]error{"a.sh": errors.New("check a.sh is not executable (chmod +x)")}
}
func (f *fakePort) WriteState(_ context.Context, gitDir string) (string, error) {
	return gitDir + "/state", nil
}
func (f *fakePort) HooksFiles(_ context.Context, _ string, _ []string) map[string]setup.HooksFile {
	return nil
}
func (f *fakePort) CodexEnabled(_ context.Context, _ string) bool { return false }
func (f *fakePort) Plugin(_ context.Context, _ string) (map[string][]byte, string, bool, error) {
	return nil, "0.2.0", true, nil
}
func (f *fakePort) ClaudePlugin(_ context.Context) (bool, bool, string, error) {
	f.note("ClaudePlugin")
	return false, false, "", setup.ErrNoCLI
}
func (f *fakePort) GitHooks(_ context.Context, root string, hooks []setup.GitHook) (map[string][]string,
	map[string][]string, bool) {
	f.gitHooks = append(f.gitHooks, root)
	listed := map[string][]string{}
	for _, h := range hooks {
		listed[string(h)] = []string{h.Name()}
	}
	return listed, nil, true
}

// AC15: inside a repository the shell asks git for the hooks of git polix registers, and the report says they
// run; outside one it asks nothing.
func TestShellGitHooks(t *testing.T) {
	port := &fakePort{}
	res, err := run(context.Background(), port, Request{Version: "0.2.0", Offline: true})
	if err != nil || strings.Join(port.gitHooks, " ") != "/r" ||
		!slices.Contains(res.Findings, setup.Finding{Level: setup.LevelOK, Area: "githooks",
			Message: "git runs polix githook on pre-commit, commit-msg (githooks need git 2.54 or later)"}) {
		t.Fatalf("%+v %v %q", res, err, port.gitHooks)
	}
	port = &fakePort{repoErr: errors.New("not inside")}
	if _, err := run(context.Background(), port, Request{Version: "0.2.0", Offline: true}); err != nil ||
		port.gitHooks != nil {
		t.Fatalf("outside: %v %q", err, port.gitHooks)
	}
}

func TestShell(t *testing.T) {
	port := &fakePort{}
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	res, err := run(context.Background(), port, Request{Version: "0.2.0", Now: now})
	if err != nil || !res.Failed || port.saved.Latest != "0.3.0" ||
		strings.Join(port.calls, " ") != "Latest LoadWorkflow ClaudePlugin" {
		t.Fatalf("%+v %v %v %+v", res, err, port.calls, port.saved)
	}
	// The hook asks neither the releases nor Claude Code, and outside a repository nothing of one is read.
	port = &fakePort{repoErr: errors.New("not inside")}
	res, err = run(context.Background(), port, Request{Version: "0.2.0", Hook: true})
	if err != nil || len(port.calls) != 0 || !res.Hook {
		t.Fatalf("%v %v", err, port.calls)
	}
	// A plugin of another binary is not asked to Claude Code.
	port = &fakePort{}
	if _, err := run(context.Background(), port, Request{Version: "0.3.0", Offline: true}); err != nil ||
		len(port.calls) != 1 {
		t.Fatalf("%v %v", err, port.calls)
	}
	// A development build never asks the releases.
	port = &fakePort{}
	if _, err := run(context.Background(), port, Request{Version: "dev"}); err != nil ||
		port.calls[0] == "Latest" {
		t.Fatal("dev asks the releases")
	}
}

// The shell looks up on the PATH each program the workflow requires, and --hook prints the missing ones.
func TestShellRequirements(t *testing.T) {
	port := &fakePort{wf: workflow.Workflow{Requirements: []string{"go", "nope"}}, absent: map[string]bool{"nope": true}}
	res, err := run(context.Background(), port, Request{Version: "0.2.0", Hook: true})
	if err != nil || strings.Join(port.looked, " ") != "polix git go nope git" {
		t.Fatalf("%v %v", err, port.looked)
	}
	var b strings.Builder
	res.print(&b)
	if got := b.String(); !strings.Contains(got, "- [warn] requirements: nope is not on the PATH: polix.yaml requires it") ||
		strings.Contains(got, "go is not") {
		t.Fatal(got)
	}
	// Outside a repository no program of a workflow is looked up.
	port = &fakePort{repoErr: errors.New("not inside"), wf: workflow.Workflow{Requirements: []string{"go"}}}
	if _, err := run(context.Background(), port, Request{Version: "0.2.0", Hook: true}); err != nil ||
		strings.Join(port.looked, " ") != "polix git" {
		t.Fatalf("%v %v", err, port.looked)
	}
}

func TestPrint(t *testing.T) {
	fine := setup.Finding{Level: setup.LevelOK, Area: "git", Message: "fine"}
	res := Response{Hook: true,
		Findings: []setup.Finding{fine, {Level: setup.LevelFail, Area: "hooks", Message: "Stop does not run"}}}
	var b strings.Builder
	res.print(&b)
	if !strings.Contains(b.String(), "tell the user about them") || !strings.Contains(b.String(), "- [fail] hooks: Stop does not run") ||
		strings.Contains(b.String(), "[ok]") {
		t.Fatal(b.String())
	}
	b.Reset()
	res.Findings = []setup.Finding{fine}
	if res.print(&b); b.String() != "" {
		t.Fatal("a healthy hook says nothing")
	}
}
