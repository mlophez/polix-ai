package workflow

import (
	"errors"
	"maps"
	"slices"
	"strings"
)

// Guardrails is the section guardrails of polix.yaml as written: the built-in good practices of builtin and the items
// of each of its shortcuts, the common cases, in file order and each with its line. protected_files and
// forbidden_commands are protections the workflow holds as lists (Protected and Forbidden); the rest are only sugar,
// which Expand turns into entries of on and Githooks into entries of githooks, those of builtin included. The
// decoding of the file finds what is wrong with the values of an item and keeps it with the item, for Problems to
// report it with the rest of the problems of the workflow.
type Guardrails struct {
	Builtin           Builtin              // the built-in good practices, all on when the file leaves builtin out
	ProtectedFiles    []GuardrailItem      // the path globs of protected_files
	ProtectedBranches []GuardrailItem      // the branch names of protected_branches
	ForbiddenCommands []GuardrailForbidden // the items of forbidden_commands
	ExecOnStart       []GuardrailCheck     // the items of exec_on_start
	FormatOnEdit      []GuardrailCheck     // the items of format_on_edit
	FormatOnSave      []GuardrailCheck     // the items of format_on_save
	CheckOnStop       []GuardrailCheck     // the items of check_on_stop
	CheckOnSave       []GuardrailCheck     // the items of check_on_save
}

// GuardrailItem is an item of a shortcut that takes a list of strings: a path glob of protected_files or a branch
// name of protected_branches.
type GuardrailItem struct {
	Text string
	Line int // the line of the item in the file
}

// GuardrailForbidden is an item of forbidden_commands: the pattern of a command and its reason, optional. Problems
// holds what is wrong with its values, reported at its line.
type GuardrailForbidden struct {
	Pattern  string
	Reason   string
	Line     int
	Problems []string
}

// GuardrailCheck is an item of a lifecycle shortcut (exec_on_start, format_on_edit, format_on_save, check_on_stop or
// check_on_save): the command to run, the globs that scope it (the path of format_on_edit or the changed of the
// others, as written, nil without them; exec_on_start has none), the message before the output of a failure and, for
// check_on_stop, whether it waits for the subagents still running (wait_agents, true when left out) and whether it
// also runs on every commit, as an item of check_on_save (also_on_save, false when left out). Problems holds what is
// wrong with its values, reported at its line.
type GuardrailCheck struct {
	Run        []string
	Scope      any
	Message    string
	Waits      bool
	AlsoOnSave bool
	Line       int
	Problems   []string
}

// The shortcuts of the section guardrails.
const (
	ShortcutProtectedFiles    = "protected_files"
	ShortcutProtectedBranches = "protected_branches"
	ShortcutForbiddenCommands = "forbidden_commands"
	ShortcutExecOnStart       = "exec_on_start"
	ShortcutFormatOnEdit      = "format_on_edit"
	ShortcutFormatOnSave      = "format_on_save"
	ShortcutCheckOnStop       = "check_on_stop"
	ShortcutCheckOnSave       = "check_on_save"
)

// branchMessage is the message of the entry of protected_branches, in English since polix writes it.
const branchMessage = "{branch} is protected in " + File + ": create a branch for this change and commit there."

// waitMessage is the notice of the stops that hold back the items of check_on_stop that wait while subagents run, in
// English since polix writes it.
const waitMessage = ShortcutCheckOnStop + " waits for the subagents still running: its checks run on the next stop " +
	"after they end."

// GuardrailsKeys are the keys of the section guardrails, in the order polix config fmt writes them: first builtin,
// what polix applies by itself, then the protections, then the lifecycle shortcuts in the order the work reaches
// them.
var GuardrailsKeys = []string{ShortcutBuiltin, ShortcutProtectedFiles, ShortcutProtectedBranches,
	ShortcutForbiddenCommands, ShortcutExecOnStart, ShortcutFormatOnEdit, ShortcutFormatOnSave, ShortcutCheckOnStop,
	ShortcutCheckOnSave}

// ForbiddenKeys, ExecOnStartKeys, FormatOnEditKeys, FormatOnSaveKeys, CheckOnStopKeys and CheckOnSaveKeys are the
// keys of an item of forbidden_commands, exec_on_start, format_on_edit, format_on_save, check_on_stop and
// check_on_save. git runs the hooks without knowing the subagents of a session, so check_on_save has no wait_agents,
// nor also_on_save, which only an item of check_on_stop needs.
var (
	ForbiddenKeys    = []string{"pattern", "reason"}
	ExecOnStartKeys  = []string{"run", "message"}
	FormatOnEditKeys = []string{"run", "path", "message"}
	FormatOnSaveKeys = []string{"run", "changed", "message"}
	CheckOnStopKeys  = []string{"run", "changed", "message", "wait_agents", "also_on_save"}
	CheckOnSaveKeys  = []string{"run", "changed", "message"}
)

// stagingCommands are the command patterns of the lines that stage files, on which format_on_save runs.
var stagingCommands = []any{"git add*", "git stage*"}

// Valid tells why the section is not valid: the items that are not, each with why.
func (g Guardrails) Valid() error {
	errs := []error{g.Builtin.Valid()}
	for _, it := range append(append([]GuardrailItem(nil), g.ProtectedFiles...), g.ProtectedBranches...) {
		errs = append(errs, it.Valid())
	}
	for _, f := range g.ForbiddenCommands {
		errs = append(errs, f.Valid())
	}
	for _, c := range g.checks() {
		errs = append(errs, c.Valid())
	}
	return errors.Join(errs...)
}

// Valid tells why the item is not valid: it is empty.
func (it GuardrailItem) Valid() error {
	if strings.TrimSpace(it.Text) == "" {
		return errors.New("an item of guardrails cannot be empty")
	}
	return nil
}

// Valid tells why the item is not valid: the problems of its values, or a pattern that is not one.
func (f GuardrailForbidden) Valid() error {
	if len(f.Problems) > 0 {
		return InvalidError(f.Problems)
	}
	c := ForbiddenCommand{Pattern: f.Pattern, Reason: f.Reason, Line: f.Line}
	return c.Valid()
}

// Valid tells why the item is not valid: the problems of its values, or no command to run.
func (c GuardrailCheck) Valid() error {
	if len(c.Problems) > 0 {
		return InvalidError(c.Problems)
	}
	if len(c.Run) == 0 {
		return errors.New("an item of guardrails needs run")
	}
	return nil
}

// Expand returns the entries of on with those builtin and the shortcuts protected_branches, exec_on_start,
// format_on_edit, format_on_save and check_on_stop expand into, before the entries written in on in each event, each
// group in file order; rules is the folder of the rules, below path. An item with problems expands into nothing:
// Problems reports them. check_on_save and check_polix_files expand into entries of githooks instead (Githooks).
//
//   - builtin.format_polix_files: two entries first on post_edit, before those of format_on_edit, that run polix
//     config fmt on the edits of the root polix.yaml and polix rules fmt on those of the yaml files of rules
//     (Builtin.formatEntries).
//   - protected_branches: an entry on pre_commit with branch, an anchored regular expression of the names.
//   - exec_on_start: an entry on start per item, without when, that runs its command with always, so it runs on every
//     start, also on a content it passed on.
//   - format_on_edit: an entry on post_edit per item that runs its command, on the edits of its path when it has one.
//   - format_on_save: an entry on pre_command per item that runs its command on the lines that stage files (command
//     git add* or git stage*), when its changed holds if it has one.
//   - check_on_stop: an entry on stop per item that runs its command, when its changed holds if it has one and, for an
//     item that waits, when no subagent is running (agents_are_running: false); and, when an item waits, one notify
//     that tells the user, every time, that the checks wait, while subagents run and a changed holding the globs of
//     every item that waits holds, left out when one of them has none (waitNotice). The notify goes last on stop,
//     after the entries written in on too, so no entry after it may block the stop and hide it.
//
// Each entry takes the line of its item, the first one for an entry of several items, and the path of its shortcut.
// Those of builtin take the line of builtin, 0 when the file leaves it out, and the path of their key.
// protected_files and forbidden_commands expand into nothing: they are protections, not entries.
func (g Guardrails) Expand(on map[string][]Entry, rules string) map[string][]Entry {
	regular := g.entries(rules)
	if len(regular) == 0 {
		return on
	}
	out := make(map[string][]Entry, len(on)+len(regular))
	maps.Copy(out, on)
	for ev, entries := range regular {
		out[ev] = append(entries, on[ev]...)
	}
	if e, ok := g.waitNotice(); ok {
		out[Stop] = append(out[Stop], e)
	}
	return out
}

// Protected returns the globs of protected_files that are not empty, in file order; nil without them. The empty ones
// are problems of the shortcut, which Problems reports.
func (g Guardrails) Protected() []string {
	var out []string
	for _, it := range g.ProtectedFiles {
		if it.Valid() == nil {
			out = append(out, it.Text)
		}
	}
	return out
}

// Forbidden returns the items of forbidden_commands without problems, in file order; nil without them. The items
// with problems are left out: Problems reports them.
func (g Guardrails) Forbidden() []ForbiddenCommand {
	var out []ForbiddenCommand
	for _, f := range g.ForbiddenCommands {
		if len(f.Problems) > 0 {
			continue
		}
		c := ForbiddenCommand{Pattern: f.Pattern, Reason: f.Reason, Line: f.Line}
		if c.Valid() != nil {
			continue
		}
		out = append(out, c)
	}
	return out
}

// Problems returns the problems of the values of the shortcuts, each at the line of its item, together with those
// of the workflow w they expand into, in the order of the file; nil when the shortcuts have none.
func (g Guardrails) Problems(w Workflow) []string {
	v := &validator{w: &w}
	g.Builtin.problems(v)
	for _, it := range g.ProtectedFiles {
		if it.Valid() != nil {
			v.add(it.Line, "guardrails.%s: a path glob cannot be empty", ShortcutProtectedFiles)
		}
	}
	for _, f := range g.ForbiddenCommands {
		for _, p := range f.Problems {
			v.add(f.Line, "%s", p)
		}
	}
	for _, it := range g.ProtectedBranches {
		if it.Valid() != nil {
			v.add(it.Line, "guardrails.%s: a branch name cannot be empty", ShortcutProtectedBranches)
		}
	}
	for _, c := range g.checks() {
		for _, p := range c.Problems {
			v.add(c.Line, "%s", p)
		}
	}
	if len(v.problems) == 0 {
		return nil
	}
	v.workflow(w)
	return v.byLine()
}

// checks returns the items of the lifecycle shortcuts, shortcut by shortcut in the order of GuardrailsKeys.
func (g Guardrails) checks() []GuardrailCheck {
	return slices.Concat(g.ExecOnStart, g.FormatOnEdit, g.FormatOnSave, g.CheckOnStop, g.CheckOnSave)
}
