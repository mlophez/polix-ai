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
func (f *fakePort) AutoMemoryEnv(_ context.Context) string        { return "" }
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
	if res.Version != "0.2.0" || res.Root != "/r" {
		t.Fatalf("version %q, root %q", res.Version, res.Root)
	}
	// The hook asks neither the releases nor Claude Code, and outside a repository nothing of one is read.
	port = &fakePort{repoErr: errors.New("not inside")}
	res, err = run(context.Background(), port, Request{Version: "0.2.0", Hook: true})
	if err != nil || len(port.calls) != 0 || !res.Hook || res.Root != "" {
		t.Fatalf("%v %v %q", err, port.calls, res.Root)
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

// With the hook, a finding with a fix prints its message and its fix joined, as the base version printed it.
func TestPrintHook(t *testing.T) {
	res := Response{Hook: true, Findings: []setup.Finding{
		{Level: setup.LevelOK, Area: "git", Message: "repository at /work/repo"},
		{Level: setup.LevelWarn, Area: "version", Message: "polix dev is a development build",
			Fix: "install a release with polix update --version <version>"}}}
	var b strings.Builder
	res.print(&b)
	want := "polix doctor found problems on this machine or repository; tell the user about them and that they can fix " +
		"them by running /polix:doctor (polix doctor gives the detail):\n" +
		"- [warn] version: polix dev is a development build: install a release with polix update --version <version>\n"
	if b.String() != want {
		t.Fatalf("%q", b.String())
	}
}

// cuj01 is the Response of polix dev of CUJ-01: the version warns and the five others pass.
func cuj01() Response {
	return Response{Version: "dev", Root: "/work/repo", Findings: []setup.Finding{
		{Level: setup.LevelWarn, Area: "version", Message: "polix dev is a development build",
			Fix: "install a release with polix update --version <version>"},
		{Level: setup.LevelOK, Area: "path", Message: "the hooks run /usr/local/bin/polix"},
		{Level: setup.LevelOK, Area: "git", Message: "repository at /work/repo"},
		{Level: setup.LevelOK, Area: "requirements", Message: "3 programs of requirements on the PATH"},
		{Level: setup.LevelOK, Area: "hooks", Message: "registered in /work/repo/.claude/settings.json"},
		{Level: setup.LevelOK, Area: "plugin", Message: "polix@polix 0.0.0-dev"}}}
}

// CUJ-01: the report in plain output is grouped, aligned and counted (LIT-01).
func TestPrintReport(t *testing.T) {
	res := cuj01()
	var b strings.Builder
	res.print(&b)
	want := "polix doctor · polix dev\n\n" +
		"Machine\n" +
		"  warn  version       polix dev is a development build\n" +
		"                      → install a release with polix update --version <version>\n" +
		"  ok    path          the hooks run /usr/local/bin/polix\n\n" +
		"Repository  /work/repo\n" +
		"  ok    git           repository at /work/repo\n" +
		"  ok    requirements  3 programs of requirements on the PATH\n\n" +
		"Agents\n" +
		"  ok    hooks         registered in /work/repo/.claude/settings.json\n" +
		"  ok    plugin        polix@polix 0.0.0-dev\n\n" +
		"5 ok · 1 warn · 0 fail\n"
	if b.String() != want || res.Failed {
		t.Fatalf("%q", b.String())
	}
}

// CUJ-04: outside a repository the Repository section has no root (LIT-04).
func TestPrintReportOutside(t *testing.T) {
	res := Response{Version: "1.2.0", Findings: []setup.Finding{
		{Level: setup.LevelOK, Area: "version", Message: "polix 1.2.0 (latest release unknown)"},
		{Level: setup.LevelOK, Area: "path", Message: "the hooks run /usr/local/bin/polix"},
		{Level: setup.LevelOK, Area: "repo", Message: "not inside a git repository: only the checks of this machine apply"},
		{Level: setup.LevelOK, Area: "plugin", Message: "polix@polix 1.2.0"}}}
	var b strings.Builder
	res.print(&b)
	want := "polix doctor · polix 1.2.0\n\n" +
		"Machine\n" +
		"  ok    version  polix 1.2.0 (latest release unknown)\n" +
		"  ok    path     the hooks run /usr/local/bin/polix\n\n" +
		"Repository\n" +
		"  ok    repo     not inside a git repository: only the checks of this machine apply\n\n" +
		"Agents\n" +
		"  ok    plugin   polix@polix 1.2.0\n\n" +
		"4 ok · 0 warn · 0 fail\n"
	if b.String() != want || res.Failed {
		t.Fatalf("%q", b.String())
	}
}

// CUJ-04: without git the failure of git goes to Machine and Repository is left out (LIT-05).
func TestPrintReportWithoutGit(t *testing.T) {
	res := Response{Version: "1.2.0", Failed: true, Findings: []setup.Finding{
		{Level: setup.LevelOK, Area: "version", Message: "polix 1.2.0 (latest release unknown)"},
		{Level: setup.LevelOK, Area: "path", Message: "the hooks run /usr/local/bin/polix"},
		{Level: setup.LevelFail, Area: "git", Message: "git is not on the PATH: polix reads every fact of the repository through it"},
		{Level: setup.LevelOK, Area: "plugin", Message: "polix@polix 1.2.0"}}}
	var b strings.Builder
	res.print(&b)
	want := "polix doctor · polix 1.2.0\n\n" +
		"Machine\n" +
		"  ok    version  polix 1.2.0 (latest release unknown)\n" +
		"  ok    path     the hooks run /usr/local/bin/polix\n" +
		"  fail  git      git is not on the PATH: polix reads every fact of the repository through it\n\n" +
		"Agents\n" +
		"  ok    plugin   polix@polix 1.2.0\n\n" +
		"3 ok · 0 warn · 1 fail\n"
	if b.String() != want || !res.Failed {
		t.Fatalf("%q", b.String())
	}
}

// CUJ-05: a failure shows its fix below it and is counted (LIT-06).
func TestPrintReportFailure(t *testing.T) {
	res := cuj01()
	res.Failed = true
	res.Findings[1] = setup.Finding{Level: setup.LevelFail, Area: "path",
		Message: "polix is not on the PATH, so the hooks cannot run it", Fix: "add /usr/local/bin to the PATH"}
	var b strings.Builder
	res.print(&b)
	lit := "  fail  path          polix is not on the PATH, so the hooks cannot run it\n" +
		"                      → add /usr/local/bin to the PATH\n"
	if !strings.Contains(b.String(), lit) || !strings.Contains(b.String(), "\n4 ok · 1 warn · 1 fail\n") || !res.Failed {
		t.Fatalf("%q", b.String())
	}
}
