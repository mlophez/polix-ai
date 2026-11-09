package install

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/mlophez/polix-ai/internal/app/setup"
	"github.com/mlophez/polix-ai/internal/platform/testenv"
)

// install writes the plugin to the marketplace and registers it with Claude Code; a second install removes what a
// release dropped; uninstall takes it all back.
func TestAdapterClaude(t *testing.T) {
	st := testenv.Machine(t, true, false)
	a := Adapter{}
	ctx := context.Background()
	s := setup.Setup{Home: os.Getenv("HOME"), Agents: []string{setup.Claude}, Version: "1.2.3"}
	embedded, err := a.Plugin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	files, _ := setup.MarketplaceFiles(embedded, "1.2.3")
	if ch, err := a.WritePlugin(ctx, s.MarketplaceDir(), files); err != nil || len(ch) != len(files) {
		t.Fatal(len(ch), err)
	}
	ch, err := a.RegisterPlugin(ctx, s, setup.Claude)
	if err != nil || !slices.Equal(ch, []setup.Change{{Path: setup.PluginID + " in Claude Code", Action: "registered"}}) {
		t.Fatal(ch, err)
	}
	want := []string{"claude plugin marketplace add " + s.MarketplaceDir() + " --scope user --json",
		"claude plugin install polix@polix --scope user --json"}
	if !slices.Equal(testenv.Calls(st), want) {
		t.Fatalf("calls:\n%s", strings.Join(testenv.Calls(st), "\n"))
	}
	if ch, _ := a.RegisterPlugin(ctx, s, setup.Claude); ch != nil || !strings.HasPrefix(testenv.Calls(st)[4], "claude plugin update") {
		t.Fatalf("an installed plugin of another version is updated: %v %v", ch, testenv.Calls(st))
	}
	stale := filepath.Join(s.PluginDir(), "skills", "gone", "SKILL.md")
	testenv.Write(t, stale, "old")
	ch, err = a.WritePlugin(ctx, s.MarketplaceDir(), files)
	if err != nil || len(ch) != 1 || ch[0].Action != "removed" || ch[0].Path != stale {
		t.Fatal(ch, err)
	}
	if _, err := os.Stat(filepath.Dir(stale)); err == nil {
		t.Fatal("the emptied folder of a dropped skill is removed")
	}
	if _, err := os.Stat(filepath.Join(s.Home, ".claude")); err == nil {
		t.Fatal("install only touches ~/.claude through the CLI of Claude Code")
	}
}

// Without the CLIs, the commands are left to run by hand; Codex gets its plugin enabled in its configuration.
func TestAdapterWithoutCLIs(t *testing.T) {
	testenv.Machine(t, false, false)
	a := Adapter{}
	ctx := context.Background()
	s := setup.Setup{Home: os.Getenv("HOME"), Agents: []string{setup.Claude, setup.Codex}, Version: "1.2.3"}
	ch, err := a.RegisterPlugin(ctx, s, setup.Claude)
	if err != nil || len(ch) != 1 || !ch[0].Todo() || !strings.Contains(ch[0].Path, "claude plugin install polix@polix --scope user") {
		t.Fatal(ch, err)
	}
	ch, err = a.RegisterPlugin(ctx, s, setup.Codex)
	if err != nil || len(ch) != 2 || !ch[0].Todo() || ch[1].Path != s.CodexConfigFile() || !setup.CodexEnabled([]byte(testenv.Read(t, s.CodexConfigFile()))) {
		t.Fatal(ch, err)
	}
	if a.OnPath(ctx, "polix") == nil {
		t.Fatal("polix is not on the PATH")
	}
}

// The hooks of a symlinked settings file are written to its target, which keeps its mode.
func TestAdapterKeepsLinkAndMode(t *testing.T) {
	target := filepath.Join(t.TempDir(), "settings.json")
	if err := os.WriteFile(target, []byte(`{"model": "opus"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	settings := filepath.Join(t.TempDir(), ".claude", "settings.json")
	if err := os.MkdirAll(filepath.Dir(settings), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, settings); err != nil {
		t.Skipf("cannot create symlinks here: %v", err)
	}
	if _, err := (Adapter{}).RegisterHooks(context.Background(), settings, setup.Claude); err != nil {
		t.Fatal(err)
	}
	if info, err := os.Lstat(settings); err != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("the link stays: %v", err)
	}
	if info, _ := os.Stat(target); !setup.Registered([]byte(testenv.Read(t, target))) || runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
		t.Fatal("the target is written with its mode")
	}
	testenv.Write(t, settings, "[]")
	if _, err := (Adapter{}).RegisterHooks(context.Background(), settings, setup.Claude); err == nil ||
		!strings.HasPrefix(err.Error(), settings+": ") {
		t.Fatalf("the error names the file: %v", err)
	}
}
