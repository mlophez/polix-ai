package state

import (
	"slices"

	"github.com/mlophez/polix-ai/internal/app/workflow"
)

// running is the started lines of the subagents open at a point of the walk of Running, in the order they started.
type running []Entry

// SourceCompact is the key of the started line of a start of the session that only compacted its context: the
// subagents of the session go on through it, so it closes none of them (see Running).
const SourceCompact = "compact"

// Running returns the started lines of the subagents still running in the session, in the order they started:
// walking its observed lines in file order, a started of a subagent_start opens one; a finished, or an ended, closes
// the one of its agent_id, else the oldest open one of its type (the key of a finished, the actor of an ended), which
// must have no agent_id when the line has one, so the lines of Codex, of an older Claude Code and of an older polix
// pair by type; and a started of a start whose key is not SourceCompact (a new session, a resume, a clear, or a start
// that gives no source) closes every one, since a subagent never outlives them. The lines of other sessions and of
// other outcomes never count.
func (s State) Running(session string) []Entry {
	q := Query{Outcome: OutcomeStarted, Session: session}
	if q.Valid() != nil {
		return nil
	}
	var open running
	for _, e := range s.Entries {
		if e.Kind == KindObserved && q.selects(e) {
			open = open.step(e)
		}
	}
	return open
}

// step returns the open subagents after the observed line e of their session (see Running).
func (o running) step(e Entry) running {
	switch {
	case e.Outcome == OutcomeStarted && e.Event == workflow.SubagentStart:
		return append(slices.Clip(o), e)
	case e.Outcome == OutcomeStarted && e.Event == workflow.Start && e.Key != SourceCompact:
		return nil
	case e.Outcome == OutcomeFinished:
		return o.close(e.AgentID, e.Key)
	case e.Outcome == OutcomeEnded:
		return o.close(e.AgentID, e.Actor)
	}
	return o
}

// RunningOthers returns the subagents still running in the session (see Running) but the one that acts, of agent id
// and type kind, chosen as its finished would close it: the one of id, else the oldest of kind, without id when id
// is set.
func (s State) RunningOthers(session, id, kind string) []Entry {
	return running(s.Running(session)).close(id, kind)
}

// close returns the open subagents without the one a line of agent id and type kind closes: the one of id, else the
// oldest of kind, without id when id is set; all of them when none matches. It never changes o, whose array the
// copy does not share.
func (o running) close(id, kind string) running {
	i := -1
	if id != "" {
		i = slices.IndexFunc(o, func(e Entry) bool { return e.AgentID == id })
	}
	if i < 0 {
		i = slices.IndexFunc(o, func(e Entry) bool { return e.Actor == kind && (id == "" || e.AgentID == "") })
	}
	if i < 0 {
		return o
	}
	return slices.Delete(slices.Clone(o), i, i+1)
}
