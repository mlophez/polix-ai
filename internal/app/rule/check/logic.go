package check

import (
	"slices"

	"github.com/mlophez/polix-ai/internal/app/rule"
	"github.com/mlophez/polix-ai/internal/app/workflow"
)

// Output is the decision of the logic of CheckRule, in the step shape: the next run the shell carries out
// through the port or, once it is nil, the results of every rule, in the order of the rules.
type Output struct {
	Next    *rule.Run
	Results []rule.Result
}

// Decide decides which checks run and how: those of the deterministic rules, all of them or, with onlyChanged,
// only those whose scope matches the changed files, grouped into runs by the runners of wf (rule.Rules.Plan). It walks the
// runs, and the reruns of each failed shared run (rule.Run.Settle), taking one of outcomes, the runs carried out so
// far, for each; the first run without outcome is the next one, and with none left it returns the results.
func Decide(rules []rule.Rule, wf workflow.Workflow, onlyChanged bool, changed []string,
	outcomes []rule.Outcome) Output {
	if onlyChanged {
		rules = rule.Rules(rules).Matching(changed)
	}
	queue := rule.Rules(rules).Plan(wf)
	var results []rule.Result
	for k := 0; len(queue) > 0; k++ {
		run := queue[0]
		if k == len(outcomes) {
			return Output{Next: &run}
		}
		res, reruns := run.Settle(outcomes[k], wf)
		results = append(results, res...)
		queue = append(reruns, queue[1:]...)
	}
	order := map[string]int{}
	for i, r := range rules {
		order[r.ID] = i
	}
	slices.SortStableFunc(results, func(a, b rule.Result) int { return order[a.Rule.ID] - order[b.Rule.ID] })
	return Output{Results: results}
}
