package diagnose

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/mlophez/polix-ai/internal/app/rule"
	"github.com/mlophez/polix-ai/internal/app/setup"
	"github.com/mlophez/polix-ai/internal/app/version"
	"github.com/mlophez/polix-ai/internal/app/workflow"
)

// fakePort answers the port of DiagnoseSetup from memory and records the calls.
type fakePort struct {
	calls   []string
	saved   version.Cache
	repoErr error
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
	return "/bin/" + name, nil
}
func (f *fakePort) SameFile(_ context.Context, a, b string) bool { return a == b }
func (f *fakePort) OpenRepo(_ context.Context, _ string) (string, string, error) {
	return "/r", "/r/gitdir", f.repoErr
}
func (f *fakePort) LoadWorkflow(_ context.Context, _, _ string) (workflow.Workflow, bool, error) {
	f.note("LoadWorkflow")
	return workflow.Workflow{}, true, nil
}
func (f *fakePort) LoadRules(_ context.Context, _, _ string) ([]rule.Rule, []string, error) {
	return []rule.Rule{{ID: "A-1", Description: "d", Deterministic: true, Check: "a.sh", File: "f"}}, nil, nil
}
func (f *fakePort) Missing(_ context.Context, _, _ string) bool { return false }
func (f *fakePort) Runnable(_ context.Context, _, check string) error {
	return errors.New("check " + check + " is not executable (chmod +x)")
}
func (f *fakePort) ResolveBase(_ context.Context, _ string, _ []string) error {
	return nil
}
func (f *fakePort) WriteState(_ context.Context, gitDir string) (string, error) {
	return gitDir + "/state", nil
}
func (f *fakePort) ClaudeHooks(_ context.Context, _ []string) map[string]setup.HooksFile { return nil }
func (f *fakePort) CodexHooks(_ context.Context, _ []string) map[string]setup.HooksFile  { return nil }
func (f *fakePort) CodexEnabled(_ context.Context, _ string) bool                        { return false }
func (f *fakePort) ReadManifest(_ context.Context, _ string) (string, bool)              { return "0.2.0", true }
func (f *fakePort) Plugin(_ context.Context) (map[string][]byte, error)                  { return nil, nil }
func (f *fakePort) ClaudePlugin(_ context.Context) (bool, bool, string, error) {
	f.note("ClaudePlugin")
	return false, false, "", setup.ErrNoCLI
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

func TestPrint(t *testing.T) {
	fine := setup.Finding{Level: setup.LevelOK, Area: "git", Message: "fine"}
	res := Response{Hook: true,
		Findings: []setup.Finding{fine, {Level: setup.LevelFail, Area: "hooks", Message: "Stop does not run"}}}
	var b strings.Builder
	res.print(&b)
	if !strings.Contains(b.String(), "cuéntaselos al usuario") || !strings.Contains(b.String(), "- [fail] hooks: Stop does not run") ||
		strings.Contains(b.String(), "[ok]") {
		t.Fatal(b.String())
	}
	b.Reset()
	res.Findings = []setup.Finding{fine}
	if res.print(&b); b.String() != "" {
		t.Fatal("a healthy hook says nothing")
	}
}
