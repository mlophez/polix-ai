package answer

import (
	"fmt"
	"slices"

	"github.com/mlophez/polix-ai/internal/app/agent"
	"github.com/mlophez/polix-ai/internal/app/state"
	"github.com/mlophez/polix-ai/internal/app/workflow"
)

// stop answers stop and subagent_stop, the end of the turn of the main session or of a subagent. They come with no
// protection, and an unusable workflow only warns: a blocked stop would keep the agent in a loop it cannot leave.
// Without polix.yaml nothing happens. Before the entries, the log is reconciled with the subagents in flight the
// agent listed (see reconcile). Every block of their entries goes through the loop guard (see loopGuard). With the
// answer known, a subagent_stop the answer does not refuse writes that the subagent finished (see observeFinished).
func (r *pass) stop() agent.Answer {
	if r.ev.Invalid != nil {
		return r.warnUnusable()
	}
	if !r.ev.Present {
		return agent.Answer{}
	}
	r.loop = true
	r.reconcile()
	if r.waiting() {
		return agent.Answer{}
	}
	ans := r.entries(agent.Answer{})
	if r.waiting() {
		return ans
	}
	r.writeReleased()
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
// this same content (the whole hash), the agent may stop and a trace is left. before is the content hash at the
// start of the event when its runs changed it, else "" (see startOf): the blocks chain through it (see
// state.State.Chain), so a run whose command writes files the tree does not ignore, the blocking one or an earlier
// one, does not make the blocks of a content the agent left alone a new content on every stop. The warning names
// max_stop_retries, not the lines counted, which grow with the stops let through. An unknown hash counts as one
// content (state.UnknownHash). When the log cannot be read the guard cannot count, so it fails open. It returns the hash the
// block is written with and, when the guard lets the event through, the warning for the user, else "".
func (r *pass) loopGuard(e workflow.Entry, st state.State, err error, hash, before string, known bool,
	reason string) (string, string) {
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
	if n, limit := len(st.Chain(q, before)), r.ev.Workflow.MaxStopRetries; n >= limit {
		return hash, fmt.Sprintf("polix lets the agent stop after %d blocks: %s", limit, reason)
	}
	return hash, ""
}

// observeFinished writes that the subagent of a subagent_stop the answer does not refuse finished, its type as the
// key: with the content hash when a `finished` of the workflow names the type, so the condition can hold; without
// it, and without computing it, when the workflow only injects once, so the injects of that subagent are due again,
// or counts the subagents still running (workflow.Workflow.CountsAgents), so it no longer counts as running.
// Without a known hash, whose cause the facts already keep as a warning, the condition gets nothing, with a warn
// decision, and the line is written without hash only when the workflow injects once or counts the subagents. A stop
// the loop guard let through did not end as asked: it is written without hash, only when the workflow injects once or
// counts the subagents, so the subagent never counts as finished but its next run, with a new context, gets the
// injects again, and it no longer counts as running. Either line carries the id of the subagent.
func (r *pass) observeFinished() {
	ev := r.ev.Event
	ends := r.ev.Workflow.InjectsOnce() || r.ev.Workflow.CountsAgents()
	if r.guarded {
		if ends {
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
	if hash != "" || ends {
		_ = r.write(state.KindObserved, "", 0, state.OutcomeFinished, "", ev.Actor, hash)
	}
}

// reconcile writes an ended line for each subagent the log has open in the session (state.State.Running) that the
// subagents in flight of the event do not list (unlisted), the actor aside, so a subagent whose end never reached
// polix, an interrupted one, stops counting as running. It acts only when the workflow counts the subagents
// (workflow.Workflow.CountsAgents) and the agent gave the list: without it, or with one that does not decode, the
// log stays as it is. A log that cannot be read reconciles nothing, with a warning. The entries of the event never
// depend on these lines: with the list known, agents_are_running reads the list (see agentsRunning).
func (r *pass) reconcile() {
	ev := r.ev.Event
	if !ev.InFlightKnown || !r.ev.Workflow.CountsAgents() {
		return
	}
	st, err := r.currentState()
	if r.waiting() {
		return
	}
	if err != nil {
		r.warnLog(err)
		return
	}
	open := st.Running(ev.Session)
	if ev.Actor != agent.ActorSession {
		open = st.RunningOthers(ev.Session, ev.AgentID, ev.Actor)
	}
	for _, e := range unlisted(open, ev.InFlight) {
		r.writeEnded(e)
		if r.waiting() {
			return
		}
	}
}

// writeEnded appends the ended line of the open subagent of the started line open: about the event, with the type
// and the id of that subagent, without key nor hash.
func (r *pass) writeEnded(open state.Entry) {
	ev := r.ev.Event
	branch := r.branch()
	if r.waiting() {
		return
	}
	e := state.Entry{Kind: state.KindObserved, Agent: ev.Agent, Session: ev.Session, Event: ev.Name, Actor: open.Actor,
		AgentID: open.AgentID, Branch: branch, Outcome: state.OutcomeEnded}
	_ = r.appendEntry(e, e.Valid())
}

// unlisted returns the started lines of open whose subagent inFlight does not list: a line with an id is listed by
// a subagent of that id; then each line without id by a subagent of its type not taken yet, in order.
func unlisted(open []state.Entry, inFlight []agent.Subagent) []state.Entry {
	taken := make([]bool, len(inFlight))
	listed := make([]bool, len(open))
	for i, e := range open {
		j := slices.IndexFunc(inFlight, func(s agent.Subagent) bool { return e.AgentID != "" && s.ID == e.AgentID })
		if j >= 0 {
			listed[i], taken[j] = true, true
		}
	}
	for i, e := range open {
		j := -1
		for k, s := range inFlight {
			if j < 0 && e.AgentID == "" && !taken[k] && s.Type == e.Actor {
				j = k
			}
		}
		if j >= 0 {
			listed[i], taken[j] = true, true
		}
	}
	var out []state.Entry
	for i, e := range open {
		if !listed[i] {
			out = append(out, e)
		}
	}
	return out
}

// unusableLog is the warning of a stop the loop guard lets through because the log cannot be used.
func unusableLog(err error, reason string) string {
	return fmt.Sprintf("polix lets the agent stop because it cannot use its log (%v): %s", err, reason)
}
