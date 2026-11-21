package format

import (
	"slices"

	"github.com/mlophez/polix-ai/internal/platform/yamlfmt"
)

// topKeys is the order of the top level keys of a formatted rules file: rules first.
var topKeys = []string{"rules"}

// ruleKeys is the order of the keys of a rule: the fields polix reads, in the order of rule.Rule, then the reason
// of a retired rule.
var ruleKeys = []string{"id", "description", "scope", "deterministic", "check", "reason"}

// formatData formats the data of a rules file: two spaces of indentation, the comments and the blank lines kept,
// rules first, the keys of each rule in the order of ruleKeys with the unknown ones after in their order, and one
// blank line between two rules. The rules never move. The error tells why the data cannot be formatted (broken
// YAML, or what yamlfmt does not support).
func formatData(data []byte) ([]byte, error) {
	return yamlfmt.Format(data, order, spaced)
}

// order returns the order of the keys of the mapping of a rules file at path, nil to keep the source order.
func order(path []string) []string {
	switch {
	case len(path) == 0:
		return topKeys
	case slices.Equal(path, []string{"rules", "-"}):
		return ruleKeys
	}
	return nil
}

// spaced tells whether the items of the sequence at path are separated by one blank line: only the rules.
func spaced(path []string) bool {
	return slices.Equal(path, []string{"rules"})
}
