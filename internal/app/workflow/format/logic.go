package format

import (
	"strings"

	"github.com/mlophez/polix-ai/internal/app/workflow"
)

// Output is the decision of the logic of FormatWorkflow: what to write to the workflow file, or why nothing.
type Output struct {
	File    string // the file, as the console names it: the file of the request, or polix.yaml
	Fix     string // with a change, the command that formats the file; empty without one
	Missing bool   // the workflow file does not exist: nothing to format
	Problem string // why the workflow file cannot be formatted; empty when it can
	Changed bool   // the formatted workflow file differs from the one on disk
	Write   []byte // the formatted workflow file to write; nil when nothing is written
}

// Decide decides how to format the workflow file, whose data is data and whose existence is found: without the file
// nothing; when formatData fails, its error as the problem; else whether the format changes it, and the data to
// write when it does, unless check only asks whether it is formatted, with the command that formats it. file is the
// workflow file of the request, empty for polix.yaml: the output names it.
func Decide(data []byte, found, check bool, file string) Output {
	name := file
	if name == "" {
		name = workflow.File
	}
	if !found {
		return Output{File: name, Missing: true}
	}
	formatted, err := formatData(data)
	if err != nil {
		return Output{File: name, Problem: err.Error()}
	}
	out := Output{File: name, Changed: string(formatted) != string(data)}
	if out.Changed {
		out.Fix = fixCommand(file)
	}
	if out.Changed && !check {
		out.Write = formatted
	}
	return out
}

// fixCommand returns the command that formats file, the file of the request: polix config fmt alone without one,
// else followed by the file, even when it is named polix.yaml, as a shell word, between single quotes when it holds a blank or a special character, so
// the command can be copied and run.
func fixCommand(file string) string {
	switch {
	case file == "":
		return "polix config fmt"
	case !strings.ContainsAny(file, " \t\n'\"$`\\;&|<>()*?[]{}#~!"):
		return "polix config fmt " + file
	}
	return "polix config fmt '" + strings.ReplaceAll(file, "'", `'\''`) + "'"
}
