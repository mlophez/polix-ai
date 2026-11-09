package refresh

import (
	"context"
	"testing"

	"github.com/mlophez/polix-ai/internal/app/setup"
)

// fakePort answers the port of RefreshSetup from memory and records the calls.
type fakePort struct {
	files   map[string][]byte
	changed bool
	calls   []string
}

func (f *fakePort) Home(_ context.Context) (string, error)  { return "/h", nil }
func (f *fakePort) Root(_ context.Context, _ string) string { return "/r" }
func (f *fakePort) Plugin(_ context.Context) (map[string][]byte, error) {
	return map[string][]byte{"polix/.claude-plugin/plugin.json": []byte(`{"name": "polix", "version": "0"}`)}, nil
}
func (f *fakePort) ReadFiles(_ context.Context, _ []string) map[string][]byte {
	return f.files
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
func (f *fakePort) RegisterHooks(_ context.Context, _, agent string) ([]setup.Change, error) {
	f.calls = append(f.calls, "RegisterHooks "+agent)
	return nil, nil
}

func TestShell(t *testing.T) {
	s := setup.Setup{Home: "/h", Root: "/r", Agents: []string{setup.Claude, setup.Codex}, Version: "1"}
	hooks, _, _ := setup.RegisterHooks(nil, false, setup.Codex)
	files := map[string][]byte{s.PluginManifest(): []byte(`{"name": "polix", "version": "0"}`), s.HooksFile(setup.Codex): hooks}
	port := &fakePort{files: files, changed: true}
	res, err := run(context.Background(), port, Request{Version: "1", Hooks: true})
	if err != nil || len(res.Changes) != 1 || len(port.calls) != 3 || port.calls[2] != "RegisterHooks codex" {
		t.Fatalf("%+v %v %v", res, err, port.calls)
	}
	port = &fakePort{files: files}
	if _, err := run(context.Background(), port, Request{Version: "1"}); err != nil ||
		len(port.calls) != 1 {
		t.Fatalf("nothing changed, nothing to tell; no hooks without --hooks: %v", port.calls)
	}
}
