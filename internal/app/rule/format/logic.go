package format

import (
	"bytes"
	"maps"
	"slices"
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

// fixCommand is the command that formats the rules files.
const fixCommand = "polix rules fmt"

// Decide decides how to format the rules files of the folder dir, whose data is files by name: without files
// nothing; else each file formatted with formatData, a failure being its problem and a different result a change,
// with the command that formats them. The ids repeated among the rules, as the loader of the rules words them
// (rule.Loaded.Repeated), or the error where the loader stops, are problems too, after those of the files, each
// once: the error of a file that is not yaml reads as its own problem. It is all or nothing: with any problem, or
// when check only asks whether the files are formatted, nothing is written.
func Decide(dir string, files map[string][]byte, repeated []string, check bool) Output {
	out := Output{Dir: dir, Empty: len(files) == 0}
	write := map[string][]byte{}
	for _, name := range slices.Sorted(maps.Keys(files)) {
		formatted, err := formatData(files[name])
		switch {
		case err != nil:
			out.Problems = append(out.Problems, dir+"/"+name+": "+err.Error())
		case !bytes.Equal(formatted, files[name]):
			out.Changed = append(out.Changed, dir+"/"+name)
			write[name] = formatted
		}
	}
	for _, r := range repeated {
		if !slices.Contains(out.Problems, r) {
			out.Problems = append(out.Problems, r)
		}
	}
	if len(out.Changed) > 0 {
		out.Fix = fixCommand
	}
	if len(out.Problems) == 0 && len(write) > 0 && !check {
		out.Write = write
	}
	return out
}
