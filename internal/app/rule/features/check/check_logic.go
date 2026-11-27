package check

import (
	"slices"

	"github.com/mlophez/polix-ai/internal/app/rule"
	"github.com/mlophez/polix-ai/internal/app/workflow"
)

// Output is the decision of the logic of CheckRule, in the step shape: the next run the shell carries out
// through the port or, once it is nil, the results of every rule, in the order of the rules.
type Output struct {
	Unformatted []string // the files of the folder of rules not formatted, as <dir>/<name>; then nothing runs
	Problems    []string // the files that cannot be formatted, as <dir>/<name>: <error>; then nothing runs
	Fix         string   // with a file not formatted, the command that formats the files
	Ran         bool     // whether the checks ran: with run and every file formatted
	NotRun      int      // without Run, how many rules have a check that did not run
	Next        *rule.Run
	Results     []rule.Result
}

// Decide decides first whether the files of folder are formatted (rule.Folder.Format): one that is not, or cannot
// be, stops it, and so does a selection without Run, which counts the checks it leaves unrun. Then it decides which
// checks run and how, of the rules of folder: those of the deterministic rules that sel selects, all of them or only
// those whose scope matches the changed files, grouped into runs by the runners of wf (rule.Rules.Plan). It walks the
// runs, and the reruns of each failed shared run (rule.Run.Settle), taking one of outcomes, the runs carried out so
// far, for each; the first run without outcome is the next one, and with none left it returns the results.
func Decide(folder rule.Folder, wf workflow.Workflow, sel rule.Selection, outcomes []rule.Outcome) Output {
	var out Output
	formatted := folder.Format()
	out.Unformatted, out.Problems = formatted.Changed, formatted.Problems
	if len(out.Unformatted) > 0 {
		out.Fix = rule.FixCommand
	}
	if len(out.Unformatted) > 0 || len(out.Problems) > 0 {
		return out
	}
	if !sel.Run {
		for _, r := range folder.Rules {
			if r.HasCheck() {
				out.NotRun++
			}
		}
		return out
	}
	out.Ran = true
	rules := sel.Narrow(folder.Rules)
	queue := rules.Plan(wf)
	var results []rule.Result
	for k := 0; len(queue) > 0; k++ {
		next := queue[0]
		if k == len(outcomes) {
			out.Next = &next
			return out
		}
		res, reruns := next.Settle(outcomes[k], wf)
		results = append(results, res...)
		queue = append(reruns, queue[1:]...)
	}
	order := map[string]int{}
	for i, r := range rules {
		order[r.ID] = i
	}
	slices.SortStableFunc(results, func(a, b rule.Result) int { return order[a.Rule.ID] - order[b.Rule.ID] })
	out.Results = results
	return out
}
