package answer

import (
	"fmt"
	"slices"

	"github.com/mlophez/polix-ai/internal/app/diff"
	"github.com/mlophez/polix-ai/internal/app/hook"
	"github.com/mlophez/polix-ai/internal/app/rule"
	"github.com/mlophez/polix-ai/internal/app/state"
	"github.com/mlophez/polix-ai/internal/platform/glob"
)

// waiting reports whether the pass waits for a step.
func (r *pass) waiting() bool { return r.step != nil }

// ask makes s the step the pass waits for, unless it already waits for one or s is not valid.
func (r *pass) ask(s hook.Step, valid error) {
	if r.step == nil && valid == nil && !s.None() {
		r.step = &s
	}
}

// act returns the outcome of a step that acts, from the journal; when the journal has no more, the pass waits
// for s.
func (r *pass) act(s hook.Step, valid error) hook.Outcome {
	if r.waiting() {
		return hook.Outcome{}
	}
	if r.next < len(r.ev.Journal) {
		o := r.ev.Journal[r.next]
		r.next++
		return o
	}
	r.ask(s, valid)
	return hook.Outcome{}
}

// branch returns the current branch.
func (r *pass) branch() string {
	if r.ev.Facts.HasBranch {
		return r.ev.Facts.Branch
	}
	s := hook.Step{Branch: true}
	r.ask(s, s.Valid())
	return ""
}

// hash returns the content hash of this epoch and whether it is known.
func (r *pass) hash() (string, bool) {
	if h, known, computed := r.ev.Facts.Hash(r.epoch); computed {
		return h, known
	}
	s := hook.Step{Hash: true, Epoch: r.epoch}
	r.ask(s, s.Valid())
	return "", false
}

// changed returns the files not committed yet.
func (r *pass) changed() []string {
	if r.ev.Facts.HasChanged {
		return r.ev.Facts.Changed
	}
	s := hook.Step{Changed: true}
	r.ask(s, s.Valid())
	return nil
}

// diff returns the size of the change of each file not committed yet, and whether it could be measured.
func (r *pass) diff() (diff.Diff, bool) {
	if r.ev.Facts.HasDiff {
		return r.ev.Facts.Diff, r.ev.Facts.DiffErr == nil
	}
	s := hook.Step{Diff: true}
	r.ask(s, s.Valid())
	return diff.Diff{}, false
}

// files returns the files of the working tree git does not ignore.
func (r *pass) files() []string {
	if r.ev.Facts.HasFiles {
		return r.ev.Facts.Files
	}
	s := hook.Step{Files: true}
	r.ask(s, s.Valid())
	return nil
}

// rules returns the rules of the repository, or why they cannot be loaded.
func (r *pass) rules() ([]rule.Rule, error) {
	if r.ev.Facts.HasRules {
		return r.ev.Facts.Rules, r.ev.Facts.RulesErr
	}
	s := hook.Step{Rules: true}
	r.ask(s, s.Valid())
	return nil, nil
}

// currentState returns the log as this pass left it, or why it cannot be read (then it is empty).
func (r *pass) currentState() (state.State, error) {
	if r.ev.Facts.HasState {
		return r.state, r.ev.Facts.StateErr
	}
	s := hook.Step{State: true}
	r.ask(s, s.Valid())
	return state.State{}, nil
}

// warnLog warns the user, once per pass, that the log cannot be read: the verbs that repeat by it act every time, and
// finished never holds.
func (r *pass) warnLog(err error) {
	w := fmt.Sprintf("polix cannot use its log (%v): run, inject and the blocks with once act every time, and finished "+
		"does not hold", err)
	if !slices.Contains(r.warnings, w) {
		r.warnings = append(r.warnings, w)
	}
}

// ignored reports whether git ignores the path.
func (r *pass) ignored(p string) bool {
	if ignored, ok := r.ev.Facts.Ignored[p]; ok {
		return ignored
	}
	s := hook.Step{Ignored: p}
	r.ask(s, s.Valid())
	return false
}

// reals returns the real paths of the paths, asking for all those not known yet at once.
func (r *pass) reals(paths ...string) map[string]string {
	var missing []string
	for _, p := range paths {
		if _, ok := r.ev.Facts.Reals[p]; !ok && !slices.Contains(missing, p) {
			missing = append(missing, p)
		}
	}
	if len(missing) > 0 {
		s := hook.Step{Real: missing}
		r.ask(s, s.Valid())
		return nil
	}
	return r.ev.Facts.Reals
}

// foreign reports whether the absolute folder dir is inside an existing git repository other than this one.
func (r *pass) foreign(dir string) bool {
	if foreign, ok := r.ev.Facts.Foreign[dir]; ok {
		return foreign
	}
	s := hook.Step{Foreign: dir}
	r.ask(s, s.Valid())
	return false
}

// contains reports whether a file matching the patterns holds want, at this epoch.
func (r *pass) contains(patterns []string, want string) bool {
	if r.waiting() {
		return false
	}
	if found, known := r.ev.Facts.Found(r.epoch, patterns, want); known {
		return found
	}
	s := hook.Step{Contains: patterns, Text: want, Epoch: r.epoch}
	r.ask(s, s.Valid())
	return false
}

// read returns the content of the file of this epoch and whether it can be read.
func (r *pass) read(p string) (string, bool) {
	if r.waiting() || p == "" {
		return "", false // an empty path names no file: it cannot be read
	}
	if data, readable, read := r.ev.Facts.Content(r.epoch, p); read {
		return data, readable
	}
	s := hook.Step{Read: p, Epoch: r.epoch}
	r.ask(s, s.Valid())
	return "", false
}

// sameScope reports whether the files in scope are the same in the trees from and to: the trees are equal, or no
// path that differs between them matches the globs of scope; with no scope, any differing path counts. It asks for
// the comparison it lacks, and returns why the trees could not be compared (a tree git no longer has, or git
// failing): they then count as different.
func (r *pass) sameScope(from, to string, scope []string) (bool, error) {
	if from == to {
		return true, nil
	}
	paths, compared, err := r.ev.Facts.TreeDiff(from, to)
	if !compared {
		s := hook.Step{Compare: from, Tree: to}
		r.ask(s, s.Valid())
		return false, nil
	}
	if err != nil {
		return false, err
	}
	for _, p := range paths {
		if len(scope) == 0 || glob.AnyPath(scope, p) {
			return false, nil
		}
	}
	return true, nil
}
