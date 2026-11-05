package hook

import (
	"path"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/mlophez/polix-ai/pkg/glob"
)

// The reasons of the protections that come before the workflow.
const (
	ReasonStateEdit    = "El estado de polix solo lo escribe polix."
	ReasonStateCommand = "El estado de polix solo lo escribe polix; para leerlo usa polix log."
)

// stateRef finds the state folder in a command line, written relative to a git directory (`.git/polix`, or
// `.git/worktrees/<name>/polix` in a worktree). The name alone is not enough: it is also the name of the binary,
// so it must end the word or be followed by a separator of the shell, a slash or a glob. The folder is
// state.DirName.
var stateRef = regexp.MustCompile(`\.git/(worktrees/[^/\s'"]+/)?polix(/|[\s'";&|)*]|$)`)

// stateRefFold is stateRef ignoring case, for the systems whose file names ignore it.
var stateRefFold = regexp.MustCompile(`(?i)\.git/(worktrees/[^/\s'"]+/)?polix(/|[\s'";&|)*]|$)`)

// writesFiles recognises commands that can create, change or delete files: a heuristic, since a shell command
// cannot be fully analysed, aimed at accidents rather than at an agent determined to get around it. A `>`
// followed by `&` and a digit or `-` duplicates a descriptor (`2>&1`) and writes nothing; `>&file` writes file.
// It guards the state, which any mention may reach; protected paths go through Writes instead.
var writesFiles = regexp.MustCompile(`(^|[\s;&|(])(rm|mv|cp|tee|truncate|dd|touch|ln|install|chmod|rsync)(\s|$)|(sed|perl)\s+(-\S*\s+)*-i|>([^&]|&\s*[^0-9\s-]|$)`)

// FoldsCase reports whether the file names of the system goos usually ignore case.
func FoldsCase(goos string) bool { return goos == "darwin" || goos == "windows" }

// MayWrite reports whether a command line may create, change or delete files, by the heuristic that guards the
// state.
func MayWrite(command string) bool { return writesFiles.MatchString(command) }

// NamesState reports whether a command line names the state folder dir, by its path below a git directory or by
// dir itself or its real path, without case when fold is true.
func NamesState(command, dir, realDir string, fold bool) bool {
	re, norm := stateRef, func(s string) string { return s }
	if fold {
		re, norm = stateRefFold, strings.ToLower
	}
	if re.MatchString(filepath.ToSlash(command)) {
		return true
	}
	command = norm(command)
	for _, d := range []string{dir, realDir} {
		d = norm(d)
		if strings.Contains(command, d) || strings.Contains(command, filepath.ToSlash(d)) {
			return true
		}
	}
	return false
}

// Within reports whether the real path p is the real folder dir or inside it; without case when fold is true.
func Within(dir, p string, fold bool) bool {
	if fold {
		dir, p = strings.ToLower(dir), strings.ToLower(p)
	}
	rel, err := filepath.Rel(dir, p)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// AnyPathFold is glob.AnyPath without case when fold is true, so `.ENV` is `.env` where file names ignore case.
func AnyPathFold(patterns []string, p string, fold bool) bool {
	if !fold {
		return glob.AnyPath(patterns, p)
	}
	lower := make([]string, len(patterns))
	for i, pattern := range patterns {
		lower[i] = strings.ToLower(pattern)
	}
	return glob.AnyPath(lower, strings.ToLower(p))
}

// holdsAny returns the first pattern with a slash whose paths the folder dir holds: dir is the literal folder the
// pattern starts with (ai/rules for ai/rules/**, docs for docs/secret.md) or one of its parents. A pattern without
// a slash matches a name anywhere in the tree, so it names no folder. Without case when fold is true.
func holdsAny(patterns []string, dir string, fold bool) (string, bool) {
	if fold {
		dir = strings.ToLower(dir)
	}
	for _, pattern := range patterns {
		folders, _ := literalFolders(pattern, fold)
		for _, f := range folders {
			if f == dir {
				return pattern, true
			}
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
