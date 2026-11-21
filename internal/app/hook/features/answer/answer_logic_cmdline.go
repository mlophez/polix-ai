package answer

import (
	"slices"
	"strings"

	"github.com/mlophez/polix-ai/internal/platform/glob"
	"github.com/mlophez/polix-ai/internal/platform/shell"
)

// fileWriters are the programs that create, change or remove the files they name.
var fileWriters = map[string]bool{"rm": true, "mv": true, "cp": true, "tee": true, "truncate": true, "dd": true,
	"touch": true, "ln": true, "install": true, "chmod": true, "rsync": true}

// onlyPolixCommands are the commands onlyPolix lets through.
var onlyPolixCommands = []string{"cd *", "polix config check*", "polix config fmt --check", "polix update*",
	"polix doctor*", "polix version*"}

// program returns the name of the program a command runs and its arguments, from its normalized words (past the
// assignments, the wrappers such as sudo, env or xargs, and the global options of git, see shell.Command.Normal):
// the base name of its path, and `rm` or `mv` for `git rm` and `git mv`.
func program(c shell.Command) (name string, args []string) {
	ws := c.Normal()
	if len(ws) == 0 {
		return "", nil
	}
	name = shell.Program(ws[0])
	if name == "git" {
		return gitProgram(ws[1:])
	}
	return name, ws[1:]
}

// gitProgram returns the program git runs with the arguments ws, from its subcommand: rm or mv for git rm and git
// mv, else git itself.
func gitProgram(ws []string) (name string, args []string) {
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

// matchingCommands returns the commands of the line, nested ones included, that match one of the patterns as written
// or in their normalized form (see shell.Command.Normal), so `git add*` holds for `git -C x add .`; windows says
// whether the system is Windows.
func matchingCommands(patterns []string, command string, windows bool) []shell.Command {
	var out []shell.Command
	cmds, _ := shell.Parse(command, windows)
	for _, c := range cmds {
		normal := c.NormalText()
		for _, p := range patterns {
			if glob.Command(p, c.Text) || glob.Command(p, normal) {
				out = append(out, c)
				break
			}
		}
	}
	return out
}

// propagates reports whether the failure of one of the commands reaches the exit code of the line: in
// `just test | tail`, tail decides it, so the exit code of the line says nothing of how just test ended.
func propagates(cmds []shell.Command) bool {
	for _, c := range cmds {
		if c.Propagates {
			return true
		}
	}
	return false
}

// onlyPolix reports whether a command line only checks the workflow or updates polix (`polix config check`, `polix
// config fmt --check`, `polix update`, `polix doctor`, `polix version`, maybe after a cd): what an invalid or too new
// workflow needs. `polix config fmt` writes polix.yaml, so only its exact form with --check is one of them: any
// other flag after it (--check=false) could turn the write back on. A line with a command substitution ($( or `) or
// a redirection is never one of them, since it could run or write anything behind an allowed command; only 2>&1 and
// 1>&2, which join the outputs and write no file, are let through. On a line parsed as bash, every command, the
// nested ones included, must be one of onlyPolixCommands as written and have no other redirection, so a quoted `>`
// is text (`polix doctor "x>y"`) and `polix doctor $(rm -rf ai)` is refused by its rm; a clause (if, for, case...)
// or a test, a command without words, is refused, so a word the parse does not walk fails closed. A line the parser
// rejects is refused when its text holds `$(`, a backtick, `<` or `>` (2>&1 and 1>&2 aside). windows says whether
// the system is Windows.
func onlyPolix(command string, windows bool) bool {
	cmds, ast := shell.Parse(command, windows)
	if !ast {
		if bare := strings.NewReplacer("2>&1", "", "1>&2", "").Replace(command); strings.ContainsAny(bare, "`<>") ||
			strings.Contains(bare, "$(") {
			return false
		}
	}
	for _, c := range cmds {
		if ast && len(c.Words) == 0 || // a clause or a test, whose words are not all read: fail closed
			slices.ContainsFunc(c.Redirections, func(r string) bool { return r != "2>&1" && r != "1>&2" }) ||
			!slices.ContainsFunc(onlyPolixCommands, func(p string) bool { return glob.Command(p, c.Text) }) {
			return false
		}
	}
	return len(cmds) > 0
}
