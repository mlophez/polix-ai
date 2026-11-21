package workflow

import (
	"regexp"
	"slices"
)

// Problem is a problem of the workflow: one line of Problems, or the text of a UnknownKeyError.
type Problem string

// renamedEvents maps the events of the old vocabulary to their new name.
var renamedEvents = map[string]string{"session_start": Start, "turn_stop": Stop}

// renamed holds, per place of the file, the names of the old vocabulary and the hint of their new spelling, which
// the validator, or UnknownKeyError for a top-level key (place "top"), appends to the problem of the unknown name.
// A removed key is one too: its hint tells how to migrate it.
var renamed = map[string]map[string]string{
	"top": {
		"protected":        "use guardrails.protected_files",
		"base":             "use base_branch",
		"max_stop_blocks":  "use stop_retries",
		"max_stop_retries": "use stop_retries",
		"plans":            "remove it: the plans go to plans/ below path (ai/plans by default)",
		"reports":          "remove it: the reports go to reports/ below path (ai/reports by default)",
		"guidance":         "remove it: polix always injects the rules on commits and the folders below path on start",
	},
	"rules":      {"path": "move it to the top-level key path, the folder that holds rules/ (ai by default)"},
	"guardrails": {"run_on_stop": "use check_on_stop"},
	"key": {
		"if":      "use when",
		"why":     "remove it, or keep the text as a YAML comment",
		"mode":    "remove it: a warning is an action: { inject: ... }, a failing run on post_edit or post_command already reaches the AI as context, and audit has no replacement",
		"deny":    "use action: { block: <message> }",
		"require": "use action: { block: <message> }",
		"allow":   "write the exception as a not in when, or in unless",
		"run":     "move it into action: { run: ... }",
		"context": "use action: { inject: ... }",
		"record":  "remove it: polix observes the end of a subagent by itself, so use finished: <subagent> in when or unless; next to a run, a run that passed is not run again on the same content",
	},
	"action":    {"deny": "use block", "require": "use block", "context": "use inject"},
	"condition": {"done": "use finished: <subagent>, which holds once that subagent ended on the same files; for the fact of a run, remove it: a run that passed is not run again on the same content"},
}

// oldActions are the keys of the old vocabulary that were the action of an entry: an entry that has one is told
// how to rewrite it, not that it has no action.
var oldActions = []string{"deny", "require", "allow", "run", "context", "record"}

// renamedProblem finds, in a problem of the validator or in a UnknownKeyError, the unknown name it reports with its new
// spelling: the place (key, event or condition), the name and the section of a key below the top level, "action"
// for a key of the action, "rules" for a key of rules and "guardrails" for a shortcut, whose problems list the known
// keys before the hint. A top-level key is reported as a key.
var renamedProblem = regexp.MustCompile(
	`unknown (key|event|condition) "([^"]*)"(?: in (action|rules|guardrails)(?: \([^)]*\))?)?: `)

// Hinted returns the problem p, about the unknown name of the place of the file (a section of renamed, such as
// "guardrails"), followed by the hint of its new spelling when the name is one of the old vocabulary there; p as it
// is otherwise.
func (p Problem) Hinted(place, name string) Problem {
	if hint, ok := renamed[place][name]; ok {
		return p + Problem(": "+hint)
	}
	return p
}

// savePlaceholder and saveKey find, in a problem of guardrails.check_on_save, the name it reports as unknown: a
// placeholder, which the vocabulary of githooks reports with the ones it allows, or a key of an item. savePlaceholder
// also finds the placeholder of an item of guardrails.check_on_stop with also_on_save, whose entry of githooks the
// same vocabulary checks. The entries written in githooks match neither: their problems stay as they were before
// check_on_save.
var (
	savePlaceholder = regexp.MustCompile(
		`guardrails\.check_on_(?:save|stop): unknown placeholder \{([^}]*)\} \(githooks allow `)
	saveKey = regexp.MustCompile(`unknown key "([^"]*)" in guardrails\.check_on_save \(`)
)

// Known reports whether the problem is about a name this polix knows, never an option of a newer polix: a name of the
// old vocabulary (Renamed), or, in an item of check_on_save or of check_on_stop with also_on_save, a name of on that
// git can never give its hooks, since it runs them without a session and before the commit: a placeholder only an
// event of an agent gives (AgentOnlyBuiltins), or, in check_on_save, a key of check_on_stop (wait_agents,
// also_on_save). A measure of the diff, which git can compute, may be an option of a newer polix there, as any
// unknown name in an entry written in githooks.
func (p Problem) Known() bool {
	if p.Renamed() {
		return true
	}
	if m := savePlaceholder.FindStringSubmatch(string(p)); m != nil {
		return slices.Contains(AgentOnlyBuiltins, m[1])
	}
	if m := saveKey.FindStringSubmatch(string(p)); m != nil {
		return slices.Contains(CheckOnStopKeys, m[1])
	}
	return false
}

// Renamed reports whether the problem is about a name of the old vocabulary, reported with its new spelling: it is
// a name this polix knows it replaced, never an option of a newer polix.
func (p Problem) Renamed() bool {
	m := renamedProblem.FindStringSubmatch(string(p))
	if m == nil {
		return false
	}
	place, name, section := m[1], m[2], m[3]
	switch {
	case place == "event":
		_, ok := renamedEvents[name]
		return ok
	case place == "key" && section != "":
		_, ok := renamed[section][name]
		return ok
	case place == "key":
		_, entry := renamed["key"][name]
		_, top := renamed["top"][name]
		return entry || top
	default:
		_, ok := renamed[place][name]
		return ok
	}
}
