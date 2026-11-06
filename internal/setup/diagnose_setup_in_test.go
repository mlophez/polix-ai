package setup

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/mlophez/polix-ai/internal/rule"
	"github.com/mlophez/polix-ai/internal/version"
	"github.com/mlophez/polix-ai/internal/workflow"
)

// fakeDiagnoseSetupPort answers the port of DiagnoseSetup from memory and records the calls.
type fakeDiagnoseSetupPort struct {
	calls   []string
	saved   version.Cache
	repoErr error
}

func (f *fakeDiagnoseSetupPort) note(c string) { f.calls = append(f.calls, c) }

func (f *fakeDiagnoseSetupPort) Home(_ context.Context) string { return "/h" }
func (f *fakeDiagnoseSetupPort) Executable(_ context.Context) (string, error) {
	return "/bin/polix", nil
}
func (f *fakeDiagnoseSetupPort) LoadCache(_ context.Context) version.Cache {
	return version.Cache{Latest: "0.1.0"}
}
func (f *fakeDiagnoseSetupPort) SaveCache(_ context.Context, c version.Cache) error {
	f.saved = c
	return nil
}
func (f *fakeDiagnoseSetupPort) Latest(_ context.Context) (string, error) {
	f.note("Latest")
	return "0.3.0", nil
}
func (f *fakeDiagnoseSetupPort) LookPath(_ context.Context, name string) (string, error) {
	return "/bin/" + name, nil
}
func (f *fakeDiagnoseSetupPort) SameFile(_ context.Context, a, b string) bool { return a == b }
func (f *fakeDiagnoseSetupPort) OpenRepo(_ context.Context, _ string) (string, string, error) {
	return "/r", "/r/gitdir", f.repoErr
}
func (f *fakeDiagnoseSetupPort) LoadWorkflow(_ context.Context, _, _ string) (workflow.Workflow, bool, error) {
	f.note("LoadWorkflow")
	return workflow.Workflow{}, true, nil
}
func (f *fakeDiagnoseSetupPort) LoadRules(_ context.Context, _, _ string) ([]rule.Rule, []string, error) {
	return []rule.Rule{{ID: "A-1", Description: "d", Deterministic: true, Check: "a.sh", File: "f"}}, nil, nil
}
func (f *fakeDiagnoseSetupPort) Missing(_ context.Context, _, _ string) bool { return false }
func (f *fakeDiagnoseSetupPort) Runnable(_ context.Context, _, check string) error {
	return errors.New("check " + check + " is not executable (chmod +x)")
}
func (f *fakeDiagnoseSetupPort) ResolveBase(_ context.Context, _ string, _ []string) error {
	return nil
}
func (f *fakeDiagnoseSetupPort) WriteState(_ context.Context, gitDir string) (string, error) {
	return gitDir + "/state", nil
}
func (f *fakeDiagnoseSetupPort) ReadFiles(_ context.Context, _ []string) (map[string][]byte, map[string]error) {
	s := Setup{Home: "/h", Version: "0.2.0"}
	return map[string][]byte{s.PluginManifest(): []byte(`{"name": "polix", "version": "0.2.0"}`)}, nil
}
func (f *fakeDiagnoseSetupPort) Plugin(_ context.Context) (map[string][]byte, error) { return nil, nil }
func (f *fakeDiagnoseSetupPort) ClaudePlugins(_ context.Context) ([]byte, error) {
	f.note("ClaudePlugins")
	return nil, ErrNoCLI
}
func (f *fakeDiagnoseSetupPort) CodexUsed(_ context.Context, _, _ string) bool { return false }

func TestDiagnoseSetupShell(t *testing.T) {
	port := &fakeDiagnoseSetupPort{}
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	res, err := diagnoseSetup(context.Background(), port, DiagnoseSetupRequest{Version: "0.2.0", Now: now})
	if err != nil || !res.Failed || port.saved.Latest != "0.3.0" ||
		strings.Join(port.calls, " ") != "Latest LoadWorkflow ClaudePlugins" {
		t.Fatalf("%+v %v %v %+v", res, err, port.calls, port.saved)
	}
	// The hook asks neither the releases nor Claude Code, and outside a repository nothing of one is read.
	port = &fakeDiagnoseSetupPort{repoErr: errors.New("not inside")}
	res, err = diagnoseSetup(context.Background(), port, DiagnoseSetupRequest{Version: "0.2.0", Hook: true})
	if err != nil || len(port.calls) != 0 || !res.Hook {
		t.Fatalf("%v %v", err, port.calls)
	}
	// A plugin of another binary is not asked to Claude Code.
	port = &fakeDiagnoseSetupPort{}
	if _, err := diagnoseSetup(context.Background(), port, DiagnoseSetupRequest{Version: "0.3.0", Offline: true}); err != nil ||
		len(port.calls) != 1 {
		t.Fatalf("%v %v", err, port.calls)
	}
	// A development build never asks the releases.
	port = &fakeDiagnoseSetupPort{}
	if _, err := diagnoseSetup(context.Background(), port, DiagnoseSetupRequest{Version: "dev"}); err != nil ||
		port.calls[0] == "Latest" {
		t.Fatal("dev asks the releases")
	}
}

func TestDiagnoseSetupPrint(t *testing.T) {
	res := DiagnoseSetupResponse{Hook: true,
		Findings: append(found(LevelOK, "git", "fine"), found(LevelFail, "hooks", "Stop does not run")...)}
	var b strings.Builder
	res.print(&b)
	if !strings.Contains(b.String(), "cuéntaselos al usuario") || !strings.Contains(b.String(), "- [fail] hooks: Stop does not run") ||
		strings.Contains(b.String(), "[ok]") {
		t.Fatal(b.String())
	}
	b.Reset()
	res.Findings = found(LevelOK, "git", "fine")
	if res.print(&b); b.String() != "" {
		t.Fatal("a healthy hook says nothing")
	}
}
