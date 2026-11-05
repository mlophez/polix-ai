package setup

import (
	"context"
	"testing"
)

// fakeRefreshSetupPort answers the port of RefreshSetup from memory and records the calls.
type fakeRefreshSetupPort struct {
	files   map[string][]byte
	changed bool
	calls   []string
}

func (f *fakeRefreshSetupPort) Home(_ context.Context) (string, error)  { return "/h", nil }
func (f *fakeRefreshSetupPort) Root(_ context.Context, _ string) string { return "/r" }
func (f *fakeRefreshSetupPort) Plugin(_ context.Context) (map[string][]byte, error) {
	return map[string][]byte{"polix/.claude-plugin/plugin.json": []byte(`{"name": "polix", "version": "0"}`)}, nil
}
func (f *fakeRefreshSetupPort) ReadFiles(_ context.Context, _ []string) map[string][]byte {
	return f.files
}
func (f *fakeRefreshSetupPort) WritePlugin(_ context.Context, dir string, _ map[string][]byte) ([]Change, error) {
	f.calls = append(f.calls, "WritePlugin")
	if f.changed {
		return []Change{{Path: dir, Action: "updated"}}, nil
	}
	return nil, nil
}
func (f *fakeRefreshSetupPort) UpdatePlugin(_ context.Context, _ Setup) {
	f.calls = append(f.calls, "UpdatePlugin")
}
func (f *fakeRefreshSetupPort) RegisterHooks(_ context.Context, _, agent string) ([]Change, error) {
	f.calls = append(f.calls, "RegisterHooks "+agent)
	return nil, nil
}

func TestRefreshSetupShell(t *testing.T) {
	s := Setup{Home: "/h", Root: "/r", Agents: []string{Claude, Codex}, Version: "1"}
	hooks, _, _ := RegisterHooks(nil, false, Codex)
	files := map[string][]byte{s.PluginManifest(): []byte(`{"name": "polix", "version": "0"}`), s.HooksFile(Codex): hooks}
	port := &fakeRefreshSetupPort{files: files, changed: true}
	res, err := refreshSetup(context.Background(), port, RefreshSetupRequest{Version: "1", Hooks: true})
	if err != nil || len(res.Changes) != 1 || len(port.calls) != 3 || port.calls[2] != "RegisterHooks codex" {
		t.Fatalf("%+v %v %v", res, err, port.calls)
	}
	port = &fakeRefreshSetupPort{files: files}
	if _, err := refreshSetup(context.Background(), port, RefreshSetupRequest{Version: "1"}); err != nil ||
		len(port.calls) != 1 {
		t.Fatalf("nothing changed, nothing to tell; no hooks without --hooks: %v", port.calls)
	}
}
