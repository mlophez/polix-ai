package answer

import (
	"github.com/mlophez/polix-ai/internal/app/hook"
	"github.com/mlophez/polix-ai/internal/app/state"
)

// log records a decision on the event.
func (r *pass) log(action string, line int, outcome, reason string) {
	_ = r.write(state.KindDecision, action, line, outcome, reason, "", "")
}

// write appends to the log an entry of kind about the event (see writeEntry) and returns the error of appending it.
func (r *pass) write(kind, action string, line int, outcome, reason, key, hash string) error {
	return r.writeEntry(kind, action, line, outcome, reason, key, hash, "")
}

// writeEntry appends to the log an entry of kind about the event (see entryOf) and returns the error of appending
// it.
func (r *pass) writeEntry(kind, action string, line int, outcome, reason, key, hash, refs string) error {
	e, valid := r.entryOf(kind, action, line, outcome, reason, key, hash, refs)
	if r.waiting() {
		return nil
	}
	return r.appendEntry(e, valid)
}

// entryOf builds the entry of kind about the event that writeEntry appends, and the error of an entry that is not
// valid to append; the branch it carries may be a step the pass waits for. The content hash costs a git call over
// the whole tree, so a decision only carries the one this call already computed; an observed entry carries the hash
// it is given, none for those that are not about a content (injected, started, notified, watched and a finished that
// only makes injects due again). The observed entries about a content that are read by branch (finished with a
// hash, passed and blocked) also carry the HEAD commit, so another branch on the same commit reads them too, and so
// does a snapshot, so its post_command sees HEAD moved; every other entry goes without it. Only a snapshot carries
// refs, what git moves without moving HEAD, so its post_command sees a git stash or a merge stopped on its conflicts.
// Every entry carries the id of the subagent of the event, when the agent gives one.
func (r *pass) entryOf(kind, action string, line int, outcome, reason, key, hash, refs string) (state.Entry, error) {
	ev := r.ev.Event
	branch, head := r.branch(), r.head()
	if hash == "" && kind == state.KindDecision {
		hash, _, _ = r.ev.Facts.Hash(r.epoch)
	}
	if !readByHead(kind, outcome, hash) {
		head = ""
	}
	e := state.Entry{Kind: kind, Agent: ev.Agent, Session: ev.Session, Event: ev.Name, Actor: ev.Actor,
		AgentID: ev.AgentID, Branch: branch, Hash: hash, Head: head, Refs: refs, Action: action, Line: line, Key: key, Outcome: outcome,
		Reason: reason}
	return e, e.Valid()
}

// appendEntry appends e to the log and returns the error of appending it. Once the log is read, an entry
// appended afterwards is added to it as well, so a later entry of the call sees it.
func (r *pass) appendEntry(e state.Entry, valid error) error {
	if valid != nil {
		return valid
	}
	s := hook.Step{Append: &e}
	o := r.act(s, s.Valid())
	if r.waiting() {
		return nil
	}
	f := r.ev.Facts
	if o.Err == nil && f.HasState && f.StateErr == nil && r.next > f.StateAt {
		r.state, _ = r.state.Append(e)
	}
	return o.Err
}

// readByHead reports whether an entry of kind and outcome with hash is an observed line about a content that is read
// by branch or by HEAD commit (finished with a hash, passed and blocked), or a snapshot, whose post_command compares
// HEAD (see watch), so it carries the HEAD commit.
func readByHead(kind, outcome, hash string) bool {
	if kind != state.KindObserved || hash == "" {
		return false
	}
	switch outcome {
	case state.OutcomeFinished, state.OutcomePassed, state.OutcomeBlocked, state.OutcomeSnapshot:
		return true
	}
	return false
}
