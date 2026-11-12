package answer

import (
	"path/filepath"
	"regexp"
	"strings"

	"github.com/mlophez/polix-ai/internal/app/agent"
	"github.com/mlophez/polix-ai/internal/app/workflow"
	"github.com/mlophez/polix-ai/internal/platform/glob"
	"github.com/mlophez/polix-ai/internal/platform/shell"
)

// The reasons of the protection of the state of polix, which comes before the workflow.
const (
	reasonStateEdit    = "Only polix writes the state of polix."
	reasonStateCommand = "Only polix writes the state of polix; to read it, use polix log."
)

// reasonForbidden is the reason of a forbidden command without a reason of its own, in English since polix writes
// it; the line of the command follows it.
const reasonForbidden = "This command is forbidden by " + workflow.File + ": a person must run it."

// Where a denial of the protections comes from, as the log records it: the protection of the state, built in, or the
// shortcut of guardrails it applies.
const (
	builtinProtection         = "builtin"
	protectedFilesShortcut    = "guardrails.protected_files"
	forbiddenCommandsShortcut = "guardrails.forbidden_commands"
)

// stateRef finds the state folder in a command line, written relative to a git directory (`.git/polix`, or
// `.git/worktrees/<name>/polix` in a worktree). The name alone is not enough: it is also the name of the binary,
// so it must end the word or be followed by a separator of the shell, a slash or a glob. The folder is
// state.DirName.
var stateRef = regexp.MustCompile(`\.git/(worktrees/[^/\s'"]+/)?polix(/|[\s'";&|)*]|$)`)

// stateRefFold is stateRef ignoring case, for the systems whose file names ignore it.
var stateRefFold = regexp.MustCompile(`(?i)\.git/(worktrees/[^/\s'"]+/)?polix(/|[\s'";&|)*]|$)`)

// writesFiles recognises commands that can create, change or delete files in the text of a line: a heuristic, the
// fallback of mayWrite for a line the bash parser rejects. A `>` followed by `&` and a digit or `-` duplicates a
// descriptor (`2>&1`) and writes nothing; `>&file` writes file.
var writesFiles = regexp.MustCompile(`(^|[\s;&|(])(rm|mv|cp|tee|truncate|dd|touch|ln|install|chmod|rsync)(\s|$)|(sed|perl)\s+(-\S*\s+)*-i|>([^&]|&\s*[^0-9\s-]|$)`)

// interpreters are the programs that run a script polix does not see whole (from stdin, a file, a substitution, a
// word that is not literal, or the language of another interpreter): for mayWrite they may write.
var interpreters = map[string]bool{"sh": true, "bash": true, "zsh": true, "dash": true, "ksh": true, "mksh": true,
	"fish": true, "ash": true, "csh": true, "tcsh": true, "pwsh": true, "powershell": true, "eval": true,
	"source": true, "awk": true, "gawk": true, "mawk": true, "nawk": true, "perl": true, "python": true, "pypy": true,
	"ruby": true, "node": true, "deno": true, "bun": true, "php": true, "lua": true}

// version is the version a program name may end with (python3.12, perl5.34), dropped to find its family.
var version = regexp.MustCompile(`[0-9][0-9.]*$`)

// guardrails returns the workflow whose guardrails the protections apply: the workflow when it is valid, else none.
func (r *pass) guardrails() workflow.Workflow {
	if r.ev.Invalid == nil && r.ev.Present {
		return r.ev.Workflow
	}
	return workflow.Workflow{}
}

// deny refuses the event for a protection with reason, logged as a denial of from (builtinProtection or the shortcut
// of guardrails) at line, the line of its item or 0. The pending warnings of the call are logged as not shown (see
// warn).
func (r *pass) deny(from string, line int, reason string) (agent.Answer, bool) {
	r.log(from, line, "deny", reason)
	return r.warn(agent.Answer{}.Refuse(reason)), true
}

// mayWrite reports whether a command line may create, change or delete files, to guard the state. It errs on the
// side of the guard, since it only refuses a line that also names the state (see namesState), and a refusal costs
// little: polix log reads it. It is never weaker than the heuristic of the text: writesFiles on the line, without its
// redirections into the null device (see withoutNullRedirects), decides first, on any line. On a line the bash
// parser accepts, the line also may write when some command of it, nested ones included:
//   - redirects into a file other than the null device;
//   - has a word, its program or an argument, that writes by itself (see writesItself) with the words after it,
//     so a writer run by find -exec, xargs or a wrapper with a value (`sudo -u x rm`) counts;
//   - has a word naming one of the shells or interpreters polix knows, with any version (see interpreters and
//     version, without case), whose script polix does not see whole, runs `.`, or runs a program whose name is not
//     literal (see unknownProgram).
func mayWrite(command string, windows bool) bool {
	if writesFiles.MatchString(withoutNullRedirects(command)) {
		return true
	}
	cmds, ast := shell.Parse(command, windows)
	if !ast {
		return false
	}
	for _, c := range cmds {
		for _, w := range c.Writes {
			if !nullDevice(w, windows) {
				return true
			}
		}
		// `.` is source only as the program: as an argument it is a folder or a filter (`jq . x`).
		if name, args := program(c); name == "." || writesItself(strings.ToLower(name), args) || unknownProgram(c) {
			return true
		}
		for i, w := range c.Words {
			// Without case: macOS and Windows run `Bash` and `PWSH.EXE` as bash and pwsh.
			name := shell.Program(strings.ToLower(w))
			if interpreters[version.ReplaceAllString(name, "")] || writesItself(name, c.Words[i+1:]) {
				return true
			}
		}
	}
	return false
}

// unknownProgram reports whether the program of the command c, its first word past the assignments, is not literal
// (`$SHELL -c ...`, `"$X" ...`): it may be any program, a shell or a writer.
func unknownProgram(c shell.Command) bool {
	for i, w := range c.Words {
		if !assignmentWord.MatchString(w) {
			return !c.Literal[i]
		}
	}
	return false
}

// nullRedirect is a redirection into /dev/null (`2>/dev/null`, `&> /dev/null`), which writes no file on any system,
// Git Bash included: mayWrite drops it from the text before writesFiles reads it. It ends at a blank, the end of the
// line or an operator (`2>/dev/null|head`), which stays. NUL is left out: under Git Bash `> nul` may create a file.
var nullRedirect = regexp.MustCompile(`(^|\s)(\d*|&)>>?\s*/dev/null(\s|$|[;&|)])`)

// withoutNullRedirects returns command without its redirections into /dev/null (see nullRedirect), each replaced by
// a space and the operator after it, so two of them in a row are both dropped.
func withoutNullRedirects(command string) string {
	for {
		next := nullRedirect.ReplaceAllString(command, " $3")
		if next == command {
			return command
		}
		command = next
	}
}

// assignmentWord is a variable set for the command, as in `LANG=C sort`.
var assignmentWord = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*=`)

// namesState reports whether a command line names the state folder dir, by its path below a git directory or by
// dir itself or its real path, without case when fold is true.
func namesState(command, dir, realDir string, fold bool) bool {
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

// within reports whether the real path p is the real folder dir or inside it; without case when fold is true.
// It compares the cleaned paths by prefix rather than with filepath.Rel, which ignores case on Windows whatever
// fold says.
func within(dir, p string, fold bool) bool {
	dir, p = filepath.Clean(dir), filepath.Clean(p)
	if fold {
		dir, p = strings.ToLower(dir), strings.ToLower(p)
	}
	return p == dir || strings.HasPrefix(p, strings.TrimSuffix(dir, string(filepath.Separator))+string(filepath.Separator))
}

// anyPathFold is glob.AnyPath without case when fold is true, so `.ENV` is `.env` where file names ignore case.
func anyPathFold(patterns []string, p string, fold bool) bool {
	if !fold {
		return glob.AnyPath(patterns, p)
	}
	lower := make([]string, len(patterns))
	for i, pattern := range patterns {
		lower[i] = strings.ToLower(pattern)
	}
	return glob.AnyPath(lower, strings.ToLower(p))
}

// forbidden returns the first of the forbidden commands, in file order, whose pattern matches one of the commands of
// the line, wherever it comes in it (see matchingCommands); false when none does. windows says whether the system is
// Windows.
func forbidden(list []workflow.ForbiddenCommand, line string, windows bool) (workflow.ForbiddenCommand, bool) {
	for _, f := range list {
		if len(matchingCommands([]string{f.Pattern}, line, windows)) > 0 {
			return f, true
		}
	}
	return workflow.ForbiddenCommand{}, false
}
