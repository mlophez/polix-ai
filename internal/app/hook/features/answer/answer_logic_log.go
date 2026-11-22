package answer

import (
	"errors"
	"slices"
	"time"

	"github.com/mlophez/polix-ai/internal/app/agent"
	"github.com/mlophez/polix-ai/internal/app/hook"
	"github.com/mlophez/polix-ai/internal/app/state"
	"github.com/mlophez/polix-ai/internal/app/workflow"
)

// log records an action on the event: what an entry of the workflow or a protection did with it, with its result.
// The record of the event holds it until the call ends (see record).
func (r *pass) log(action string, line int, outcome, reason string) {
	a := state.Action{Line: line, Action: action, Outcome: outcome, Reason: reason}
	if a.Valid() != nil {
		return
	}
	r.actions = append(r.actions, a)
}

// millis returns d in whole milliseconds, truncated, as the duration of an action: never more time than passed.
func millis(d time.Duration) *int64 {
	ms := max(d, 0).Milliseconds()
	return &ms
}

// logRun records a run action on the event: its outcome, the key of its repeat, why when it did not pass, and how
// long it took.
func (r *pass) logRun(line int, outcome, key, reason string, took time.Duration) {
	a := state.Action{Line: line, Action: workflow.VerbRun, Outcome: outcome, Key: key, Reason: reason,
		DurationMS: millis(took)}
	if a.Valid() != nil {
		return
	}
	r.actions = append(r.actions, a)
}

// stamp gives the key of its repeat and its duration to the run actions recorded since the n-th action of the event,
// those a failing run left through the block, whose entry the loop guard may have let through.
func (r *pass) stamp(n int, key string, took time.Duration) {
	for i := n; i < len(r.actions); i++ {
		a := r.actions[i]
		if a.Action != workflow.VerbRun {
			continue
		}
		ran := state.Action{Line: a.Line, Action: a.Action, Outcome: a.Outcome, Key: key, Reason: a.Reason,
			DurationMS: millis(took)}
		if ran.Valid() == nil {
			r.actions[i] = ran
		}
	}
}

// logInjected records an inject action on the event: what reached the AI, by key, which is the file, the word rules
// and the expanded target, or the expanded text cut to maxInjectKey characters. It is recorded where the inject is
// evaluated, so the actions keep the order of the entries, and dropped when the answer shows it never reached the AI
// (see unreached).
func (r *pass) logInjected(line int, key string) {
	a := state.Action{Line: line, Action: workflow.VerbInject, Outcome: state.OutcomeInjected, Key: key}
	if a.Valid() != nil {
		return
	}
	r.actions = append(r.actions, a)
}

// unreached drops the inject actions of the event: its answer is refused, so the contexts never reach the AI.
func (r *pass) unreached() {
	r.actions = slices.DeleteFunc(r.actions, func(a state.Action) bool {
		return a.Action == workflow.VerbInject && a.Outcome == state.OutcomeInjected
	})
}

// note records an observation of outcome on the event, about key and, for the outcomes that are about a content,
// hash (see noteWith), and returns why it is not valid.
func (r *pass) note(line int, outcome, key, hash string) error {
	return r.noteWith(line, outcome, key, hash, "", "")
}

// noteWith records the observation of outcome on the event, which every later query of the call reads (see
// currentState), with the refs git moved without HEAD, on a snapshot, and before, the hash the event started from when
// its runs changed it, on a blocked one (see state.Observation.Chained); it returns why it is not valid, in which case
// it is not recorded. Only the observations of the memory of the session go through it; an inject that reached the AI,
// for one, is recorded as injected once the answer is known (see writeInjected).
func (r *pass) noteWith(line int, outcome, key, hash, refs, before string) error {
	o := state.Observation{Line: line, Outcome: outcome, Key: key, Hash: hash, Refs: refs}
	o = o.Chained(before)
	if err := o.Valid(); err != nil {
		return err
	}
	r.observed = append(r.observed, o)
	return nil
}

// writeBlocked records the observation blocked of the entry at line, with key and hash, and before, the hash its
// event started from when the runs of the event changed it, else "" (see state.Observation.Before), and returns why it
// is not valid.
func (r *pass) writeBlocked(line int, key, hash, before string) error {
	return r.noteWith(line, state.OutcomeBlocked, key, hash, "", before)
}

// recordOf returns the record of the event with what acted and was observed on it so far, and why it is not valid: its
// session names no file, or nothing acted nor was observed. It carries the agent, the session, the event, the actor
// and its agent id, the time the evaluation started, the data of the event, and the branch and the HEAD commit when
// the call already learned them (whoever knows the branch asks for it first, see branch, so a query by branch reads
// the one of the record). The content hash is the one at the start of the event, when the call computed it, which
// costs a git call over the whole tree, so a record only carries the one the call already computed.
func (r *pass) recordOf() (state.Record, error) {
	ev := r.ev.Event
	hash, _, _ := r.ev.Facts.Hash(r.ev.Facts.Epoch)
	in, _ := inputOf(ev)
	rec := state.Record{At: r.ev.Started, Agent: ev.Agent, Session: ev.Session, Event: ev.Name, Actor: ev.Actor,
		AgentID: ev.AgentID, Branch: r.ev.Facts.Branch, Head: r.ev.Facts.Head, Hash: hash, Input: in,
		Actions: r.actions, Observations: r.observed}
	return rec, rec.Valid()
}

// record returns the record of the event, to be appended at the end of the call (see flush): the empty record when
// nothing acted nor was observed on it, or when its session names no file, so an event where nothing acted writes
// nothing. It asks for the branch and the HEAD commit the record carries.
func (r *pass) record() state.Record {
	if len(r.actions) == 0 && len(r.observed) == 0 {
		return state.Record{}
	}
	r.branch()
	r.head()
	if r.waiting() {
		return state.Record{}
	}
	rec, err := r.recordOf()
	if err != nil || rec.Writable() != nil {
		return state.Record{}
	}
	return rec
}

// flush appends the records of the call, one line each, in the order of its events, once the answer ans of the whole
// call is known: the last event asks it, with the pending records of the facts and its own. A record that cannot be
// appended is no error of the call, as it was no error to write a line: the log is evidence and memory, and polix goes
// on without it. On an event of the loop guard (see stop) it is: the guard could not count a block it cannot write, so
// a refused answer becomes the release of the guard, each of its reasons a warning for the user (see unusableLog).
func (r *pass) flush(ans agent.Answer, records []state.Record) agent.Answer {
	var failed error
	for i := range records {
		s := hook.Step{Append: &records[i]}
		o := r.act(s, s.Valid())
		if r.waiting() {
			return ans
		}
		failed = errors.Join(failed, o.Err)
	}
	if failed == nil || !r.loop || !ans.Block {
		return ans
	}
	released := agent.Answer{Contexts: ans.Contexts, Warnings: ans.Warnings}
	if released.Valid() != nil {
		return ans
	}
	for _, reason := range ans.Reasons {
		released = released.Warn(unusableLog(failed, reason))
	}
	return released
}

// inputOf returns the data of the event the log keeps: the command and the id of the call of a command or a commit,
// the path of an edit, the skill that starts, the prompt and the source of a start. The agent does not give the
// arguments of a skill, so the input leaves them out.
func inputOf(ev agent.Event) (state.Input, error) {
	var command, toolUse, path, skill, prompt, source string
	switch ev.Name {
	case workflow.PreCommand, workflow.PostCommand, workflow.PreCommit, workflow.PostCommit:
		command, toolUse = ev.Command, ev.ToolUse
	case workflow.PreEdit, workflow.PostEdit:
		path = ev.Path
	case workflow.Skill:
		skill = ev.Skill
	case workflow.Prompt:
		prompt = ev.Prompt
	case workflow.Start:
		source = ev.Source
	}
	in := state.Input{Command: command, ToolUse: toolUse, Path: path, Skill: skill, Prompt: prompt, Source: source}
	return in, in.Valid()
}
