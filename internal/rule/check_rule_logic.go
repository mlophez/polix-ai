package rule

import "github.com/mlophez/polix-ai/internal/workflow"

// CheckRuleOutput is the decision of the logic of CheckRule, which the shell carries out through the port: the
// checks that run, in the order of their rules, each with the argv of its runner.
type CheckRuleOutput struct {
	Run []Run
}

// CheckRule decides which checks run and how: those of the deterministic rules, all of them or, with onlyChanged,
// only those whose scope matches the changed files; each through the first runner of wf whose glob matches its
// file, or as a program when none does.
func CheckRule(rules []Rule, wf workflow.Workflow, onlyChanged bool, changed []string) CheckRuleOutput {
	if onlyChanged {
		rules = Matching(rules, changed)
	}
	var out CheckRuleOutput
	for _, r := range rules {
		run := Run{Rule: r, Argv: wf.CheckCommand(r.Check, r.ID)}
		if err := run.Valid(); err != nil {
			continue // a rule without check has nothing to run
		}
		out.Run = append(out.Run, run)
	}
	return out
}
