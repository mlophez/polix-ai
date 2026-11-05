package setup

import "github.com/mlophez/polix-ai/internal/rule"

// CheckSetupOutput is the decision of the logic of CheckSetup: the problems of the workflow and the rules, or the
// summary of a valid repository.
type CheckSetupOutput struct {
	Problems []string // every problem of polix.yaml and of ai/rules/; empty when both are valid
	Hint     string   // with problems, the hint that they may come from a newer polix; empty for none
	Warning  string   // without problems, the warning that rules check --changed would fail; empty for none
	Found    bool     // whether polix.yaml exists
	Rules    int      // how many rules ai/rules/ holds
}

// CheckSetup decides what polix check reports for the polix of version running: the problems of polix.yaml
// (found tells whether it exists, wfErr why it cannot be loaded or is not valid) and of the rules (invalid, the
// problems of the invalid ones, rulesErr why they cannot be read, missing the checks that do not exist), with the
// hint of a newer polix; or, when there are none, the warning when no base branch exists (baseErr) and a
// deterministic rule has a check.
func CheckSetup(running string, found bool, wfErr error, rules []rule.Rule, invalid []string, rulesErr error,
	missing map[string]bool, baseErr error) CheckSetupOutput {
	out := CheckSetupOutput{Found: found, Rules: len(rules)}
	problems, newer := WorkflowProblems(wfErr)
	if rulesErr != nil {
		problems = append(problems, rulesErr.Error())
	} else {
		problems = append(problems, RuleProblems(rules, invalid, missing)...)
	}
	if len(problems) > 0 {
		out.Problems = problems
		if newer {
			out.Hint = NewerHint(running)
		}
		return out
	}
	out.Warning = BaseWarning(rules, baseErr)
	return out
}
