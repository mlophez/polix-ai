package git

import (
	"bytes"
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// Restore puts back what a command changed after a content hash: a changed file with its content and mode, a file in
// a folder the command removed with its folder, and removes a file the command created; a path neither the tree nor
// the working tree holds is no error. The real index stays byte for byte the same and the content hash is the one
// before the command.
func TestRestore(t *testing.T) {
	ctx := context.Background()
	dir, git, _ := gitRepo(t)
	writeBytes(t, dir, "AGENTS.md", "agents")
	writeBytes(t, dir, "run.sh", "run")
	if err := os.Chmod(filepath.Join(dir, "run.sh"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeBytes(t, dir, "docs/guide/a.md", "guide")
	git("add", "--all")
	git("commit", "--quiet", "-m", "first")
	writeBytes(t, dir, "notes.txt", "untracked")
	r, err := Open(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	scratch := filepath.Join(r.GitDir, "polix")
	before, err := r.ContentHash(ctx, scratch)
	if err != nil {
		t.Fatal(err)
	}
	index, err := os.ReadFile(filepath.Join(r.GitDir, "index"))
	if err != nil {
		t.Fatal(err)
	}

	writeBytes(t, dir, "AGENTS.md", "changed by a command")
	writeBytes(t, dir, "run.sh", "changed")
	if err := os.Chmod(filepath.Join(dir, "run.sh"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(filepath.Join(dir, "docs")); err != nil {
		t.Fatal(err)
	}
	writeBytes(t, dir, "new.txt", "created by a command")
	after, err := r.ContentHash(ctx, scratch)
	if err != nil {
		t.Fatal(err)
	}
	paths, _, err := r.TreeDiff(ctx, before, after)
	if err != nil {
		t.Fatal(err)
	}

	if err := r.Restore(ctx, scratch, before, append(paths, "gone.txt")); err != nil {
		t.Fatalf("Restore: %v", err)
	}
	for rel, want := range map[string]string{"AGENTS.md": "agents", "run.sh": "run", "docs/guide/a.md": "guide",
		"notes.txt": "untracked"} {
		if got, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(rel))); err != nil || string(got) != want {
			t.Errorf("%s = %q, %v, want %q", rel, got, err, want)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "new.txt")); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("the file created by the command is still there: %v", err)
	}
	// Windows keeps no executable bit (core.filemode is false there).
	if runtime.GOOS != "windows" {
		if info, err := os.Stat(filepath.Join(dir, "run.sh")); err != nil || info.Mode().Perm()&0o100 == 0 {
			t.Errorf("run.sh lost its executable bit: %v, %v", info, err)
		}
	}
	if again, err := os.ReadFile(filepath.Join(r.GitDir, "index")); err != nil || !bytes.Equal(again, index) {
		t.Errorf("the real index changed: %v", err)
	}
	if got, err := r.ContentHash(ctx, scratch); err != nil || got != before {
		t.Errorf("ContentHash after Restore = %s, %v, want %s", got, err, before)
	}
	if left, err := filepath.Glob(filepath.Join(scratch, "index-*")); err != nil || len(left) != 0 {
		t.Errorf("the temporary index is left behind: %v %v", left, err)
	}
}

// Restore refuses a tree git does not have, an empty tree, an option and a path outside the repository, and touches
// nothing then.
func TestRestoreRefuses(t *testing.T) {
	ctx := context.Background()
	dir, git, write := gitRepo(t)
	commit(git, write, "a.txt")
	r, err := Open(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	scratch := filepath.Join(r.GitDir, "polix")
	tree, err := r.ContentHash(ctx, scratch)
	if err != nil {
		t.Fatal(err)
	}
	writeBytes(t, dir, "b.txt", "new")
	cases := map[string]struct {
		tree  string
		paths []string
		want  string
	}{
		"missing tree": {strings.Repeat("1", len(tree)), []string{"b.txt"}, "tree 1111"},
		"empty tree":   {"", []string{"b.txt"}, "is not a tree"},
		"option":       {"--all", []string{"b.txt"}, "is not a tree"},
		"outside":      {tree, []string{"b.txt", "../x.txt"}, "restore ../x.txt: not a path inside the repository"},
		"absolute":     {tree, []string{"/etc/hosts"}, "not a path inside the repository"},
	}
	for name, c := range cases {
		if err := r.Restore(ctx, scratch, c.tree, c.paths); err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: Restore = %v, want an error with %q", name, err, c.want)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "b.txt")); err != nil {
		t.Fatalf("a refused Restore touched the working tree: %v", err)
	}
}

// A path that cannot be restored fails naming it, and the other paths are still restored.
func TestRestoreFirstError(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("a folder without write permission stops a removal neither on Windows nor for root")
	}
	ctx := context.Background()
	dir, git, write := gitRepo(t)
	commit(git, write, "a.txt")
	r, err := Open(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	scratch := filepath.Join(r.GitDir, "polix")
	tree, err := r.ContentHash(ctx, scratch)
	if err != nil {
		t.Fatal(err)
	}
	writeBytes(t, dir, "locked/new.txt", "new")
	writeBytes(t, dir, "a.txt", "changed")
	locked := filepath.Join(dir, "locked")
	if err := os.Chmod(locked, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(locked, 0o755) })
	err = r.Restore(ctx, scratch, tree, []string{"locked/new.txt", "a.txt"})
	if err == nil || !strings.Contains(err.Error(), "restore locked/new.txt") {
		t.Fatalf("Restore = %v, want the error of locked/new.txt", err)
	}
	if got, err := os.ReadFile(filepath.Join(dir, "a.txt")); err != nil || string(got) != "a.txt" {
		t.Fatalf("a.txt = %q, %v, want it restored", got, err)
	}
}
