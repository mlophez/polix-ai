package git

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"syscall"
	"time"
)

// ContentHash returns the hash of the git tree the working tree would produce if everything were staged,
// untracked files included and ignored files excluded, so any edit changes it. It stages into a copy of the index
// in a temporary folder of scratchDir, so the real index is never touched and its stat cache keeps it fast; a
// repository without an index yet (no git add since git init) starts from none.
func (r *Repo) ContentHash(ctx context.Context, scratchDir string) (string, error) {
	if err := os.MkdirAll(scratchDir, 0o755); err != nil {
		return "", err
	}
	dir, err := os.MkdirTemp(scratchDir, "index-*")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(dir)
	path := filepath.Join(dir, "index")
	if err := r.copyIndex(path); err != nil {
		return "", err
	}
	return stageTree(ctx, r.Root, path)
}

// copyIndex writes a copy of the real index to path, or nothing when the repository has no index yet. The copy keeps
// the time of the real index: git compares the content of the files changed in the same tick as the index (racily
// clean), and the later time of a new file would hide an edit made in that tick.
func (r *Repo) copyIndex(path string) error {
	src := filepath.Join(r.GitDir, "index")
	info, err := os.Stat(src)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	data, err := readShared(src)
	if err != nil {
		return err
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return err
	}
	return os.Chtimes(path, info.ModTime(), info.ModTime())
}

// readShared reads the file at path, retrying for about a second while another process holds it without sharing
// it: on Windows a git command that replaces the index (git add, git reset) keeps it locked for an instant, and a
// read in that instant fails with a sharing violation instead of waiting.
func readShared(path string) ([]byte, error) {
	for wait := 10 * time.Millisecond; ; wait *= 2 {
		data, err := os.ReadFile(path)
		if err == nil || !lockedByOther(runtime.GOOS, err) || wait > 640*time.Millisecond {
			return data, err
		}
		time.Sleep(wait)
	}
}

// lockedByOther reports whether err, on the system goos, means another process holds the file for now: on Windows
// a sharing violation, or access denied while a file replaced by a rename is still pending its delete.
func lockedByOther(goos string, err error) bool {
	const (
		errorAccessDenied     = syscall.Errno(5)  // ERROR_ACCESS_DENIED
		errorSharingViolation = syscall.Errno(32) // ERROR_SHARING_VIOLATION
	)
	return goos == "windows" && (errors.Is(err, errorSharingViolation) || errors.Is(err, errorAccessDenied))
}

// stageTree stages the whole working tree into the index at path and returns the tree it writes.
func stageTree(ctx context.Context, root, path string) (string, error) {
	env := []string{"GIT_INDEX_FILE=" + path}
	if _, err := run(ctx, root, env, "add", "--all", "--", "."); err != nil {
		return "", err
	}
	return run(ctx, root, env, "write-tree")
}

// TreeDiff returns the repository relative paths that differ between the trees from and to, as ContentHash gives
// them, sorted; none for equal trees. A tree git does not have (never written, or pruned by git gc) is an error.
func (r *Repo) TreeDiff(ctx context.Context, from, to string) ([]string, error) {
	if from == "" || to == "" {
		return nil, fmt.Errorf("tree diff needs two trees, got %q and %q", from, to)
	}
	for _, tree := range []string{from, to} {
		if _, err := r.git(ctx, "cat-file", "-e", tree+"^{tree}"); err != nil {
			return nil, fmt.Errorf("tree %s: %w", tree, err)
		}
	}
	out, err := r.paths(ctx, "diff-tree", "-r", "--name-only", "--no-renames", "-z", from, to)
	if err != nil {
		return nil, err
	}
	list := splitPaths(out)
	sort.Strings(list)
	return list, nil
}
