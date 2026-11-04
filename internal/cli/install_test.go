package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mlophez/polix-ai/internal/setup"
)

// userOptions are the options of the home of the tests.
func userOptions() setup.Options { return setup.Options{Home: os.Getenv("HOME")} }

func pluginDir() string { return userOptions().MarketplaceDir() }

func pluginVersion() (string, bool) { return setup.PluginVersion(userOptions()) }

// Outside a repository install only writes the plugin of the user, and says how to register it when the CLI of
// Claude Code is missing; it is idempotent, and uninstall removes the plugin.
func TestInstallUser(t *testing.T) {
	fakeBinary(t, "0.2.0")
	code, stdout, stderr := runCmd("install", "--yes", "--agent", "claude", "--dir", t.TempDir())
	if code != 0 || !strings.Contains(stdout, "polix 0.2.0 installed") || !strings.Contains(stdout, "/polix:add-rule") ||
		!strings.Contains(stdout, "polix:rules-reviewer") || !strings.Contains(stdout, "not inside a git repository") ||
		!strings.Contains(stdout, "claude plugin install polix@polix") {
		t.Fatalf("install: %d\n%s%s", code, stdout, stderr)
	}
	if v, ok := pluginVersion(); !ok || v != "0.2.0" {
		t.Fatalf("plugin: %q %v", v, ok)
	}
	code, stdout, _ = runCmd("install", "--yes", "--agent", "claude", "--user")
	if code != 0 || strings.Contains(stdout, "created") ||
		strings.Contains(stdout, "repository") {
		t.Fatalf("second install: %d\n%s", code, stdout)
	}
	code, stdout, _ = runCmd("uninstall", "--yes", "--user")
	if code != 0 || !strings.Contains(stdout, "removed") {
		t.Fatalf("uninstall: %d\n%s", code, stdout)
	}
	if _, err := os.Stat(pluginDir()); err == nil {
		t.Fatal("uninstall removes the plugin")
	}
	_, stdout, _ = runCmd("uninstall", "--yes", "--user")
	if !strings.Contains(stdout, "nothing to remove") {
		t.Fatalf("second uninstall: %s", stdout)
	}
}

// In a repository install registers the hooks in its settings, which doctor accepts, and the skills for the user.
func TestInstallInARepository(t *testing.T) {
	fakeBinary(t, "0.2.0")
	oldLook := lookPath
	lookPath = func(name string) (string, error) { return "/bin/" + name, nil } // polix and codex are here
	t.Cleanup(func() { lookPath = oldLook })
	r := newRepo(t, "main")
	r.write("polix.yaml", "version: 1\n")
	r.write(".claude/settings.json", `{"permissions": {"allow": ["Bash(go test:*)"]}}`)

	code, stdout, stderr := r.run("install", "--yes")
	if code != 0 || !strings.Contains(stdout, "commit them") || !strings.Contains(stdout, "/hooks") || stderr != "" {
		t.Fatalf("install: %d\n%s%s", code, stdout, stderr)
	}
	settings := filepath.Join(r.dir, ".claude", "settings.json")
	if !setup.Registered(settings) || !setup.Registered(filepath.Join(r.dir, ".codex", "hooks.json")) {
		t.Fatal("the hooks of both agents are registered in the repository")
	}
	if data, _ := os.ReadFile(settings); !strings.Contains(string(data), "Bash(go test:*)") {
		t.Fatalf("the settings of the project are kept:\n%s", data)
	}
	if _, ok := pluginVersion(); !ok {
		t.Fatal("the plugin goes to the user")
	}
	if _, err := os.Stat(filepath.Join(r.dir, ".claude", "skills")); err == nil {
		t.Fatal("no plugin in the repository")
	}
	lookPath = oldLook
	healthyMachine(t)
	if code, stdout, _ := r.run("doctor", "--offline"); code != 0 || strings.Contains(stdout, "fail") {
		t.Fatalf("doctor after install: %d\n%s", code, stdout)
	}

	// The same hooks in the settings of the user too run twice.
	data, _ := os.ReadFile(filepath.Join("..", "..", "examples", "claude-settings.json"))
	os.MkdirAll(filepath.Join(os.Getenv("HOME"), ".claude"), 0o755)
	os.WriteFile(filepath.Join(os.Getenv("HOME"), ".claude", "settings.json"), data, 0o644)
	if code, stdout, _ := r.run("doctor", "--offline"); code != 1 || !strings.Contains(stdout, "runs polix twice") {
		t.Fatalf("doctor with duplicated hooks: %d\n%s", code, stdout)
	}
	if _, _, stderr := r.run("install", "--yes", "--agent", "claude"); !strings.Contains(stderr, "runs them twice") {
		t.Fatalf("install warns about the duplicated hooks: %q", stderr)
	}

	if code, _, stderr := r.run("install", "--agent", "cursor"); code != 1 || !strings.Contains(stderr, "unknown agent") {
		t.Fatalf("unknown agent: %d %s", code, stderr)
	}
	if code, _, _ := r.run("uninstall", "--yes"); code != 0 || setup.Registered(settings) {
		t.Fatal("uninstall removes the hooks of the repository")
	}
}

// A person who updates gets the plugin of the user and the hooks of the repository refreshed; the session start
// hook only refreshes the plugin already installed.
func TestUpdateRefreshesThePlugin(t *testing.T) {
	outsideRepo(t)
	newFakeRelease(t, []byte("new"))
	fakeBinary(t, "0.2.0")

	code, stdout, stderr := runCmd("update", "--hook")
	if code != 0 || withoutDoctor(stdout) != "" {
		t.Fatalf("the hook installs nothing new: %d %q %q", code, stdout, stderr)
	}
	if _, err := os.Stat(pluginDir()); err == nil {
		t.Fatal("the hook must not install the plugin")
	}

	code, stdout, stderr = runCmd("update")
	if code != 0 || !strings.Contains(stdout, "up to date") {
		t.Fatalf("update: %d %q %q", code, stdout, stderr)
	}
	if v, ok := pluginVersion(); !ok || v != "0.2.0" {
		t.Fatalf("update installs the plugin of the user: %q %v", v, ok)
	}

	Version = "0.2.1" // as if another binary had been installed without polix install
	code, stdout, _ = runCmd("update", "--hook")
	if code != 0 || !strings.Contains(stdout, "ha actualizado su plugin") {
		t.Fatalf("the hook refreshes an older plugin: %d %q", code, stdout)
	}
	if v, _ := pluginVersion(); v != "0.2.1" {
		t.Fatalf("plugin version after the refresh: %q", v)
	}
	_, stdout, _ = runCmd("update", "--hook")
	if withoutDoctor(stdout) != "" {
		t.Fatalf("nothing to refresh, nothing to say: %q", stdout)
	}
}

// refreshInstalled rewrites the hooks a repository registers, and only with hooks.
func TestRefreshTheHooksOfTheRepository(t *testing.T) {
	fakeBinary(t, "0.2.0")
	r := newRepo(t, "main")
	stale := `{"hooks": {"Stop": [{"hooks": [{"type": "command", "command": "polix hook claude"}]}]}}`
	r.write(".claude/settings.json", stale)
	settings := filepath.Join(r.dir, ".claude", "settings.json")
	var out strings.Builder
	if err := refreshInstalled(r.dir, &out, true, false); err != nil || out.String() != "" {
		t.Fatalf("without hooks: %v %q", err, out.String())
	}
	if err := refreshInstalled(r.dir, &out, false, true); err != nil || !strings.Contains(out.String(), "updated") {
		t.Fatalf("with hooks: %v %q", err, out.String())
	}
	if hooks, _ := setup.ReadHooks(settings); len(hooks) != len(setup.Hooks(setup.Claude)) {
		t.Fatalf("every hook of polix: %v", hooks)
	}
	if _, err := os.Stat(filepath.Join(r.dir, ".codex")); err == nil {
		t.Fatal("hooks the repository does not register are not added")
	}
}
