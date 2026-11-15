package workflow

import (
	"errors"
	"maps"
	"regexp"
	"slices"
	"strings"
)

// The entries the shortcuts of guardrails expand into: Guardrails.Expand puts those of on before the entries written
// there, and Guardrails.Githooks those of githooks before the entries written in githooks.

// Githooks returns the entries of githooks with those check_on_save expands into before the entries written in
// githooks.pre_commit: one entry per item, in file order, that runs its command with its message, when a staged file
// matches its changed if it has one. Each takes the line of its item and the path of its shortcut, which the
// validator of githooks names in its problems. An item with problems expands into nothing: Problems reports them.
// Without such items githooks is returned as it is.
func (g Guardrails) Githooks(githooks map[string][]Entry) map[string][]Entry {
	var saved []Entry
	for _, c := range g.CheckOnSave {
		if e, ok := c.entry(ShortcutCheckOnSave, nil, "changed", false); ok {
			saved = append(saved, e)
		}
	}
	if len(saved) == 0 {
		return githooks
	}
	out := make(map[string][]Entry, len(githooks)+1)
	maps.Copy(out, githooks)
	out[PreCommit] = append(saved, githooks[PreCommit]...)
	return out
}

// entries returns the entries the shortcuts expand into in on, by event, each group in file order (see Expand).
func (g Guardrails) entries() map[string][]Entry {
	regular := map[string][]Entry{}
	if expr, line := g.branchExpr(); expr != "" {
		when := Cond{"branch": expr}
		e, _ := when.shortcutBlock("guardrails."+ShortcutProtectedBranches, line, branchMessage)
		regular[PreCommit] = append(regular[PreCommit], e)
	}
	add := func(ev, key string, items []GuardrailCheck, base Cond, cond string, always bool) {
		for _, c := range items {
			if e, ok := c.entry(key, base, cond, always); ok {
				regular[ev] = append(regular[ev], e)
			}
		}
	}
	add(Start, ShortcutExecOnStart, g.ExecOnStart, nil, "", true)
	add(PostEdit, ShortcutFormatOnEdit, g.FormatOnEdit, nil, "path", false)
	add(PreCommand, ShortcutFormatOnSave, g.FormatOnSave, Cond{"command": slices.Clone(stagingCommands)}, "changed",
		false)
	add(Stop, ShortcutCheckOnStop, g.CheckOnStop, nil, "changed", false)
	return regular
}

// entry returns the entry of the item c of the shortcut key: its when holds the conditions of base, the scope of the
// item in the condition cond and, when the item waits, agents_are_running: false, and none when all three are empty;
// its run has always when always is true. False when the item has problems.
func (c GuardrailCheck) entry(key string, base Cond, cond string, always bool) (Entry, bool) {
	if len(c.Problems) > 0 {
		return Entry{}, false
	}
	when := maps.Clone(base)
	if (c.Scope != nil || c.Waits) && when == nil {
		when = Cond{}
	}
	if c.Scope != nil {
		when[cond] = c.Scope
	}
	if c.Waits {
		when["agents_are_running"] = false
	}
	e, _ := when.shortcutRun("guardrails."+key, c.Line, c.Run, c.Message, always)
	return e, true
}

// waitNotice returns the notify of check_on_stop that tells the user its checks wait for the subagents still running,
// at the line of the first item that waits, when subagents run and a file in the globs of every item that waits, in
// file order and once each, changed; without changed when one of them has none. False when no item without problems
// waits.
func (g Guardrails) waitNotice() (Entry, bool) {
	line := 0
	var globs []any
	scoped := true
	for _, c := range g.CheckOnStop {
		if len(c.Problems) > 0 || !c.Waits {
			continue
		}
		if line == 0 {
			line = c.Line
		}
		list, err := Cond{"changed": c.Scope}.Strings("changed")
		scoped = scoped && c.Scope != nil
		for _, s := range list {
			if err == nil && !slices.Contains(globs, any(s)) {
				globs = append(globs, s)
			}
		}
	}
	if line == 0 {
		return Entry{}, false
	}
	when := Cond{"agents_are_running": true}
	if scoped {
		when = Cond{"agents_are_running": true, "changed": globs}
	}
	e, _ := when.shortcutNotify("guardrails."+ShortcutCheckOnStop, line, waitMessage)
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
// failure, when the conditions of c, its when, hold; with always, it runs every time, also on a content it passed on.
func (c Cond) shortcutRun(from string, line int, command []string, message string, always bool) (Entry, error) {
	keys := []string{"command"}
	if message != "" {
		keys = append(keys, "message")
	}
	if always {
		keys = append(keys, "always")
	}
	r := Run{Command: command, Message: message, Always: always, Keys: keys}
	a := Action{Run: &r, Keys: []string{VerbRun}}
	e := Entry{When: c, Action: a, From: from, Line: line, Keys: c.shortcutKeys()}
	return e, errors.Join(r.Valid(), a.Valid(), e.Valid())
}

// shortcutNotify returns the entry of a shortcut from at line that shows message to the user every time the
// conditions of c, its when, hold.
func (c Cond) shortcutNotify(from string, line int, message string) (Entry, error) {
	n := Notify{Message: message, Always: true, Keys: []string{"always", "message"}}
	a := Action{Notify: &n, Keys: []string{VerbNotify}}
	e := Entry{When: c, Action: a, From: from, Line: line, Keys: c.shortcutKeys()}
	return e, errors.Join(n.Valid(), a.Valid(), e.Valid())
}

// shortcutKeys returns the keys an entry of a shortcut whose when is c would have written in on.
func (c Cond) shortcutKeys() []string {
	var keys []string
	if c != nil {
		keys = append(keys, "when")
	}
	return append(keys, "action")
}
