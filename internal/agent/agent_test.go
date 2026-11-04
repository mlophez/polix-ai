package agent

import (
	"path/filepath"
	"testing"

	"github.com/mlophez/polix-ai/internal/engine"
)

// A commit is recognised after global options of git and inside a quoted command, so pre_commit cannot be
// skipped by spelling it differently.
func TestGitCommit(t *testing.T) {
	cases := []struct {
		command string
		want    bool
	}{
		{"git commit -m x", true},
		{"cd x && git commit -m x", true},
		{"git -C . commit -m x", true},
		{"git -C ../repo commit", true},
		{"git -c user.name=a commit -m x", true},
		{"git --no-pager commit", true},
		{"git --git-dir .git --work-tree . commit", true},
		{"git --git-dir=.git commit", true},
		{`bash -c "git commit -m x"`, true},
		{"sh -c 'git commit'", true},
		{"git commit-tree HEAD^{tree}", false},
		{"git log --grep commit", false},
		{"git status", false},
	}
	for _, c := range cases {
		if got := gitCommit.MatchString(c.command); got != c.want {
			t.Errorf("gitCommit(%q) = %v, want %v", c.command, got, c.want)
		}
	}
}

// A relative path (apply_patch) is made absolute from the cwd of the call, so the protections that compare
// absolute paths see it.
func TestEditEventsRelativePath(t *testing.T) {
	cwd := filepath.Join(t.TempDir(), "repo")
	none := func(string, string) (string, bool) { return "", false }
	abs := filepath.Join(t.TempDir(), "file")
	evs := editEvents(engine.Event{Cwd: cwd}, []string{".git/polix/log.jsonl", abs}, none)
	if want := filepath.Join(cwd, ".git", "polix", "log.jsonl"); evs[0].AbsPath != want {
		t.Errorf("relative path: AbsPath = %q, want %q", evs[0].AbsPath, want)
	}
	if evs[1].AbsPath != abs {
		t.Errorf("absolute path: AbsPath = %q", evs[1].AbsPath)
	}
}
