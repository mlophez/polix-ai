package setup

import (
	"context"
	"os"
	"slices"
	"strings"
	"testing"
)

// The hidden command the update runs: the hook refreshes only a plugin already installed, with one line when it
// changed; a person gets the plugin with install --user and __refresh --hooks.
func TestRefreshSetupCommand(t *testing.T) {
	machine(t, false, false)
	t.Chdir(t.TempDir())
	if code, stdout, _ := runSetup("__refresh", "--quiet"); code != 0 || stdout != "" {
		t.Fatalf("the hook installs nothing new: %d %q", code, stdout)
	}
	if code, _, stderr := runSetup("install", "--user", "--yes", "--quiet"); code != 0 {
		t.Fatalf("install --user: %d %q", code, stderr)
	}
	if code, stdout, _ := runSetup("__refresh", "--hooks"); code != 0 || stdout != "" {
		t.Fatalf("nothing to refresh: %d %q", code, stdout)
	}
	s := Setup{Home: os.Getenv("HOME"), Version: "dev"}
	write(t, s.PluginManifest(), `{"name": "polix", "version": "0.1.0"}`)
	if code, stdout, _ := runSetup("__refresh", "--quiet"); code != 0 || !strings.Contains(stdout, "ha actualizado su plugin") {
		t.Fatalf("the hook refreshes an older plugin: %d %q", code, stdout)
	}
	if _, stdout, _ := runSetup("__refresh", "--quiet"); stdout != "" {
		t.Fatalf("nothing to refresh, nothing to say: %q", stdout)
	}
}

// UpdatePlugin asks Claude Code to copy the new plugin, and Codex only when it enables it; the hooks of the
// repository are rewritten.
func TestRefreshSetupAdapter(t *testing.T) {
	st := machine(t, true, true)
	ctx := context.Background()
	s := Setup{Home: os.Getenv("HOME"), Root: gitRepo(t), Agents: []string{Claude, Codex}, Version: "1"}
	a := RefreshSetupAdapter{}
	a.UpdatePlugin(ctx, s)
	if c := calls(st); len(c) != 1 || !strings.HasPrefix(c[0], "claude plugin update") {
		t.Fatalf("calls: %v", c)
	}
	config, _ := CodexPlugin(nil, false, true)
	write(t, s.CodexConfigFile(), string(config))
	a.UpdatePlugin(ctx, s)
	if !slices.Contains(calls(st), "codex plugin marketplace upgrade polix") {
		t.Fatalf("calls: %v", calls(st))
	}
	write(t, s.HooksFile(Claude), `{"hooks": {"Stop": [{"hooks": [{"type": "command", "command": "polix hook claude"}]}]}}`)
	if ch, err := a.RegisterHooks(ctx, s.HooksFile(Claude), Claude); err != nil || len(ch) != 1 || ch[0].Action != "updated" {
		t.Fatal(ch, err)
	}
	if got := a.ReadFiles(ctx, []string{s.HooksFile(Claude), s.HooksFile(Codex)}); len(got) != 1 {
		t.Fatalf("only the files that exist: %v", got)
	}
}

// Every release runs these commands of the next one after an update (internal/version), so they keep existing
// with these flags.
func TestCommandsTheUpdateRuns(t *testing.T) {
	for _, args := range [][]string{{"__refresh", "--quiet"}, {"__refresh", "--hooks"}, {"doctor", "--hook"},
		{"install", "--user", "--yes", "--quiet"}} {
		if code, _, stderr := runSetup(append(args, "--help")...); code != 0 {
			t.Errorf("polix %s: %d %q", strings.Join(args, " "), code, stderr)
		}
	}
}
