package setup

import (
	"context"
	"errors"
	"testing"
)

// fakeUninstallSetupPort answers the port of UninstallSetup from memory and records the calls.
type fakeUninstallSetupPort struct {
	calls []string
	err   error // of UnregisterPlugin
}

func (f *fakeUninstallSetupPort) Confirm(_ context.Context, _ string) error { return nil }
func (f *fakeUninstallSetupPort) Home(_ context.Context) (string, error)    { return "/h", nil }
func (f *fakeUninstallSetupPort) Root(_ context.Context, _ string) string   { return "/r" }
func (f *fakeUninstallSetupPort) UnregisterPlugin(_ context.Context, _ Setup, agent string) ([]Change, error) {
	f.calls = append(f.calls, "UnregisterPlugin "+agent)
	return []Change{{Path: agent, Action: "unregistered"}}, f.err
}
func (f *fakeUninstallSetupPort) UnregisterHooks(_ context.Context, file string) ([]Change, error) {
	f.calls = append(f.calls, "UnregisterHooks "+file)
	return nil, nil
}
func (f *fakeUninstallSetupPort) RemovePlugin(_ context.Context, dir string) ([]Change, error) {
	f.calls = append(f.calls, "RemovePlugin "+dir)
	return nil, nil
}

func TestUninstallSetupShell(t *testing.T) {
	port := &fakeUninstallSetupPort{}
	res, err := uninstallSetup(context.Background(), port, UninstallSetupRequest{Version: "1"})
	if err != nil || len(res.Changes) != 2 || len(port.calls) != 5 {
		t.Fatalf("%+v %v %v", res, err, port.calls)
	}
	port = &fakeUninstallSetupPort{err: errors.New("boom")}
	res, err = uninstallSetup(context.Background(), port, UninstallSetupRequest{Version: "1", User: true})
	if err == nil || len(res.Changes) != 1 || len(port.calls) != 1 {
		t.Fatalf("the first error stops it: %+v %v %v", res, err, port.calls)
	}
}
