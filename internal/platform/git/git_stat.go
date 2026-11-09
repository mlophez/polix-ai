package git

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// Stat is the size of the change of one file: its added and deleted lines, or Binary with both at zero.
type Stat struct {
	Path    string // relative to the root, with forward slashes
	Added   int
	Deleted int
	Binary  bool
}

// binaryProbe is how many leading bytes of a file are searched for a NUL byte, as git does, to call it binary.
const binaryProbe = 8000

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
