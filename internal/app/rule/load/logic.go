package load

import (
	"errors"
	"fmt"
	"sort"

	"github.com/mlophez/polix-ai/internal/app/rule"
)

// Output is the decision of LoadRule: what the folder of rules loads.
type Output struct {
	Loaded rule.Loaded
}

// Decide validates the rules read from a folder of rules, in the order of its files, with the domain, and finds the
// ids repeated among them or taken by the retired rules (rule.Rules.Repeated). Without err, it keeps every rule,
// invalid ones included, ordered by id, stably, with the problem of each invalid one in the words of polix config
// check and then those of the ids repeated; with err, a file that cannot be read or is not yaml, it keeps none.
// Either way it tells where a reading that admits no invalid rule stops: at the first invalid rule, with its file,
// else at the first id repeated, else at err.
func Decide(rules, retired []rule.Rule, exists bool, err error) Output {
	repeated := rule.Rules(rules).Repeated(retired)
	strict := firstInvalid(rules)
	if strict == nil && len(repeated) > 0 {
		strict = errors.New(repeated[0])
	}
	if strict == nil {
		strict = err
	}
	if err != nil {
		return failed(exists, err, strict)
	}
	return read(rules, repeated, exists, strict)
}

// read is what a folder whose files are all read loads.
func read(rules []rule.Rule, repeated []string, exists bool, strict error) Output {
	var problems []string
	for _, r := range rules {
		if r.Valid() != nil {
			problems = append(problems, problem(r))
		}
	}
	problems = append(problems, repeated...)
	all := append([]rule.Rule(nil), rules...)
	sort.SliceStable(all, func(i, j int) bool { return all[i].ID < all[j].ID })
	loaded := rule.Loaded{Rules: all, Exists: exists, Problems: problems, Repeated: repeated, Strict: strict}
	if err := loaded.Valid(); err != nil {
		return Output{Loaded: loaded.Fail(err)}
	}
	return Output{Loaded: loaded}
}

// failed is what a folder with a file that cannot be read or is not yaml loads.
func failed(exists bool, err, strict error) Output {
	loaded := rule.Loaded{Exists: exists, Err: err, Strict: strict}
	if verr := loaded.Valid(); verr != nil {
		return Output{Loaded: loaded.Fail(verr)}
	}
	return Output{Loaded: loaded}
}

// firstInvalid is why the first invalid rule of rules is not valid, with its file; nil when every rule is valid.
func firstInvalid(rules []rule.Rule) error {
	for _, r := range rules {
		if err := r.Valid(); err != nil {
			return fmt.Errorf("%s: %w", r.File, err)
		}
	}
	return nil
}

// problem is why an invalid rule is not valid, in the words of polix config check: the file, the id and what is wrong.
func problem(r rule.Rule) string {
	switch {
	case r.ID == "":
		return r.File + ": a rule has no id"
	case r.Description == "":
		return fmt.Sprintf("%s %s: no description", r.File, r.ID)
	case r.Check != "" && !r.Deterministic:
		return fmt.Sprintf("%s %s: check %s needs deterministic: true", r.File, r.ID, r.Check)
	}
	return fmt.Sprintf("%s %s: check %s is an absolute path, not relative to the root", r.File, r.ID, r.Check)
}
