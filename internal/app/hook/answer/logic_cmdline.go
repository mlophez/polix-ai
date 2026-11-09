package answer

import (
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/mlophez/polix-ai/internal/platform/glob"
	"github.com/mlophez/polix-ai/internal/platform/shell"
)

// simpleCommand is one simple command of a line, parsed with its quotes: the words it runs, quotes removed, and the
// targets of its redirections apart. It is a heuristic, like shell.Split: variables, globs and substitutions are
// not expanded.
type simpleCommand struct {
	words  []string // the words of the command, without the redirections and their targets
	writes []string // targets of the redirections that write: >, >>, >|, <>, &>, &>> and >&file
}

// cmdParser is parseCommand reading a simple command one character at a time: the command parsed so far, the word
// being read, whether it holds a quoted or escaped character, the quote it is inside of and what the next word is.
type cmdParser struct {
	rs     []rune
	c      simpleCommand
	cur    strings.Builder
	inWord bool
	quoted bool
	quote  rune
	// pending says what the next word is: a target that writes ("w"), the word of >& ("dup"), a word to skip
	// ("skip": the target of <, <<, <<< and <&) or a plain word ("").
	pending string
}

// shellSpecial are the characters a backslash outside quotes makes literal.
const shellSpecial = " \t\n'\"\\$`<>|&;()*?[]{}#~!"

// wrappers run the command that follows them, after their own options.
var wrappers = map[string]bool{"sudo": true, "env": true, "command": true, "nohup": true, "time": true,
	"exec": true, "xargs": true, "nice": true}

// assignment is a variable set for the command, as in `LANG=C sort`.
var assignment = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*=`)

// fileWriters are the programs that create, change or remove the files they name.
var fileWriters = map[string]bool{"rm": true, "mv": true, "cp": true, "tee": true, "truncate": true, "dd": true,
	"touch": true, "ln": true, "install": true, "chmod": true, "rsync": true}

// onlyPolixCommands are the commands onlyPolix lets through.
var onlyPolixCommands = []string{"cd *", "polix config check*", "polix config fmt --check", "polix update*",
	"polix doctor*", "polix version*"}

// parseCommand splits a simple command into its words and the targets of its redirections. Outside quotes an
// operator of a redirection ends a word (`x>AGENTS.md` is x and a redirection to AGENTS.md), and a descriptor
// right before it (`2>>log`) belongs to it. `>&` followed by a digit or `-` duplicates or closes a descriptor and
// writes nothing; followed by anything else it writes that file, as in bash. An input (<), a here-document (<<)
// and a here-string (<<<) write nothing. | ; and & also end a word.
func parseCommand(text string) simpleCommand {
	p := &cmdParser{rs: []rune(text)}
	for i := 0; i < len(p.rs); i++ {
		i = p.read(i)
	}
	p.flush()
	return p.c
}

// at returns the character at i, or 0 past the end.
func (p *cmdParser) at(i int) rune {
	if i < len(p.rs) {
		return p.rs[i]
	}
	return 0
}

// read reads the character at i and returns the index of the last character it consumed.
func (p *cmdParser) read(i int) int {
	ch := p.rs[i]
	switch {
	case p.quote != 0:
		return p.inQuote(i)
	case ch == '\\' && p.at(i+1) == '\n':
		return i + 1 // a line continuation: both characters go away, and the word goes on or ends as without them
	case ch == '\\' && p.at(i+1) != 0 && strings.ContainsRune(shellSpecial, p.at(i+1)):
		// A backslash outside quotes makes a character of the shell literal (`\'`, `\>`, `\ `); before any other
		// character it stays, so a path of Windows such as C:\Users\x keeps its separators.
		p.cur.WriteRune(p.rs[i+1])
		p.inWord, p.quoted = true, true
		return i + 1
	case ch == '>' || ch == '<' || ch == '&' && p.at(i+1) == '>':
		return p.redirect(i)
	}
	p.plain(ch)
	return i
}

// inQuote reads the character at i inside a quote and returns the index of the last character it consumed.
func (p *cmdParser) inQuote(i int) int {
	ch := p.rs[i]
	switch {
	case ch == p.quote:
		p.quote = 0
	case ch == '\\' && p.quote == '"' && p.at(i+1) == '\n':
		return i + 1 // a line continuation: both characters go away
	case ch == '\\' && p.quote == '"' && strings.ContainsRune(`"\\$`+"`", p.at(i+1)):
		p.cur.WriteRune(p.rs[i+1])
		return i + 1
	default:
		p.cur.WriteRune(ch)
	}
	return i
}

// redirect reads the operator of a redirection at i, which ends the word before it unless that word is the
// descriptor of the redirection, and returns the index of its last character.
func (p *cmdParser) redirect(i int) int {
	if p.inWord && !p.quoted && p.pending == "" && strings.Trim(p.cur.String(), "0123456789") == "" {
		p.cur.Reset() // the descriptor of the redirection
		p.inWord = false
	} else {
		p.flush()
	}
	switch p.rs[i] {
	case '&': // &> and &>>
		i++
		if p.at(i+1) == '>' {
			i++
		}
		p.pending = "w"
		return i
	case '<':
		return p.input(i)
	}
	return p.output(i)
}

// output reads an operator that starts with >: >& duplicates a descriptor or writes a file (dup), and >, >> and >|
// write.
func (p *cmdParser) output(i int) int {
	p.pending = "w"
	switch p.at(i + 1) {
	case '&':
		p.pending = "dup"
		return i + 1
	case '>', '|':
		return i + 1
	}
	return i
}

// input reads an operator that starts with <: <> writes, and <, <<, <<< and <& write nothing.
func (p *cmdParser) input(i int) int {
	p.pending = "skip"
	switch p.at(i + 1) {
	case '>':
		p.pending = "w"
		return i + 1
	case '<':
		i++
		if p.at(i+1) == '<' {
			i++
		}
		return i
	case '&':
		return i + 1
	}
	return i
}

// plain reads a character outside quotes that is no redirection: a quote opens, | ; and & end the word and what it
// was pending, a blank ends the word, and any other character goes on it.
func (p *cmdParser) plain(ch rune) {
	switch ch {
	case '\'', '"':
		p.quote, p.inWord, p.quoted = ch, true, true
	case '|', ';', '&':
		p.flush()
		p.pending = ""
	case ' ', '\t', '\n':
		p.flush()
	default:
		p.cur.WriteRune(ch)
		p.inWord = true
	}
}

// flush ends the word being read, as a word of the command or the target of the pending redirection.
func (p *cmdParser) flush() {
	if !p.inWord {
		return
	}
	w := p.cur.String()
	switch p.pending {
	case "w":
		p.c.writes = append(p.c.writes, w)
	case "dup":
		if strings.Trim(w, "0123456789") != "" && w != "-" {
			p.c.writes = append(p.c.writes, w)
		}
	case "skip":
	default:
		p.c.words = append(p.c.words, w)
	}
	p.pending = ""
	p.cur.Reset()
	p.inWord, p.quoted = false, false
}

// program returns the name of the program a command runs and its arguments: after the assignments and the
// wrappers (sudo, env, xargs...), by the base name of its path, and `rm` or `mv` for `git rm` and `git mv`.
func (c simpleCommand) program() (name string, args []string) {
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
			return gitProgram(ws)
		}
		return name, ws
	}
	return "", nil
}

// gitProgram returns the program git runs with the arguments ws, past its options: rm or mv for git rm and git mv,
// else git itself.
func gitProgram(ws []string) (name string, args []string) {
	for len(ws) > 0 && strings.HasPrefix(ws[0], "-") {
		if ws[0] == "-C" || ws[0] == "-c" {
			ws = ws[1:]
		}
		ws = ws[1:]
	}
	if len(ws) > 0 && (ws[0] == "rm" || ws[0] == "mv") {
		return ws[0], ws[1:]
	}
	return "git", ws
}

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
		if inPlace(a[1:], plain) {
			return true
		}
	}
	return false
}

// inPlace reports whether a cluster of flags of one letter holds i before any flag that takes an argument (-e, -M,
// -I...), whose value is the rest of the word; plain are the flags that take none.
func inPlace(flags, plain string) bool {
	for _, f := range flags {
		if f == 'i' {
			return true
		}
		if !strings.ContainsRune(plain, f) {
			return false
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

// matchingCommands returns the commands of the line that match one of the patterns.
func matchingCommands(patterns []string, command string) []shell.Segment {
	var out []shell.Segment
	for _, seg := range shell.Split(command) {
		for _, p := range patterns {
			if glob.Command(p, seg.Text) {
				out = append(out, seg)
				break
			}
		}
	}
	return out
}

// propagates reports whether the failure of one of the commands reaches the exit code of the line: in
// `just test | tail`, tail decides it, so the exit code of the line says nothing of how just test ended.
func propagates(segs []shell.Segment) bool {
	for _, seg := range segs {
		if seg.Propagates {
			return true
		}
	}
	return false
}

// onlyPolix reports whether a command line only checks the workflow or updates polix (`polix config check`, `polix
// config fmt --check`, `polix update`, `polix doctor`, `polix version`, maybe after a cd): what an invalid or too new
// workflow needs. `polix config fmt` writes polix.yaml, so only its exact form with --check is one of them: any
// other flag after it (--check=false) could turn the write back on. A line with a command substitution ($( or `) or
// a redirection (< or >) is never one of them, since it could run or write anything behind an allowed command; only
// 2>&1 and 1>&2, which join the outputs and write no file, are let through.
func onlyPolix(command string) bool {
	if bare := strings.NewReplacer("2>&1", "", "1>&2", "").Replace(command); strings.ContainsAny(bare, "`<>") ||
		strings.Contains(bare, "$(") {
		return false
	}
	segs := shell.Split(command)
	for _, seg := range segs {
		if !slices.ContainsFunc(onlyPolixCommands, func(p string) bool { return glob.Command(p, seg.Text) }) {
			return false
		}
	}
	return len(segs) > 0
}
