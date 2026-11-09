package install

import (
	"bytes"
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/mlophez/polix-ai/internal/app/setup"
)

// fakePort answers the port of InstallSetup from memory and records the calls.
type fakePort struct {
	calls     []string
	pluginErr error // of RegisterPlugin
	files     map[string][]byte
	refuse    bool // Confirm refuses
}

func (f *fakePort) Confirm(_ context.Context, _ string) error {
	if f.refuse {
		return setup.ErrCancelled
	}
	return nil
}

func (f *fakePort) Home(_ context.Context) (string, error)  { return "/h", nil }
func (f *fakePort) Root(_ context.Context, _ string) string { return "/r" }
func (f *fakePort) Plugin(_ context.Context) (map[string][]byte, error) {
	return map[string][]byte{"polix/.claude-plugin/plugin.json": []byte(`{"name": "polix", "version": "0"}`)}, nil
}
func (f *fakePort) WritePlugin(_ context.Context, dir string, _ map[string][]byte) ([]setup.Change, error) {
	f.calls = append(f.calls, "WritePlugin "+dir)
	return []setup.Change{{Path: dir, Action: "created"}}, nil
}
func (f *fakePort) RegisterPlugin(_ context.Context, _ setup.Setup, agent string) ([]setup.Change, error) {
	f.calls = append(f.calls, "RegisterPlugin "+agent)
	return nil, f.pluginErr
}
func (f *fakePort) RegisterHooks(_ context.Context, file, _ string) ([]setup.Change, error) {
	f.calls = append(f.calls, "RegisterHooks "+file)
	return nil, nil
}
func (f *fakePort) ReadFiles(_ context.Context, _ []string) map[string][]byte {
	return f.files
}
func (f *fakePort) OnPath(_ context.Context, _ string) error { return errors.New("no") }

func TestShell(t *testing.T) {
	port := &fakePort{}
	res, err := run(context.Background(), port, Request{Version: "1.2.3"})
	if err != nil || len(res.Changes) != 1 || !res.NotOnPath || len(res.HooksFiles) != 1 || len(port.calls) != 3 ||
		!strings.HasPrefix(port.calls[2], "RegisterHooks") {
		t.Fatalf("%+v %v %v", res, err, port.calls)
	}
	// The hooks are registered also when the plugin fails, and the error comes back with the changes.
	port = &fakePort{pluginErr: errors.New("refused")}
	res, err = run(context.Background(), port, Request{Version: "1.2.3"})
	if err == nil || len(res.Changes) != 1 || len(port.calls) != 3 {
		t.Fatalf("%+v %v %v", res, err, port.calls)
	}
	// A person that does not confirm stops it before anything is written.
	port = &fakePort{refuse: true}
	_, err = run(context.Background(), port, Request{Version: "1.2.3", User: true})
	if !errors.Is(err, setup.ErrCancelled) || len(port.calls) != 0 {
		t.Fatalf("%v %v", err, port.calls)
	}
}

// Codex is set up only when asked, and then the output says its support is not implemented.
func TestShellCodex(t *testing.T) {
	const notImplemented = "Codex support is not implemented in this release and the setup is untested"
	port := &fakePort{}
	res, err := run(context.Background(), port, Request{Version: "1.2.3"})
	var stdout, stderr bytes.Buffer
	res.print(&stdout, &stderr)
	if err != nil || slices.Contains(res.Agents, setup.Codex) || strings.Contains(stdout.String(), notImplemented) {
		t.Fatalf("a default install leaves Codex out: %+v %v\n%s", res, err, stdout.String())
	}
	port = &fakePort{}
	res, err = run(context.Background(), port, Request{Version: "1.2.3", Agents: []string{setup.Codex}})
	stdout.Reset()
	res.print(&stdout, &stderr)
	if err != nil || !slices.Equal(res.Agents, []string{setup.Codex}) || !slices.Contains(port.calls, "RegisterPlugin "+setup.Codex) ||
		!strings.Contains(stdout.String(), notImplemented) {
		t.Fatalf("--agent codex sets up Codex and says it is not implemented: %+v %v %v\n%s", res, err, port.calls, stdout.String())
	}
}
