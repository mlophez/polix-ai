package install

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/mlophez/polix-ai/internal/app/setup"
	"github.com/mlophez/polix-ai/internal/platform/agentconf"
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
	files, err := a.MarketplaceFiles(ctx, setup.Plugin{Embedded: embedded, Version: "1.2.3"})
	if err != nil || files[".claude-plugin/marketplace.json"] == nil ||
		!strings.Contains(string(files["polix/.claude-plugin/plugin.json"]), `"version": "1.2.3"`) {
		t.Fatal(err)
	}
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
	if err != nil || len(ch) != 2 || !ch[0].Todo() || ch[1].Path != s.CodexConfigFile() || !agentconf.CodexEnabled([]byte(testenv.Read(t, s.CodexConfigFile())), setup.PluginID) {
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
	ctx := context.Background()
	hooks := (setup.Setup{}).Hooks(setup.Claude)
	if _, err := (Adapter{}).RegisterHooks(ctx, settings, hooks); err != nil {
		t.Fatal(err)
	}
	if info, err := os.Lstat(settings); err != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("the link stays: %v", err)
	}
	if info, _ := os.Stat(target); !(Adapter{}).Registered(ctx, []string{target})[target] ||
		runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
		t.Fatal("the target is written with its mode")
	}
	testenv.Write(t, settings, "[]")
	if _, err := (Adapter{}).RegisterHooks(ctx, settings, hooks); err == nil ||
		!strings.HasPrefix(err.Error(), settings+": ") {
		t.Fatalf("the error names the file: %v", err)
	}
}

// The example hooks files of the repository are, byte for byte, what install writes for each agent in an empty
// repository; a second registration changes nothing.
func TestAdapterHooksExamples(t *testing.T) {
	ctx := context.Background()
	for agent, example := range map[string]string{setup.Claude: "claude-settings.json", setup.Codex: "codex-hooks.json"} {
		path := filepath.Join(t.TempDir(), "hooks.json")
		ch, err := (Adapter{}).RegisterHooks(ctx, path, (setup.Setup{}).Hooks(agent))
		if err != nil || len(ch) != 1 || ch[0].Action != "created" {
			t.Fatal(ch, err)
		}
		if want := testenv.Read(t, filepath.Join("..", "..", "..", "..", "examples", example)); want != testenv.Read(t, path) {
			t.Errorf("examples/%s differs from what install writes:\n%s", example, testenv.Read(t, path))
		}
		if ch, err := (Adapter{}).RegisterHooks(ctx, path, (setup.Setup{}).Hooks(agent)); err != nil || ch != nil {
			t.Fatalf("a second registration changes nothing: %v %v", ch, err)
		}
	}
}

// The session start command of an earlier build is replaced in place, not kept next to the new one; the hooks of
// the user stay; the files that do not exist or register no hook of polix are not registered.
func TestAdapterRegisterHooksReplacesALegacyCommand(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "settings.json")
	testenv.Write(t, path, `{"hooks": {"SessionStart": [
  {"hooks": [{"type": "command", "command": "polix hook claude", "timeout": 900}]},
  {"matcher": "startup", "hooks": [{"type": "command", "command": "polix update --hook; polix doctor --hook", "timeout": 180}]}
], "Stop": [{"hooks": [{"type": "command", "command": "notify-send done"}]}]}}`)
	if _, err := (Adapter{}).RegisterHooks(ctx, path, (setup.Setup{}).Hooks(setup.Claude)); err != nil {
		t.Fatal(err)
	}
	got := testenv.Read(t, path)
	if strings.Contains(got, "polix doctor --hook") || strings.Count(got, `"`+setup.UpdateCommand+`"`) != 1 ||
		!strings.Contains(got, "notify-send done") {
		t.Fatalf("the legacy command is replaced:\n%s", got)
	}
	other := filepath.Join(t.TempDir(), "other.json")
	testenv.Write(t, other, `{"hooks": {"Stop": [{"hooks": [{"type": "command", "command": "notify-send done"}]}]}}`)
	registered := (Adapter{}).Registered(ctx, []string{path, other, filepath.Join(t.TempDir(), "none.json")})
	if !registered[path] || registered[other] || len(registered) != 2 {
		t.Fatalf("registered: %v", registered)
	}
}

// A manifest of the plugin without version cannot be written to the marketplace.
func TestAdapterMarketplaceFilesBroken(t *testing.T) {
	broken := setup.Plugin{Embedded: map[string][]byte{"polix/.claude-plugin/plugin.json": []byte("{}")}, Version: "1.2.3"}
	if _, err := (Adapter{}).MarketplaceFiles(context.Background(), broken); err == nil {
		t.Fatal("a plugin without version cannot be installed")
	}
}

func TestAdapterTodos(t *testing.T) {
	a := Adapter{}
	if got := a.claudeTodo("/a b"); !strings.Contains(got, `add "/a b" --scope user && claude plugin install polix@polix`) {
		t.Fatal(got)
	}
	if got := a.codexTodo("/m", errors.New("boom\nmore")); got != "codex plugin marketplace add /m (it failed: boom)" {
		t.Fatal(got)
	}
	if !a.codexAdded("", errors.New("already added")) || a.codexAdded("", errors.New("boom")) || !a.codexAdded("", nil) {
		t.Fatal("added")
	}
}
