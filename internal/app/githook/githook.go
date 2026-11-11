// Package githook is the domain of a call of git to polix: one event of githooks of polix.yaml, the gates git runs
// on every commit, whoever makes it, apart from the events of the AI. A call knows the branch, the files staged for
// the commit and the outcomes of the runs of its entries carried out so far, so the evaluation, in the use case
// AnswerGithook (internal/app/githook/answer), takes the step shape: an entry after a run knows whether it passed.
//
//	githooks:
//	  pre_commit:
//	    - when: { branch: "^main$" }
//	      action: { block: "No se commitea en main" }
package githook

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/mlophez/polix-ai/internal/app/workflow"
)

// Githook is one call of git: the event of githooks it runs (workflow.PreCommit or workflow.CommitMsg), the facts
// of the repository the entries of the event read, and the outcomes of the runs carried out so far.
type Githook struct {
	Event  string   // the event of githooks, one of workflow.GithookEvents
	Branch string   // the current branch, "" on a detached HEAD
	Staged []string // the files staged for the commit, relative to the root with slashes
	// Message is the message of the commit as git wrote it to the file of commit-msg, comments included; only on
	// workflow.CommitMsg. The logic takes its first line, without the lines that start with Comment.
	Message string
	Comment string // the string that starts a comment line of the message (# by default); only on workflow.CommitMsg
	// Outcomes are the outcomes of the runs of the entries of the event carried out so far, in order.
	Outcomes []Outcome
}

// Outcome is the outcome of one run of a githooks entry: why it failed, nil when it passed. Its output went live to
// the terminal of the commit, so the outcome does not hold it.
type Outcome struct {
	Err error
}

// Valid tells why the call cannot be answered: an event that is not one of githooks, an empty staged file, or a
// message or a comment string on an event other than workflow.CommitMsg.
func (g Githook) Valid() error {
	switch {
	case !slices.Contains(workflow.GithookEvents, g.Event):
		return fmt.Errorf("unknown githooks event %q (githooks events: %s)", g.Event,
			strings.Join(workflow.GithookEvents, ", "))
	case slices.Contains(g.Staged, ""):
		return errors.New("a staged file needs its path")
	case g.Event != workflow.CommitMsg && (g.Message != "" || g.Comment != ""):
		return fmt.Errorf("a message only comes with %s", workflow.CommitMsg)
	}
	return nil
}

// Ran returns the call knowing the outcome of one more run, which failed with err, nil when it passed.
func (g Githook) Ran(err error) Githook {
	o := Outcome{Err: err}
	if o.Valid() != nil {
		return g
	}
	g.Outcomes = append(slices.Clone(g.Outcomes), o)
	return g
}

// Valid tells why the outcome is not one: it never is, a run passes or fails with any error.
func (o Outcome) Valid() error { return nil }

// Passed reports whether the run passed.
func (o Outcome) Passed() bool { return o.Err == nil }
