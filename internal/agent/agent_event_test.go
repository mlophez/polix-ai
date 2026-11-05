package agent

import (
	"path/filepath"
	"testing"

	"github.com/mlophez/polix-ai/internal/workflow"
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
	abs := filepath.Join(t.TempDir(), "file")
	evs := editEvents(Event{Cwd: cwd}, []string{".git/polix/log.jsonl", abs})
	if want := filepath.Join(cwd, ".git", "polix", "log.jsonl"); evs[0].AbsPath != want {
		t.Errorf("relative path: AbsPath = %q, want %q", evs[0].AbsPath, want)
	}
	if evs[1].AbsPath != abs {
		t.Errorf("absolute path: AbsPath = %q", evs[1].AbsPath)
	}
	if got := editEvents(Event{}, []string{"a.go"}); got[0].AbsPath != "a.go" {
		t.Errorf("without a cwd the path stays as given: %q", got[0].AbsPath)
	}
}

// A command about to run that commits is also a pre_commit; after it ran, it is not.
func TestCommandEvents(t *testing.T) {
	pre := Event{Name: workflow.PreCommand, Command: "git commit -m x"}
	if evs := commandEvents(pre); len(evs) != 2 || evs[1].Name != workflow.PreCommit || evs[1].Command != pre.Command {
		t.Fatalf("pre_command: %+v", evs)
	}
	post := Event{Name: workflow.PostCommand, Command: "git commit -m x"}
	if evs := commandEvents(post); len(evs) != 1 {
		t.Fatalf("post_command: %+v", evs)
	}
}

// Locate places the path of an edit in the repository, or outside it.
func TestEventLocate(t *testing.T) {
	ev := Event{Name: workflow.PreEdit, Agent: Claude}
	if got := ev.Locate("src/a.go", true); got.Path != "src/a.go" || got.Outside {
		t.Fatalf("inside: %+v", got)
	}
	if got := ev.Locate("../x", false); got.Path != "" || !got.Outside {
		t.Fatalf("outside: %+v", got)
	}
	if !ev.Edits() || (Event{Name: workflow.Stop}).Edits() {
		t.Fatal("Edits")
	}
}

// An event needs a neutral name and its agent.
func TestEventValid(t *testing.T) {
	if err := (Event{Name: workflow.Stop, Agent: Claude}).Valid(); err != nil {
		t.Fatal(err)
	}
	if (Event{Name: "Stop", Agent: Claude}).Valid() == nil || (Event{Name: workflow.Stop}).Valid() == nil {
		t.Fatal("a native name or no agent is valid")
	}
}

// The exit code of a command is read from any of the names the agents use, 0 when it says none.
func TestExitCode(t *testing.T) {
	for raw, want := range map[string]int{`{"exit_code":3}`: 3, `{"exitCode":4}`: 4, `{"returncode":5}`: 5,
		`{"stdout":"ok"}`: 0, `not json`: 0} {
		if got := exitCode([]byte(raw)); got != want {
			t.Errorf("exitCode(%s) = %d, want %d", raw, got, want)
		}
	}
}
