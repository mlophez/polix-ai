// Package setup tests the commands of setup end to end, composed as cmd/polix composes them: install, uninstall,
// doctor and the hidden __refresh, on a machine of their own (internal/platform/testenv) with the fake CLIs of the
// agents. The tests of each use case stay in its package, which never imports another use case.
package setup

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mlophez/polix-ai/internal/app/setup"
	setupdiagnose "github.com/mlophez/polix-ai/internal/app/setup/diagnose"
	setupinstall "github.com/mlophez/polix-ai/internal/app/setup/install"
	setuprefresh "github.com/mlophez/polix-ai/internal/app/setup/refresh"
	setupuninstall "github.com/mlophez/polix-ai/internal/app/setup/uninstall"
	"github.com/mlophez/polix-ai/internal/platform/cli"
	"github.com/mlophez/polix-ai/internal/platform/testenv"
)

// runSetup runs the commands of setup as cmd/polix composes them, with their exit codes.
func runSetup(args ...string) (int, string, string) {
	root := cli.Root("polix", "short", "long", "")
	root.AddCommand(setupinstall.Command(), setupuninstall.Command(), setuprefresh.Command(), setupdiagnose.Command())
	var out, errb bytes.Buffer
	code := cli.Run(root, args, bytes.NewReader(nil), &out, &errb)
	return code, out.String(), errb.String()
}

// install and uninstall end to end: install outside a repository, again, in a repository with its settings, the
// duplicates of the user, an unknown agent, and uninstall.
func TestInstall(t *testing.T) {
	testenv.Machine(t, false, false)
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
	r := testenv.GitRepo(t)
	testenv.Write(t, filepath.Join(r, ".claude", "settings.json"), `{"permissions": {"allow": ["Bash(go test:*)"]}}`)
	code, stdout, stderr = runSetup("install", "--yes", "--agent", "claude,codex", "--dir", r)
	if code != 0 || !strings.Contains(stdout, "commit them") || !strings.Contains(stdout, "/hooks") || stderr != "" ||
		!strings.Contains(testenv.Read(t, filepath.Join(r, ".claude", "settings.json")), "Bash(go test:*)") {
		t.Fatalf("install in a repository: %d\n%s%s", code, stdout, stderr)
	}
	data := testenv.Read(t, filepath.Join("..", "..", "examples", "claude-settings.json"))
	testenv.Write(t, filepath.Join(os.Getenv("HOME"), ".claude", "settings.json"), data)
	if _, _, stderr := runSetup("install", "--yes", "--agent", "claude", "--dir", r); !strings.Contains(stderr, "runs them twice") {
		t.Fatalf("install warns about the duplicated hooks: %q", stderr)
	}
	if code, _, stderr := runSetup("install", "--agent", "cursor"); code != 1 || !strings.Contains(stderr, `polix: unknown agent "cursor" (claude, codex)`) {
		t.Fatalf("unknown agent: %d %s", code, stderr)
	}
	if code, _, _ := runSetup("uninstall", "--yes", "--dir", r); code != 0 ||
		setup.Registered([]byte(testenv.Read(t, filepath.Join(r, ".claude", "settings.json")))) {
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

// doctor end to end: in a repository after install, doctor finds the hooks; with the hooks of the user too, they
// run twice; outside a repository only the machine.
func TestDoctor(t *testing.T) {
	testenv.Machine(t, false, false)
	r := testenv.GitRepo(t)
	testenv.Write(t, filepath.Join(r, "polix.yaml"), "version: 1\n")
	if code, _, stderr := runSetup("install", "--yes", "--agent", "claude", "--dir", r); code != 0 {
		t.Fatal(stderr)
	}
	code, stdout, _ := runSetup("doctor", "--offline", "--dir", r)
	if !strings.Contains(stdout, "hooks     registered in") || !strings.Contains(stdout, "polix.yaml is valid") ||
		code != 1 || !strings.Contains(stdout, "polix is not on the PATH") {
		t.Fatalf("doctor after install: %d\n%s", code, stdout)
	}
	data := testenv.Read(t, filepath.Join(r, ".claude", "settings.json"))
	testenv.Write(t, filepath.Join(os.Getenv("HOME"), ".claude", "settings.json"), data)
	if _, stdout, _ := runSetup("doctor", "--offline", "--dir", r); !strings.Contains(stdout, "runs polix twice") {
		t.Fatalf("duplicated hooks:\n%s", stdout)
	}
	if code, stdout, _ := runSetup("doctor", "--hook", "--dir", r); code != 0 || !strings.Contains(stdout, "cuéntaselos") {
		t.Fatalf("the hook never fails: %d\n%s", code, stdout)
	}
	if _, stdout, _ := runSetup("doctor", "--offline", "--dir", t.TempDir()); !strings.Contains(stdout, "not inside a git repository") {
		t.Fatalf("outside:\n%s", stdout)
	}
}

// The hidden command the update runs: the hook refreshes only a plugin already installed, with one line when it
// changed; a person gets the plugin with install --user and __refresh --hooks.
func TestRefresh(t *testing.T) {
	testenv.Machine(t, false, false)
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
	s := setup.Setup{Home: os.Getenv("HOME"), Version: "dev"}
	testenv.Write(t, s.PluginManifest(), `{"name": "polix", "version": "0.1.0"}`)
	if code, stdout, _ := runSetup("__refresh", "--quiet"); code != 0 || !strings.Contains(stdout, "ha actualizado su plugin") {
		t.Fatalf("the hook refreshes an older plugin: %d %q", code, stdout)
	}
	if _, stdout, _ := runSetup("__refresh", "--quiet"); stdout != "" {
		t.Fatalf("nothing to refresh, nothing to say: %q", stdout)
	}
}

// Every release runs these commands of the next one after an update (internal/app/version), so they keep existing
// with these flags.
func TestCommandsTheUpdateRuns(t *testing.T) {
	for _, args := range [][]string{{"__refresh", "--quiet"}, {"__refresh", "--hooks"}, {"doctor", "--hook"},
		{"install", "--user", "--yes", "--quiet"}} {
		if code, _, stderr := runSetup(append(args, "--help")...); code != 0 {
			t.Errorf("polix %s: %d %q", strings.Join(args, " "), code, stderr)
		}
	}
}
