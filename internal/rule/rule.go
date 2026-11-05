// Package rule holds the rules of ai/rules/: how the code of the project must be written.
//
// Polix only reads the fields it needs (id, description, scope, deterministic, check); the rest of the rule
// format belongs to the skills that write and use the rules.
package rule

import (
	"errors"
	"fmt"
	"strings"
)

// Rule is one rule of ai/rules/. Its fields are read freely and set only when it is built, followed by Valid;
// after that it changes only through its methods, which return a copy.
type Rule struct {
	ID            string
	Description   string
	Scope         []string // globs of the files it applies to; empty applies to the whole project
	Deterministic bool
	Check         string // file that decides it (a script, a test), relative to the root, with slashes
	File          string // file it was read from, relative to the root
}

// Dir is the folder of the rules, relative to the repository root.
const Dir = "ai/rules"

// Valid reports why the rule is not a rule: it needs an id and a description, only a deterministic rule may
// name a check, and the check is relative to the root.
func (r Rule) Valid() error {
	switch {
	case r.ID == "":
		return errors.New("a rule has no id")
	case r.Description == "":
		return fmt.Errorf("rule %s has no description", r.ID)
	case r.Check != "" && !r.Deterministic:
		return fmt.Errorf("rule %s names a check but is not deterministic", r.ID)
	case absolute(r.Check):
		return fmt.Errorf("rule %s names the check %s by an absolute path, not relative to the root", r.ID, r.Check)
	}
	return nil
}

// absolute reports whether p is an absolute path on any system: /x, \x or a drive letter (C:).
func absolute(p string) bool {
	if strings.HasPrefix(p, "/") || strings.HasPrefix(p, `\`) {
		return true
	}
	return len(p) >= 2 && p[1] == ':' && ('a' <= p[0]|0x20 && p[0]|0x20 <= 'z')
}

// HasCheck reports whether a file decides the rule.
func (r Rule) HasCheck() bool { return r.Deterministic && r.Check != "" }
