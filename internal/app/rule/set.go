package rule

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/mlophez/polix-ai/internal/platform/glob"
)

// Rules are several rules, such as those of a folder of rules, in their order. It is domain, not of a use case:
// CheckRule, AnswerHook and the setup use cases ask it.
type Rules []Rule

// Selection is which rules a check is asked for: all of them, or only those whose scope matches the changed files.
// Run says whether the checks run at all; Narrow ignores it, so the caller asks it first.
// It is built with a literal followed by Valid.
type Selection struct {
	Run         bool     // the checks run at all; without it the rules are only validated
	OnlyChanged bool     // only the rules whose scope matches one of Changed
	Changed     []string // the changed files, read only with OnlyChanged
}

// Valid tells why s is not a selection: changed files without OnlyChanged select nothing.
// The changed rules need Run, and changed files need OnlyChanged.
func (s Selection) Valid() error {
	if s.OnlyChanged && !s.Run {
		return errors.New("a selection of the changed rules needs Run")
	}
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

// Repeated returns one problem for each rule whose id an earlier rule already has, naming its file and that of the
// last earlier one, so an id names one rule only. An id belongs only to the rules that exist: a deleted rule frees it.
func (rs Rules) Repeated() []string {
	var problems []string
	seen := map[string]string{}
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

// Loaded validates the rules rs read from a folder of rules, in the order of its files, and finds the ids repeated
// among them (Repeated). retired is the path, relative to the root with slashes, of the RetiredFile the folder holds,
// "" without it: its presence is a problem, never read as rules, that comes first among the problems and where a strict
// reading stops. Without err, it keeps every rule, invalid ones included, ordered by id, stably, with the problem of
// each invalid one in the words of polix config check and then those of the ids repeated; with err, a file that
// cannot be read or is not yaml, it keeps none. Either way it tells where a reading that admits no invalid rule stops:
// at retired, else at the first invalid rule, with its file, else at the first id repeated, else at err. exists tells
// whether the folder exists.
func (rs Rules) Loaded(retired string, files map[string]Source, exists bool, err error) Loaded {
	repeated := rs.Repeated()
	strict := rs.firstInvalid()
	if strict == nil && len(repeated) > 0 {
		strict = errors.New(repeated[0])
	}
	if strict == nil {
		strict = err
	}
	var leftover string
	if retired != "" {
		leftover = retired + " is no longer read: delete it, its ids are free"
		strict = errors.New(leftover)
	}
	if err != nil {
		loaded := Loaded{Exists: exists, Files: files, Err: err, Strict: strict}
		if verr := loaded.Valid(); verr != nil {
			return loaded.Fail(verr)
		}
		return loaded
	}
	all, problems := rs.read(leftover, repeated)
	loaded := Loaded{Rules: all, Exists: exists, Files: files, Problems: problems, Repeated: repeated, Strict: strict}
	if verr := loaded.Valid(); verr != nil {
		return loaded.Fail(verr)
	}
	return loaded
}

// read is what a folder whose files are all read loads: every rule, ordered by id, stably, and the problems of the
// invalid ones and then the repeated ids, after the problem of a leftover retired file ("" without it).
func (rs Rules) read(leftover string, repeated []string) ([]Rule, []string) {
	var problems []string
	if leftover != "" {
		problems = append(problems, leftover)
	}
	for _, r := range rs {
		if r.Valid() != nil {
			problems = append(problems, r.problem())
		}
	}
	problems = append(problems, repeated...)
	all := append([]Rule(nil), rs...)
	sort.SliceStable(all, func(i, j int) bool { return all[i].ID < all[j].ID })
	return all, problems
}

// firstInvalid is why the first invalid rule of rs is not valid, with its file; nil when every rule is valid.
func (rs Rules) firstInvalid() error {
	for _, r := range rs {
		if err := r.Valid(); err != nil {
			return fmt.Errorf("%s: %w", r.File, err)
		}
	}
	return nil
}

// problem is why an invalid rule is not valid, in the words of polix config check: the file, the id and what is wrong.
func (r Rule) problem() string {
	switch {
	case r.ID == "":
		return r.File + ": a rule has no id"
	case r.Description == "":
		return fmt.Sprintf("%s %s: no description", r.File, r.ID)
	case r.Kind == "":
		return fmt.Sprintf("%s %s: no kind", r.File, r.ID)
	case !r.Kind.Known():
		return fmt.Sprintf("%s %s: kind %s is not one of %s", r.File, r.ID, r.Kind, r.Kind.list())
	case strings.TrimSpace(r.Subject) == "":
		return fmt.Sprintf("%s %s: no subject", r.File, r.ID)
	case len(r.Scope) == 0:
		return fmt.Sprintf("%s %s: no scope", r.File, r.ID)
	case r.DeterministicMissing:
		return fmt.Sprintf("%s %s: no deterministic", r.File, r.ID)
	case r.Deterministic && r.Check == "":
		return fmt.Sprintf("%s %s: deterministic: true needs a check", r.File, r.ID)
	case r.Check != "" && !r.Deterministic:
		return fmt.Sprintf("%s %s: check %s needs deterministic: true", r.File, r.ID, r.Check)
	}
	return fmt.Sprintf("%s %s: check %s is an absolute path, not relative to the root", r.File, r.ID, r.Check)
}
