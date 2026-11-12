package answer

import (
	"path"
	"path/filepath"
	"slices"
	"strings"

	"github.com/mlophez/polix-ai/internal/platform/glob"
	"github.com/mlophez/polix-ai/internal/platform/shell"
)

// writeCheck is what lineWrites checks the commands of a line against: the patterns, the cwd and the root their
// words are resolved from, the real paths of reals, and whether the file names ignore case (fold) and the system is
// Windows.
type writeCheck struct {
	patterns      []string
	cwd, root     string
	reals         map[string]string
	fold, windows bool
}

// writePaths returns the paths, absolute, that a command line may write, as lineWrites resolves them from cwd and
// the root: the targets of its redirections and the files named by the programs that write by themselves. Their
// real paths are what lineWrites needs, so a caller resolves them first.
func writePaths(command, cwd, root string, windows bool) []string {
	var out []string
	cmds, _ := shell.Parse(command, windows)
	for _, c := range cmds {
		name, args := program(c)
		words := c.Writes
		if writesItself(name, args) {
			words = append(slices.Clone(words), operands(name, args)...)
		}
		for _, w := range words {
			if abs := absolute(root, cwd, filepath.FromSlash(w)); abs != "" {
				out = append(out, abs)
			}
		}
	}
	return out
}

// lineWrites returns the first path, relative to the root, that a command of the line writes and that matches one of
// the patterns, without case when fold is true. reals holds the real path of the root and of every path of
// writePaths. A command writes the targets of its redirections inside the repository (not a null device), and,
// when its program writes files by itself (rm, mv, cp, sed -i...), every file it names, the ones it only reads
// included. Such a program also matches on a folder that holds what a pattern matches, or a wildcard that may
// reach it, and then held is that pattern: `rm -rf ai` or `rm -rf ai/*` for `ai/rules/**`; rm, and mv for a
// source, also on the root or a parent of it. A redirection only writes its target, never a folder.
func lineWrites(patterns []string, command, cwd, root string, reals map[string]string, fold, windows bool) (p,
	held string, ok bool) {
	if len(patterns) == 0 {
		return "", "", false
	}
	w := writeCheck{patterns: patterns, cwd: cwd, root: root, reals: reals, fold: fold, windows: windows}
	cmds, _ := shell.Parse(command, windows)
	for _, c := range cmds {
		if p, held, ok := w.command(c); ok {
			return p, held, true
		}
	}
	return "", "", false
}

// command returns the first path the command c writes that matches a pattern: one it writes (see written) or, for a
// program that writes by itself, a folder or a wildcard of its files that holds one (see holds).
func (w writeCheck) command(c shell.Command) (p, held string, ok bool) {
	name, args := program(c)
	itself := writesItself(name, args)
	files := operands(name, args)
	for _, r := range w.written(c.Writes, files, itself) {
		if anyPathFold(w.patterns, r, w.fold) {
			return r, "", true
		}
	}
	if !itself {
		return "", "", false
	}
	return w.holds(name, files)
}

// written returns the paths relative to the root a command writes: the targets of its redirections inside the
// repository but the null devices and, when its program writes by itself, every one of its files inside it.
func (w writeCheck) written(targets, files []string, itself bool) []string {
	var out []string
	for _, t := range targets {
		if r, inside := w.rel(t); inside && !nullDevice(t, w.windows) {
			out = append(out, r)
		}
	}
	if !itself {
		return out
	}
	for _, f := range files {
		if r, inside := w.rel(f); inside {
			out = append(out, r)
		}
	}
	return out
}

// holds returns the first file of the program name, which writes by itself, that holds what a pattern matches, with
// that pattern: a folder of it (see holdsAny), a wildcard that may reach it (see wildcardHolds) and, for rm and the
// sources of mv, the root or a parent of it.
func (w writeCheck) holds(name string, files []string) (p, held string, ok bool) {
	for i, f := range files {
		r, inside := w.rel(f)
		removes := name == "rm" || name == "mv" && i < len(files)-1
		var pattern string
		var found bool
		switch {
		case r == "." || !inside && parentOnly(r):
			pattern, found = w.patterns[0], removes
		case !inside:
		case strings.ContainsAny(r, "*?"):
			pattern, found = wildcardHolds(w.patterns, r, w.fold)
		default:
			pattern, found = holdsAny(w.patterns, r, w.fold)
		}
		if found {
			return r, pattern, true
		}
	}
	return "", "", false
}

// rel returns a word of the line relative to the root (see relative).
func (w writeCheck) rel(word string) (string, bool) {
	return relative(w.root, w.cwd, filepath.FromSlash(word), w.reals)
}

// absolute returns the path p given from cwd as the agent would reach it: p itself when absolute, else joined to
// cwd, or to the root without a cwd; "" for an empty path. A path rooted without a drive (`\x` on Windows) is
// taken on the drive of that base, not joined to it.
func absolute(root, cwd, p string) string {
	if p == "" {
		return ""
	}
	if !filepath.IsAbs(p) {
		base := cwd
		if base == "" {
			base = root
		}
		if filepath.VolumeName(p) == "" && (p[0] == '/' || p[0] == filepath.Separator) {
			p = filepath.VolumeName(base) + p
		} else {
			p = filepath.Join(base, p)
		}
	}
	return filepath.Clean(p)
}

// relative turns a path given from cwd (absolute, relative, with backslashes on Windows) into a slash separated path
// relative to the root, with the real paths of reals (symlinks resolved, as git reports the root); ok is false
// for a path outside the root. A path without its real path in reals is taken as it is.
func relative(root, cwd, p string, reals map[string]string) (rel string, ok bool) {
	abs := absolute(root, cwd, p)
	if abs == "" {
		return "", false
	}
	r, err := filepath.Rel(realOf(reals, root), realOf(reals, abs))
	if err != nil {
		return "", false
	}
	r = filepath.ToSlash(r)
	if r == ".." || strings.HasPrefix(r, "../") {
		return r, false
	}
	return r, true
}

// realOf returns the real path of p in reals, or p when it is not there.
func realOf(reals map[string]string, p string) string {
	if r, ok := reals[p]; ok {
		return r
	}
	return p
}

// parentOnly reports whether a path relative to the root names a parent of it (.., ../..), which holds it whole.
func parentOnly(rel string) bool {
	if rel == "" {
		return false
	}
	for s := range strings.SplitSeq(rel, "/") {
		if s != ".." {
			return false
		}
	}
	return true
}

// holdsAny returns the first pattern with a slash whose paths the folder dir holds: dir is the literal folder the
// pattern starts with (ai/rules for ai/rules/**, docs for docs/secret.md) or one of its parents. A pattern without
// a slash matches a name anywhere in the tree, so it names no folder. Without case when fold is true.
func holdsAny(patterns []string, dir string, fold bool) (string, bool) {
	if fold {
		dir = strings.ToLower(dir)
	}
	for _, pattern := range patterns {
		if folders, _ := literalFolders(pattern, fold); slices.Contains(folders, dir) {
			return pattern, true
		}
	}
	return "", false
}

// wildcardHolds returns the first pattern that a word with wildcards may write: the word matches the pattern
// when it has no wildcards (`*` for `.env`, `docs/*.md` for `docs/secret.md`), or one of the literal folders the
// pattern starts with (`ai/*` and `*` for `ai/rules/**`), or it lies inside the folder of a pattern that takes
// everything below it (`ai/rules/*` for `ai/rules/**`). Without case when fold is true.
func wildcardHolds(patterns []string, word string, fold bool) (string, bool) {
	if fold {
		word = strings.ToLower(word)
	}
	dir := word
	if i := strings.IndexAny(dir, "*?"); i >= 0 {
		dir = path.Dir(dir[:i+1])
	}
	for _, pattern := range patterns {
		folders, literal := literalFolders(pattern, fold)
		candidates := folders
		if literal {
			candidates = append(candidates, foldPattern(pattern, fold))
		}
		for _, c := range candidates {
			// A word without a slash stays in its folder: `*.md` at the root reaches docs, not docs/secret.md.
			if !strings.Contains(word, "/") {
				c, _, _ = strings.Cut(c, "/")
			}
			if glob.Path(word, c) {
				return pattern, true
			}
		}
		if p := foldPattern(pattern, fold); len(folders) > 0 && p == folders[len(folders)-1]+"/**" {
			if f := folders[len(folders)-1]; dir == f || strings.HasPrefix(dir, f+"/") {
				return pattern, true
			}
		}
	}
	return "", false
}

// literalFolders returns the folders a pattern with a slash starts with, from the outermost (ai and ai/rules for
// ai/rules/**), folded when fold is true, and whether the pattern has no wildcards at all.
func literalFolders(pattern string, fold bool) (folders []string, literal bool) {
	pattern = foldPattern(pattern, fold)
	literal = !strings.ContainsAny(pattern, "*?")
	if !strings.Contains(pattern, "/") {
		return nil, literal
	}
	segs := strings.Split(pattern, "/")
	for n := 1; n < len(segs) && !strings.ContainsAny(segs[n-1], "*?"); n++ {
		folders = append(folders, strings.Join(segs[:n], "/"))
	}
	return folders, literal
}

// foldPattern trims the slashes around a pattern and folds its case when fold is true.
func foldPattern(pattern string, fold bool) string {
	pattern = strings.Trim(pattern, "/")
	if fold {
		pattern = strings.ToLower(pattern)
	}
	return pattern
}
