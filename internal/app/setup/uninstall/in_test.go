package uninstall

import (
	"context"
	"errors"
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
