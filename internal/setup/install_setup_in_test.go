package setup

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// fakeInstallSetupPort answers the port of InstallSetup from memory and records the calls.
type fakeInstallSetupPort struct {
	calls     []string
	pluginErr error // of RegisterPlugin
	files     map[string][]byte
	refuse    bool // Confirm refuses
}

func (f *fakeInstallSetupPort) Confirm(_ context.Context, _ string) error {
	if f.refuse {
		return ErrCancelled
	}
	return nil
}

func (f *fakeInstallSetupPort) Home(_ context.Context) (string, error)        { return "/h", nil }
func (f *fakeInstallSetupPort) Root(_ context.Context, _ string) string       { return "/r" }
func (f *fakeInstallSetupPort) CodexUsed(_ context.Context, _, _ string) bool { return false }
func (f *fakeInstallSetupPort) Plugin(_ context.Context) (map[string][]byte, error) {
	return map[string][]byte{"polix/.claude-plugin/plugin.json": []byte(`{"name": "polix", "version": "0"}`)}, nil
}
func (f *fakeInstallSetupPort) WritePlugin(_ context.Context, dir string, _ map[string][]byte) ([]Change, error) {
	f.calls = append(f.calls, "WritePlugin "+dir)
	return []Change{{Path: dir, Action: "created"}}, nil
}
func (f *fakeInstallSetupPort) RegisterPlugin(_ context.Context, _ Setup, agent string) ([]Change, error) {
	f.calls = append(f.calls, "RegisterPlugin "+agent)
	return nil, f.pluginErr
}
func (f *fakeInstallSetupPort) RegisterHooks(_ context.Context, file, _ string) ([]Change, error) {
	f.calls = append(f.calls, "RegisterHooks "+file)
	return nil, nil
}
func (f *fakeInstallSetupPort) ReadFiles(_ context.Context, _ []string) map[string][]byte {
	return f.files
}
func (f *fakeInstallSetupPort) OnPath(_ context.Context, _ string) error { return errors.New("no") }

func TestInstallSetupShell(t *testing.T) {
	port := &fakeInstallSetupPort{}
	res, err := installSetup(context.Background(), port, InstallSetupRequest{Version: "1.2.3"})
	if err != nil || len(res.Changes) != 1 || !res.NotOnPath || len(res.HooksFiles) != 1 || len(port.calls) != 3 ||
		!strings.HasPrefix(port.calls[2], "RegisterHooks") {
		t.Fatalf("%+v %v %v", res, err, port.calls)
	}
	// The hooks are registered also when the plugin fails, and the error comes back with the changes.
	port = &fakeInstallSetupPort{pluginErr: errors.New("refused")}
	res, err = installSetup(context.Background(), port, InstallSetupRequest{Version: "1.2.3"})
	if err == nil || len(res.Changes) != 1 || len(port.calls) != 3 {
		t.Fatalf("%+v %v %v", res, err, port.calls)
	}
	// A person that does not confirm stops it before anything is written.
	port = &fakeInstallSetupPort{refuse: true}
	_, err = installSetup(context.Background(), port, InstallSetupRequest{Version: "1.2.3", User: true})
	if !errors.Is(err, ErrCancelled) || len(port.calls) != 0 {
		t.Fatalf("%v %v", err, port.calls)
	}
}
