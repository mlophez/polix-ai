package rule

import (
	"bytes"
	"errors"
	"fmt"
	"maps"
	"path"
	"slices"
	"strings"

	"github.com/mlophez/polix-ai/internal/platform/yamlfmt"
)

// Source is the data of a rules file of the folder of rules, as read from the disk.
type Source []byte

// Folder is the folder of rules as a use case reads it: its path, its rules and the data of each yaml file of it.
type Folder struct {
	Dir   string            // relative to the root, with slashes
	Rules []Rule            // ordered by id, each valid
	Files map[string]Source // by name
}

// Formatted is what formatting a folder of rules gives: the files it changes, with their data, and those it cannot
// format.
type Formatted struct {
	Changed  []string          // the files the format changes, as <dir>/<name>, sorted by name
	Data     map[string][]byte // the formatted data of each changed file, by name
	Problems []string          // why each file cannot be formatted, as <dir>/<name>: <error>, sorted by name
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

// Valid requires what the fields of the folder declare: a path relative to the root with slashes, the rules ordered
// by id and each valid, and the files named without a folder.
func (f Folder) Valid() error {
	switch {
	case f.Dir == "":
		return errors.New("a folder of rules has no path")
	case path.IsAbs(f.Dir) || strings.ContainsAny(f.Dir, `\:`):
		return fmt.Errorf("the folder of rules %q is not relative to the root with slashes", f.Dir)
	case !slices.IsSortedFunc(f.Rules, func(a, b Rule) int { return strings.Compare(a.ID, b.ID) }):
		return fmt.Errorf("the rules of %s are not ordered by id", f.Dir)
	}
	for name := range f.Files {
		if name == "" || strings.ContainsAny(name, `/\`) {
			return fmt.Errorf("the file %q of %s is not a name", name, f.Dir)
		}
	}
	for _, r := range f.Rules {
		if err := r.Valid(); err != nil {
			return err
		}
	}
	return nil
}

// Format formats every file of the folder with Source.Format, in the order of the names: a file it would change goes
// to Changed as <dir>/<name>, with its formatted data in Data by name, which FixCommand writes; one it cannot format
// goes to Problems as <dir>/<name>: <error>, which FixCommand cannot fix.
func (f Folder) Format() Formatted {
	var changed, problems []string
	var data map[string][]byte
	for _, name := range slices.Sorted(maps.Keys(f.Files)) {
		formatted, err := f.Files[name].Format()
		switch {
		case err != nil:
			problems = append(problems, f.Dir+"/"+name+": "+err.Error())
		case !bytes.Equal(formatted, f.Files[name]):
			if data == nil {
				data = map[string][]byte{}
			}
			changed = append(changed, f.Dir+"/"+name)
			data[name] = formatted
		}
	}
	out := Formatted{Changed: changed, Data: data, Problems: problems}
	if err := out.Valid(); err != nil {
		return out.Fail(f.Dir + ": " + err.Error())
	}
	return out
}

// Valid requires the formatted data of each file changed by its name, and no more, and the files changed and the
// problems sorted by name.
func (f Formatted) Valid() error {
	if len(f.Data) != len(f.Changed) {
		return fmt.Errorf("%d files changed with the data of %d", len(f.Changed), len(f.Data))
	}
	for _, c := range f.Changed {
		if _, ok := f.Data[path.Base(c)]; !ok {
			return fmt.Errorf("the file changed %s has no formatted data", c)
		}
	}
	byName := func(a, b string) int {
		a, _, _ = strings.Cut(a, ": ")
		b, _, _ = strings.Cut(b, ": ")
		return strings.Compare(a, b)
	}
	if !slices.IsSorted(f.Changed) || !slices.IsSortedFunc(f.Problems, byName) {
		return errors.New("the files changed or the problems are not sorted by name")
	}
	return nil
}

// Fail returns f failed with problem: nothing changed nor to write, and problem its only one.
func (f Formatted) Fail(problem string) Formatted {
	failed := f
	failed.Changed, failed.Data, failed.Problems = nil, nil, []string{problem}
	return failed
}
