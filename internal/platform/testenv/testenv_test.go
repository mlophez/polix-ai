package testenv

import (
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// TestMain runs the tests of testenv without the variables git exports to its hooks, as every package that runs git.
func TestMain(m *testing.M) {
	ClearGitHookEnv()
	os.Exit(m.Run())
}

// ClearGitHookEnv unsets every variable git exports to a hook about the repository being committed, and nothing else.
func TestClearGitHookEnv(t *testing.T) {
	for _, v := range GitHookVars {
		t.Setenv(v, "/elsewhere")
	}
	t.Setenv("GIT_CONFIG_GLOBAL", "/dev/null")
	ClearGitHookEnv()
	for _, v := range GitHookVars {
		if got, ok := os.LookupEnv(v); ok {
			t.Errorf("%s = %q after ClearGitHookEnv", v, got)
		}
	}
	if os.Getenv("GIT_CONFIG_GLOBAL") != "/dev/null" {
		t.Error("ClearGitHookEnv unset a variable that is not about the repository")
	}
	for _, v := range []string{"GIT_INDEX_FILE", "GIT_DIR", "GIT_WORK_TREE", "GIT_CONFIG_PARAMETERS", "GIT_CONFIG_COUNT"} {
		if !slices.Contains(GitHookVars, v) {
			t.Errorf("GitHookVars misses %s", v)
		}
	}
}

func TestGitRepo(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not on the PATH")
	}
	dir := GitRepo(t)
	for args, want := range map[string]string{
		"rev-parse --abbrev-ref HEAD": "main",
		"rev-list --count HEAD":       "1",
	} {
		out, err := exec.Command("git", append([]string{"-C", dir}, strings.Fields(args)...)...).CombinedOutput()
		if got := strings.TrimSpace(string(out)); err != nil || got != want {
			t.Errorf("git %s: got %q (%v), want %q", args, got, err, want)
		}
	}
}

func TestWriteRead(t *testing.T) {
	p := filepath.Join(t.TempDir(), "a", "b", "f.txt")
	Write(t, p, "data\n")
	if got := Read(t, p); got != "data\n" {
		t.Fatalf("got %q", got)
	}
}

func TestMachine(t *testing.T) {
	st := Machine(t, true, false)
	if home := os.Getenv("HOME"); home == "" || os.Getenv("USERPROFILE") != home || os.Getenv("POLIX_CACHE_DIR") == "" {
		t.Fatalf("a home and a cache of its own: HOME=%q USERPROFILE=%q", home, os.Getenv("USERPROFILE"))
	}
	if _, err := exec.LookPath("codex"); err == nil {
		t.Fatal("codex was not asked for")
	}
	if out, err := exec.Command("claude", "plugin", "list").CombinedOutput(); err != nil ||
		strings.TrimSpace(string(out)) != "[]" {
		t.Fatalf("claude plugin list: %q %v", out, err)
	}
	if got := Calls(st); !slices.Equal(got, []string{"claude plugin list"}) {
		t.Fatalf("calls: %v", got)
	}
}
