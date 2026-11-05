package setup

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/mlophez/polix-ai/pkg/cli"
)

// runSetup runs the commands of setup as cmd/polix composes them, with their exit codes.
func runSetup(args ...string) (int, string, string) {
	root := cli.Root("polix", "short", "long", "")
	root.AddCommand(InstallSetupCommand(), UninstallSetupCommand(), RefreshSetupCommand(), DiagnoseSetupCommand())
	var out, errb bytes.Buffer
	code := cli.Run(root, args, bytes.NewReader(nil), &out, &errb)
	return code, out.String(), errb.String()
}

// install writes the plugin to the marketplace and registers it with Claude Code; a second install removes what a
// release dropped; uninstall takes it all back.
func TestInstallSetupAdapterClaude(t *testing.T) {
	st := machine(t, true, false)
	a := InstallSetupAdapter{}
	ctx := context.Background()
	s := Setup{Home: os.Getenv("HOME"), Agents: []string{Claude}, Version: "1.2.3"}
	embedded, err := a.Plugin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	files, _ := MarketplaceFiles(embedded, "1.2.3")
	if ch, err := a.WritePlugin(ctx, s.MarketplaceDir(), files); err != nil || len(ch) != len(files) {
		t.Fatal(len(ch), err)
	}
	ch, err := a.RegisterPlugin(ctx, s, Claude)
	if err != nil || !slices.Equal(ch, []Change{{Path: PluginID + " in Claude Code", Action: "registered"}}) {
		t.Fatal(ch, err)
	}
	want := []string{"claude plugin marketplace add " + s.MarketplaceDir() + " --scope user --json",
		"claude plugin install polix@polix --scope user --json"}
	if !slices.Equal(calls(st), want) {
		t.Fatalf("calls:\n%s", strings.Join(calls(st), "\n"))
	}
	if ch, _ := a.RegisterPlugin(ctx, s, Claude); ch != nil || !strings.HasPrefix(calls(st)[4], "claude plugin update") {
		t.Fatalf("an installed plugin of another version is updated: %v %v", ch, calls(st))
	}
	stale := filepath.Join(s.PluginDir(), "skills", "gone", "SKILL.md")
	write(t, stale, "old")
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
func TestInstallSetupAdapterWithoutCLIs(t *testing.T) {
	machine(t, false, false)
	a := InstallSetupAdapter{}
	ctx := context.Background()
	s := Setup{Home: os.Getenv("HOME"), Agents: []string{Claude, Codex}, Version: "1.2.3"}
	ch, err := a.RegisterPlugin(ctx, s, Claude)
	if err != nil || len(ch) != 1 || !ch[0].Todo() || !strings.Contains(ch[0].Path, "claude plugin install polix@polix --scope user") {
		t.Fatal(ch, err)
	}
	ch, err = a.RegisterPlugin(ctx, s, Codex)
	if err != nil || len(ch) != 2 || !ch[0].Todo() || ch[1].Path != s.CodexConfigFile() || !CodexEnabled([]byte(read(t, s.CodexConfigFile()))) {
		t.Fatal(ch, err)
	}
	if a.CodexUsed(ctx, s.Home, "") != true || a.OnPath(ctx, "polix") == nil {
		t.Fatal("~/.codex exists now, and polix is not on the PATH")
	}
}

// The hooks of a symlinked settings file are written to its target, which keeps its mode.
func TestInstallSetupAdapterKeepsLinkAndMode(t *testing.T) {
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
	if _, err := (InstallSetupAdapter{}).RegisterHooks(context.Background(), settings, Claude); err != nil {
		t.Fatal(err)
	}
	if info, err := os.Lstat(settings); err != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("the link stays: %v", err)
	}
	if info, _ := os.Stat(target); !Registered([]byte(read(t, target))) || runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
		t.Fatal("the target is written with its mode")
	}
	write(t, settings, "[]")
	if _, err := (InstallSetupAdapter{}).RegisterHooks(context.Background(), settings, Claude); err == nil ||
		!strings.HasPrefix(err.Error(), settings+": ") {
		t.Fatalf("the error names the file: %v", err)
	}
}

// The commands end to end: install outside a repository, again, in a repository with its settings, the
// duplicates of the user, an unknown agent, and uninstall.
func TestInstallSetupCommand(t *testing.T) {
	machine(t, false, false)
	code, stdout, stderr := runSetup("install", "--yes", "--agent", "claude", "--dir", t.TempDir())
	if code != 0 || !strings.Contains(stdout, "polix dev installed") || !strings.Contains(stdout, "/polix:add-rule") ||
		!strings.Contains(stdout, "polix:rules-reviewer") || !strings.Contains(stdout, "not inside a git repository") ||
		!strings.Contains(stdout, "run by hand: claude plugin marketplace add") || !strings.Contains(stdout, "polix is not on the PATH") {
		t.Fatalf("install: %d\n%s%s", code, stdout, stderr)
	}
	code, stdout, _ = runSetup("install", "--yes", "--agent", "claude", "--user")
	if code != 0 || strings.Contains(stdout, "created") || strings.Contains(stdout, "repository") {
		t.Fatalf("second install: %d\n%s", code, stdout)
	}
	r := gitRepo(t)
	write(t, filepath.Join(r, ".claude", "settings.json"), `{"permissions": {"allow": ["Bash(go test:*)"]}}`)
	code, stdout, stderr = runSetup("install", "--yes", "--agent", "claude,codex", "--dir", r)
	if code != 0 || !strings.Contains(stdout, "commit them") || !strings.Contains(stdout, "/hooks") || stderr != "" ||
		!strings.Contains(read(t, filepath.Join(r, ".claude", "settings.json")), "Bash(go test:*)") {
		t.Fatalf("install in a repository: %d\n%s%s", code, stdout, stderr)
	}
	data := read(t, filepath.Join("..", "..", "examples", "claude-settings.json"))
	write(t, filepath.Join(os.Getenv("HOME"), ".claude", "settings.json"), data)
	if _, _, stderr := runSetup("install", "--yes", "--agent", "claude", "--dir", r); !strings.Contains(stderr, "runs them twice") {
		t.Fatalf("install warns about the duplicated hooks: %q", stderr)
	}
	if code, _, stderr := runSetup("install", "--agent", "cursor"); code != 1 || !strings.Contains(stderr, `polix: unknown agent "cursor" (claude, codex)`) {
		t.Fatalf("unknown agent: %d %s", code, stderr)
	}
	if code, _, _ := runSetup("uninstall", "--yes", "--dir", r); code != 0 ||
		Registered([]byte(read(t, filepath.Join(r, ".claude", "settings.json")))) {
		t.Fatal("uninstall removes the hooks of the repository")
	}
	if _, err := os.Stat(filepath.Join(r, ".codex", "hooks.json")); err == nil {
		t.Fatal("a hooks file of polix alone is removed")
	}
	if _, err := os.Stat(filepath.Join(os.Getenv("HOME"), ".polix")); err == nil {
		t.Fatal("uninstall of both agents removes the marketplace")
	}
	if _, stdout, _ := runSetup("uninstall", "--yes", "--user"); !strings.Contains(stdout, "nothing to remove") {
		t.Fatalf("second uninstall: %s", stdout)
	}
}
