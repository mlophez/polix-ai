package cli

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"

	"github.com/mlophez/polix-ai/internal/setup"
	"github.com/mlophez/polix-ai/internal/update"
)

// healthyMachine makes doctor see a released polix on the PATH, the latest one, and an empty home.
func healthyMachine(t *testing.T) string {
	t.Helper()
	exe := fakeBinary(t, "0.2.0")
	if err := update.SaveCache(update.Cache{Latest: "0.2.0"}); err != nil {
		t.Fatal(err)
	}
	oldLook := lookPath
	lookPath = func(name string) (string, error) {
		if name == "polix" {
			return exe, nil
		}
		return exec.LookPath(name)
	}
	t.Cleanup(func() { lookPath = oldLook })
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", os.Getenv("HOME"))
	if _, err := setup.Install(setup.Options{Home: os.Getenv("HOME"), Version: "0.2.0"}, []string{setup.Claude}); err != nil {
		t.Fatal(err)
	}
	return exe
}

// readyRepo is a repository with a valid workflow and the example settings of Claude Code.
func readyRepo(t *testing.T) *repo {
	t.Helper()
	r := newRepo(t, "main")
	r.write("polix.yaml", "version: 1\n")
	data, err := os.ReadFile(filepath.Join("..", "..", "examples", "claude-settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	r.write(".claude/settings.json", string(data))
	return r
}

func TestDoctorHealthy(t *testing.T) {
	healthyMachine(t)
	r := readyRepo(t)
	code, stdout, stderr := r.run("doctor", "--offline")
	if code != 0 || strings.Contains(stdout, "warn") || strings.Contains(stdout, "fail") {
		t.Fatalf("doctor: %d\n%s%s", code, stdout, stderr)
	}
	code, stdout, _ = r.run("doctor", "--hook")
	if code != 0 || stdout != "" {
		t.Fatalf("a healthy hook says nothing: %d %q", code, stdout)
	}
}

// doctor warns that rules check --changed would fail when no base branch exists and a rule has a check.
func TestDoctorMissingBase(t *testing.T) {
	healthyMachine(t)
	r := readyRepo(t)
	r.git("branch", "-m", "main", "trunk")
	if _, stdout, _ := r.run("doctor", "--offline"); strings.Contains(stdout, "would fail") {
		t.Fatalf("no deterministic rule, no warning: %s", stdout)
	}
	r.write("ai/rules/style.yaml", "rules:\n  - id: S-1\n    description: d\n    scope: [\"src/**\"]\n    deterministic: true\n    check: checks/ok.sh\n")
	r.write("checks/ok.sh", "#!/bin/sh\nexit 0\n")
	if _, stdout, _ := r.run("doctor", "--offline"); !strings.Contains(stdout, "rules check --changed would fail") {
		t.Fatalf("a deterministic rule without main: %s", stdout)
	}
}

// The session start command of an earlier build, or none, means polix does not keep itself up to date.
func TestDoctorFindsAnOutdatedUpdateHook(t *testing.T) {
	healthyMachine(t)
	r := readyRepo(t)
	settings := filepath.Join(r.dir, ".claude", "settings.json")
	data, _ := os.ReadFile(settings)
	r.write(".claude/settings.json", strings.Replace(string(data), `"polix update --hook"`, `"polix update --hook; polix doctor --hook"`, 1))
	if _, stdout, _ := r.run("doctor", "--offline"); !strings.Contains(stdout, "command of an earlier polix") {
		t.Fatalf("legacy command: %s", stdout)
	}
	r.write(".claude/settings.json", strings.Replace(string(data), `"polix update --hook"`, `"echo hi"`, 1))
	if _, stdout, _ := r.run("doctor", "--offline"); !strings.Contains(stdout, "does not update itself") {
		t.Fatalf("no update command: %s", stdout)
	}
}

// Settings written before polix registered PostToolUseFailure only lose the feedback of failed commands: a warning.
func TestDoctorOldSettingsWarn(t *testing.T) {
	healthyMachine(t)
	r := readyRepo(t)
	var settings map[string]map[string]any
	data, _ := os.ReadFile(filepath.Join(r.dir, ".claude", "settings.json"))
	if err := json.Unmarshal(data, &settings); err != nil {
		t.Fatal(err)
	}
	delete(settings["hooks"], "PostToolUseFailure")
	data, _ = json.Marshal(settings)
	r.write(".claude/settings.json", string(data))
	code, stdout, _ := r.run("doctor", "--offline")
	if code != 0 || !regexp.MustCompile(`warn +hooks +PostToolUseFailure does not run polix hook claude`).MatchString(stdout) {
		t.Fatalf("doctor should only warn: %d\n%s", code, stdout)
	}
}

func TestDoctorFindsProblems(t *testing.T) {
	healthyMachine(t)
	r := readyRepo(t)
	if err := update.SaveCache(update.Cache{Latest: "0.3.0"}); err != nil {
		t.Fatal(err)
	}
	var settings map[string]map[string][]map[string]any
	data, _ := os.ReadFile(filepath.Join(r.dir, ".claude", "settings.json"))
	if err := json.Unmarshal(data, &settings); err != nil {
		t.Fatal(err)
	}
	delete(settings["hooks"], "Stop")
	settings["hooks"]["PreToolUse"][0]["matcher"] = "Edit|Write"
	settings["hooks"]["PostToolUse"][0]["hooks"] = []map[string]any{{"type": "command", "command": "polix hook claude"}}
	data, _ = json.Marshal(settings)
	r.write(".claude/settings.json", string(data))
	r.write("polix.yaml", "version: 1\non:\n  pre_edit:\n    - deny: x\n      foo: 1\n")
	r.write("ai/rules/a.yaml", "rules:\n  - id: A-1\n    description: d\n    deterministic: true\n    check: checks/a.sh\n")
	r.write("checks/a.sh", "#!/bin/sh\n")
	if err := os.Chmod(filepath.Join(r.dir, "checks", "a.sh"), 0o644); err != nil {
		t.Fatal(err)
	}

	code, stdout, _ := r.run("doctor", "--offline")
	if code != 1 {
		t.Fatalf("doctor should fail: %d\n%s", code, stdout)
	}
	want := []string{
		"the latest release is 0.3.0",
		`unknown key "foo"`,
		"Stop does not run polix hook claude",
		"the matcher of PreToolUse does not cover MultiEdit, NotebookEdit, Bash",
		"PostToolUse: the timeout",
	}
	if runtime.GOOS != "windows" {
		want = append(want, "check checks/a.sh is not executable")
	}
	for _, w := range want {
		if !strings.Contains(stdout, w) {
			t.Errorf("doctor should report %q:\n%s", w, stdout)
		}
	}

	code, stdout, _ = r.run("doctor", "--hook")
	if code != 0 || !strings.Contains(stdout, "cuéntaselos al usuario") || !strings.Contains(stdout, "Stop does not run") ||
		strings.Contains(stdout, "[ok]") {
		t.Fatalf("the hook reports only the problems and never fails: %d\n%s", code, stdout)
	}
}

func TestDoctorPath(t *testing.T) {
	healthyMachine(t)
	r := readyRepo(t)
	lookPath = func(name string) (string, error) {
		if name == "polix" {
			return "", exec.ErrNotFound
		}
		return exec.LookPath(name)
	}
	code, stdout, _ := r.run("doctor", "--offline")
	if code != 1 || !strings.Contains(stdout, "polix is not on the PATH") {
		t.Fatalf("doctor: %d\n%s", code, stdout)
	}
	other := filepath.Join(t.TempDir(), "polix")
	if err := os.WriteFile(other, []byte("other"), 0o755); err != nil {
		t.Fatal(err)
	}
	lookPath = func(name string) (string, error) {
		if name == "polix" {
			return other, nil
		}
		return exec.LookPath(name)
	}
	_, stdout, _ = r.run("doctor", "--offline")
	if !strings.Contains(stdout, "the PATH runs another polix") {
		t.Fatalf("doctor: %s", stdout)
	}
}

// Outside a repository only the checks of the machine apply.
func TestDoctorOutsideRepository(t *testing.T) {
	healthyMachine(t)
	var out, errb strings.Builder
	code := Run([]string{"doctor", "--offline", "--dir", t.TempDir()}, nil, &out, &errb)
	if code != 0 || !strings.Contains(out.String(), "not inside a git repository") {
		t.Fatalf("doctor: %d\n%s%s", code, out.String(), errb.String())
	}
}

// On Windows, doctor reports a check whose interpreter is not on the PATH.
func TestDoctorMissingInterpreter(t *testing.T) {
	healthyMachine(t)
	r := readyRepo(t)
	r.write("ai/rules/style.yaml", "rules:\n  - id: S-1\n    description: d\n    deterministic: true\n    check: checks/ok.py\n")
	r.write("checks/ok.py", "print('ok')\n")
	oldOS, oldLook := goos, lookPath
	goos = "windows"
	lookPath = func(name string) (string, error) {
		if name == "python" || name == "python3" || name == "py" {
			return "", exec.ErrNotFound
		}
		return oldLook(name)
	}
	t.Cleanup(func() { goos, lookPath = oldOS, oldLook })
	code, stdout, _ := r.run("doctor", "--offline")
	if code == 0 || !strings.Contains(stdout, "S-1") || !strings.Contains(stdout, "python") {
		t.Fatalf("doctor should report the missing python: %d\n%s", code, stdout)
	}
}
