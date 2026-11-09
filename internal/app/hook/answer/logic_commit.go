package answer

import (
	"path/filepath"
	"strings"

	"github.com/mlophez/polix-ai/internal/platform/shell"
)

// unquoted are the characters commitDirs refuses outside quotes: expansions ($, backtick, globs, braces, ~), groups
// and process substitutions, redirections, comments, escapes and a second line.
const unquoted = "$`*?[]{}()<>#\\\n~"

// commitDirs returns the folders where the commits of a command line commit, and whether the line can be placed at
// all. It is a strict allowlist, so a caller keeps the workflow whenever it is false. The line holds none of the
// characters with which bash expands, groups, escapes or comments (see placeable), and every command of it is
// `git` itself, with no option but -C, running `add` or `commit` (aliases, wrappers, assignments and any other
// subcommand, which may run programs, are not placed). Each -C folder is a literal path, no `~`, and on Windows one
// with its drive: not rooted without one (a Git Bash `/c/x`, or `\x`) nor relative to a drive (`C:x`). Each commit
// is placed in cwd (the root without one) after its -C options, joined without cleaning, so a `..` after a symlink
// is resolved as git resolves it on that system (through the link on Unix, before it on Windows).
func commitDirs(command, cwd, root string, windows bool) ([]string, bool) {
	if !placeable(command) {
		return nil, false
	}
	var out []string
	for _, seg := range shell.Split(command) {
		dir, commits, ok := parseCommand(seg.Text).gitPlace(cwd, root, windows)
		if !ok {
			return nil, false
		}
		if commits {
			out = append(out, dir)
		}
	}
	return out, len(out) > 0
}

// placeable reports whether bash runs the line as its words read: outside quotes, none of the characters with which
// it expands, groups, redirects, escapes or comments (unquoted) nor a second line; inside double quotes, no `$`,
// backtick or backslash, which still act there; inside single quotes, anything. An unclosed quote is not placeable.
func placeable(line string) bool {
	var quote rune
	for _, r := range line {
		switch {
		case quote == '\'':
			if r == '\'' {
				quote = 0
			}
		case quote == '"':
			if r == '"' {
				quote = 0
			} else if strings.ContainsRune("$`\\", r) {
				return false
			}
		case r == '\'' || r == '"':
			quote = r
		case strings.ContainsRune(unquoted, r):
			return false
		}
	}
	return quote == 0
}

// gitPlace places a command `git [-C dir]... add|commit ...`: the folder it runs in, cwd (the root without one)
// after its -C options (see changeDir), and whether it commits. ok is false for any other command (see commitDirs).
func (c simpleCommand) gitPlace(cwd, root string, windows bool) (dir string, commits, ok bool) {
	ws := c.words
	if len(ws) == 0 || ws[0] != "git" && ws[0] != "git.exe" {
		return "", false, false
	}
	ws = ws[1:]
	dir = cwd
	if dir == "" {
		dir = root
	}
	for len(ws) > 1 && ws[0] == "-C" {
		if dir, ok = changeDir(dir, ws[1], windows); !ok {
			return "", false, false
		}
		ws = ws[2:]
	}
	if len(ws) == 0 || ws[0] != "add" && ws[0] != "commit" {
		return "", false, false
	}
	return dir, ws[0] == "commit", true
}

// changeDir returns the folder git runs in after the option -C raw from dir, and false when it is not placed: on
// Windows, a path rooted without a drive (a Git Bash /c/x, or \x on the current drive) or relative to the folder of
// a drive (C:x).
func changeDir(dir, raw string, windows bool) (string, bool) {
	v := filepath.FromSlash(raw)
	switch {
	case windows && driveless(raw):
		return "", false
	case v == "":
		return dir, true
	case filepath.IsAbs(v) || strings.HasPrefix(raw, "/"):
		return v, true
	}
	return dir + string(filepath.Separator) + v, true
}

// driveless reports whether a path of Windows is rooted without a drive (/c/x or \x) or relative to the folder of a
// drive (C:x).
func driveless(raw string) bool {
	if filepath.VolumeName(raw) == "" && (strings.HasPrefix(raw, "/") || strings.HasPrefix(raw, `\`)) {
		return true
	}
	return len(raw) >= 2 && raw[1] == ':' && (len(raw) == 2 || raw[2] != '/' && raw[2] != '\\')
}
