package refresh

import (
	"context"
	"errors"
	"testing"

	"github.com/mlophez/polix-ai/internal/app/setup"
)

// fakePort answers the port of RefreshSetup from memory and records the calls.
type fakePort struct {
	written    bool            // the manifest of the plugin polix is in the marketplace
	registered map[string]bool // the hooks files that register polix
	filesErr   error           // of MarketplaceFiles
	changed    bool
	calls      []string
}

func (f *fakePort) Home(_ context.Context) (string, error)  { return "/h", nil }
func (f *fakePort) Root(_ context.Context, _ string) string { return "/r" }
func (f *fakePort) Plugin(_ context.Context) (map[string][]byte, error) {
	return map[string][]byte{"polix/.claude-plugin/plugin.json": []byte(`{"name": "polix", "version": "0"}`)}, nil
}
func (f *fakePort) ReadManifest(_ context.Context, _ string) (string, bool) {
	if f.written {
		return "0", true
	}
	return "", false
}
func (f *fakePort) Registered(_ context.Context, _ []string) map[string]bool { return f.registered }
func (f *fakePort) MarketplaceFiles(_ context.Context, _ setup.Plugin) (map[string][]byte, error) {
	return map[string][]byte{}, f.filesErr
}
func (f *fakePort) WritePlugin(_ context.Context, dir string, _ map[string][]byte) ([]setup.Change, error) {
	f.calls = append(f.calls, "WritePlugin")
	if f.changed {
		return []setup.Change{{Path: dir, Action: "updated"}}, nil
	}
	return nil, nil
}
func (f *fakePort) UpdatePlugin(_ context.Context, _ setup.Setup) {
	f.calls = append(f.calls, "UpdatePlugin")
}
func (f *fakePort) RegisterHooks(_ context.Context, _ string, hooks map[string][]setup.HookGroup) ([]setup.Change, error) {
	f.calls = append(f.calls, "RegisterHooks "+hooks["Stop"][0].Handlers[0].Command)
	return nil, nil
}

func TestShell(t *testing.T) {
	s := setup.Setup{Home: "/h", Root: "/r", Agents: []string{setup.Claude, setup.Codex}, Version: "1"}
	registered := map[string]bool{s.HooksFile(setup.Codex): true}
	port := &fakePort{written: true, registered: registered, changed: true}
	res, err := run(context.Background(), port, Request{Version: "1", Hooks: true})
	if err != nil || len(res.Changes) != 1 || len(port.calls) != 3 || port.calls[2] != "RegisterHooks polix hook codex" {
		t.Fatalf("%+v %v %v", res, err, port.calls)
	}
	port = &fakePort{written: true, registered: registered}
	if _, err := run(context.Background(), port, Request{Version: "1"}); err != nil ||
		len(port.calls) != 1 {
		t.Fatalf("nothing changed, nothing to tell; no hooks without --hooks: %v", port.calls)
	}
	// A plugin whose marketplace cannot be written stops it before anything is written.
	port = &fakePort{written: true, registered: registered, filesErr: errors.New("no version")}
	if _, err := run(context.Background(), port, Request{Version: "1", Hooks: true}); err == nil || len(port.calls) != 0 {
		t.Fatalf("%v %v", err, port.calls)
	}
}
