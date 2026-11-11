package workflow

import (
	"errors"
	"maps"
	"regexp"
	"slices"
	"strings"
)

// fromGithooks is the From of the entry of start a workflow with githooks expands into.
const fromGithooks = "githooks"

// githooksInstallMessage is the message of the entry of start that installs the hooks of git, which reaches the AI
// before the line of the failure when the install fails.
const githooksInstallMessage = "The hooks of git of githooks are not installed, so the gates of githooks do not apply " +
	"to the commits:"

// githooksInstallCommand is the command of the entry of start that installs the hooks of git: only those of the
// repository, without asking, and exiting 1 when git does not run them.
var githooksInstallCommand = []string{"polix", "install", "--githooks"}

// ExpandGithooks returns a copy of w whose On has, when some event of githooks has entries, one more entry first in
// start, before those written in on and those of the shortcuts: a run of polix install --githooks with always, so
// every session start puts back the hooks of git, with the fixed message githooksInstallMessage, from githooks and
// at the line of the first event of githooks with entries. Without entries in githooks w is returned as it is.
// polix config fmt formats the file as written, so it never writes this entry.
func (w Workflow) ExpandGithooks() Workflow {
	line, used := 0, false
	for ev, entries := range w.Githooks {
		if len(entries) == 0 {
			continue
		}
		if l := w.GithookLines[ev]; !used || l < line {
			line = l
		}
		used = true
	}
	if !used {
		return w
	}
	e, _ := Cond(nil).githooksInstall(line)
	on := make(map[string][]Entry, len(w.On)+1)
	maps.Copy(on, w.On)
	on[Start] = append([]Entry{e}, w.On[Start]...)
	w.On = on
	return w
}

// githooksInstall returns the entry of start, at line, that runs polix install --githooks every time the conditions
// of c, its when, hold, with the message githooksInstallMessage, as the run entries of the shortcuts are built.
func (c Cond) githooksInstall(line int) (Entry, error) {
	r := Run{Command: slices.Clone(githooksInstallCommand), Message: githooksInstallMessage, Always: true,
		Keys: []string{"command", "message", "always"}}
	a := Action{Run: &r, Keys: []string{VerbRun}}
	e := Entry{When: c, Action: a, From: fromGithooks, Line: line, Keys: c.shortcutKeys()}
	return e, errors.Join(r.Valid(), a.Valid(), e.Valid())
}

// githooks reports the problems of the section githooks of the workflow w, in the order of its event names: a
// variable that shadows a built-in placeholder of githooks, an unknown event, and the entries of each known one,
// checked with the vocabulary of githooks, never with that of on.
func (v *validator) githooks(w Workflow) {
	v.githookVars(w)
	for _, ev := range slices.Sorted(maps.Keys(w.Githooks)) {
		if !slices.Contains(GithookEvents, ev) {
			v.add(w.GithookLines[ev], "unknown githooks event %q (githooks events: %s)", ev,
				strings.Join(GithookEvents, ", "))
			continue
		}
		for _, e := range w.Githooks[ev] {
			v.githookEntry(ev, e)
		}
	}
}

// githookVars reports, once githooks has an entry, the variables of vars named as a built-in placeholder of githooks
// that on does not have (message), which would shadow it; without entries in githooks the name stays valid, as it
// was before githooks existed.
func (v *validator) githookVars(w Workflow) {
	used := false
	for _, entries := range w.Githooks {
		used = used || len(entries) > 0
	}
	if !used {
		return
	}
	for _, name := range GithookBuiltins {
		if _, ok := w.Vars[name]; ok && !slices.Contains(Builtins, name) {
			v.add(0, "var %s: the name is a built-in placeholder of githooks (%s)", name,
				strings.Join(GithookBuiltins, ", "))
		}
	}
}

// githookEntry reports the problems of the entry e of the githooks event ev: its unknown keys, its conditions and
// its action.
func (v *validator) githookEntry(ev string, e Entry) {
	for _, k := range e.Keys {
		if !slices.Contains(EntryKeys, k) {
			v.add(e.Line, "githooks.%s: unknown key %q in an entry (%s)", ev, k, strings.Join(EntryKeys, ", "))
		}
	}
	v.githookCond(ev, e.When, e.Line)
	v.githookCond(ev, e.Unless, e.Line)
	v.githookAction(ev, e.Line, e.Action)
}

// githookCond reports the problems of the conditions c of the githooks event ev, in the order of their names: a
// condition that is not one of GithookConditions (any and not included: a negation goes in unless), one that does
// not hold on ev, and the patterns of the others.
func (v *validator) githookCond(ev string, c Cond, line int) {
	for _, k := range slices.Sorted(maps.Keys(c)) {
		switch only := githookOnly[k]; {
		case !slices.Contains(GithookConditions, k):
			v.add(line, "githooks.%s: condition %q is not allowed (githooks allow %s)", ev, k,
				strings.Join(GithookConditions, ", "))
		case only != "" && only != ev:
			v.add(line, "githooks.%s: condition %q only holds on %s", ev, k, only)
		default:
			v.githookPatterns(ev, c, k, line)
		}
	}
}

// githookPatterns reports the problems of the patterns of the condition k of c on the githooks event ev: branch
// takes one regular expression, as written, since its placeholders are not expanded, as in on; changed and message
// a string or a list of strings, whose placeholders are those of ev. No pattern is empty, and those of branch and
// message compile as regular expressions, message once its placeholders are expanded.
func (v *validator) githookPatterns(ev string, c Cond, k string, line int) {
	var list []string
	if k == "branch" {
		s, ok := c[k].(string)
		if !ok {
			v.add(line, "githooks.%s: branch needs a regular expression", ev)
			return
		}
		list = []string{s}
	} else {
		l, err := c.Strings(k)
		if err != nil {
			v.add(line, "githooks.%s: %s: %v", ev, k, err)
			return
		}
		list = l
	}
	if len(list) == 0 {
		v.add(line, "githooks.%s: %s needs a pattern", ev, k)
	}
	for _, s := range list {
		pattern := s
		if k != "branch" {
			v.githookPlaceholders(ev, line, s)
			// A placeholder stands for a text: its value is only known on the commit.
			pattern = placeholder.ReplaceAllString(s, "x")
		}
		if strings.TrimSpace(s) == "" {
			v.add(line, "githooks.%s: %s: a pattern cannot be empty", ev, k)
			continue
		}
		if k == "changed" {
			continue
		}
		if _, err := regexp.Compile(pattern); err != nil {
			v.add(line, "githooks.%s: %s: %v", ev, k, err)
		}
	}
}

// githookAction reports the problems of the action a of the githooks event ev at line: a verb that is not one of
// GithookVerbs, an unknown key, a verb without a value, no verb or several, and the shape of run and block.
func (v *validator) githookAction(ev string, line int, a Action) {
	verbs := a.verbs()
	allowed := strings.Join(GithookVerbs, ", ")
	for _, verb := range verbs {
		if !slices.Contains(GithookVerbs, verb) {
			v.add(line, "githooks.%s: verb %q is not allowed (githooks allow %s)", ev, verb, allowed)
		}
	}
	for _, k := range a.Keys {
		switch {
		case slices.Contains(verbs, k):
		case !slices.Contains(Actions, k):
			v.add(line, "githooks.%s: unknown key %q in action (githooks allow %s)", ev, k, allowed)
		case !slices.Contains(GithookVerbs, k):
			v.add(line, "githooks.%s: verb %q is not allowed (githooks allow %s)", ev, k, allowed)
		default:
			v.add(line, "githooks.%s: %s needs a value", ev, k)
		}
	}
	switch {
	case len(verbs) == 0 && len(a.Keys) == 0:
		v.add(line, "githooks.%s: the entry needs an action with one of %s", ev, allowed)
	case len(verbs) > 1:
		v.add(line, "githooks.%s: the action has several verbs (%s): use one entry per verb", ev,
			strings.Join(verbs, ", "))
	}
	if a.Run != nil {
		v.githookKeys(ev, line, VerbRun, a.Run.Keys, GithookRunKeys)
		if len(a.Run.Command) == 0 {
			v.add(line, "githooks.%s: run needs a command", ev)
		}
		for _, s := range append(slices.Clone(a.Run.Command), a.Run.Message) {
			v.githookPlaceholders(ev, line, s)
		}
	}
	if a.Block != nil {
		v.githookKeys(ev, line, VerbBlock, a.Block.Keys, GithookBlockKeys)
		if strings.TrimSpace(a.Block.Message) == "" {
			v.add(line, "githooks.%s: block needs a message", ev)
		}
		v.githookPlaceholders(ev, line, a.Block.Message)
	}
}

// githookKeys reports the keys of the long form of the verb that githooks do not allow.
func (v *validator) githookKeys(ev string, line int, verb string, keys, allowed []string) {
	for _, k := range keys {
		if !slices.Contains(allowed, k) {
			v.add(line, "githooks.%s: key %q is not allowed in %s (githooks allow %s)", ev, k, verb,
				strings.Join(allowed, ", "))
		}
	}
}

// githookPlaceholders reports the placeholders of s that the githooks event ev does not expand: a built-in one that
// only expands on another event, or one that is neither built in (GithookBuiltins) nor a variable of vars.
func (v *validator) githookPlaceholders(ev string, line int, s string) {
	for _, m := range placeholder.FindAllStringSubmatch(s, -1) {
		name := m[1]
		_, variable := v.w.Vars[name]
		switch only := githookOnly[name]; {
		case slices.Contains(GithookBuiltins, name) && only != "" && only != ev:
			v.add(line, "githooks.%s: placeholder {%s} only expands on %s", ev, name, only)
		case !slices.Contains(GithookBuiltins, name) && !variable:
			v.add(line, "githooks.%s: unknown placeholder {%s} (githooks allow %s, or a variable of vars)", ev, name,
				v.githookBuiltins(ev))
		}
	}
}

// githookBuiltins lists the built-in placeholders the githooks event ev expands, as written in a string.
func (v *validator) githookBuiltins(ev string) string {
	var out []string
	for _, name := range GithookBuiltins {
		if only := githookOnly[name]; only == "" || only == ev {
			out = append(out, "{"+name+"}")
		}
	}
	return strings.Join(out, ", ")
}
