package testenv

import (
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

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
