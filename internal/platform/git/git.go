// Package git reads the facts of a repository: root, branch, HEAD commit, changed files and a hash of the current
// content, always through the git CLI with explicit arguments (never a shell), so it behaves the same on Windows
// and Linux. Every call that runs git takes the context of the caller and is bounded by its own timeout too.
package git

import (
	"bytes"
	"context"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Repo is a git working tree.
type Repo struct {
	Root   string // absolute path of the working tree
	GitDir string // absolute path of its git directory (per worktree)
}

// timeout bounds every git call: a hook must never hang the agent.
const timeout = 20 * time.Second

// Open finds the repository that contains dir.
func Open(ctx context.Context, dir string) (*Repo, error) {
	root, err := run(ctx, dir, nil, "rev-parse", "--show-toplevel")
	if err != nil {
		return nil, fmt.Errorf("not inside a git repository: %w", err)
	}
	gitDir, err := run(ctx, dir, nil, "rev-parse", "--absolute-git-dir")
	if err != nil {
		return nil, err
	}
	return &Repo{Root: filepath.Clean(root), GitDir: filepath.Clean(gitDir)}, nil
}

// Head returns the current branch, "" on a detached HEAD, and the full commit HEAD points to, from one git call.
// Both are "" when git fails, as on a repository without commits. It swallows the errors of git, so it runs
// detached from the cancellation of ctx (bounded by the timeout of the package): a caller whose context is done
// still gets the branch and the commit.
func (r *Repo) Head(ctx context.Context) (branch, commit string) {
	out, err := r.git(context.WithoutCancel(ctx), "rev-parse", "HEAD", "--abbrev-ref", "HEAD")
	if err != nil {
		return "", ""
	}
	// The commit, then the abbreviated name; Fields also drops the \r of a line ending of Windows.
	fields := strings.Fields(out)
	if len(fields) != 2 {
		return "", ""
	}
	commit, branch = fields[0], fields[1]
	if branch == "HEAD" {
		branch = ""
	}
	return branch, commit
}

// Changed returns the repository relative paths that differ from ref, plus untracked files not ignored by
// .gitignore. An empty ref means HEAD: what is not committed yet. ResolveBase gives the ref of a whole branch.
func (r *Repo) Changed(ctx context.Context, ref string) ([]string, error) {
	if ref == "" {
		ref = "HEAD"
	}
	files := map[string]bool{}
	if _, err := r.git(ctx, "rev-parse", "--verify", "--quiet", "HEAD"); err == nil {
		out, err := r.paths(ctx, "diff", "--name-only", "--no-renames", "-z", ref)
		if err != nil {
			return nil, err
		}
		addPaths(files, out)
	} else {
		// No commit yet: everything staged counts as changed.
		out, err := r.paths(ctx, "ls-files", "-z")
		if err != nil {
			return nil, err
		}
		addPaths(files, out)
	}
	out, err := r.paths(ctx, "ls-files", "-z", "--others", "--exclude-standard")
	if err != nil {
		return nil, err
	}
	addPaths(files, out)
	list := make([]string, 0, len(files))
	for f := range files {
		list = append(list, f)
	}
	sort.Strings(list)
	return list, nil
}

// Summary returns the short hash and the subject of the commit rev, as one line.
func (r *Repo) Summary(ctx context.Context, rev string) (string, error) {
	if rev == "" || strings.HasPrefix(rev, "-") {
		return "", fmt.Errorf("%q is not a commit", rev)
	}
	return r.git(ctx, "log", "-1", "--no-decorate", "--format=%h %s", rev+"^{commit}", "--")
}

// Files returns every file of the working tree that git does not ignore, tracked or not.
func (r *Repo) Files(ctx context.Context) ([]string, error) {
	out, err := r.paths(ctx, "ls-files", "-z", "--cached", "--others", "--exclude-standard")
	if err != nil {
		return nil, err
	}
	set := map[string]bool{}
	addPaths(set, out)
	list := make([]string, 0, len(set))
	for f := range set {
		list = append(list, f)
	}
	sort.Strings(list)
	return list, nil
}

// Foreign reports whether the folder dir is inside an existing git repository other than this one: its git common
// directory, its objects and its refs are each another file than ours, compared by identity (os.SameFile), so a
// linked worktree of this repository, another spelling of its path or a repository whose objects or refs link into
// ours is not foreign. A folder that does not exist, is in no repository or that git cannot read is not foreign
// either. Like Head, it runs detached from the cancellation of ctx, since an error reads as not foreign.
func (r *Repo) Foreign(ctx context.Context, dir string) bool {
	ctx = context.WithoutCancel(ctx)
	ours, err := storage(ctx, r.Root)
	if err != nil {
		return false
	}
	theirs, err := storage(ctx, dir)
	if err != nil {
		return false
	}
	for i := range ours {
		if os.SameFile(ours[i], theirs[i]) {
			return false
		}
	}
	return true
}

// storage returns the git common directory, the objects and the refs of the repository that holds dir. git gives
// them absolute: a relative answer joined to dir would undo a `..` that git took through a link.
func storage(ctx context.Context, dir string) ([]fs.FileInfo, error) {
	out, err := run(ctx, dir, nil, "rev-parse", "--path-format=absolute", "--git-common-dir", "--git-path", "objects",
		"--git-path", "refs")
	if err != nil {
		return nil, err
	}
	paths := strings.Split(out, "\n")
	if len(paths) != 3 {
		return nil, fmt.Errorf("git rev-parse gave %d paths, want 3", len(paths))
	}
	infos := make([]fs.FileInfo, len(paths))
	for i, p := range paths {
		if infos[i], err = os.Stat(strings.TrimSpace(p)); err != nil {
			return nil, err
		}
	}
	return infos, nil
}

// Ignored reports whether the repository relative path is ignored by git; tracked files never are. Like Head,
// it runs detached from the cancellation of ctx, since an error would read as not ignored.
func (r *Repo) Ignored(ctx context.Context, rel string) bool {
	_, err := r.git(context.WithoutCancel(ctx), "check-ignore", "--quiet", "--", rel)
	return err == nil
}

// Rel turns a path given by an agent (absolute, relative to the working directory, with backslashes on
// Windows) into a slash separated path relative to the repository root. ok is false for a path outside it.
func (r *Repo) Rel(p, cwd string) (rel string, ok bool) {
	if p == "" {
		return "", false
	}
	if !filepath.IsAbs(p) {
		base := cwd
		if base == "" {
			base = r.Root
		}
		p = filepath.Join(base, p)
	}
	// Resolve symlinks on both sides (macOS /var is /private/var): git reports the real path of the root.
	rel, err := filepath.Rel(RealPath(r.Root), RealPath(filepath.Clean(p)))
	if err != nil {
		return "", false
	}
	rel = filepath.ToSlash(rel)
	if rel == ".." || strings.HasPrefix(rel, "../") {
		return rel, false
	}
	return rel, true
}

// RealPath resolves the symlinks of the longest existing prefix of p, so it works for files not created yet.
func RealPath(p string) string {
	rest := ""
	for cur := p; ; {
		if resolved, err := filepath.EvalSymlinks(cur); err == nil {
			return filepath.Join(resolved, rest)
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			return p
		}
		rest = filepath.Join(filepath.Base(cur), rest)
		cur = parent
	}
}

func (r *Repo) git(ctx context.Context, args ...string) (string, error) {
	return run(ctx, r.Root, nil, args...)
}

// paths runs a git command that lists paths with -z and returns its raw stdout. Without -z, git quotes and
// escapes any path that is not plain ASCII ("a\303\261o.md" for año.md), and no glob would match it.
func (r *Repo) paths(ctx context.Context, args ...string) (string, error) {
	return runRaw(ctx, r.Root, nil, args...)
}

// run executes git in dir and returns its trimmed stdout.
func run(ctx context.Context, dir string, env []string, args ...string) (string, error) {
	out, err := runRaw(ctx, dir, env, args...)
	return strings.TrimSpace(out), err
}

// runRaw executes git in dir and returns its stdout as is.
func runRaw(ctx context.Context, dir string, env []string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), env...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}
	return stdout.String(), nil
}

// addPaths adds the NUL separated paths of a -z output to set.
func addPaths(set map[string]bool, out string) {
	for p := range strings.SplitSeq(out, "\x00") {
		if p != "" {
			set[p] = true
		}
	}
}

// splitPaths returns the non empty NUL separated paths of a -z output.
func splitPaths(out string) []string {
	var list []string
	for p := range strings.SplitSeq(out, "\x00") {
		if p != "" {
			list = append(list, p)
		}
	}
	return list
}
