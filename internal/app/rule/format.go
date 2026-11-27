package rule

import (
	"bytes"
	"errors"
	"maps"
	"slices"

	"github.com/mlophez/polix-ai/internal/platform/yamlfmt"
)

// Source is the data of a rules file of the folder of rules, as read from the disk.
type Source []byte

// Folder is the folder of rules as a check reads it: its path, its rules and the data of each yaml file of it.
type Folder struct {
	Dir   string            // relative to the root, with slashes
	Rules []Rule            // ordered by id, each valid
	Files map[string]Source // by name
}

// FixCommand is the command that formats the rules files.
const FixCommand = "polix rules fmt"

// topKeys is the order of the top level keys of a formatted rules file: rules first.
var topKeys = []string{"rules"}

// ruleKeys is the order of the keys of a rule: the fields polix reads, in the order of rule.Rule.
var ruleKeys = []string{"id", "kind", "subject", "description", "scope", "deterministic", "check"}

// Format formats the data of a rules file: two spaces of indentation, the comments and the blank lines kept,
// rules first, the keys of each rule in the order of ruleKeys with the unknown ones after in their order, and one
// blank line between two rules. The rules never move. The error tells why the data cannot be formatted (broken
// YAML, or what yamlfmt does not support).
func (s Source) Format() ([]byte, error) {
	return yamlfmt.Format(s, s.order, s.spaced)
}

// order returns the order of the keys of the mapping of a rules file at path, nil to keep the source order.
func (Source) order(path []string) []string {
	switch {
	case len(path) == 0:
		return topKeys
	case slices.Equal(path, []string{"rules", "-"}):
		return ruleKeys
	}
	return nil
}

// spaced tells whether the items of the sequence at path are separated by one blank line: only the rules.
func (Source) spaced(path []string) bool {
	return slices.Equal(path, []string{"rules"})
}

// Valid requires the path of the folder and every rule of it valid.
func (f Folder) Valid() error {
	if f.Dir == "" {
		return errors.New("a folder of rules has no path")
	}
	for _, r := range f.Rules {
		if err := r.Valid(); err != nil {
			return err
		}
	}
	return nil
}

// Unformatted returns, sorted by name, the files of the folder that Source.Format would change, each as
// <dir>/<name>, which FixCommand formats, and those it cannot format, each as <dir>/<name>: <error>, which it cannot.
func (f Folder) Unformatted() (changed, problems []string) {
	for _, name := range slices.Sorted(maps.Keys(f.Files)) {
		formatted, err := f.Files[name].Format()
		switch {
		case err != nil:
			problems = append(problems, f.Dir+"/"+name+": "+err.Error())
		case !bytes.Equal(formatted, f.Files[name]):
			changed = append(changed, f.Dir+"/"+name)
		}
	}
	return changed, problems
}
