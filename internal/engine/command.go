package engine

import (
	"path"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"

	"github.com/mlophez/polix-ai/internal/shell"
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
		switch {
		case ch == '\'' || ch == '"':
			quote, inWord, quoted = ch, true, true
		case ch == '|' || ch == ';' || ch == '&':
			flush()
			pending = ""
		case ch == ' ' || ch == '\t' || ch == '\n':
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
func nullDevice(target string) bool {
	switch target {
	case "/dev/null", "/dev/stdout", "/dev/stderr":
		return true
	}
	return runtime.GOOS == "windows" && strings.EqualFold(target, "nul")
}

// writes returns the first path, relative to the root, that a command of the line writes and that matches one of
// the patterns, without case on the systems whose file names ignore it. A command writes the targets of its
// redirections inside the repository (not a null device), and, when its program writes files by itself (rm, mv,
// cp, sed -i...), every file it names, the ones it only reads included. Such a program also matches on a folder
// that holds what a pattern matches, or a wildcard that may reach it, and then held is that pattern: `rm -rf ai`
// or `rm -rf ai/*` for `ai/rules/**`; rm, and mv for a source, also on the root or a parent of it. A redirection
// only writes its target, never a folder.
func (en *Engine) writes(patterns []string, ev Event) (path, held string, ok bool) {
	if len(patterns) == 0 {
		return "", "", false
	}
	rel := func(p string) (string, bool) { return en.Repo.Rel(filepath.FromSlash(p), ev.Cwd) }
	for _, seg := range shell.Split(ev.Command) {
		c := parseCommand(seg.Text)
		name, args := c.program()
		itself := writesItself(name, args)
		var written []string
		for _, t := range c.writes {
			if r, inside := rel(t); inside && !nullDevice(t) {
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
		for _, p := range written {
			if anyPathFold(patterns, p) {
				return p, "", true
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
				pattern, found = wildcardHolds(patterns, r)
			default:
				pattern, found = holdsAny(patterns, r)
			}
			if found {
				return r, pattern, true
			}
		}
	}
	return "", "", false
}

// parentOnly reports whether a path relative to the root names a parent of it (.., ../..), which holds it whole.
func parentOnly(rel string) bool {
	if rel == "" {
		return false
	}
	for _, s := range strings.Split(rel, "/") {
		if s != ".." {
			return false
		}
	}
	return true
}
