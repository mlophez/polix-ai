package format

import (
	"github.com/mlophez/polix-ai/internal/app/workflow"
	"github.com/mlophez/polix-ai/internal/platform/yamlfmt"
)

// topKeys is the order of the top level keys of a formatted polix.yaml: version and path, the folder of the AI,
// first, then the other settings, then requirements right before guardrails, since it is not a shortcut that
// expands into entries, the two sections that hold the workflow of the AI, guardrails and on, and githooks, the
// gates git runs, last.
var topKeys = []string{"version", "path", "requires", "base_branch", "max_stop_retries", "vars", "rules",
	"risk", "requirements", "guardrails", "on", "githooks"}

// varKeys are the keys of a variable of vars.
var varKeys = []string{"branch"}

// formatData formats the data of polix.yaml: two spaces of indentation, the comments and the blank lines kept, the
// keys of each section in the order of the reference, with guardrails and on at the end, and one blank line between
// two items of a list of maps (the entries of an event and the items of a shortcut). The items of a list and the
// keys a user names (variables, globs of risk.weights and rules.run) keep their order. It only needs YAML: a
// workflow with problems is formatted as well. The error tells why the data cannot be formatted (broken YAML, or
// what yamlfmt does not support).
func formatData(data []byte) ([]byte, error) {
	return yamlfmt.Format(data, order, spaced)
}

// spaced tells whether the items of the sequence at path are separated by one blank line: the entries of each event
// of on and of githooks, and the items of the shortcuts of guardrails that take a list of maps. The lists of texts
// stay together.
func spaced(path []string) bool {
	if len(path) != 2 {
		return false
	}
	switch path[0] {
	case "on", "githooks":
		return true
	case "guardrails":
		return path[1] == workflow.ShortcutForbiddenCommands || path[1] == workflow.ShortcutFormatOnEdit ||
			path[1] == workflow.ShortcutRunOnStop
	}
	return false
}

// order returns the order of the keys of the mapping of polix.yaml at path, nil to keep the source order.
func order(path []string) []string {
	switch {
	case len(path) == 0:
		return topKeys
	case len(path) == 1:
		return orderSection(path[0])
	case path[0] == "vars" && len(path) == 2:
		return varKeys
	case path[0] == "guardrails" && len(path) == 3 && path[2] == "-":
		return orderShortcut(path[1])
	case path[0] == "on" && len(path) >= 3 && path[2] == "-":
		return orderEntry(path[3:])
	case path[0] == "githooks" && len(path) >= 3 && path[2] == "-":
		return orderGithookEntry(path[3:])
	}
	return nil
}

// orderSection returns the order of the keys of the top level section key.
func orderSection(key string) []string {
	switch key {
	case "guardrails":
		return workflow.GuardrailsKeys
	case "on":
		return workflow.Events
	case "githooks":
		return workflow.GithookEvents
	case "rules":
		return workflow.RulesKeys
	case "risk":
		return workflow.RiskKeys
	}
	return nil
}

// orderShortcut returns the order of the keys of an item of the shortcut key of guardrails.
func orderShortcut(key string) []string {
	switch key {
	case workflow.ShortcutForbiddenCommands:
		return workflow.ForbiddenKeys
	case workflow.ShortcutFormatOnEdit:
		return workflow.FormatOnEditKeys
	case workflow.ShortcutRunOnStop:
		return workflow.RunOnStopKeys
	}
	return nil
}

// orderGithookEntry returns the order of the keys of an entry of githooks at the path below the entry: that of an
// entry of on (orderEntry), but for the conditions of its when and unless, in the order of
// workflow.GithookConditions.
func orderGithookEntry(path []string) []string {
	if len(path) == 1 && (path[0] == "when" || path[0] == "unless") {
		return workflow.GithookConditions
	}
	return orderEntry(path)
}

// orderEntry returns the order of the keys of an entry of on at the path below the entry: the entry, its
// conditions (when, unless, and any or not nested in them) and its action.
func orderEntry(path []string) []string {
	switch {
	case len(path) == 0:
		return workflow.EntryKeys
	case path[0] == "when" || path[0] == "unless":
		for _, k := range path[1:] {
			if k != "any" && k != "not" {
				return nil
			}
		}
		return workflow.Conditions
	case path[0] != "action":
		return nil
	case len(path) == 1:
		return workflow.Actions
	case len(path) > 2:
		return nil
	}
	switch path[1] {
	case workflow.VerbRun:
		return workflow.RunKeys
	case workflow.VerbInject:
		return workflow.InjectKeys
	case workflow.VerbBlock:
		return workflow.BlockKeys
	case workflow.VerbNotify:
		return workflow.NotifyKeys
	}
	return nil
}
