package hook

import (
	"path/filepath"
	"testing"
)

// holdsAny sees a folder that holds what a pattern with a slash matches, and nothing for a pattern without one.
func TestHoldsAny(t *testing.T) {
	patterns := []string{"ai/rules/**", "docs/secret.md", ".env", "**/gen/*.go"}
	for dir, want := range map[string]bool{
		"ai/rules": true, "ai": true, "docs": true, "ai/other": false, "ai/rules/a.yaml": false,
		"docs/secret.md": false, "src": false, "gen": false,
	} {
		if _, got := holdsAny(patterns, dir, false); got != want {
			t.Errorf("holdsAny(%q) = %v, want %v", dir, got, want)
		}
	}
}

// wildcardHolds sees a word with wildcards that may write what a pattern matches, or a folder holding it.
func TestWildcardHolds(t *testing.T) {
	patterns := []string{"ai/rules/**", "docs/secret.md", ".env"}
	for word, want := range map[string]bool{
		"*": true, "ai/*": true, "ai/r*": true, "ai/rules/*": true, "ai/rules/sub/*.yaml": true,
		"docs/*.md": true, "docs/*.txt": false, "*.md": false, "ai/plans/*": false, "src/*.go": false, "*.log": false,
	} {
		if _, got := wildcardHolds(patterns, word, false); got != want {
			t.Errorf("wildcardHolds(%q) = %v, want %v", word, got, want)
		}
	}
}

// Without case, a folder or a word in another case holds the same patterns.
func TestHoldsFold(t *testing.T) {
	if _, ok := holdsAny([]string{"AI/Rules/**"}, "ai", true); !ok {
		t.Error("holdsAny folds the case")
	}
	if _, ok := wildcardHolds([]string{"ai/rules/**"}, "AI/*", true); !ok {
		t.Error("wildcardHolds folds the case")
	}
}

// AnyPathFold matches without case only when asked.
func TestAnyPathFold(t *testing.T) {
	if !AnyPathFold([]string{".env"}, ".ENV", true) || AnyPathFold([]string{".env"}, ".ENV", false) {
		t.Fatal("AnyPathFold")
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
		if got := NamesState(command, dir, dir, false); got != want {
			t.Errorf("NamesState(%q) = %v", command, got)
		}
	}
	if !NamesState("rm -rf .GIT/Polix", dir, dir, true) || NamesState("rm -rf .GIT/Polix", dir, dir, false) {
		t.Error("NamesState folds the case only when asked")
	}
	if !NamesState("rm /real/polix/x", dir, "/real/polix", false) {
		t.Error("NamesState sees the real path of the folder")
	}
}

// MayWrite sees the commands that can write, and not a descriptor duplicated.
func TestMayWrite(t *testing.T) {
	for command, want := range map[string]bool{"rm x": true, "echo x > y": true, "sed -i s/a/b/ x": true,
		"cat x 2>&1": false, "jq . x": false, "echo x >&y": true} {
		if got := MayWrite(command); got != want {
			t.Errorf("MayWrite(%q) = %v", command, got)
		}
	}
}

// Within sees a path inside a folder, the folder itself, and nothing outside, without case when asked.
func TestWithin(t *testing.T) {
	dir := filepath.FromSlash("/repo/.git/polix")
	for p, want := range map[string]bool{dir: true, filepath.Join(dir, "log.jsonl"): true,
		filepath.FromSlash("/repo/.git"): false, filepath.FromSlash("/repo/.git/polixx"): false} {
		if got := Within(dir, p, false); got != want {
			t.Errorf("Within(%q) = %v", p, got)
		}
	}
	upper := filepath.FromSlash("/repo/.git/POLIX/x")
	if !Within(dir, upper, true) || Within(dir, upper, false) {
		t.Error("Within folds the case only when asked")
	}
}

// The file names of macOS and Windows ignore case.
func TestFoldsCase(t *testing.T) {
	if !FoldsCase("darwin") || !FoldsCase("windows") || FoldsCase("linux") {
		t.Fatal("FoldsCase")
	}
}
