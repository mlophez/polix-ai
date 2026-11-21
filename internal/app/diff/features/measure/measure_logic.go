package measure

import (
	"github.com/mlophez/polix-ai/internal/app/diff"
	"github.com/mlophez/polix-ai/internal/app/workflow"
)

// Output is the decision of the logic of MeasureDiff: the measures of the diff narrowed to the globs,
// and the share of each file in the risk score.
type Output struct {
	Lines int // changed lines, as the condition changed_lines counts them
	Files int // changed files, as the condition changed_files counts them
	Risk  int // risk score, as the condition risk_score computes it
	// Contributions are the shares of each file in the risk score, the highest first, then by path.
	Contributions []diff.Contribution
	// Changes are the changes of the files of Contributions, in the same order, for their added and deleted lines.
	Changes []diff.Change
}

// Decide measures the diff d narrowed to the globs (all of it without globs), with the weights of the risk
// score: its lines, its files, its risk score and the contribution of each file.
func Decide(d diff.Diff, weights []workflow.Weight, globs []string) Output {
	narrowed := d.Narrow(globs)
	contributions := narrowed.Contributions(weights)
	byPath := make(map[string]diff.Change, len(narrowed.Changes))
	for _, c := range narrowed.Changes {
		byPath[c.Path] = c
	}
	changes := make([]diff.Change, 0, len(contributions))
	for _, c := range contributions {
		changes = append(changes, byPath[c.Path])
	}
	return Output{
		Lines:         narrowed.Lines(),
		Files:         narrowed.Count(),
		Risk:          narrowed.Risk(weights),
		Contributions: contributions,
		Changes:       changes,
	}
}
