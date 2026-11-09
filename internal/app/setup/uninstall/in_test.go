package uninstall

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/mlophez/polix-ai/internal/app/setup"
)

// fakePort answers the port of UninstallSetup from memory and records the calls.
type fakePort struct {
	calls []string
	err   error // of UnregisterPlugin
}

func (f *fakePort) Confirm(_ context.Context, _ string) error { return nil }
func (f *fakePort) Home(_ context.Context) (string, error)    { return "/h", nil }
func (f *fakePort) Root(_ context.Context, _ string) string   { return "/r" }
func (f *fakePort) UnregisterPlugin(_ context.Context, _ setup.Setup, agent string) ([]setup.Change, error) {
	f.calls = append(f.calls, "UnregisterPlugin "+agent)
	return []setup.Change{{Path: agent, Action: "unregistered"}}, f.err
}
func (f *fakePort) UnregisterHooks(_ context.Context, file string) ([]setup.Change, error) {
	f.calls = append(f.calls, "UnregisterHooks "+file)
	return nil, nil
}
func (f *fakePort) RemovePlugin(_ context.Context, dir string) ([]setup.Change, error) {
	f.calls = append(f.calls, "RemovePlugin "+dir)
	return nil, nil
}

func TestShell(t *testing.T) {
	port := &fakePort{}
	res, err := run(context.Background(), port, Request{Version: "1"})
	if err != nil || len(res.Changes) != 2 || len(port.calls) != 5 {
		t.Fatalf("%+v %v %v", res, err, port.calls)
	}
	port = &fakePort{err: errors.New("boom")}
	res, err = run(context.Background(), port, Request{Version: "1", User: true})
	if err == nil || len(res.Changes) != 1 || len(port.calls) != 1 {
		t.Fatalf("the first error stops it: %+v %v %v", res, err, port.calls)
	}
}

// Without the running version there is no setup: nothing is asked nor removed.
func TestShellWithoutVersion(t *testing.T) {
	port := &fakePort{}
	if _, err := run(context.Background(), port, Request{}); err == nil || len(port.calls) != 0 {
		t.Fatalf("%v %v", err, port.calls)
	}
}

func TestParseAgents(t *testing.T) {
	req := Request{Version: "1"}
	if err := req.parseAgents([]string{" Claude", "codex", "claude"}); err != nil ||
		!slices.Equal(req.Agents, []string{setup.Claude, setup.Codex}) {
		t.Fatalf("%v %v", req.Agents, err)
	}
	req = Request{Version: "1"}
	if err := req.parseAgents([]string{"claude", "cursor"}); err == nil || err.Error() != `unknown agent "cursor" (claude, codex)` ||
		req.Agents != nil {
		t.Fatalf("unknown agent: %v", err)
	}
}
