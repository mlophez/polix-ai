// Package rule holds the rules of the folder of rules (rules/ below path of polix.yaml, ai/rules/ by default): how
// the code of the project must be written.
//
// Polix only reads the fields it needs (id, description, scope, deterministic, check); the rest of the rule
// format belongs to the skills that write and use the rules.
package rule

import (
	"context"
	"errors"
	"fmt"
	"slices"
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

// Load loads the rules of the folder of rules dir (relative to root, with slashes) of the repository at root. It is
// the loader of the use case LoadRule (internal/app/rule/load), which cmd/polix builds once and passes to every use
// case that reads the rules. It never fails: what stops the reading is data of its result.
type Load func(ctx context.Context, root, dir string) Loaded

// Loaded is what Load read from a folder of rules: the yaml files of the folder but RetiredFile, in the order of
// their names. Each use case decides what an absent folder, an invalid rule or a file that cannot be read means to
// it: it takes every rule with the problems of the invalid ones, or stops at Err, or stops at Strict.
type Loaded struct {
	Rules    []Rule   // every rule of the folder, invalid ones included, ordered by id, stably; none with Err
	Exists   bool     // whether the folder exists
	Problems []string // why each invalid rule is not valid, then each id repeated, in the words of polix config check
	Repeated []string // the problems of the ids repeated, retired ones included: the last ones of Problems
	Err      error    // why a file of the folder cannot be read or is not yaml, the first by name; nil when none
	Strict   error    // where a strict reading stops: an invalid rule, else a repeated id, of the files read, else Err
}

// RetiredFile holds retired rules: they keep their id but never apply.
const RetiredFile = "retired.yaml"

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
	case r.absoluteCheck():
		return fmt.Errorf("rule %s names the check %s by an absolute path, not relative to the root", r.ID, r.Check)
	}
	return nil
}

// absoluteCheck reports whether the check of r is an absolute path on any system: /x, \x or a drive letter (C:).
func (r Rule) absoluteCheck() bool {
	p := r.Check
	if strings.HasPrefix(p, "/") || strings.HasPrefix(p, `\`) {
		return true
	}
	return len(p) >= 2 && p[1] == ':' && ('a' <= p[0]|0x20 && p[0]|0x20 <= 'z')
}

// HasCheck reports whether a file decides the rule.
func (r Rule) HasCheck() bool { return r.Deterministic && r.Check != "" }

// Valid reports why l is not what a folder of rules loads: rules, problems or repeated ids with the error that stops
// the reading, that error without a strict reading that stops too, repeated ids that are not the last problems, or
// rules without the folder.
func (l Loaded) Valid() error {
	switch {
	case l.Err != nil && (len(l.Rules) > 0 || len(l.Problems) > 0 || len(l.Repeated) > 0):
		return fmt.Errorf("rules loaded from a folder that cannot be read: %w", l.Err)
	case l.Err != nil && l.Strict == nil:
		return fmt.Errorf("a strict reading of the rules goes past the error that stops them: %w", l.Err)
	case len(l.Repeated) > len(l.Problems) || !slices.Equal(l.Problems[len(l.Problems)-len(l.Repeated):], l.Repeated):
		return errors.New("repeated ids that are not the last problems of the rules")
	case !l.Exists && len(l.Rules) > 0:
		return errors.New("rules loaded without their folder")
	}
	return nil
}

// Fail returns l failed with err: no rules, problems nor repeated ids, and err stops every reading of the folder.
func (l Loaded) Fail(err error) Loaded {
	failed := l
	failed.Rules, failed.Problems, failed.Repeated, failed.Err, failed.Strict = nil, nil, nil, err, err
	return failed
}
