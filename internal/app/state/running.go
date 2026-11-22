package state

import (
	"slices"

	"github.com/mlophez/polix-ai/internal/app/workflow"
)

// running is the started observations of the subagents open at a point of the walk of Running, in the order they
// started, each with its record.
type running []Match

// SourceCompact is the key of the started observation of a start of the session that only compacted its context: the
// subagents of the session go on through it, so it closes none of them (see Running).
const SourceCompact = "compact"

// Running returns the started observations of the subagents still running in the session, in the order they started:
// walking the observations of its records in file order, a started of a subagent_start opens one; a finished, or an
// ended, closes the one of its agent id, else the oldest open one of its type (the key of a finished, the type of
// an ended, see Match.Ended), which must have no agent id when the observation has one, so the observations of Codex
// and of an older Claude Code pair by type; and a started of a start whose key is not SourceCompact (a new session, a
// resume, a clear, or a start that gives no source) closes every one, since a subagent never outlives them. The
// observations of other sessions and of other outcomes never count, nor those of legacy records.
func (s State) Running(session string) []Match {
	q := Query{Outcome: OutcomeStarted, Session: session}
	if q.Valid() != nil {
		return nil
	}
	var open running
	s.observations(func(seq int, r *Record, o *Observation) bool {
		if !q.selects(r, o) {
			return true
		}
		if m, err := s.matched(seq, r, o); err == nil {
			open = open.step(m)
		}
		return true
	})
	return open
}

// step returns the open subagents after the observation m of their session (see Running).
func (o running) step(m Match) running {
	switch {
	case m.Outcome == OutcomeStarted && m.Record.Event == workflow.SubagentStart:
		return append(slices.Clip(o), m)
	case m.Outcome == OutcomeStarted && m.Record.Event == workflow.Start && m.Key != SourceCompact:
		return nil
	case m.Outcome == OutcomeFinished:
		return o.close(m.Record.AgentID, m.Key)
	case m.Outcome == OutcomeEnded:
		kind, id := m.Ended()
		return o.close(id, kind)
	}
	return o
}

// RunningOthers returns the subagents still running in the session (see Running) but the one that acts, of agent id
// id and type kind, chosen as its finished would close it: the one of id, else the oldest of kind, without id when
// id is set.
func (s State) RunningOthers(session, id, kind string) []Match {
	return running(s.Running(session)).close(id, kind)
}

// close returns the open subagents without the one an observation of agent id id and type kind closes: the one of
// id, else the oldest of kind, without id when id is set; all of them when none matches. It never changes o, whose
// array the copy does not share.
func (o running) close(id, kind string) running {
	i := -1
	if id != "" {
		i = slices.IndexFunc(o, func(m Match) bool { return m.Record.AgentID == id })
	}
	if i < 0 {
		i = slices.IndexFunc(o, func(m Match) bool { return m.Record.Actor == kind && (id == "" || m.Record.AgentID == "") })
	}
	if i < 0 {
		return o
	}
	return slices.Delete(slices.Clone(o), i, i+1)
}
