// Package rule holds the rules of ai/rules/: how the code of the project must be written.
//
// Polix only reads the fields it needs (id, description, scope, deterministic, check); the rest of the rule
// format belongs to the skills that write and use the rules.
package rule

import (
	"errors"
	"fmt"

	"github.com/mlophez/polix-ai/pkg/glob"
)

// Rule is one rule of ai/rules/. Its fields are read freely and set only when it is built, followed by Valid;
// after that it changes only through its methods, which return a copy.
type Rule struct {
	ID            string
	Description   string
	Scope         []string // globs of the files it applies to; empty applies to the whole project
	Deterministic bool
	Check         string // script that decides it, relative to the root
	File          string // file it was read from, relative to the root
}

// Dir is the folder of the rules, relative to the repository root.
const Dir = "ai/rules"

// Valid reports why the rule is not a rule: it needs an id and a description, and only a deterministic rule may
// name a check.
func (r Rule) Valid() error {
	switch {
	case r.ID == "":
		return errors.New("a rule has no id")
	case r.Description == "":
		return fmt.Errorf("rule %s has no description", r.ID)
	case r.Check != "" && !r.Deterministic:
		return fmt.Errorf("rule %s names a check but is not deterministic", r.ID)
	}
	return nil
}

// HasCheck reports whether a script decides the rule.
func (r Rule) HasCheck() bool { return r.Deterministic && r.Check != "" }

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
