package workflow

import (
	"errors"
	"maps"
	"regexp"
	"strings"
)

// Guardrails is the section guardrails of polix.yaml as written: the items of each of its shortcuts, the common
// cases, in file order and each with its line. protected_files and forbidden_commands are protections the workflow
// holds as lists (Protected and Forbidden); the rest are only sugar, which Expand turns into entries of on. The
// decoding of the file finds what is wrong with the values of an item and keeps it with the item, for Problems to
// report it with the rest of the problems of the workflow.
type Guardrails struct {
	ProtectedFiles    []GuardrailItem      // the path globs of protected_files
	ForbiddenCommands []GuardrailForbidden // the items of forbidden_commands
	ProtectedBranches []GuardrailItem      // the branch names of protected_branches
	FormatOnEdit      []GuardrailCheck     // the items of format_on_edit
	RunBeforeDone     []GuardrailCheck     // the items of run_before_done
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

// GuardrailCheck is an item of format_on_edit or run_before_done: the command to run, the globs that scope it (the
// path of format_on_edit or the changed of run_before_done, as written, nil without them) and the message before
// the output of a failure. Problems holds what is wrong with its values, reported at its line.
type GuardrailCheck struct {
	Run      []string
	Scope    any
	Message  string
	Line     int
	Problems []string
}

// The shortcuts of the section guardrails.
const (
	ShortcutProtectedFiles    = "protected_files"
	ShortcutForbiddenCommands = "forbidden_commands"
	ShortcutProtectedBranches = "protected_branches"
	ShortcutFormatOnEdit      = "format_on_edit"
	ShortcutRunBeforeDone     = "run_before_done"
)

// branchMessage is the message of the entry of protected_branches, in English since polix writes it.
const branchMessage = "{branch} is protected in " + File + ": create a branch for this change and commit there."

// GuardrailsKeys are the shortcuts of the section guardrails, in the order they expand.
var GuardrailsKeys = []string{ShortcutProtectedFiles, ShortcutForbiddenCommands, ShortcutProtectedBranches,
	ShortcutFormatOnEdit, ShortcutRunBeforeDone}

// ForbiddenKeys, FormatOnEditKeys and RunBeforeDoneKeys are the keys of an item of forbidden_commands,
// format_on_edit and run_before_done.
var (
	ForbiddenKeys     = []string{"pattern", "reason"}
	FormatOnEditKeys  = []string{"run", "path", "message"}
	RunBeforeDoneKeys = []string{"run", "changed", "message"}
)

// Valid tells why the section is not valid: the items that are not, each with why.
func (g Guardrails) Valid() error {
	var errs []error
	for _, it := range append(append([]GuardrailItem(nil), g.ProtectedFiles...), g.ProtectedBranches...) {
		errs = append(errs, it.Valid())
	}
	for _, f := range g.ForbiddenCommands {
		errs = append(errs, f.Valid())
	}
	for _, c := range append(append([]GuardrailCheck(nil), g.FormatOnEdit...), g.RunBeforeDone...) {
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

// Expand returns the entries of on with those the shortcuts protected_branches, format_on_edit and run_before_done
// expand into, before the entries written in on in each event, each group in file order. An item with problems
// expands into nothing: Problems reports them.
//
//   - protected_branches: an entry on pre_commit with branch, an anchored regular expression of the names.
//   - format_on_edit: an entry on post_edit per item that runs its command, on the edits of its path when it has one.
//   - run_before_done: an entry on stop per item that runs its command, when its changed holds if it has one.
//
// Each entry takes the line of its item, the first one for an entry of several items, and the path of its shortcut.
// protected_files and forbidden_commands expand into nothing: they are protections, not entries.
func (g Guardrails) Expand(on map[string][]Entry) map[string][]Entry {
	regular := map[string][]Entry{}
	if expr, line := g.branchExpr(); expr != "" {
		when := Cond{"branch": expr}
		e, _ := when.shortcutBlock("guardrails."+ShortcutProtectedBranches, line, branchMessage)
		regular[PreCommit] = append(regular[PreCommit], e)
	}
	for _, c := range g.FormatOnEdit {
		if e, ok := c.entry(ShortcutFormatOnEdit, "path"); ok {
			regular[PostEdit] = append(regular[PostEdit], e)
		}
	}
	for _, c := range g.RunBeforeDone {
		if e, ok := c.entry(ShortcutRunBeforeDone, "changed"); ok {
			regular[Stop] = append(regular[Stop], e)
		}
	}
	if len(regular) == 0 {
		return on
	}
	out := make(map[string][]Entry, len(on)+len(regular))
	maps.Copy(out, on)
	for ev, entries := range regular {
		out[ev] = append(entries, on[ev]...)
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
	for _, c := range append(append([]GuardrailCheck(nil), g.FormatOnEdit...), g.RunBeforeDone...) {
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

// entry returns the entry of the item c of the shortcut key, whose scope goes in the condition cond; false when the
// item has problems.
func (c GuardrailCheck) entry(key, cond string) (Entry, bool) {
	if len(c.Problems) > 0 {
		return Entry{}, false
	}
	var when Cond
	if c.Scope != nil {
		when = Cond{cond: c.Scope}
	}
	e, _ := when.shortcutRun("guardrails."+key, c.Line, c.Run, c.Message)
	return e, true
}

// branchExpr returns the regular expression of the branch condition of protected_branches: any of the names that are
// not empty, whole, where `*` matches any text; and the line of the first one. It is "" without them.
func (g Guardrails) branchExpr() (string, int) {
	var alts []string
	line := 0
	for _, it := range g.ProtectedBranches {
		if it.Valid() != nil {
			continue
		}
		if line == 0 {
			line = it.Line
		}
		parts := strings.Split(it.Text, "*")
		for i := range parts {
			parts[i] = regexp.QuoteMeta(parts[i])
		}
		alts = append(alts, strings.Join(parts, ".*"))
	}
	if len(alts) == 0 {
		return "", 0
	}
	return "^(?:" + strings.Join(alts, "|") + ")$", line
}

// shortcutBlock returns the entry of a shortcut from at line that blocks with message when the conditions of c, its
// when, hold.
func (c Cond) shortcutBlock(from string, line int, message string) (Entry, error) {
	b := Block{Message: message}
	a := Action{Block: &b, Keys: []string{VerbBlock}}
	e := Entry{When: c, Action: a, From: from, Line: line, Keys: c.shortcutKeys()}
	return e, errors.Join(b.Valid(), a.Valid(), e.Valid())
}

// shortcutRun returns the entry of a shortcut from at line that runs command, with message before the output of a
// failure, when the conditions of c, its when, hold.
func (c Cond) shortcutRun(from string, line int, command []string, message string) (Entry, error) {
	keys := []string{"command"}
	if message != "" {
		keys = append(keys, "message")
	}
	r := Run{Command: command, Message: message, Keys: keys}
	a := Action{Run: &r, Keys: []string{VerbRun}}
	e := Entry{When: c, Action: a, From: from, Line: line, Keys: c.shortcutKeys()}
	return e, errors.Join(r.Valid(), a.Valid(), e.Valid())
}

// shortcutKeys returns the keys an entry of a shortcut whose when is c would have written in on.
func (c Cond) shortcutKeys() []string {
	var keys []string
	if c != nil {
		keys = append(keys, "when")
	}
	return append(keys, "action")
}
