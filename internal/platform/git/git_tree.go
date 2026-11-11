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
	"strings"
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
// them, sorted, and among them those to no longer holds, removed; none for equal trees. A tree git does not have
// (never written, or pruned by git gc) is an error.
func (r *Repo) TreeDiff(ctx context.Context, from, to string) (paths, removed []string, err error) {
	if from == "" || to == "" {
		return nil, nil, fmt.Errorf("tree diff needs two trees, got %q and %q", from, to)
	}
	for _, tree := range []string{from, to} {
		if _, err := r.git(ctx, "cat-file", "-e", tree+"^{tree}"); err != nil {
			return nil, nil, fmt.Errorf("tree %s: %w", tree, err)
		}
	}
	out, err := r.paths(ctx, "diff-tree", "-r", "--name-status", "--no-renames", "-z", from, to)
	if err != nil {
		return nil, nil, err
	}
	// Without renames each change is its status, then its path, both NUL terminated.
	fields := splitPaths(out)
	for i := 0; i+1 < len(fields); i += 2 {
		paths = append(paths, fields[i+1])
		if fields[i] == "D" {
			removed = append(removed, fields[i+1])
		}
	}
	sort.Strings(paths)
	sort.Strings(removed)
	return paths, removed, nil
}

// operations are the files git keeps in the git directory while an operation that rewrites the working tree is
// stopped halfway, as on conflicts or with --no-commit: a merge, a cherry-pick, a revert, a rebase or git am.
var operations = []string{"MERGE_HEAD", "CHERRY_PICK_HEAD", "REVERT_HEAD", "rebase-merge", "rebase-apply"}

// Refs returns, as one text, what git moves without moving HEAD: the commit refs/stash points to, as
// `stash <commit>`, and each operation stopped halfway, by the name of its file in the git directory followed by
// the commit that file names, or alone for a folder; joined by ", ", and "" when there is neither. Like Head, it runs
// detached from the cancellation of ctx, and a git that fails reads as no stash.
func (r *Repo) Refs(ctx context.Context) string {
	var refs []string
	out, err := r.git(context.WithoutCancel(ctx), "rev-parse", "--verify", "--quiet", "refs/stash")
	if out = strings.TrimSpace(out); err == nil && out != "" {
		refs = append(refs, "stash "+out)
	}
	for _, name := range operations {
		data, err := os.ReadFile(filepath.Join(r.GitDir, name))
		switch {
		case err == nil:
			refs = append(refs, strings.TrimSpace(name+" "+strings.TrimSpace(string(data))))
		case !errors.Is(err, fs.ErrNotExist):
			refs = append(refs, name)
		}
	}
	return strings.Join(refs, ", ")
}

// Restore puts the paths, relative to the root with slashes, back as the tree holds them: those it holds are written
// to the working tree from it, content and mode, with the folders they need; the others are removed from the working
// tree, a missing file being no error. It reads the tree into a scratch index in a temporary folder of scratchDir, so
// the real index is never touched. Every path is tried, and the first error is returned, naming its path; a tree git
// does not have (never written, or pruned by git gc) is an error, and then nothing is touched.
func (r *Repo) Restore(ctx context.Context, scratchDir, tree string, paths []string) error {
	if tree == "" || strings.HasPrefix(tree, "-") {
		return fmt.Errorf("%q is not a tree", tree)
	}
	if _, err := r.git(ctx, "cat-file", "-e", tree+"^{tree}"); err != nil {
		return fmt.Errorf("tree %s: %w", tree, err)
	}
	for _, p := range paths {
		if !filepath.IsLocal(filepath.FromSlash(p)) {
			return fmt.Errorf("restore %s: not a path inside the repository", p)
		}
	}
	if len(paths) == 0 {
		return nil
	}
	if err := os.MkdirAll(scratchDir, 0o755); err != nil {
		return err
	}
	dir, err := os.MkdirTemp(scratchDir, "index-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	env := []string{"GIT_INDEX_FILE=" + filepath.Join(dir, "index")}
	if _, err := run(ctx, r.Root, env, "read-tree", tree); err != nil {
		return err
	}
	held, err := r.held(ctx, env, paths)
	if err != nil {
		return err
	}
	return r.restorePaths(ctx, env, paths, held)
}

// held returns which of the paths the index of env holds, compared as written: the pathspecs are literal, and the
// folder a path would match as a prefix does not count.
func (r *Repo) held(ctx context.Context, env, paths []string) (map[string]bool, error) {
	env = append(env, "GIT_LITERAL_PATHSPECS=1")
	out, err := runRaw(ctx, r.Root, env, append([]string{"ls-files", "-z", "--"}, paths...)...)
	if err != nil {
		return nil, err
	}
	wanted := map[string]bool{}
	for _, p := range paths {
		wanted[p] = true
	}
	held := map[string]bool{}
	for _, p := range splitPaths(out) {
		if wanted[p] {
			held[p] = true
		}
	}
	return held, nil
}

// restorePaths removes from the working tree each path the index of env does not hold, and then writes each held
// one from it, overwriting what is there: on a file system that folds case, a path that differs from a held one only
// in case names the same file, so it is removed before the held one is written back, never after. It goes on after
// a failure and returns the first one, naming its path.
func (r *Repo) restorePaths(ctx context.Context, env, paths []string, held map[string]bool) error {
	var first error
	fail := func(p string, err error) {
		if err != nil && first == nil {
			first = fmt.Errorf("restore %s: %w", p, err)
		}
	}
	for _, p := range paths {
		if !held[p] {
			if err := os.Remove(filepath.Join(r.Root, filepath.FromSlash(p))); !errors.Is(err, fs.ErrNotExist) {
				fail(p, err)
			}
		}
	}
	for _, p := range paths {
		if held[p] {
			_, err := run(ctx, r.Root, env, "checkout-index", "--force", "--", p)
			fail(p, err)
		}
	}
	return first
}
