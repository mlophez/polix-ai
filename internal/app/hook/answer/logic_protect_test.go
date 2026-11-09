package answer

import (
	"path/filepath"
	"testing"

	"github.com/mlophez/polix-ai/internal/app/workflow"
)

// anyPathFold matches without case only when asked.
func TestAnyPathFold(t *testing.T) {
	if !anyPathFold([]string{".env"}, ".ENV", true) || anyPathFold([]string{".env"}, ".ENV", false) {
		t.Fatal("anyPathFold")
	}
}

// The state is named by its path below a git directory or by its folder, not by the name of the binary.
func TestNamesState(t *testing.T) {
	dir := filepath.FromSlash("/repo/.git/polix")
	for command, want := range map[string]bool{
		"rm -rf .git/polix":                     true,
		"cat .git/worktrees/w1/polix/log.jsonl": true,
		"rm " + dir + "/log.jsonl":              true,
		"go install ./cmd/polix":                false,
		"cp build/polix ~/bin/":                 false,
	} {
		if got := namesState(command, dir, dir, false); got != want {
			t.Errorf("namesState(%q) = %v", command, got)
		}
	}
	if !namesState("rm -rf .GIT/Polix", dir, dir, true) || namesState("rm -rf .GIT/Polix", dir, dir, false) {
		t.Error("namesState folds the case only when asked")
	}
	if !namesState("rm /real/polix/x", dir, "/real/polix", false) {
		t.Error("namesState sees the real path of the folder")
	}
}

// mayWrite sees the commands that can write, and not a descriptor duplicated.
func TestMayWrite(t *testing.T) {
	for command, want := range map[string]bool{"rm x": true, "echo x > y": true, "sed -i s/a/b/ x": true,
		"cat x 2>&1": false, "jq . x": false, "echo x >&y": true} {
		if got := mayWrite(command); got != want {
			t.Errorf("mayWrite(%q) = %v", command, got)
		}
	}
}

// within sees a path inside a folder, the folder itself, and nothing outside, without case when asked.
func TestWithin(t *testing.T) {
	dir := filepath.FromSlash("/repo/.git/polix")
	for p, want := range map[string]bool{dir: true, filepath.Join(dir, "log.jsonl"): true,
		filepath.FromSlash("/repo/.git"): false, filepath.FromSlash("/repo/.git/polixx"): false} {
		if got := within(dir, p, false); got != want {
			t.Errorf("within(%q) = %v", p, got)
		}
	}
	upper := filepath.FromSlash("/repo/.git/POLIX/x")
	if !within(dir, upper, true) || within(dir, upper, false) {
		t.Error("within folds the case only when asked")
	}
}

// forbidden finds the first forbidden command, in file order, that matches any command of the line.
func TestForbidden(t *testing.T) {
	list := []workflow.ForbiddenCommand{{Pattern: "rm -rf /*", Line: 4},
		{Pattern: "git push --force*", Reason: "no", Line: 5}, {Pattern: "git push*", Line: 7}}
	for line, want := range map[string]int{
		"rm -rf /":                       4,
		"cd tmp && rm -rf / ; ls":        4,
		"git fetch && git push --force":  5,
		"git push origin main":           7,
		"rm -rf build":                   0,
		"echo 'git push --force' > note": 0,
	} {
		f, ok := forbidden(list, line)
		if got := map[bool]int{true: f.Line}[ok]; got != want {
			t.Errorf("%q: line %d, want %d", line, got, want)
		}
	}
}
