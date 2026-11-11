package diagnose

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/mlophez/polix-ai/internal/app/rule"
	"github.com/mlophez/polix-ai/internal/app/setup"
	"github.com/mlophez/polix-ai/internal/app/version"
	"github.com/mlophez/polix-ai/internal/platform/git"
	"github.com/mlophez/polix-ai/internal/platform/testenv"
)

func TestAdapter(t *testing.T) {
	st := testenv.Machine(t, true, false)
	ctx := context.Background()
	// The rules and the cache come from their loaders, which read them in internal/app/rule/load and
	// internal/app/version/load; these answer what they are given and record what they were asked for.
	var asked []string
	cache := version.Cache{Latest: "0.2.0"}
	invalid := []string{"ai/rules/a.yaml A-2: no description"}
	a := Adapter{OS: runtime.GOOS, Look: exec.LookPath,
		Rules: func(_ context.Context, root, dir string) rule.Loaded {
			asked = append(asked, root+" "+dir)
			return rule.Loaded{Rules: []rule.Rule{{ID: "A-1", Description: "d", Deterministic: true,
				Check: "checks/missing.sh"}, {ID: "A-2"}}, Exists: true,
				Problems: invalid}
		},
		Cache: func(_ context.Context, path string) version.Loaded {
			asked = append(asked, path)
			return version.Loaded{Cache: cache, Exists: true}
		}}
	r := testenv.GitRepo(t)
	root, gitDir, err := a.OpenRepo(ctx, r)
	if err != nil || root != r {
		t.Fatal(root, err)
	}
	if dir, err := a.WriteState(ctx, gitDir); err != nil || filepath.Dir(dir) != gitDir {
		t.Fatal(dir, err)
	}
	testenv.Write(t, filepath.Join(r, "checks", "a.sh"), "#!/bin/sh\n")
	if runtime.GOOS != "windows" {
		if err := a.runnable(r, "checks/a.sh"); err == nil || err.Error() != "check checks/a.sh is not executable (chmod +x)" {
			t.Fatal(err)
		}
	}
	if a.runnable(r, "checks/missing.sh") != nil {
		t.Fatal("a missing check is only missing")
	}
	if installed, _, _, err := a.ClaudePlugin(ctx); err != nil || installed {
		t.Fatal(installed, err)
	}
	testenv.Write(t, filepath.Join(st, "installed"), "")
	if installed, enabled, folder, err := a.ClaudePlugin(ctx); err != nil || !installed || !enabled || folder != "" {
		t.Fatal(installed, enabled, folder, err)
	}
	if _, err := a.LookPath(ctx, "nothing-like-this"); err == nil {
		t.Fatal("look path")
	}
	if !a.SameFile(ctx, r, r) || a.SameFile(ctx, "", r) {
		t.Fatal("same file")
	}
	if got := a.LoadCache(ctx); got != cache {
		t.Fatalf("LoadCache = %+v", got)
	}
	if err := a.SaveCache(ctx, a.LoadCache(ctx)); err != nil {
		t.Fatal(err)
	}
	loaded, missing, runnable := a.LoadRules(ctx, r, "ai/rules")
	if loaded.Err != nil || len(loaded.Rules) != 2 || !slices.Equal(loaded.Problems, invalid) {
		t.Fatal(loaded)
	}
	if len(missing) != 1 || !missing["checks/missing.sh"] || len(runnable) != 1 || runnable["checks/missing.sh"] != nil {
		t.Fatal("a missing check is only missing, and only the rules with a check are looked at", missing, runnable)
	}
	if len(asked) != 3 || filepath.Base(asked[0]) != "update.json" || asked[2] != r+" ai/rules" {
		t.Fatalf("the loaders were asked for %v", asked)
	}
	t.Setenv("PATH", t.TempDir())
	if _, _, _, err := a.ClaudePlugin(ctx); !errors.Is(err, setup.ErrNoCLI) {
		t.Fatal(err)
	}
}

// A program of requirements is written without extension and found on every system: on Windows the real lookup
// tries the extensions of PATHEXT, so go finds go.exe; a made-up name is missing.
func TestAdapterLookPathRequirements(t *testing.T) {
	a := Adapter{OS: runtime.GOOS, Look: exec.LookPath}
	path, err := a.LookPath(context.Background(), "go")
	if err != nil || path == "" {
		t.Fatalf("go is not found without extension on %s: %q %v", runtime.GOOS, path, err)
	}
	if runtime.GOOS == "windows" && !strings.EqualFold(filepath.Ext(path), ".exe") {
		t.Fatalf("go is found through PATHEXT: %s", path)
	}
	if _, err := a.LookPath(context.Background(), "polix-no-such-program"); err == nil {
		t.Fatal("a made-up program is missing")
	}
}

func TestAdapterClaudePlugin(t *testing.T) {
	a := Adapter{}
	inst, en, folder, err := a.claudePlugin([]byte(`[{"id":"polix@polix","scope":"user","enabled":true,"readFromFolder":"/f"}]`))
	if !inst || !en || folder != "/f" || err != nil {
		t.Fatal(inst, en, folder, err)
	}
	if inst, _, folder, err := a.claudePlugin([]byte(`[{"id":"polix@polix","scope":"project"}]`)); inst || folder != "" || err != nil {
		t.Fatal("only the plugin of the user", inst, folder, err)
	}
	if _, _, _, err := a.claudePlugin([]byte("x")); err == nil || !strings.HasPrefix(err.Error(), "claude plugin list: ") {
		t.Fatal(err)
	}
}

// The hooks files are read and decoded: the handlers that run a command, in the groups that keep one; an empty
// settings file of Claude Code is not JSON, an empty hooks file of Codex has no hooks; a missing file is left out.
func TestAdapterHooks(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	settings, empty, broken := filepath.Join(dir, "settings.json"), filepath.Join(dir, "empty.json"), filepath.Join(dir, "broken.json")
	testenv.Write(t, settings, `{"permissions": {}, "hooks": {
		"PreToolUse": [{"matcher": "Edit|Bash", "hooks": [{"type": "command", "command": "polix hook claude", "timeout": 900}]}],
		"Stop": [{"hooks": [{"type": "command", "command": "echo hi"}, {"type": "prompt", "prompt": "p"}]},
			{"hooks": [{"type": "prompt", "prompt": "p"}]}]}}`)
	testenv.Write(t, empty, "")
	testenv.Write(t, broken, `{"hooks": []}`)
	paths := []string{settings, empty, broken, filepath.Join(dir, "missing.json")}
	claude := Adapter{}.HooksFiles(ctx, setup.Claude, paths)
	pre, stop := claude[settings].Hooks["PreToolUse"], claude[settings].Hooks["Stop"]
	if len(claude) != 3 || len(claude[settings].Hooks) != 2 || len(pre) != 1 || pre[0].Matcher != "Edit|Bash" ||
		len(pre[0].Handlers) != 1 || pre[0].Handlers[0] != (setup.HookHandler{Command: "polix hook claude", Timeout: 900}) {
		t.Fatalf("%+v", claude)
	}
	if len(stop) != 1 || stop[0].Matcher != "" || len(stop[0].Handlers) != 1 || stop[0].Handlers[0].Command != "echo hi" {
		t.Fatalf("a handler without a command and a group without handlers are left out: %+v", stop)
	}
	if claude[empty].Err == nil || claude[broken].Err == nil || claude[settings].Err != nil {
		t.Fatalf("an empty file and hooks that are not an object are not settings: %+v", claude)
	}
	codex := Adapter{}.HooksFiles(ctx, setup.Codex, paths)
	if len(codex) != 3 || codex[empty].Err != nil || codex[broken].Err == nil {
		t.Fatalf("an empty hooks file of Codex has no hooks: %+v", codex)
	}
	if runtime.GOOS != "windows" {
		testenv.Write(t, settings, "{}")
		if err := os.Chmod(settings, 0o000); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = os.Chmod(settings, 0o600) })
		if f := (Adapter{}).HooksFiles(ctx, setup.Claude, []string{settings})[settings]; f.ReadErr == nil && os.Geteuid() != 0 {
			t.Fatalf("a file that cannot be read: %+v", f)
		}
	}
}

// The configuration of Codex enables the plugin, and the manifest of the marketplace is the plugin polix.
func TestAdapterCodexAndManifest(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	config, manifest := filepath.Join(dir, "config.toml"), filepath.Join(dir, "plugin.json")
	if (Adapter{}).CodexEnabled(ctx, config) {
		t.Fatal("no configuration, nothing enabled")
	}
	testenv.Write(t, config, "[plugins.\"polix@polix\"]\nenabled = true\n")
	if !(Adapter{}).CodexEnabled(ctx, config) {
		t.Fatal("enabled")
	}
	if _, ok := (Adapter{}).readManifest(manifest); ok {
		t.Fatal("no manifest")
	}
	testenv.Write(t, manifest, `{"name": "polix", "version": "0.2.0"}`)
	if v, ok := (Adapter{}).readManifest(manifest); !ok || v != "0.2.0" {
		t.Fatal(v, ok)
	}
	if embedded, v, written, err := (Adapter{}).Plugin(ctx, manifest); err != nil || len(embedded) == 0 || !written ||
		v != "0.2.0" {
		t.Fatal(len(embedded), v, written, err)
	}
	testenv.Write(t, manifest, `{"name": "other", "version": "1"}`)
	if _, ok := (Adapter{}).readManifest(manifest); ok {
		t.Fatal("only the plugin polix")
	}
}

// On Windows a check needs its interpreter: a .py check without python on the PATH cannot run, whatever the
// system the test runs on; a missing check is only missing.
func TestAdapterRunnableOnWindows(t *testing.T) {
	r := t.TempDir()
	testenv.Write(t, filepath.Join(r, "checks", "a.py"), "print(1)\n")
	a := Adapter{OS: "windows", Look: func(string) (string, error) { return "", exec.ErrNotFound }}
	if err := a.runnable(r, "checks/a.py"); err == nil || !strings.Contains(err.Error(), "python") {
		t.Fatal(err)
	}
	if err := a.runnable(r, "checks/b.py"); err != nil {
		t.Fatal(err)
	}
}

// SaveCache writes update.json in POLIX_CACHE_DIR with the same bytes as UpdateVersion, the failure included, so
// both writers and the loader of internal/app/version/load agree on the shape of the file.
func TestAdapterSaveCache(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("POLIX_CACHE_DIR", filepath.Join(dir, "polix"))
	path := filepath.Join(dir, "polix", "update.json")
	now := time.Now().UTC().Truncate(time.Second)
	stamp := now.Format(time.RFC3339)
	a := Adapter{}
	if err := a.SaveCache(context.Background(), version.Cache{CheckedAt: now, Latest: "0.2.0"}); err != nil {
		t.Fatal(err)
	}
	if got, want := readCache(t, path), fmt.Sprintf(`{"checked_at":%q,"latest":"0.2.0"}`, stamp); got != want {
		t.Fatalf("the cache without a failure: %s, want %s", got, want)
	}
	if err := a.SaveCache(context.Background(), version.Cache{CheckedAt: now, Latest: "0.2.0"}.Fail(now)); err != nil {
		t.Fatal(err)
	}
	want := fmt.Sprintf(`{"checked_at":%q,"latest":"0.2.0","failed_at":%q}`, stamp, stamp)
	if got := readCache(t, path); got != want {
		t.Fatalf("the failure is kept: %s, want %s", got, want)
	}
}

// AC15: the adapter reads the hooks git lists for each event, after the git library wrote those of polix, and
// whether this git lists them at all; a git that lists them names the hooks of polix, and one with enabled=false
// apart, as disabled.
func TestAdapterGitHooks(t *testing.T) {
	ctx := context.Background()
	root := testenv.GitRepo(t)
	repo, err := git.Open(ctx, root)
	if err != nil {
		t.Fatal(err)
	}
	for _, h := range setup.GitHooks {
		if _, err := repo.SetHook(ctx, h.Name(), string(h), h.Command()); err != nil {
			t.Fatal(err)
		}
	}
	listed, disabled, ok := Adapter{}.GitHooks(ctx, root, setup.GitHooks)
	if listed == nil || len(disabled) != 0 {
		t.Fatalf("git was not asked, or lists a hook as disabled: %q %q", listed, disabled)
	}
	for _, h := range setup.GitHooks {
		if ok != slices.Contains(listed[string(h)], h.Name()) {
			t.Fatalf("lists %v, %s: %q", ok, h, listed)
		}
	}
	if !ok {
		return
	}
	if err := exec.Command("git", "-C", root, "config", "--local", "hook.polix-pre-commit.enabled", "false").Run(); err != nil {
		t.Fatal(err)
	}
	listed, disabled, ok = Adapter{}.GitHooks(ctx, root, setup.GitHooks)
	if !ok || slices.Contains(listed["pre-commit"], "polix-pre-commit") ||
		!slices.Equal(disabled["pre-commit"], []string{"polix-pre-commit"}) ||
		!slices.Contains(listed["commit-msg"], "polix-commit-msg") || len(disabled["commit-msg"]) != 0 {
		t.Fatalf("a disabled hook: %q %q %v", listed, disabled, ok)
	}
}

// readCache returns the content of the cache file at path.
func readCache(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}
