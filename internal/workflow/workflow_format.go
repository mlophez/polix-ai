package workflow

import (
	"github.com/mlophez/polix-ai/pkg/yamlfmt"
)

// formatTopKeys is the order of the top level keys of a formatted polix.yaml: the settings first, in the order of
// topKeys, and the two sections that hold the workflow, guardrails and on, last.
var formatTopKeys = []string{"version", "requires", "base_branch", "max_stop_retries", "vars", "rules", "plans",
	"reports", "guidance", "risk", "guardrails", "on"}

// formatVarKeys and formatRulesKeys are the keys of a variable of vars and of the section rules.
var (
	formatVarKeys   = []string{"branch"}
	formatRulesKeys = []string{"path", "run"}
)

// Format formats the data of polix.yaml: two spaces of indentation, the comments and the blank lines kept, and the
// keys of each section in the order of the reference, with guardrails and on at the end. The items of a list and
// the keys a user names (variables, globs of risk.weights and rules.run) keep their order. It only needs YAML: a
// workflow with problems is formatted as well. The error tells why the data cannot be formatted (broken YAML, or
// what yamlfmt does not support).
func Format(data []byte) ([]byte, error) {
	return yamlfmt.Format(data, formatOrder)
}

// formatOrder returns the order of the keys of the mapping of polix.yaml at path, nil to keep the source order.
func formatOrder(path []string) []string {
	switch {
	case len(path) == 0:
		return formatTopKeys
	case len(path) == 1:
		return formatSection(path[0])
	case path[0] == "vars" && len(path) == 2:
		return formatVarKeys
	case path[0] == "guardrails" && len(path) == 3 && path[2] == "-":
		return formatShortcut(path[1])
	case path[0] == "on" && len(path) >= 3 && path[2] == "-":
		return formatEntry(path[3:])
	}
	return nil
}

// formatSection returns the order of the keys of the top level section key.
func formatSection(key string) []string {
	switch key {
	case "guardrails":
		return guardrailsKeys
	case "on":
		return Events
	case "rules":
		return formatRulesKeys
	case "plans", "reports":
		return FolderKeys
	case "guidance":
		return GuidanceKeys
	case "risk":
		return riskKeys
	}
	return nil
}

// formatShortcut returns the order of the keys of an item of the shortcut key of guardrails.
func formatShortcut(key string) []string {
	switch key {
	case shortcutForbiddenCommands:
		return forbiddenKeys
	case shortcutFormatOnEdit:
		return formatKeys
	case shortcutRunBeforeDone:
		return doneKeys
	}
	return nil
}

// formatEntry returns the order of the keys of an entry of on at the path below the entry: the entry, its
// conditions (when, unless, and any or not nested in them) and its action.
func formatEntry(path []string) []string {
	switch {
	case len(path) == 0:
		return entryKeys
	case path[0] == "when" || path[0] == "unless":
		for _, k := range path[1:] {
			if k != "any" && k != "not" {
				return nil
			}
		}
		return Conditions
	case path[0] != "action":
		return nil
	case len(path) == 1:
		return Actions
	case len(path) > 2:
		return nil
	}
	switch path[1] {
	case VerbRun:
		return runKeys
	case VerbInject:
		return injectKeys
	case VerbBlock:
		return blockKeys
	}
	return nil
}
