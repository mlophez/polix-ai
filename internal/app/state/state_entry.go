package state

import (
	"errors"
	"fmt"
	"slices"
)

// Entry is one line of the log.
type Entry struct {
	At      string // RFC 3339, set by whoever appends it
	Kind    string
	Agent   string
	Session string
	Event   string
	Actor   string
	Branch  string
	Hash    string // content hash; on decisions only when the call already computed it
	Rule    string // the rule id of the lines of kind injected an older polix wrote, shown by polix log
	// Fact is the name of the lines of kind fact an older polix wrote, kept only to show them in polix log: read
	// from the log, never written (Valid rejects an entry that carries it).
	Fact    string
	Action  string // decision entries: the verb of the workflow entry, or builtin
	Line    int    // decision and observed entries: line of the entry in the workflow
	Key     string // observed entries: what was observed, such as the type of the subagent that finished
	Outcome string
	Reason  string
}

// Entry kinds.
const (
	KindObserved = "observed" // something polix observed by itself, with its outcome
	KindDecision = "decision" // what polix answered to an event
)

// Outcomes of an observed entry.
const (
	// OutcomeFinished: a subagent ended and its answer did not refuse; Key is its type. With the hash when a
	// `finished` of the workflow names the type, without it when it only makes the injects of the subagent due again.
	OutcomeFinished = "finished"
	// OutcomePassed: a run of polix passed on a content that did not change during it; Key is what it ran.
	OutcomePassed = "passed"
	// OutcomeBlocked: an entry with once blocked, or any entry blocked a stop, counted by the loop guard.
	OutcomeBlocked = "blocked"
	// OutcomeInjected: a once-only inject reached the AI; Key is its expanded text, file or rule id.
	OutcomeInjected = "injected"
	// OutcomeStarted: a session started or its context was compacted, so its once-only injects are due again.
	OutcomeStarted = "started"
)

// UnknownHash is the hash of a blocked entry written on a stop whose content hash could not be computed, so the loop
// guard still bounds the blocks of a broken git, counted together as one content. It names no tree: nothing
// compares it with one, and no other outcome is written with it.
const UnknownHash = "unknown"

// Outcomes lists the outcomes of an observed entry.
var Outcomes = []string{OutcomeFinished, OutcomePassed, OutcomeBlocked, OutcomeInjected, OutcomeStarted}

// hashed are the outcomes that are about a content, so their entries need its hash.
var hashed = []string{OutcomePassed, OutcomeBlocked}

// Valid tells why the entry is not valid to be appended: it has no kind or a negative line, it carries the fact or
// the rule of the lines an older polix wrote, or it is observed without a known outcome, or without the hash its
// outcome is about. A kind this polix does not know is accepted,
// as a log may hold lines of a newer polix. The lines read from the log are kept whatever they hold (see Load).
func (e Entry) Valid() error {
	var errs []error
	if e.Kind == "" {
		errs = append(errs, errors.New("an entry needs a kind"))
	}
	if e.Line < 0 {
		errs = append(errs, errors.New("the line of an entry must not be negative"))
	}
	if e.Fact != "" || e.Rule != "" {
		errs = append(errs, errors.New("the fact and the rule of the lines an older polix wrote are never written"))
	}
	if e.Kind == KindObserved {
		switch {
		case !slices.Contains(Outcomes, e.Outcome):
			errs = append(errs, fmt.Errorf("an observed entry needs an outcome of %v, got %q", Outcomes, e.Outcome))
		case slices.Contains(hashed, e.Outcome) && e.Hash == "":
			errs = append(errs, fmt.Errorf("an observed entry %s needs the hash of the content", e.Outcome))
		}
	}
	return errors.Join(errs...)
}
