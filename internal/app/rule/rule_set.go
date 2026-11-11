package rule

import (
	"errors"
	"fmt"

	"github.com/mlophez/polix-ai/internal/platform/glob"
)

// Rules are several rules, such as those of a folder of rules, in their order. It is domain, not of a use case:
// CheckRule, AnswerHook and the setup use cases ask it.
type Rules []Rule

// Selection is which rules a check is asked for: all of them, or only those whose scope matches the changed files.
// It is built with a literal followed by Valid.
type Selection struct {
	OnlyChanged bool     // only the rules whose scope matches one of Changed
	Changed     []string // the changed files, read only with OnlyChanged
}

// Valid tells why s is not a selection: changed files without OnlyChanged select nothing.
func (s Selection) Valid() error {
	if !s.OnlyChanged && len(s.Changed) > 0 {
		return errors.New("a selection of every rule takes no changed files")
	}
	return nil
}

// Narrow returns the rules of rs the selection asks for: every one, or with OnlyChanged those whose scope matches
// the changed files (Matching).
func (s Selection) Narrow(rs Rules) Rules {
	if s.OnlyChanged {
		return rs.Matching(s.Changed)
	}
	return rs
}

// Matching returns the rules whose scope matches at least one of the paths; a rule without scope applies to
// the whole project.
func (rs Rules) Matching(paths []string) Rules {
	var out Rules
	for _, r := range rs {
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

// Repeated returns one problem for each rule whose id an earlier rule or a retired rule already has, naming its file
// and that of the last earlier one, so an id names one rule only, retired ones included. The ids of retired are taken
// first, with their file; a repeat among the retired rules themselves is never reported.
func (rs Rules) Repeated(retired Rules) []string {
	var problems []string
	seen := map[string]string{}
	for _, r := range retired {
		if r.ID != "" {
			seen[r.ID] = r.File
		}
	}
	for _, r := range rs {
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
