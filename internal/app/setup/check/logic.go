package check

import (
	"github.com/mlophez/polix-ai/internal/app/rule"
	"github.com/mlophez/polix-ai/internal/app/setup"
)

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

// Decide decides what polix config check reports for the polix of version running: the problems of polix.yaml
// (found tells whether it exists, wfErr why it cannot be loaded or is not valid) and of the rules of the folder
// rulesPath (invalid, the problems of the invalid ones, rulesErr why they cannot be read, missing the checks that
// do not exist), with the hint of a newer polix; or, when there are none, the warning when no base branch exists
// (baseErr) and a deterministic rule has a check.
func Decide(running string, found bool, wfErr error, rulesPath string, rules []rule.Rule, invalid []string,
	rulesErr error, missing map[string]bool, baseErr error) Output {
	out := Output{Found: found, Rules: len(rules), RulesPath: rulesPath}
	s := setup.Setup{Version: running}
	if err := s.Valid(); err != nil {
		out.Problems = []string{err.Error()}
		return out
	}
	i := setup.Inspection{Setup: s, Found: found, WorkflowErr: wfErr, Rules: rules, Invalid: invalid,
		RulesErr: rulesErr, Missing: missing, BaseErr: baseErr}
	if err := i.Valid(); err != nil {
		out.Problems = []string{err.Error()}
		return out
	}
	problems, newer := i.WorkflowProblems()
	if rulesErr != nil {
		problems = append(problems, rulesErr.Error())
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
