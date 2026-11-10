package answer

import (
	"fmt"

	"github.com/mlophez/polix-ai/internal/app/agent"
	"github.com/mlophez/polix-ai/internal/app/state"
	"github.com/mlophez/polix-ai/internal/app/workflow"
)

// stop answers stop and subagent_stop, the end of the turn of the main session or of a subagent. They come with no
// protection, and an unusable workflow only warns: a blocked stop would keep the agent in a loop it cannot leave.
// Without polix.yaml nothing happens. Every block of their entries goes through the loop guard (see loopGuard). With
// the answer known, a subagent_stop the answer does not refuse writes that the subagent finished (see
// observeFinished).
func (r *pass) stop() agent.Answer {
	if r.ev.Invalid != nil {
		return r.warnUnusable()
	}
	if !r.ev.Present {
		return agent.Answer{}
	}
	r.loop = true
	ans := r.entries(agent.Answer{})
	if r.waiting() {
		return ans
	}
	r.writeInjected()
	if r.waiting() {
		return ans
	}
	if r.ev.Event.Name == workflow.SubagentStop && !ans.Block {
		r.observeFinished()
	}
	if r.waiting() {
		return ans
	}
	return r.warn(ans)
}

// loopGuard counts the block of the entry e for the loop guard of max_stop_retries, on the log st, or err when it
// cannot be read, and the content hash: after max_stop_retries blocks of this entry of the actor in the session on
// this same content (the whole hash), the agent may stop and a trace is left. An unknown hash counts as one content
// (state.UnknownHash). When the log cannot be read the guard cannot count, so it fails open. It returns the hash the
// block is written with and, when the guard lets the event through, the warning for the user, else "".
func (r *pass) loopGuard(e workflow.Entry, st state.State, err error, hash string, known bool, reason string) (string,
	string) {
	if err != nil {
		return hash, unusableLog(err, reason)
	}
	if !known {
		hash = state.UnknownHash
	}
	ev := r.ev.Event
	q := state.Query{Outcome: state.OutcomeBlocked, Session: ev.Session, Event: ev.Name, Actor: ev.Actor,
		Line: e.Line, Hash: hash, ByHash: true}
	if err := q.Valid(); err != nil {
		return hash, unusableLog(err, reason)
	}
	if n := len(st.Observed(q)); n >= r.ev.Workflow.MaxStopRetries {
		return hash, fmt.Sprintf("polix lets the agent stop after %d blocks: %s", n, reason)
	}
	return hash, ""
}

// observeFinished writes that the subagent of a subagent_stop the answer does not refuse finished, its type as the
// key: with the content hash when a `finished` of the workflow names the type, so the condition can hold; without
// it, and without computing it, when the workflow only injects once, so the injects of that subagent are due again.
// Without a known hash, whose cause the facts already keep as a warning, the condition gets nothing, with a warn
// decision, and the line is written without hash only when the workflow injects once. A stop the loop guard let
// through did not end as asked: it is written without hash, only when the workflow injects once, so the subagent
// never counts as finished but its next run, with a new context, gets the injects again.
func (r *pass) observeFinished() {
	ev := r.ev.Event
	if r.guarded {
		if r.ev.Workflow.InjectsOnce() {
			_ = r.write(state.KindObserved, "", 0, state.OutcomeFinished, "", ev.Actor, "")
		}
		return
	}
	observed := false
	for _, name := range r.ev.Workflow.ObservedSubagents() {
		observed = observed || r.expand(name) == ev.Actor
	}
	if r.waiting() {
		return
	}
	once := r.ev.Workflow.InjectsOnce()
	hash := ""
	if observed {
		h, known := r.hash()
		if r.waiting() {
			return
		}
		if !known {
			r.log("finished", 0, "warn", "unknown hash of the content: that "+ev.Actor+
				" finished is not recorded for finished")
		}
		hash = h
	}
	if hash != "" || once {
		_ = r.write(state.KindObserved, "", 0, state.OutcomeFinished, "", ev.Actor, hash)
	}
}

// unusableLog is the warning of a stop the loop guard lets through because the log cannot be used.
func unusableLog(err error, reason string) string {
	return fmt.Sprintf("polix lets the agent stop because it cannot use its log (%v): %s", err, reason)
}
