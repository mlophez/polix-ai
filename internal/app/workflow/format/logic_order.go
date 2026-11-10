package format

import (
	"github.com/mlophez/polix-ai/internal/app/workflow"
	"github.com/mlophez/polix-ai/internal/platform/yamlfmt"
)

// topKeys is the order of the top level keys of a formatted polix.yaml: the settings first, in the order of
// workflow.TopKeys, then requirements right before guardrails, since it is not a shortcut that expands into entries,
// and the two sections that hold the workflow, guardrails and on, last.
var topKeys = []string{"version", "requires", "base_branch", "max_stop_retries", "vars", "rules", "plans",
	"reports", "guidance", "risk", "requirements", "guardrails", "on"}

// varKeys and rulesKeys are the keys of a variable of vars and of the section rules.
var (
	varKeys   = []string{"branch"}
	rulesKeys = []string{"path", "run"}
)

// formatData formats the data of polix.yaml: two spaces of indentation, the comments and the blank lines kept, and the
// keys of each section in the order of the reference, with guardrails and on at the end. The items of a list and
// the keys a user names (variables, globs of risk.weights and rules.run) keep their order. It only needs YAML: a
// workflow with problems is formatted as well. The error tells why the data cannot be formatted (broken YAML, or
// what yamlfmt does not support).
func formatData(data []byte) ([]byte, error) {
	return yamlfmt.Format(data, order)
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
	case "rules":
		return rulesKeys
	case "plans", "reports":
		return workflow.FolderKeys
	case "guidance":
		return workflow.GuidanceKeys
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
