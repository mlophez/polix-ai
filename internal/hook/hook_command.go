package hook

import (
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/mlophez/polix-ai/pkg/shell"
)

// command is one simple command of a line, parsed with its quotes: the words it runs, quotes removed, and the
// targets of its redirections apart. It is a heuristic, like shell.Split: variables, globs and substitutions are
// not expanded.
type command struct {
	words  []string // the words of the command, without the redirections and their targets
	writes []string // targets of the redirections that write: >, >>, >|, <>, &>, &>> and >&file
}

// parseCommand splits a simple command into its words and the targets of its redirections. Outside quotes an
// operator of a redirection ends a word (`x>AGENTS.md` is x and a redirection to AGENTS.md), and a descriptor
// right before it (`2>>log`) belongs to it. `>&` followed by a digit or `-` duplicates or closes a descriptor and
// writes nothing; followed by anything else it writes that file, as in bash. An input (<), a here-document (<<)
// and a here-string (<<<) write nothing. | ; and & also end a word.
func parseCommand(text string) command {
	var c command
	var cur strings.Builder
	inWord, quoted := false, false
	var quote rune
	// pending says what the next word is: a target that writes ("w"), the word of >& ("dup"), a word to skip
	// ("skip": the target of <, <<, <<< and <&) or a plain word ("").
	pending := ""
	flush := func() {
		if !inWord {
			return
		}
		w := cur.String()
		switch pending {
		case "w":
			c.writes = append(c.writes, w)
		case "dup":
			if strings.Trim(w, "0123456789") != "" && w != "-" {
				c.writes = append(c.writes, w)
			}
		case "skip":
		default:
			c.words = append(c.words, w)
		}
		pending = ""
		cur.Reset()
		inWord, quoted = false, false
	}
	rs := []rune(text)
	at := func(i int) rune {
		if i < len(rs) {
			return rs[i]
		}
		return 0
	}
	for i := 0; i < len(rs); i++ {
		ch := rs[i]
		if quote != 0 {
			switch {
			case ch == quote:
				quote = 0
			case ch == '\\' && quote == '"' && at(i+1) == '\n':
				i++ // a line continuation: both characters go away
			case ch == '\\' && quote == '"' && strings.ContainsRune(`"\\$`+"`", at(i+1)):
				i++
				cur.WriteRune(rs[i])
			default:
				cur.WriteRune(ch)
			}
			continue
		}
		if ch == '\\' && at(i+1) == '\n' {
			i++ // a line continuation: both characters go away, and the word goes on or ends as without them
			continue
		}
		// A backslash outside quotes makes a character of the shell literal (`\'`, `\>`, `\ `); before any other
		// character it stays, so a path of Windows such as C:\Users\x keeps its separators.
		if ch == '\\' && at(i+1) != 0 && strings.ContainsRune(shellSpecial, at(i+1)) {
			i++
			cur.WriteRune(rs[i])
			inWord, quoted = true, true
			continue
		}
		if ch == '>' || ch == '<' || ch == '&' && at(i+1) == '>' {
			if inWord && !quoted && pending == "" && strings.Trim(cur.String(), "0123456789") == "" {
				cur.Reset() // the descriptor of the redirection
				inWord = false
			} else {
				flush()
			}
			kind := "w"
			switch {
			case ch == '&': // &> and &>>
				i++
				if at(i+1) == '>' {
					i++
				}
			case ch == '>' && at(i+1) == '&':
				i++
				kind = "dup"
			case ch == '>' && (at(i+1) == '>' || at(i+1) == '|'):
				i++
			case ch == '<' && at(i+1) == '>':
				i++
			case ch == '<' && at(i+1) == '<':
				i++
				if at(i+1) == '<' {
					i++
				}
				kind = "skip"
			case ch == '<' && at(i+1) == '&':
				i++
				kind = "skip"
			case ch == '<':
				kind = "skip"
			}
			pending = kind
			continue
		}
		switch ch {
		case '\'', '"':
			quote, inWord, quoted = ch, true, true
		case '|', ';', '&':
			flush()
			pending = ""
		case ' ', '\t', '\n':
			flush()
		default:
			cur.WriteRune(ch)
			inWord = true
		}
	}
	flush()
	return c
}

// shellSpecial are the characters a backslash outside quotes makes literal.
const shellSpecial = " \t\n'\"\\$`<>|&;()*?[]{}#~!"

// wrappers run the command that follows them, after their own options.
var wrappers = map[string]bool{"sudo": true, "env": true, "command": true, "nohup": true, "time": true,
	"exec": true, "xargs": true, "nice": true}

// assignment is a variable set for the command, as in `LANG=C sort`.
var assignment = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*=`)

// program returns the name of the program a command runs and its arguments: after the assignments and the
// wrappers (sudo, env, xargs...), by the base name of its path, and `rm` or `mv` for `git rm` and `git mv`.
func (c command) program() (name string, args []string) {
	ws := c.words
	for len(ws) > 0 {
		if assignment.MatchString(ws[0]) {
			ws = ws[1:]
			continue
		}
		name = strings.TrimSuffix(path.Base(filepath.ToSlash(ws[0])), ".exe")
		ws = ws[1:]
		if wrappers[name] {
			for len(ws) > 0 && strings.HasPrefix(ws[0], "-") {
				ws = ws[1:]
			}
			continue
		}
		if name == "git" {
			for len(ws) > 0 && strings.HasPrefix(ws[0], "-") {
				if ws[0] == "-C" || ws[0] == "-c" {
					ws = ws[1:]
				}
				ws = ws[1:]
			}
			if len(ws) > 0 && (ws[0] == "rm" || ws[0] == "mv") {
				return ws[0], ws[1:]
			}
		}
		return name, ws
	}
	return "", nil
}

// fileWriters are the programs that create, change or remove the files they name.
var fileWriters = map[string]bool{"rm": true, "mv": true, "cp": true, "tee": true, "truncate": true, "dd": true,
	"touch": true, "ln": true, "install": true, "chmod": true, "rsync": true}

// writesItself reports whether a program writes the files it names: one of fileWriters, or sed and perl with -i.
func writesItself(name string, args []string) bool {
	if fileWriters[name] {
		return true
	}
	if name != "sed" && name != "perl" {
		return false
	}
	// The flags of one letter that take no argument, which may come before i in a cluster such as -pi or -Ei.
	plain := "nErsuz"
	if name == "perl" {
		plain = "acnpsTtUuWwXl0123456789"
	}
	for n, a := range args {
		if a == "--in-place" || strings.HasPrefix(a, "--in-place=") {
			return true
		}
		if !strings.HasPrefix(a, "-") || strings.HasPrefix(a, "--") {
			// perl reads its own flags only up to the script or the first word that is not the value of -e or -E:
			// what follows belongs to the script. sed reorders its arguments, so it reads them all.
			if name == "perl" && (n == 0 || args[n-1] != "-e" && args[n-1] != "-E") {
				return false
			}
			continue
		}
		for _, f := range a[1:] {
			if f == 'i' {
				return true
			}
			if !strings.ContainsRune(plain, f) {
				break // a flag with an argument (-e, -M, -I...): the rest of the word is its value
			}
		}
	}
	return false
}

// operands returns the arguments of a program that name files: not its options, and the file of `of=` for dd.
func operands(name string, args []string) []string {
	var out []string
	for _, a := range args {
		switch {
		case strings.HasPrefix(a, "-"):
		case name == "dd" && strings.HasPrefix(a, "of="):
			out = append(out, strings.TrimPrefix(a, "of="))
		case name == "dd":
		default:
			out = append(out, a)
		}
	}
	return out
}

// nullDevice reports whether a redirection target discards what it gets: /dev/null and the streams on every
// system (Git Bash maps them on Windows), and NUL on Windows, where it is the null device and not a file.
func nullDevice(target string, windows bool) bool {
	switch target {
	case "/dev/null", "/dev/stdout", "/dev/stderr":
		return true
	}
	return windows && strings.EqualFold(target, "nul")
}

// CommitDirs returns the folders where the commits of a command line commit, and whether the line can be placed at
// all. It is a strict allowlist, so a caller keeps the workflow whenever it is false. The line holds none of the
// characters with which bash expands, groups, escapes or comments (see unplaceable), and every command of it is
// `git` itself, with no option but -C, running `add` or `commit` (aliases, wrappers, assignments and any other
// subcommand, which may run programs, are not placed). Each -C folder is a literal path, no `~` and on Windows no
// Git Bash path such as `/c/x`. Each commit is placed in cwd (the root without one) after its -C options, joined
// without cleaning, so a `..` after a symlink is resolved as git resolves it: through the link.
func CommitDirs(command, cwd, root string, windows bool) ([]string, bool) {
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

// unquoted are the characters CommitDirs refuses outside quotes: expansions ($, backtick, globs, braces, ~), groups
// and process substitutions, redirections, comments, escapes and a second line.
const unquoted = "$`*?[]{}()<>#\\\n~"

// gitPlace places a command `git [-C dir]... add|commit ...`: the folder it runs in, cwd (the root without one)
// after its -C options, and whether it commits. ok is false for any other command (see CommitDirs).
func (c command) gitPlace(cwd, root string, windows bool) (dir string, commits, ok bool) {
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
		raw, v := ws[1], filepath.FromSlash(ws[1])
		ws = ws[2:]
		switch {
		case windows && (filepath.VolumeName(raw) == "" && (strings.HasPrefix(raw, "/") || strings.HasPrefix(raw, `\`)) ||
			len(raw) >= 2 && raw[1] == ':' && (len(raw) == 2 || raw[2] != '/' && raw[2] != '\\')):
			// On Windows, a path rooted without a drive (a Git Bash /c/x, or \x on the current drive) or relative to
			// the folder of a drive (C:x) is not placed.
			return "", false, false
		case v == "":
		case filepath.IsAbs(v) || strings.HasPrefix(raw, "/"):
			dir = v
		default:
			dir = dir + string(filepath.Separator) + v
		}
	}
	if len(ws) == 0 || ws[0] != "add" && ws[0] != "commit" {
		return "", false, false
	}
	return dir, ws[0] == "commit", true
}

// WritePaths returns the paths, absolute, that a command line may write, as Writes resolves them from cwd and the
// root: the targets of its redirections and the files named by the programs that write by themselves. Their real
// paths are what Writes needs, so a caller resolves them first.
func WritePaths(command, cwd, root string) []string {
	var out []string
	for _, seg := range shell.Split(command) {
		c := parseCommand(seg.Text)
		name, args := c.program()
		words := c.writes
		if writesItself(name, args) {
			words = append(slices.Clone(words), operands(name, args)...)
		}
		for _, w := range words {
			if abs := Absolute(root, cwd, filepath.FromSlash(w)); abs != "" {
				out = append(out, abs)
			}
		}
	}
	return out
}

// Writes returns the first path, relative to the root, that a command of the line writes and that matches one of
// the patterns, without case when fold is true. reals holds the real path of the root and of every path of
// WritePaths. A command writes the targets of its redirections inside the repository (not a null device), and,
// when its program writes files by itself (rm, mv, cp, sed -i...), every file it names, the ones it only reads
// included. Such a program also matches on a folder that holds what a pattern matches, or a wildcard that may
// reach it, and then held is that pattern: `rm -rf ai` or `rm -rf ai/*` for `ai/rules/**`; rm, and mv for a
// source, also on the root or a parent of it. A redirection only writes its target, never a folder.
func Writes(patterns []string, command, cwd, root string, reals map[string]string, fold, windows bool) (p, held string, ok bool) {
	if len(patterns) == 0 {
		return "", "", false
	}
	rel := func(w string) (string, bool) { return Rel(root, cwd, filepath.FromSlash(w), reals) }
	for _, seg := range shell.Split(command) {
		c := parseCommand(seg.Text)
		name, args := c.program()
		itself := writesItself(name, args)
		var written []string
		for _, t := range c.writes {
			if r, inside := rel(t); inside && !nullDevice(t, windows) {
				written = append(written, r)
			}
		}
		files := operands(name, args)
		if itself {
			for _, f := range files {
				if r, inside := rel(f); inside {
					written = append(written, r)
				}
			}
		}
		for _, w := range written {
			if AnyPathFold(patterns, w, fold) {
				return w, "", true
			}
		}
		if !itself {
			continue
		}
		for i, f := range files {
			r, inside := rel(f)
			removes := name == "rm" || name == "mv" && i < len(files)-1
			var pattern string
			var found bool
			switch {
			case r == "." || !inside && parentOnly(r):
				pattern, found = patterns[0], removes
			case !inside:
			case strings.ContainsAny(r, "*?"):
				pattern, found = wildcardHolds(patterns, r, fold)
			default:
				pattern, found = holdsAny(patterns, r, fold)
			}
			if found {
				return r, pattern, true
			}
		}
	}
	return "", "", false
}

// Absolute returns the path p given from cwd as the agent would reach it: p itself when absolute, else joined to
// cwd, or to the root without a cwd; "" for an empty path. A path rooted without a drive (`\x` on Windows) is
// taken on the drive of that base, not joined to it.
func Absolute(root, cwd, p string) string {
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

// Rel turns a path given from cwd (absolute, relative, with backslashes on Windows) into a slash separated path
// relative to the root, with the real paths of reals (symlinks resolved, as git reports the root); ok is false
// for a path outside the root. A path without its real path in reals is taken as it is.
func Rel(root, cwd, p string, reals map[string]string) (rel string, ok bool) {
	abs := Absolute(root, cwd, p)
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
