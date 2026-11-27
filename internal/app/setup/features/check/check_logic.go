package check

import (
	"strings"

	"github.com/mlophez/polix-ai/internal/app/setup"
	"github.com/mlophez/polix-ai/internal/app/workflow"
)

// Output is the decision of the logic of CheckSetup: the problems of the workflow, or the summary of a valid one.
type Output struct {
	File string // the workflow file, as the console names it: the argument, or polix.yaml
	// WorkflowProblems holds the problems of the workflow file, "<file> not found" included; empty when it is valid
	WorkflowProblems []string
	Hint             string // with problems of the workflow file, the hint that they may come from a newer polix; empty for none
	Found            bool   // whether the workflow file exists
}

// Decide decides what polix config check reports from the inspection i, the part of it about the workflow, for the
// polix of the version of its setup: the problems of polix.yaml (Found tells whether it exists, WorkflowErr why it
// cannot be loaded or is not valid), with the hint of a newer polix. file is the workflow file the argument names,
// checked in place of polix.yaml: its problems name it instead of polix.yaml, and it must exist.
func Decide(i setup.Inspection, file string) Output {
	out := Output{File: workflow.File, Found: i.Found}
	if file != "" {
		out.File = file
		if !i.Found {
			out.WorkflowProblems = []string{file + " not found"}
			return out
		}
	}
	found, newer := i.WorkflowProblems()
	for _, p := range found {
		out.WorkflowProblems = append(out.WorkflowProblems, out.prefixed(p))
	}
	if newer && len(out.WorkflowProblems) > 0 {
		out.Hint = i.NewerHint()
	}
	return out
}

// prefixed returns the problem of the workflow with the polix.yaml it starts with (followed by a colon or a space,
// as every problem of the workflow does) written as File: the same problem for polix.yaml, and for one that does not
// start with it, as the error of a file that cannot be read, which already names the path it read.
func (out Output) prefixed(problem string) string {
	rest, ok := strings.CutPrefix(problem, workflow.File)
	if out.File == workflow.File || !ok || (rest != "" && rest[0] != ':' && rest[0] != ' ') {
		return problem
	}
	return out.File + rest
}
