package rule

import (
	"fmt"

	"github.com/mlophez/polix-ai/pkg/glob"
)

// Matching returns the rules whose scope matches at least one of the paths; a rule without scope applies to
// the whole project. It is domain, not a use case: other packages (hook) use it.
func Matching(all []Rule, paths []string) []Rule {
	var out []Rule
	for _, r := range all {
		if len(r.Scope) == 0 && len(paths) > 0 {
			out = append(out, r)
			continue
		}
		for _, p := range paths {
			if glob.AnyPath(r.Scope, p) {
				out = append(out, r)
				break
			}
		}
	}
	return out
}

// Repeated returns one problem for each rule whose id an earlier rule already has, naming its file and that of the last earlier one, so polix
// check tells that an id names one rule only.
func Repeated(all []Rule) []string {
	var problems []string
	seen := map[string]string{}
	for _, r := range all {
		if r.ID == "" {
			continue
		}
		if prev, ok := seen[r.ID]; ok {
			problems = append(problems, fmt.Sprintf("%s %s: id repeated, also in %s", r.File, r.ID, prev))
		}
		seen[r.ID] = r.File
	}
	return problems
}
