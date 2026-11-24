package state

import (
	"errors"
	"fmt"
	"slices"
	"strings"
)

// Record is one line of the log: the event line the hook writes for one event of the agent, with the data of the event
// (Input), what polix did with it (Actions) and what it noted by itself on it (Observations), or a legacy line an
// earlier polix wrote, which Legacy names. An event line is written once the event is evaluated, and only when
// something acted or was observed on it.
type Record struct {
	At      string // RFC 3339, the time the evaluation of the event started; set by whoever appends it
	Agent   string
	Session string
	Event   string
	Actor   string
	// AgentID is the id the agent gives the subagent that acted, empty for the main session and when the agent gives
	// none (Codex, an older Claude Code): it tells two subagents of one type apart (see Running).
	AgentID string
	Branch  string
	// Head is the commit HEAD pointed to when the call learned it, empty when unknown. The queries by branch or by
	// HEAD read it (see Query): another branch on the same commit selects the observations of a record too, and a
	// query by HEAD alone (Query.ByHead) selects the finished ones on any branch; a post_command sees HEAD moved
	// through the one of its snapshot.
	Head string
	// Hash is the content hash at the start of the event, when the call computed it. Each observation about a content
	// carries the hash it was observed on.
	Hash         string
	Input        Input
	Actions      []Action
	Observations []Observation
	// Legacy is the kind of a legacy line (decision, observed, fact, injected, stop_block, or a kind this polix does
	// not know), empty on an event line. A legacy record holds the one action or observation of its line, and the
	// Rule or the Fact of a line of an old kind; it is read to be shown, never written, and every query ignores it.
	Legacy string
	Rule   string // legacy: the rule id of the lines of kind injected
	Fact   string // legacy: the name of the lines of kind fact
	// Raw is the line as the loader read it, which polix log --json prints as it is; the hook leaves it empty.
	Raw string
}

// Input is the data of the event a record is about that applies to it: the command and the id of the tool call of a
// command or a commit, the path of an edit, the skill and its arguments, the prompt, and the source of a start.
// Whoever encodes it cuts each value; the record holds them whole.
type Input struct {
	Command string
	ToolUse string
	Path    string
	Skill   string
	Args    string
	Prompt  string
	Source  string
}

// Action is what an entry of polix.yaml, or a protection built into polix, did with an event, with its result.
type Action struct {
	Line int // the line of the entry in polix.yaml, 0 for a builtin protection
	// Action is the verb of the entry or the protection (block, run, inject, notify, builtin...).
	Action  string
	Outcome string
	// Key is what the action acted on, empty when it acted on nothing in particular.
	Key    string
	Reason string
	// DurationMS is the milliseconds a run took, truncated; nil for an action that ran nothing, so a run that took
	// less than a millisecond still carries 0.
	DurationMS *int64
}

// Observation is a fact polix notes by itself on an event, which the memory of the session reads later.
type Observation struct {
	Line    int // the line of the entry in polix.yaml, 0 when it has none
	Outcome string
	Key     string // what was observed, such as the type of the subagent that finished
	Hash    string // the content hash it was observed on; for the outcomes that are about a content
	// Refs is what git moved without moving HEAD when a snapshot was noted, the commit of refs/stash and the
	// operation stopped halfway, empty when there was neither: only on a snapshot, so a post_command sees a git
	// stash or a merge stopped on its conflicts, which rewrite the working tree without moving HEAD.
	Refs string
	// Before is the content hash at the start of the event of a blocked observation, the content the agent left,
	// written only when the runs of the event changed it before the block: a command that writes files the tree does
	// not ignore. The loop guard chains the blocks through it (see Chain), so a block still counts with the earlier
	// ones on a content the agent left alone.
	Before string
}

// Match is an observation a query selected, with the record it is in. Seq is its position among the observations of
// the records that are not legacy, in file order: the observations of one record come in the order they were noted.
type Match struct {
	Observation
	Record Record
	Seq    int
}

// Outcomes of an observation.
const (
	// OutcomeFinished: a subagent ended and its answer did not refuse; Key is its type. With the hash when a
	// `finished` of the workflow names the type, without it when it only makes the injects of the subagent due again;
	// with the hash, the HEAD commit of its record is known too. Without the hash also when the workflow counts
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
	// it finishes (see Running); or, on skill, a skill started and its answer did not block, Key the skill name as
	// it came, without hash. Every reader filters by the event of the record.
	OutcomeStarted = "started"
	// OutcomeNotified: a notice of the verb notify reached the user, in an answer that did not block; Key is its
	// expanded text. It is never due again in the session, whatever the actor and the starts.
	OutcomeNotified = "notified"
	// OutcomeSnapshot: the content right before a command ran, which its post_command compares with the content
	// after it; with the hash, and the HEAD commit in its record when it is known. Key is the id of the tool call,
	// or the command line when the agent gives none.
	OutcomeSnapshot = "snapshot"
	// OutcomeWatched: a post_command found the snapshot of its key, so that snapshot is used and no later
	// post_command compares with it; Key is the key of the snapshot.
	OutcomeWatched = "watched"
	// OutcomeEnded: a subagent the log had open was missing from the subagents in flight the agent listed on a stop,
	// so it ended without its end reaching polix (an interrupted subagent); Key is its type and, when it has one, a
	// space and its id (see Match.EndedKey), without hash (see Running).
	OutcomeEnded = "ended"
)

// UnknownHash is the hash of a blocked observation written on a stop whose content hash could not be computed, so the
// loop guard still bounds the blocks of a broken git, counted together as one content. It names no tree: nothing
// compares it with one, and no other outcome is written with it.
const UnknownHash = "unknown"

// Outcomes lists the outcomes of an observation.
var Outcomes = []string{OutcomeFinished, OutcomePassed, OutcomeBlocked, OutcomeInjected, OutcomeStarted,
	OutcomeNotified, OutcomeSnapshot, OutcomeWatched, OutcomeEnded}

// Kinds of legacy lines the loader gives a meaning; any other kind is kept as read, its fields on its one action.
const (
	LegacyDecision = "decision" // what polix answered to an event: one action
	LegacyObserved = "observed" // something polix observed by itself: one observation
)

// hashed are the outcomes that are about a content, so their observations need its hash.
var hashed = []string{OutcomePassed, OutcomeBlocked, OutcomeSnapshot}

// EndedKey returns the key of the ended observation of the subagent whose started observation m is: its type, and a
// space and its agent id when it has one. The type of a subagent has no space, so Ended tells them apart.
func (m Match) EndedKey() string {
	if m.Record.AgentID == "" {
		return m.Record.Actor
	}
	return m.Record.Actor + " " + m.Record.AgentID
}

// Ended returns the type and the agent id of the subagent the key of the ended observation m names (see EndedKey).
func (m Match) Ended() (kind, id string) {
	kind, id, _ = strings.Cut(m.Key, " ")
	return kind, id
}

// Chained returns the observation carrying before, the content hash at the start of its event, when it is a blocked
// observation on a known content and before is another known content: the runs of the event changed the content
// (see Observation.Before). Otherwise the observation as it is.
func (o Observation) Chained(before string) Observation {
	known := o.Hash != "" && o.Hash != UnknownHash && before != "" && before != UnknownHash
	if o.Outcome == OutcomeBlocked && known && before != o.Hash {
		o.Before = before
	}
	return o
}

// Valid tells why the observation is not valid to be recorded: a negative line, an outcome this polix does not know,
// the hash its outcome is about missing, refs without being a snapshot, or the hash its event started from without
// being a blocked one.
func (o Observation) Valid() error {
	var errs []error
	if o.Line < 0 {
		errs = append(errs, errors.New("the line of an observation must not be negative"))
	}
	switch {
	case !slices.Contains(Outcomes, o.Outcome):
		errs = append(errs, fmt.Errorf("an observation needs an outcome of %v, got %q", Outcomes, o.Outcome))
	case slices.Contains(hashed, o.Outcome) && o.Hash == "":
		errs = append(errs, fmt.Errorf("an observation %s needs the hash of the content", o.Outcome))
	}
	if o.Refs != "" && o.Outcome != OutcomeSnapshot {
		errs = append(errs, errors.New("only a snapshot carries the refs"))
	}
	if o.Before != "" && o.Outcome != OutcomeBlocked {
		errs = append(errs, errors.New("only a blocked observation carries the hash its event started from"))
	}
	return errors.Join(errs...)
}

// Valid tells why the action is not valid to be recorded: it names no action or no outcome, or a negative line or
// duration.
func (a Action) Valid() error {
	var errs []error
	if a.Action == "" {
		errs = append(errs, errors.New("an action needs its verb or its protection"))
	}
	if a.Outcome == "" {
		errs = append(errs, errors.New("an action needs an outcome"))
	}
	if a.Line < 0 {
		errs = append(errs, errors.New("the line of an action must not be negative"))
	}
	if a.DurationMS != nil && *a.DurationMS < 0 {
		errs = append(errs, errors.New("the duration of an action must not be negative"))
	}
	return errors.Join(errs...)
}

// Valid tells why the input is not valid: it never is not, any text may come from the agent.
func (Input) Valid() error { return nil }

// Valid tells why the match is not a match: its position is negative.
func (m Match) Valid() error {
	if m.Seq < 0 {
		return fmt.Errorf("the position of a match must not be negative, got %d", m.Seq)
	}
	return nil
}

// Valid tells why the record is not valid: it has no session, which names the file it is written to, or it is an
// event line with neither actions nor observations, which is no line of the log. A legacy line is as it was written,
// and a record read from the log is kept so even when its parts are not what this polix writes (see Writable), since
// a log may hold lines of a newer polix.
func (r Record) Valid() error {
	var errs []error
	if r.Session == "" {
		errs = append(errs, errors.New("a record needs a session, which names the file it is written to"))
	}
	if r.Legacy == "" && !r.Acts() {
		errs = append(errs, errors.New("a record needs an action or an observation"))
	}
	return errors.Join(errs...)
}

// Writable tells why the record is not to be appended to the log: it is not valid, it is a legacy line, which is
// only read, or one of its actions or observations is not what this polix writes.
func (r Record) Writable() error {
	errs := []error{r.Valid(), r.Input.Valid()}
	if r.Legacy != "" || r.Rule != "" || r.Fact != "" {
		errs = append(errs, errors.New("a legacy line is read, never written"))
	}
	for _, a := range r.Actions {
		errs = append(errs, a.Valid())
	}
	for _, o := range r.Observations {
		errs = append(errs, o.Valid())
	}
	return errors.Join(errs...)
}

// Acts reports whether something acted or was observed on the event of the record: it has an action or an
// observation.
func (r Record) Acts() bool { return len(r.Actions) > 0 || len(r.Observations) > 0 }

// Unnotified returns the record without its notified observations, the notices of notify that a blocked call never
// shows to the user. It never changes r: the copy shares no list with it.
func (r Record) Unnotified() Record {
	r.Actions = slices.Clone(r.Actions)
	r.Observations = slices.DeleteFunc(slices.Clone(r.Observations), func(o Observation) bool {
		return o.Outcome == OutcomeNotified
	})
	return r
}

// Stamped returns the record with at as its time when it has none; one that has a time keeps it.
func (r Record) Stamped(at string) Record {
	if r.At == "" {
		r.At = at
	}
	return r
}
