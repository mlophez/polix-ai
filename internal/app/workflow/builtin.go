package workflow

import (
	"errors"
	"slices"
)

// Builtin is guardrails.builtin: the built-in good practices polix applies unless the repository turns them off,
// each true when polix.yaml leaves it out, as the whole map. The decoding of the file fills the switches, the line
// of the key builtin, 0 when it is not written, and what is wrong with its values, each at the line of its key, for
// Guardrails.Problems to report it with the rest; a key with a wrong value stays on.
type Builtin struct {
	// ForbidCommitNoVerify refuses an agent's git commit that skips the hooks of git, in every repository, with gates
	// of git or without them (forbid_commit_no_verify): a preset of items of forbidden_commands.
	ForbidCommitNoVerify bool
	// ForbidGitDestructiveCommands refuses an agent's forced git push, git reset --hard and forced git clean
	// (forbid_git_destructive_commands): a preset of items of forbidden_commands.
	ForbidGitDestructiveCommands bool
	// FormatPolixFiles runs polix config fmt after an edit of the root polix.yaml and polix rules fmt after an edit
	// of a yaml file of the rules folder (format_polix_files).
	FormatPolixFiles bool
	// CheckPolixFiles runs polix config check on every commit that stages the root polix.yaml, as a gate of git
	// (check_polix_files).
	CheckPolixFiles bool
	Line            int
	Problems        []GuardrailProblem
}

// GuardrailProblem is what is wrong with a value of guardrails, at the line of the key that holds it.
type GuardrailProblem struct {
	Text string
	Line int
}

// builtinPreset is an item of forbidden_commands a preset of builtin adds: the key of builtin that turns it on, its
// pattern and its reason.
type builtinPreset struct {
	key, pattern, reason string
}

// The shortcut builtin of guardrails and its keys.
const (
	ShortcutBuiltin                     = "builtin"
	BuiltinForbidCommitNoVerify         = "forbid_commit_no_verify"
	BuiltinForbidGitDestructiveCommands = "forbid_git_destructive_commands"
	BuiltinFormatPolixFiles             = "format_polix_files"
	BuiltinCheckPolixFiles              = "check_polix_files"
)

// BuiltinKeys are the keys of guardrails.builtin, in the order polix config fmt writes them: the blocks first, then
// the format and the check of the polix files.
var BuiltinKeys = []string{BuiltinForbidCommitNoVerify, BuiltinForbidGitDestructiveCommands, BuiltinFormatPolixFiles,
	BuiltinCheckPolixFiles}

// The commands of the built-in format and check of the polix files, which the polix on the PATH runs.
var (
	builtinConfigFmt   = []string{"polix", "config", "fmt"}
	builtinRulesFmt    = []string{"polix", "rules", "fmt"}
	builtinConfigCheck = []string{"polix", "config", "check"}
)

// builtinRootFile is the path glob of the polix.yaml of the root only, never one in another folder.
const builtinRootFile = "/" + File

// The reasons of the presets of the built-in blocks, in English since polix writes them; the refused line follows
// each one.
const (
	reasonNoVerify = "Never skip the git hooks: fix what they block and commit again."
	reasonPush     = "Never force a push: ask the user to run it if it is needed."
	reasonReset    = "git reset --hard discards changes for good: ask the user to run it."
	reasonClean    = "git clean -f deletes untracked files for good: ask the user to run it."
)

// builtinPresets are the items of forbidden_commands of the two presets, in the order they come first in the
// forbidden commands: those of forbid_commit_no_verify, then those of forbid_git_destructive_commands. Each pattern
// is a glob of forbidden_commands, matched against the text of each command as any written item; the option counts
// in any position after the subcommand, and -n of git commit only as a word of its own, so -nm is not refused.
var builtinPresets = []builtinPreset{
	{BuiltinForbidCommitNoVerify, "git commit*--no-verify*", reasonNoVerify},
	{BuiltinForbidCommitNoVerify, "git commit* -n *", reasonNoVerify},
	{BuiltinForbidCommitNoVerify, "git commit* -n", reasonNoVerify},
	{BuiltinForbidGitDestructiveCommands, "git push*--force*", reasonPush},
	{BuiltinForbidGitDestructiveCommands, "git push* -f*", reasonPush},
	{BuiltinForbidGitDestructiveCommands, "git reset*--hard*", reasonReset},
	{BuiltinForbidGitDestructiveCommands, "git clean* -f*", reasonClean},
	{BuiltinForbidGitDestructiveCommands, "git clean*--force*", reasonClean},
}

// Valid tells why the section builtin is not valid: the problems of its values, or a problem without its text or
// its line.
func (b Builtin) Valid() error {
	if len(b.Problems) == 0 {
		return nil
	}
	texts := make([]string, 0, len(b.Problems))
	for _, p := range b.Problems {
		if err := p.Valid(); err != nil {
			return err
		}
		texts = append(texts, p.Text)
	}
	return InvalidError(texts)
}

// Valid tells why the problem is not valid: it has no text or no line.
func (p GuardrailProblem) Valid() error {
	if p.Text == "" || p.Line < 1 {
		return errors.New("a problem of guardrails needs its text and its line")
	}
	return nil
}

// Guard returns a copy of w with the section guardrails g applied: the built-in good practices of g, the items of
// forbidden_commands its presets add, before those of w (Builtin.forbidden), whether w has gates of git or not, and
// the entries its shortcuts expand into, in On (Guardrails.Expand) and in Githooks (Guardrails.Githooks), with the
// rules folder of w (RulesPath) for the globs of the built-in format of the polix files. The entry of start
// that installs the hooks of git is left to ExpandGithooks. With a path that cannot be the folder of the AI
// (ValidPath), the format and the check of the polix files are off, so the only problem is the one of path, never one
// of a rules folder built from it; the presets stay as g says.
func (w Workflow) Guard(g Guardrails) Workflow {
	rules := w.RulesPath()
	if w.ValidPath() != nil {
		g = g.withoutPolixFiles()
	}
	w.Builtin = g.Builtin
	if presets := g.Builtin.forbidden(); len(presets) > 0 {
		w.ForbiddenCommands = append(presets, w.ForbiddenCommands...)
	}
	w.On = g.Expand(w.On, rules)
	w.Githooks = g.Githooks(w.Githooks)
	return w
}

// forbidden returns the items of forbidden_commands the presets of b add, in the order of builtinPresets, each at the
// line of builtin and from guardrails.builtin and its key; nil when both are off.
func (b Builtin) forbidden() []ForbiddenCommand {
	var out []ForbiddenCommand
	for _, p := range builtinPresets {
		if (p.key == BuiltinForbidCommitNoVerify && !b.ForbidCommitNoVerify) ||
			(p.key == BuiltinForbidGitDestructiveCommands && !b.ForbidGitDestructiveCommands) {
			continue
		}
		c := ForbiddenCommand{Pattern: p.pattern, Reason: p.reason, Line: b.Line,
			From: "guardrails." + ShortcutBuiltin + "." + p.key}
		if c.Valid() != nil {
			continue
		}
		out = append(out, c)
	}
	return out
}

// withoutPolixFiles returns a copy of g whose built-in format and check of the polix files are off.
func (g Guardrails) withoutPolixFiles() Guardrails {
	g.Builtin = g.Builtin.withoutPolixFiles()
	return g
}

// withoutPolixFiles returns a copy of b with format_polix_files and check_polix_files off, the blocks as b says.
func (b Builtin) withoutPolixFiles() Builtin {
	b.FormatPolixFiles, b.CheckPolixFiles = false, false
	return b
}

// formatEntries returns the entries of post_edit format_polix_files expands into, for the rules folder rules: polix
// config fmt on an edit of the root polix.yaml, then polix rules fmt on an edit of a yaml file of the rules folder,
// both at the line of builtin and without message, so a failure reaches the agent as the output of the command; nil
// when it is off.
func (b Builtin) formatEntries(rules string) []Entry {
	if !b.FormatPolixFiles {
		return nil
	}
	from := "guardrails." + ShortcutBuiltin + "." + BuiltinFormatPolixFiles
	config, _ := Cond{"path": builtinRootFile}.shortcutRun(from, b.Line, slices.Clone(builtinConfigFmt), "", false)
	rulesFmt, _ := Cond{"path": rules + "/*.yaml"}.shortcutRun(from, b.Line, slices.Clone(builtinRulesFmt), "", false)
	return []Entry{config, rulesFmt}
}

// checkEntries returns the entries of githooks.pre_commit check_polix_files expands into: polix config check when a
// staged file is the root polix.yaml, at the line of builtin; nil when it is off. The rules are validated by polix
// rules check, which a commit that stages a rule does not run.
func (b Builtin) checkEntries() []Entry {
	if !b.CheckPolixFiles {
		return nil
	}
	from := "guardrails." + ShortcutBuiltin + "." + BuiltinCheckPolixFiles
	when := Cond{"changed": []any{builtinRootFile}}
	e, _ := when.shortcutRun(from, b.Line, slices.Clone(builtinConfigCheck), "", false)
	return []Entry{e}
}

// problems reports the problems of the values of builtin, each at the line of its key.
func (b Builtin) problems(v *validator) {
	for _, p := range b.Problems {
		v.add(p.Line, "%s", p.Text)
	}
}
