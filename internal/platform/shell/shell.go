// Package shell reads a command line into the commands it runs, so a pattern can match each command (`just test` in
// `cd x && just test | tail -3`), the files a command writes can be found, and the exit code of the line is only
// trusted for a command whose failure propagates to it.
//
// A line is read with the bash parser of mvdan.cc/sh, whose syntax tree gives each command its exact words and
// redirections. A line the parser rejects falls back to a heuristic, not a shell parser: quotes and backslashes are
// respected, but not every construct of bash.
package shell

// Nesting says how a command is nested in another one.
type Nesting int

// Command is one command of a command line.
type Command struct {
	// Text is the command as written, with its redirections.
	Text string
	// Words are the words of the command, quotes removed, without its redirections and their targets.
	Words []string
	// Literal tells, for each word, whether it holds no expansion, glob, brace nor tilde, so its text is what the
	// command receives. The reading by heuristic does not know, so it marks none.
	Literal []bool
	// Writes are the targets of the redirections that write: >, >>, >|, <>, &>, &>> and >& to a file.
	Writes []string
	// Redirects is true when the command has a redirection other than a duplication of a descriptor (2>&1) or an
	// input.
	Redirects bool
	// Redirections are the redirections of the command as written (`2>&1`, `> out.txt`), every one of them. The
	// reading by heuristic leaves it empty.
	Redirections []string
	// Propagates is true when a failure of the command makes the whole line fail: it is the last command, or
	// it is followed by &&. After |, ;, || or &, the exit code of the line says nothing about it.
	Propagates bool
	// Nest is how the command is nested in another one, NotNested for a command of the line itself.
	Nest Nesting
}

// The kinds of nesting of a command.
const (
	// NotNested is a command of the line itself.
	NotNested Nesting = iota
	// Substitution is a command inside $(...), backticks or a process substitution.
	Substitution
	// Script is a command of the script of `sh -c`, `bash -c` or `zsh -c`.
	Script
	// Eval is a command of the arguments of `eval`.
	Eval
	// Compound is a command inside a subshell, a block, a condition, a loop or a function.
	Compound
)

// Nested reports whether the command is inside another one.
func (c Command) Nested() bool { return c.Nest != NotNested }

// Parse returns the commands of a command line, in order, and whether the syntax tree of a shell parser read it,
// false when the heuristic did. It reads the line as bash first; a line the parser rejects (an unclosed quote, a line
// of PowerShell) is read by the heuristic instead. windows says whether the system is Windows, where a backslash
// before a character that is not special to the shell stays, so a path such as C:\Users\x keeps its separators.
func Parse(line string, windows bool) (commands []Command, ast bool) {
	if cmds, err := parseAST(line, windows); err == nil {
		return cmds, true
	}
	return fallback(line), false
}
