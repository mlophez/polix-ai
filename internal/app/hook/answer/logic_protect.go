package answer

import (
	"path/filepath"
	"regexp"
	"strings"

	"github.com/mlophez/polix-ai/internal/app/agent"
	"github.com/mlophez/polix-ai/internal/app/workflow"
	"github.com/mlophez/polix-ai/internal/platform/glob"
)

// The reasons of the protection of the state of polix, which comes before the workflow.
const (
	reasonStateEdit    = "El estado de polix solo lo escribe polix."
	reasonStateCommand = "El estado de polix solo lo escribe polix; para leerlo usa polix log."
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

// writesFiles recognises commands that can create, change or delete files: a heuristic, since a shell command
// cannot be fully analysed, aimed at accidents rather than at an agent determined to get around it. A `>`
// followed by `&` and a digit or `-` duplicates a descriptor (`2>&1`) and writes nothing; `>&file` writes file.
// It guards the state, which any mention may reach; guardrails.protected_files and the condition writes go through
// lineWrites instead.
var writesFiles = regexp.MustCompile(`(^|[\s;&|(])(rm|mv|cp|tee|truncate|dd|touch|ln|install|chmod|rsync)(\s|$)|(sed|perl)\s+(-\S*\s+)*-i|>([^&]|&\s*[^0-9\s-]|$)`)

// guardrails returns the workflow whose guardrails the protections apply: the workflow when it is valid, else none.
func (r *pass) guardrails() workflow.Workflow {
	if r.ev.Invalid == nil && r.ev.Present {
		return r.ev.Workflow
	}
	return workflow.Workflow{}
}

// deny refuses the event for a protection with reason, logged as a denial of from (builtinProtection or the shortcut
// of guardrails) at line, the line of its item or 0.
func (r *pass) deny(from string, line int, reason string) (agent.Answer, bool) {
	r.log(from, line, "deny", reason)
	return agent.Answer{}.Refuse(reason), true
}

// mayWrite reports whether a command line may create, change or delete files, by the heuristic that guards the
// state.
func mayWrite(command string) bool { return writesFiles.MatchString(command) }

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
// the line, wherever it comes in it (see matchingCommands); false when none does.
func forbidden(list []workflow.ForbiddenCommand, line string) (workflow.ForbiddenCommand, bool) {
	for _, f := range list {
		if len(matchingCommands([]string{f.Pattern}, line)) > 0 {
			return f, true
		}
	}
	return workflow.ForbiddenCommand{}, false
}
