package check

import (
	"strings"

	"github.com/mlophez/polix-ai/internal/app/setup"
	"github.com/mlophez/polix-ai/internal/app/workflow"
)

// Output is the decision of the logic of CheckSetup: the problems of the workflow and the rules, or the
// summary of a valid repository.
type Output struct {
	File string // the workflow file, as the console names it: the argument, or polix.yaml
	// WorkflowProblems holds the problems of the workflow file, "<file> not found" included; empty when it is valid
	WorkflowProblems []string
	// RuleProblems holds the problems of the rules, the error of reading them included; empty when they are valid
	RuleProblems []string
	Hint         string // with problems of the workflow file, the hint that they may come from a newer polix; empty for none
	Warning      string // without problems, the warning that rules check --changed would fail; empty for none
	Found        bool   // whether the workflow file exists
	Rules        int    // how many rules the folder of rules holds
	RulesPath    string // the folder of the rules, relative to the root
}

// Decide decides what polix config check reports from the inspection i, the part of it about the workflow and the
// rules, for the polix of the version of its setup: the problems of polix.yaml (Found tells whether it exists,
// WorkflowErr why it cannot be loaded or is not valid) and of the rules of its folder of rules (Invalid, the problems
// of the invalid ones, RulesErr why they cannot be read, Missing the checks that do not exist), with the hint of a
// newer polix; or, when there are none, the warning when no base branch exists (BaseErr) and a deterministic rule
// has a check. file is the workflow file the argument names, checked in place of polix.yaml with the rules of the
// repository, empty for polix.yaml: its problems and the warning name it instead of polix.yaml, and it must exist.
func Decide(i setup.Inspection, file string) Output {
	out := Output{File: workflow.File, Found: i.Found, Rules: len(i.Rules), RulesPath: i.Workflow.RulesPath()}
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
	if i.RulesErr != nil {
		out.RuleProblems = []string{i.RulesErr.Error()}
	} else {
		out.RuleProblems = append(out.RuleProblems, i.RuleProblems()...)
	}
	if len(out.WorkflowProblems) > 0 || len(out.RuleProblems) > 0 {
		if newer && len(out.WorkflowProblems) > 0 {
			out.Hint = i.NewerHint()
		}
		return out
	}
	out.Warning = out.named(i.BaseWarning())
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

// named returns the warning with polix.yaml, where it names the workflow file as a word of its own (at the start or
// after a space, and followed by a colon, a space or the end), written as File: the same text for polix.yaml. A path
// that ends in polix.yaml, as in the error of a file that cannot be read, is left as it is.
func (out Output) named(text string) string {
	if out.File == workflow.File {
		return text
	}
	var b strings.Builder
	for {
		at := strings.Index(text, workflow.File)
		if at < 0 {
			b.WriteString(text)
			return b.String()
		}
		end := at + len(workflow.File)
		b.WriteString(text[:at])
		word := (at == 0 || text[at-1] == ' ') && (end == len(text) || text[end] == ':' || text[end] == ' ')
		if word {
			b.WriteString(out.File)
		} else {
			b.WriteString(workflow.File)
		}
		text = text[end:]
	}
}
