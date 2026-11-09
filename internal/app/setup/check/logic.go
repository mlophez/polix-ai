package check

import "github.com/mlophez/polix-ai/internal/app/setup"

// Output is the decision of the logic of CheckSetup: the problems of the workflow and the rules, or the
// summary of a valid repository.
type Output struct {
	Problems  []string // every problem of polix.yaml and of the rules; empty when both are valid
	Hint      string   // with problems, the hint that they may come from a newer polix; empty for none
	Warning   string   // without problems, the warning that rules check --changed would fail; empty for none
	Found     bool     // whether polix.yaml exists
	Rules     int      // how many rules the folder of rules holds
	RulesPath string   // the folder of the rules, relative to the root
}

// Decide decides what polix config check reports from the inspection i, the part of it about the workflow and the
// rules, for the polix of the version of its setup: the problems of polix.yaml (Found tells whether it exists,
// WorkflowErr why it cannot be loaded or is not valid) and of the rules of its folder of rules (Invalid, the problems
// of the invalid ones, RulesErr why they cannot be read, Missing the checks that do not exist), with the hint of a
// newer polix; or, when there are none, the warning when no base branch exists (BaseErr) and a deterministic rule
// has a check.
func Decide(i setup.Inspection) Output {
	out := Output{Found: i.Found, Rules: len(i.Rules), RulesPath: i.Workflow.RulesPath()}
	problems, newer := i.WorkflowProblems()
	if i.RulesErr != nil {
		problems = append(problems, i.RulesErr.Error())
	} else {
		problems = append(problems, i.RuleProblems()...)
	}
	if len(problems) > 0 {
		out.Problems = problems
		if newer {
			out.Hint = i.NewerHint()
		}
		return out
	}
	out.Warning = i.BaseWarning()
	return out
}
