// Package gitx reads the facts of the repository polix needs: root, branch, changed files and a hash
// of the current content, always through the git CLI with explicit arguments (never a shell), so it behaves
// the same on Windows and Linux.
package gitx

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// timeout bounds every git call: a hook must never hang the agent.
const timeout = 20 * time.Second

// Repo is a git working tree.
type Repo struct {
	Root   string // absolute path of the working tree
	GitDir string // absolute path of its git directory (per worktree)
}

// Open finds the repository that contains dir.
func Open(dir string) (*Repo, error) {
	root, err := run(dir, nil, "rev-parse", "--show-toplevel")
	if err != nil {
		return nil, fmt.Errorf("not inside a git repository: %w", err)
	}
	gitDir, err := run(dir, nil, "rev-parse", "--absolute-git-dir")
	if err != nil {
		return nil, err
	}
	return &Repo{Root: filepath.Clean(root), GitDir: filepath.Clean(gitDir)}, nil
}

// Branch returns the current branch, or "" on a detached HEAD.
func (r *Repo) Branch() string {
	b, err := r.git("rev-parse", "--abbrev-ref", "HEAD")
	if err != nil || b == "HEAD" {
		return ""
	}
	return b
}

// Changed returns the repository relative paths that differ from ref, plus untracked files not ignored by
// .gitignore. An empty ref means HEAD: what is not committed yet. ResolveBase gives the ref of a whole branch.
func (r *Repo) Changed(ref string) ([]string, error) {
	if ref == "" {
		ref = "HEAD"
	}
	files := map[string]bool{}
	if _, err := r.git("rev-parse", "--verify", "--quiet", "HEAD"); err == nil {
		out, err := r.paths("diff", "--name-only", "--no-renames", "-z", ref)
		if err != nil {
			return nil, err
		}
		addPaths(files, out)
	} else {
		// No commit yet: everything staged counts as changed.
		out, err := r.paths("ls-files", "-z")
		if err != nil {
			return nil, err
		}
		addPaths(files, out)
	}
	out, err := r.paths("ls-files", "-z", "--others", "--exclude-standard")
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

// ResolveBase returns the merge base of HEAD with the closest of the candidate branches: each is tried as a
// local branch and then as origin/<name>, and among those that exist the one with the fewest commits from its
// merge base to HEAD wins; ties keep the order of the list. It fails, naming the candidates, when none exists.
func (r *Repo) ResolveBase(candidates []string) (string, error) {
	best, bestCount := "", -1
	for _, name := range candidates {
		for _, ref := range []string{"refs/heads/" + name, "refs/remotes/origin/" + name} {
			if _, err := r.git("rev-parse", "--verify", "--quiet", ref+"^{commit}"); err != nil {
				continue
			}
			mb, err := r.git("merge-base", "HEAD", ref)
			if err != nil || mb == "" {
				break
			}
			out, err := r.git("rev-list", "--count", mb+"..HEAD")
			if err != nil {
				break
			}
			var n int
			if _, err := fmt.Sscan(out, &n); err != nil {
				break
			}
			if bestCount < 0 || n < bestCount {
				best, bestCount = mb, n
			}
			break
		}
	}
	if best == "" {
		return "", fmt.Errorf("no base branch found: tried %s, locally and on origin", strings.Join(candidates, ", "))
	}
	return best, nil
}

// ContentHash returns the hash of the git tree the working tree would produce if everything were staged,
// untracked files included and ignored files excluded. Facts of the state are tied to it, so any edit makes
// them stale. It uses a copy of the index, so the real index is never touched and the stat cache keeps it fast.
func (r *Repo) ContentHash(scratchDir string) (string, error) {
	if err := os.MkdirAll(scratchDir, 0o755); err != nil {
		return "", err
	}
	tmp, err := os.CreateTemp(scratchDir, "index-*")
	if err != nil {
		return "", err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if data, err := os.ReadFile(filepath.Join(r.GitDir, "index")); err == nil {
		if _, err := tmp.Write(data); err != nil {
			tmp.Close()
			return "", err
		}
	}
	tmp.Close()
	env := []string{"GIT_INDEX_FILE=" + tmpPath}
	if _, err := run(r.Root, env, "add", "--all", "--", "."); err != nil {
		return "", err
	}
	return run(r.Root, env, "write-tree")
}

// Files returns every file of the working tree that git does not ignore, tracked or not.
func (r *Repo) Files() ([]string, error) {
	out, err := r.paths("ls-files", "-z", "--cached", "--others", "--exclude-standard")
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

// Ignored reports whether the repository relative path is ignored by git; tracked files never are.
func (r *Repo) Ignored(rel string) bool {
	_, err := r.git("check-ignore", "--quiet", "--", rel)
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
		if real, err := filepath.EvalSymlinks(cur); err == nil {
			return filepath.Join(real, rest)
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			return p
		}
		rest = filepath.Join(filepath.Base(cur), rest)
		cur = parent
	}
}

func (r *Repo) git(args ...string) (string, error) { return run(r.Root, nil, args...) }

// paths runs a git command that lists paths with -z and returns its raw stdout. Without -z, git quotes and
// escapes any path that is not plain ASCII ("a\303\261o.md" for año.md), and no glob would match it.
func (r *Repo) paths(args ...string) (string, error) { return runRaw(r.Root, nil, args...) }

// run executes git in dir and returns its trimmed stdout.
func run(dir string, env []string, args ...string) (string, error) {
	out, err := runRaw(dir, env, args...)
	return strings.TrimSpace(out), err
}

// runRaw executes git in dir and returns its stdout as is.
func runRaw(dir string, env []string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
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
