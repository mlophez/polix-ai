package format

import (
	"slices"

	"github.com/mlophez/polix-ai/internal/app/rule"
)

// Output is the decision of the logic of FormatRule: what to write to the folder of rules, or why nothing.
type Output struct {
	Dir      string            // the folder of rules, relative to the root, with slashes
	Empty    bool              // the folder holds no yaml file: nothing to format
	Problems []string          // why each file cannot be formatted, by name, then each id repeated or the loader error
	Changed  []string          // the files the format changes, as <dir>/<name>, sorted by name
	Fix      string            // with a change, the command that formats the files; empty without one
	Write    map[string][]byte // the formatted data of each changed file by name; nil when nothing is written
}

// Decide decides how to format folder, the rules files of the folder of rules: without files nothing; else each file
// formatted with rule.Folder.Format, a failure being its problem and a different result a change, with the command
// that formats them. The ids repeated among the rules, as the loader of the rules words them (rule.Loaded.Repeated),
// or the error where the loader stops, are problems too, after those of the files, each once: the error of a file
// that is not yaml reads as its own problem. It is all or nothing: with any problem, or when check only asks whether
// the files are formatted, nothing is written.
func Decide(folder rule.Folder, repeated []string, check bool) Output {
	formatted := folder.Format()
	out := Output{Dir: folder.Dir, Empty: len(folder.Files) == 0, Problems: formatted.Problems,
		Changed: formatted.Changed}
	for _, r := range repeated {
		if !slices.Contains(out.Problems, r) {
			out.Problems = append(out.Problems, r)
		}
	}
	if len(out.Changed) > 0 {
		out.Fix = rule.FixCommand
	}
	if len(out.Problems) == 0 && len(formatted.Data) > 0 && !check {
		out.Write = formatted.Data
	}
	return out
}
