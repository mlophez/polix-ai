package workflow

import "regexp"

// Problem is a problem of the workflow: one line of Problems, or the text of a UnknownKeyError.
type Problem string

// renamedEvents maps the events of the old vocabulary to their new name.
var renamedEvents = map[string]string{"session_start": Start, "turn_stop": Stop}

// renamed holds, per place of the file, the names of the old vocabulary and the hint of their new spelling, which
// the validator, or UnknownKeyError for a top-level key (place "top"), appends to the problem of the unknown name.
var renamed = map[string]map[string]string{
	"top": {
		"protected":       "use guardrails.protected_files",
		"base":            "use base_branch",
		"max_stop_blocks": "use max_stop_retries",
	},
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
// spelling: the place (key, event or condition), the name and, for a key of the action, " in action". A top-level
// key is reported as a key.
var renamedProblem = regexp.MustCompile(`unknown (key|event|condition) "([^"]*)"( in action)?: `)

// Renamed reports whether the problem is about a name of the old vocabulary, reported with its new spelling: it is
// a name this polix knows it replaced, never an option of a newer polix.
func (p Problem) Renamed() bool {
	m := renamedProblem.FindStringSubmatch(string(p))
	if m == nil {
		return false
	}
	place, name, inAction := m[1], m[2], m[3] != ""
	switch {
	case place == "event":
		_, ok := renamedEvents[name]
		return ok
	case place == "key" && inAction:
		_, ok := renamed["action"][name]
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
