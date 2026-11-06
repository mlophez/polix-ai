// Package git reads the facts of a repository: root, branch, changed files and a hash of the current content,
// always through the git CLI with explicit arguments (never a shell), so it behaves the same on Windows and
// Linux. Every call that runs git takes the context of the caller and is bounded by its own timeout too.
package git

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Repo is a git working tree.
type Repo struct {
	Root   string // absolute path of the working tree
	GitDir string // absolute path of its git directory (per worktree)
}

// Stat is the size of the change of one file: its added and deleted lines, or Binary with both at zero.
type Stat struct {
	Path    string // relative to the root, with forward slashes
	Added   int
	Deleted int
	Binary  bool
}

// binaryProbe is how many leading bytes of a file are searched for a NUL byte, as git does, to call it binary.
const binaryProbe = 8000

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

// Branch returns the current branch, or "" on a detached HEAD. It swallows the errors of git, so it runs
// detached from the cancellation of ctx (bounded by the timeout of the package): a caller whose context is done
// still gets the branch.
func (r *Repo) Branch(ctx context.Context) string {
	b, err := r.git(context.WithoutCancel(ctx), "rev-parse", "--abbrev-ref", "HEAD")
	if err != nil || b == "HEAD" {
		return ""
	}
	return b
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

// Stats returns the size of the change of each file that Changed lists, sorted by path. An empty ref means HEAD.
// The tracked files come from git diff --numstat against ref; the untracked files not ignored, and every file of a
// repository without commits, are read whole from the working tree, each line counted as added. A file that
// vanished between listing and reading is left out.
func (r *Repo) Stats(ctx context.Context, ref string) ([]Stat, error) {
	if ref == "" {
		ref = "HEAD"
	}
	stats := map[string]Stat{}
	var whole []string
	if _, err := r.git(ctx, "rev-parse", "--verify", "--quiet", "HEAD"); err == nil {
		out, err := r.paths(ctx, "diff", "--numstat", "-z", "--no-renames", ref)
		if err != nil {
			return nil, err
		}
		if err := addNumstat(stats, out); err != nil {
			return nil, err
		}
	} else {
		out, err := r.paths(ctx, "ls-files", "-z")
		if err != nil {
			return nil, err
		}
		whole = append(whole, splitPaths(out)...)
	}
	out, err := r.paths(ctx, "ls-files", "-z", "--others", "--exclude-standard")
	if err != nil {
		return nil, err
	}
	whole = append(whole, splitPaths(out)...)
	for _, p := range whole {
		s, ok, err := r.readStat(p)
		if err != nil {
			return nil, err
		}
		if ok {
			stats[p] = s
		}
	}
	return sortStats(stats), nil
}

// CommitStats returns the size of the change of each file that the commit rev introduced, sorted by path: its
// diff against its first parent, or against the empty tree for a root commit. The working tree does not count.
func (r *Repo) CommitStats(ctx context.Context, rev string) ([]Stat, error) {
	if rev == "" || strings.HasPrefix(rev, "-") {
		return nil, fmt.Errorf("%q is not a commit", rev)
	}
	hash, err := r.git(ctx, "rev-parse", "--verify", "--quiet", rev+"^{commit}")
	if err != nil || hash == "" {
		return nil, fmt.Errorf("%q is not a commit", rev)
	}
	var out string
	if _, err := r.git(ctx, "rev-parse", "--verify", "--quiet", hash+"^1"); err == nil {
		out, err = r.paths(ctx, "diff", "--numstat", "-z", "--no-renames", hash+"^1", hash)
		if err != nil {
			return nil, err
		}
	} else {
		out, err = r.paths(ctx, "diff-tree", "-r", "--root", "--no-commit-id", "--numstat", "-z", "--no-renames", hash)
		if err != nil {
			return nil, err
		}
	}
	stats := map[string]Stat{}
	if err := addNumstat(stats, out); err != nil {
		return nil, err
	}
	return sortStats(stats), nil
}

// Summary returns the short hash and the subject of the commit rev, as one line.
func (r *Repo) Summary(ctx context.Context, rev string) (string, error) {
	if rev == "" || strings.HasPrefix(rev, "-") {
		return "", fmt.Errorf("%q is not a commit", rev)
	}
	return r.git(ctx, "log", "-1", "--no-decorate", "--format=%h %s", rev+"^{commit}", "--")
}

// readStat reads a file of the working tree whole: every line counts as added, and a NUL byte in its first
// bytes makes it binary. ok is false when the file is gone or is not a regular file.
func (r *Repo) readStat(rel string) (Stat, bool, error) {
	info, err := os.Lstat(filepath.Join(r.Root, filepath.FromSlash(rel)))
	if os.IsNotExist(err) {
		return Stat{}, false, nil
	}
	if err != nil {
		return Stat{}, false, err
	}
	if !info.Mode().IsRegular() {
		return Stat{}, false, nil
	}
	data, err := os.ReadFile(filepath.Join(r.Root, filepath.FromSlash(rel)))
	if os.IsNotExist(err) {
		return Stat{}, false, nil
	}
	if err != nil {
		return Stat{}, false, err
	}
	if bytes.IndexByte(data[:min(len(data), binaryProbe)], 0) >= 0 {
		return Stat{Path: rel, Binary: true}, true, nil
	}
	lines := bytes.Count(data, []byte{'\n'})
	if len(data) > 0 && data[len(data)-1] != '\n' {
		lines++
	}
	return Stat{Path: rel, Added: lines}, true, nil
}

// ResolveBase returns the merge base of HEAD with the closest of the candidate branches: each is tried as a
// local branch and then as origin/<name>, and among those that exist the one with the fewest commits from its
// merge base to HEAD wins; ties keep the order of the list. It fails, naming the candidates, when none exists.
func (r *Repo) ResolveBase(ctx context.Context, candidates []string) (string, error) {
	best, bestCount := "", -1
	for _, name := range candidates {
		for _, ref := range []string{"refs/heads/" + name, "refs/remotes/origin/" + name} {
			if _, err := r.git(ctx, "rev-parse", "--verify", "--quiet", ref+"^{commit}"); err != nil {
				continue
			}
			mb, err := r.git(ctx, "merge-base", "HEAD", ref)
			if err != nil || mb == "" {
				break
			}
			out, err := r.git(ctx, "rev-list", "--count", mb+"..HEAD")
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
func (r *Repo) ContentHash(ctx context.Context, scratchDir string) (string, error) {
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
	if _, err := run(ctx, r.Root, env, "add", "--all", "--", "."); err != nil {
		return "", err
	}
	return run(ctx, r.Root, env, "write-tree")
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

// Ignored reports whether the repository relative path is ignored by git; tracked files never are. Like Branch,
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

// addNumstat adds the records of a --numstat -z output without renames ("added\tdeleted\tpath\0") to stats; a
// "-" count marks a binary file.
func addNumstat(stats map[string]Stat, out string) error {
	for rec := range strings.SplitSeq(out, "\x00") {
		if rec == "" {
			continue
		}
		parts := strings.SplitN(rec, "\t", 3)
		if len(parts) != 3 || parts[2] == "" {
			return fmt.Errorf("unexpected numstat record %q", rec)
		}
		s := Stat{Path: parts[2]}
		if parts[0] == "-" || parts[1] == "-" {
			s.Binary = true
		} else {
			var err error
			if s.Added, err = strconv.Atoi(parts[0]); err != nil {
				return fmt.Errorf("unexpected numstat record %q", rec)
			}
			if s.Deleted, err = strconv.Atoi(parts[1]); err != nil {
				return fmt.Errorf("unexpected numstat record %q", rec)
			}
		}
		stats[s.Path] = s
	}
	return nil
}

// sortStats returns the stats of the map sorted by path.
func sortStats(stats map[string]Stat) []Stat {
	list := make([]Stat, 0, len(stats))
	for _, s := range stats {
		list = append(list, s)
	}
	sort.Slice(list, func(i, j int) bool { return list[i].Path < list[j].Path })
	return list
}
