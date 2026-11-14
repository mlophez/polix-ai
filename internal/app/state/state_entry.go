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
	// AgentID is the id the agent gives the subagent that acted, empty for the main session and when the agent gives
	// none (Codex, an older Claude Code) or the line was written by an older polix: it tells two subagents of one type
	// apart (see Running).
	AgentID string
	Branch  string
	Hash    string // content hash; on decisions only when the call already computed it
	// Before is the content hash at the start of the event of a blocked line, the content the agent left, written
	// only when the runs of the event changed it before the block: a command that writes files the tree does not
	// ignore. The loop guard chains the blocks through it (see Chain), so a block still counts with the earlier ones
	// on a content the agent left alone.
	Before string
	// Head is the commit HEAD pointed to when the line was written, empty when unknown: written only on the observed
	// lines about a content that are read by branch (finished with a hash, passed and blocked), so another branch on
	// the same commit selects them too (see Query), and on the snapshot lines, so a post_command sees HEAD moved.
	Head string
	// Refs is what git moved without moving HEAD when a snapshot line was written, the commit of refs/stash and the
	// operation stopped halfway, empty when there was neither: written only on the snapshot lines, so a post_command
	// sees a git stash or a merge stopped on its conflicts, which rewrite the working tree without moving HEAD.
	Refs string
	Rule string // the rule id of the lines of kind injected an older polix wrote, shown by polix log
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
	// `finished` of the workflow names the type, without it when it only makes the injects of the subagent due again;
	// with the hash, it also carries the HEAD commit when it is known. Without the hash also when the workflow counts
	// the subagents still running (see Running).
	OutcomeFinished = "finished"
	// OutcomePassed: a run of polix passed on a content that did not change during it; Key is what it ran.
	OutcomePassed = "passed"
	// OutcomeBlocked: an entry with once blocked, or any entry blocked a stop, counted by the loop guard; also a
	// block of a stop the loop guard let through after runs of the stop changed the content, so the chain of its
	// blocks goes on (see Chain).
	OutcomeBlocked = "blocked"
	// OutcomeInjected: a once-only inject reached the AI; Key is its expanded text, file or rule id.
	OutcomeInjected = "injected"
	// OutcomeStarted: a session started or its context was compacted, so its once-only injects are due again for
	// every actor, Key the source the agent gives (startup, resume, clear, compact), empty when it gives none; or, on
	// subagent_start, a subagent started, its type the actor, so they are due again for that type and it runs until
	// it finishes (see Running).
	OutcomeStarted = "started"
	// OutcomeNotified: a notice of the verb notify reached the user, in an answer that did not block; Key is its
	// expanded text. It is never due again in the session, whatever the actor and the starts.
	OutcomeNotified = "notified"
	// OutcomeSnapshot: the content right before a command ran, which its post_command compares with the content
	// after it; with the hash and the HEAD commit when it is known. Key is the id of the tool call, or the command
	// line when the agent gives none.
	OutcomeSnapshot = "snapshot"
	// OutcomeWatched: a post_command found the snapshot of its key, so that snapshot is used and no later
	// post_command compares with it; Key is the key of the snapshot.
	OutcomeWatched = "watched"
	// OutcomeEnded: a subagent the log had open was missing from the subagents in flight the agent listed on a stop,
	// so it ended without its end reaching polix (an interrupted subagent); its type the actor, with its id, keyed by
	// nothing and without hash (see Running).
	OutcomeEnded = "ended"
)

// UnknownHash is the hash of a blocked entry written on a stop whose content hash could not be computed, so the loop
// guard still bounds the blocks of a broken git, counted together as one content. It names no tree: nothing
// compares it with one, and no other outcome is written with it.
const UnknownHash = "unknown"

// Outcomes lists the outcomes of an observed entry.
var Outcomes = []string{OutcomeFinished, OutcomePassed, OutcomeBlocked, OutcomeInjected, OutcomeStarted,
	OutcomeNotified, OutcomeSnapshot, OutcomeWatched, OutcomeEnded}

// hashed are the outcomes that are about a content, so their entries need its hash.
var hashed = []string{OutcomePassed, OutcomeBlocked, OutcomeSnapshot}

// Chained returns the entry carrying before, the content hash at the start of its event, when it is an observed
// blocked line on a known content and before is another known content: the runs of the event changed the content
// (see Before). Otherwise the entry as it is.
func (e Entry) Chained(before string) Entry {
	known := e.Hash != "" && e.Hash != UnknownHash && before != "" && before != UnknownHash
	if e.Kind == KindObserved && e.Outcome == OutcomeBlocked && known && before != e.Hash {
		e.Before = before
	}
	return e
}

// Valid tells why the entry is not valid to be appended: it has no kind or no session, which names the file it is
// written to, or a negative line, it carries the fact or
// the rule of the lines an older polix wrote, or it is observed without a known outcome, or without the hash its
// outcome is about, or it carries refs without being a snapshot. A kind this polix does not know is accepted,
// as a log may hold lines of a newer polix. The lines read from the log are kept whatever they hold (see Load).
func (e Entry) Valid() error {
	var errs []error
	if e.Kind == "" {
		errs = append(errs, errors.New("an entry needs a kind"))
	}
	if e.Session == "" {
		errs = append(errs, errors.New("an entry needs a session, which names the file it is written to"))
	}
	if e.Line < 0 {
		errs = append(errs, errors.New("the line of an entry must not be negative"))
	}
	if e.Fact != "" || e.Rule != "" {
		errs = append(errs, errors.New("the fact and the rule of the lines an older polix wrote are never written"))
	}
	errs = append(errs, e.observedProblem())
	if e.Refs != "" && (e.Kind != KindObserved || e.Outcome != OutcomeSnapshot) {
		errs = append(errs, errors.New("only a snapshot line carries the refs"))
	}
	if e.Before != "" && (e.Kind != KindObserved || e.Outcome != OutcomeBlocked) {
		errs = append(errs, errors.New("only a blocked line carries the hash its event started from"))
	}
	return errors.Join(errs...)
}

// observedProblem tells why an observed entry has no known outcome or lacks the hash its outcome is about, nil for
// an entry of another kind.
func (e Entry) observedProblem() error {
	switch {
	case e.Kind != KindObserved:
		return nil
	case !slices.Contains(Outcomes, e.Outcome):
		return fmt.Errorf("an observed entry needs an outcome of %v, got %q", Outcomes, e.Outcome)
	case slices.Contains(hashed, e.Outcome) && e.Hash == "":
		return fmt.Errorf("an observed entry %s needs the hash of the content", e.Outcome)
	}
	return nil
}
