package setup

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
)

// read returns the content of p, failing the test when it cannot be read.
func read(t *testing.T, p string) string {
	t.Helper()
	data, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// write creates p with data, and the folders that hold it.
func write(t *testing.T, p, data string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
}

// fakeCLI is the fake CLI of the agents (testdata/fakecli), built once by TestMain.
var fakeCLI string

// TestMain builds the fake CLI of the agents, which the tests put on the PATH as claude and codex.
func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "polix-fakecli-")
	if err != nil {
		panic(err)
	}
	fakeCLI = filepath.Join(dir, "fakecli"+exeSuffix())
	if out, err := exec.Command("go", "build", "-o", fakeCLI, "./testdata/fakecli").CombinedOutput(); err != nil {
		panic(fmt.Sprintf("go build fakecli: %v\n%s", err, out))
	}
	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}

// exeSuffix is the extension of programs on this system.
func exeSuffix() string {
	if runtime.GOOS == "windows" {
		return ".exe"
	}
	return ""
}

// copyFile copies the program src to dst.
func copyFile(t *testing.T, src, dst string) {
	t.Helper()
	data, err := os.ReadFile(src)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dst, data, 0o755); err != nil {
		t.Fatal(err)
	}
}

// machine gives a test a home, a cache and a PATH of its own: git, and the fake CLIs of the agents asked, which
// record their calls. It returns the folder of their state.
func machine(t *testing.T, claude, codex bool) string {
	t.Helper()
	git, err := exec.LookPath("git")
	if err != nil {
		t.Skip("git is not on the PATH")
	}
	bin, st := t.TempDir(), t.TempDir()
	path := bin
	if err := os.Symlink(git, filepath.Join(bin, filepath.Base(git))); err != nil {
		path += string(os.PathListSeparator) + filepath.Dir(git) // Windows without symlinks: the folder of git
	}
	if claude {
		copyFile(t, fakeCLI, filepath.Join(bin, "claude"+exeSuffix()))
	}
	if codex {
		copyFile(t, fakeCLI, filepath.Join(bin, "codex"+exeSuffix()))
	}
	t.Setenv("PATH", path)
	t.Setenv("FAKE_STATE", st)
	home(t)
	return st
}

// home gives a test a home and a cache of its own.
func home(t *testing.T) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", os.Getenv("HOME"))
	t.Setenv("POLIX_CACHE_DIR", t.TempDir())
}

// calls returns the calls recorded by the fake CLIs.
func calls(st string) []string {
	data, _ := os.ReadFile(filepath.Join(st, "calls"))
	return strings.Split(strings.TrimSpace(string(data)), "\n")
}

// gitRepo creates a repository with one commit on main.
func gitRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	for _, args := range [][]string{{"init", "--quiet", "--initial-branch=main"},
		{"-c", "user.name=t", "-c", "user.email=t@x", "commit", "--quiet", "--allow-empty", "-m", "init"}} {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL="+os.DevNull, "GIT_CONFIG_SYSTEM="+os.DevNull)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	resolved, _ := filepath.EvalSymlinks(dir)
	return resolved
}

func TestSetupValid(t *testing.T) {
	s := Setup{Home: "/h", Agents: []string{Claude, Codex}, Version: "1.2.3"}
	if err := s.Valid(); err != nil {
		t.Fatal(err)
	}
	bad := Setup{Agents: []string{"cursor"}, Version: "1"}
	if err := bad.Valid(); err == nil || !strings.Contains(err.Error(), `unknown agent "cursor" (claude, codex)`) {
		t.Fatalf("unknown agent: %v", err)
	}
	noVersion := Setup{}
	if err := noVersion.Valid(); err == nil {
		t.Fatal("a setup needs the version")
	}
}

func TestParseAgents(t *testing.T) {
	got, err := ParseAgents([]string{" Claude", "codex", "claude"})
	if err != nil || !slices.Equal(got, []string{Claude, Codex}) {
		t.Fatalf("%v %v", got, err)
	}
	if _, err := ParseAgents([]string{"cursor"}); err == nil || err.Error() != `unknown agent "cursor" (claude, codex)` {
		t.Fatalf("unknown agent: %v", err)
	}
}

func TestSetupPaths(t *testing.T) {
	s := Setup{Home: "/h", Root: "/r", Agents: []string{Claude}, Version: "1"}
	for got, want := range map[string]string{
		s.HooksFile(Claude): "/r/.claude/settings.json",
		s.HooksFile(Codex):  "/r/.codex/hooks.json",
		s.PluginDir():       "/h/.polix/marketplace/polix",
		s.PluginManifest():  "/h/.polix/marketplace/polix/.claude-plugin/plugin.json",
		s.CodexConfigFile(): "/h/.codex/config.toml",
	} {
		if got != filepath.FromSlash(want) {
			t.Errorf("got %s, want %s", got, want)
		}
	}
	if got := s.ClaudeSettingsFiles(); len(got) != 3 {
		t.Fatalf("project, local and user: %v", got)
	}
	userOnly := Setup{Home: "/h", Version: "1"}
	if got := userOnly.ClaudeSettingsFiles(); len(got) != 1 {
		t.Fatalf("only the user outside a repository: %v", got)
	}
}

// The other settings of Claude Code that register polix too are duplicates; the one of the repository is not.
func TestDuplicates(t *testing.T) {
	s := Setup{Home: "/h", Root: "/r", Agents: []string{Claude}, Version: "1"}
	hooks, _, _ := RegisterHooks(nil, false, Claude)
	user := filepath.FromSlash("/h/.claude/settings.json")
	files := map[string][]byte{s.HooksFile(Claude): hooks, user: hooks}
	if got := s.Duplicates(files); !slices.Equal(got, []string{user}) {
		t.Fatalf("duplicates: %v", got)
	}
	codexOnly := Setup{Home: "/h", Root: "/r", Agents: []string{Codex}, Version: "1"}
	if got := codexOnly.Duplicates(files); got != nil {
		t.Fatalf("only for Claude Code: %v", got)
	}
}

func TestSummary(t *testing.T) {
	s := Setup{Home: "/h", Root: "/r", Agents: []string{Claude, Codex}, Version: "1.2.3"}
	got := s.Summary("write")
	for _, want := range []string{"polix 1.2.3 will write:", "Claude Code: the plugin polix@polix", "Codex: the plugin",
		"the hooks of " + s.HooksFile(Codex)} {
		if !strings.Contains(got, want) {
			t.Errorf("summary lacks %q:\n%s", want, got)
		}
	}
}
