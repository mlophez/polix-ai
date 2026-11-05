package hook

import (
	"errors"

	"github.com/mlophez/polix-ai/internal/rule"
	"github.com/mlophez/polix-ai/internal/state"
)

// Step is what the evaluation of an event needs from the outside before it can go on: read one fact of the
// repository, run a command or the check of a rule, or append an entry to the log. The zero Step is none: the
// answer is ready. Exactly one of its fields asks; Epoch goes with Hash and Read.
type Step struct {
	Branch   bool     // the current branch
	Hash     bool     // the content hash, at Epoch
	Epoch    int      // the runs carried out before the fact of Hash or Read, which a run may change
	Changed  bool     // the files not committed yet
	Files    bool     // the files of the working tree git does not ignore
	Rules    bool     // the rules of ai/rules/
	State    bool     // the log of polix
	Ignored  string   // whether git ignores this path, relative to the root
	Real     []string // the real paths of these absolute paths, symlinks resolved
	Contains []string // whether a file matching these patterns holds Text, at Epoch
	Text     string
	Read     string       // the content of this file, relative to the root with slashes, at Epoch
	Run      []string     // run this command from the root
	Check    *rule.Run    // run the check of this rule from the root
	Append   *state.Entry // append this entry to the log
}

// Outcome is the result of a step that acts (Run, Check or Append): what it printed and why it failed, nil when
// it did not.
type Outcome struct {
	Output string
	Err    error
}

// Valid tells why the step is not one step: it asks for several things at once, or a negative epoch.
func (s Step) Valid() error {
	n := 0
	for _, asks := range []bool{s.Branch, s.Hash, s.Changed, s.Files, s.Rules, s.State, s.Ignored != "",
		s.Real != nil, s.Contains != nil, s.Read != "", s.Run != nil, s.Check != nil, s.Append != nil} {
		if asks {
			n++
		}
	}
	switch {
	case n > 1:
		return errors.New("a step asks for one thing at a time")
	case s.Epoch < 0:
		return errors.New("the epoch of a step cannot be negative")
	case s.Contains != nil && s.Text == "":
		return errors.New("a search needs its text")
	case s.Run != nil && len(s.Run) == 0:
		return errors.New("a run needs a command")
	}
	return nil
}

// None reports whether the step asks for nothing: the answer is ready.
func (s Step) None() bool {
	return !s.Branch && !s.Hash && !s.Changed && !s.Files && !s.Rules && !s.State && s.Ignored == "" &&
		s.Real == nil && s.Contains == nil && s.Read == "" && s.Run == nil && s.Check == nil && s.Append == nil
}

// Valid tells why the outcome is not one: it never is, any output and error may come from a step.
func (o Outcome) Valid() error { return nil }
