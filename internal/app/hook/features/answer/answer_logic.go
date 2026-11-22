package answer

import (
	"slices"

	"github.com/mlophez/polix-ai/internal/app/agent"
	"github.com/mlophez/polix-ai/internal/app/hook"
	"github.com/mlophez/polix-ai/internal/app/state"
	"github.com/mlophez/polix-ai/internal/app/workflow"
)

// Output is the decision of the logic of AnswerHook for one event: the next step the shell carries out
// or, once it is none, the answer to the event, the facts the next event of the call starts from and the events
// the shell evaluates next in the same call.
type Output struct {
	Step   hook.Step
	Answer agent.Answer
	Facts  hook.Facts
	// Follow are the events derived from this one that the shell evaluates next in the same call, in order: the
	// edits a command made (see watch).
	Follow []agent.Event
}

// pass is one pass of the evaluation of an event, from its start: the facts are read from the
// evaluation, and the steps that act take their outcomes from its journal, in order, until one is missing.
type pass struct {
	ev    hook.Evaluation
	epoch int        // runs carried out so far, which the content hash and the files read depend on
	next  int        // the next outcome of the journal
	step  *hook.Step // the step the pass waits for; nil while it goes on
	// actions and observed are what acted and was observed on the event so far: the actions and the observations of
	// the record of the event, which the call appends once it ends (see record). The queries read them with the log
	// (see currentState).
	actions  []state.Action
	observed []state.Observation
	// entry is the entry being evaluated: the `changed` of its `when` scopes the measure placeholders and the
	// content `finished` compares.
	entry workflow.Entry
	// informed tells that the event already happened (workflow.Informed), or is an edit a command made: a failing
	// run reaches the AI as context instead of blocking.
	informed bool
	// byCommand tells that the event is an edit polix derived from the effect of a command (agent.Event.ByCommand):
	// the edit already happened, so a block reaches the AI as context instead of refusing (see apply).
	byCommand bool
	// always tells that every inject of the event injects each time it applies, as on start (see session).
	always bool
	// every tells that every notify of the event shows its notice each time it applies, with no notified line, as
	// with always (see command on post_commit).
	every bool
	// loop tells that every block of the event goes through the loop guard of stop_retries (see stop).
	loop bool
	// guarded tells that the loop guard let a block of the event through: the event did not end as asked.
	guarded bool
	// released are the blocks the loop guard let through, whose `blocked` observations are noted once the entries of
	// the event are done, with the content they ended on (see writeReleased).
	released []passReleased
	// warnings are the warnings of the pass for the user, such as a log that cannot be read.
	warnings []string
	// injected are the once-only injects that reached the answer, noted as `injected` observations once the answer
	// is known (see writeInjected).
	injected []passInject
	// notices are the notices of notify without always that reached the answer of the event, noted as `notified`
	// observations only when the whole call does not block (see settle).
	notices []passNotice
	// follow are the edits a command made, which the shell evaluates next in the same call (see derive).
	follow []agent.Event
}

// passReleased is a block the loop guard let through: the line of its entry and what a failing run ran ("" for a
// block).
type passReleased struct {
	line int
	key  string
}

// passInject is a once-only inject that reached the answer: the line of its entry and its key.
type passInject struct {
	line int
	key  string
}

// Decide decides the next step of the evaluation of an event: it routes the event to the family that answers it
// (see route), which applies the protections of its events, then, with a valid workflow, the entries of the event
// in order, each with its conditions and its action, through the engine. It walks the event from its start every
// time it is asked, so the step it returns is the first one whose result the evaluation does not know yet; with
// none, the event is answered, and the facts the next event starts from know the answer and the record of the event
// (hook.Facts.Answered), so the messages for the user a block of the call hides are logged as not shown (see warn),
// and the notices the user never sees are dropped (see settle). The record of the event, with its actions and its
// observations, waits in the facts until the last event of the call, which appends every one, one line each (see
// flush). An edit a command made already happened, so it is evaluated as informed. An event whose session names no
// file, one without a session id, reads no state and writes no record, so it warns as a log that cannot be read does
// (see warnLog), whether it reads the state or not.
func Decide(ev hook.Evaluation) Output {
	byCommand := ev.Event.ByCommand
	r := &pass{ev: ev, epoch: ev.Facts.Epoch, byCommand: byCommand,
		informed: byCommand || slices.Contains(workflow.Informed, ev.Event.Name)}
	if _, err := state.Session(ev.Event.Session).File(); err != nil {
		r.warnLog(err)
	}
	var ans agent.Answer
	if family := route(ev.Event.Name); family != nil {
		ans = family(r)
	}
	var rec state.Record
	if !r.waiting() {
		r.settle(ans)
		rec = r.record()
	}
	if r.step != nil {
		return Output{Step: *r.step}
	}
	facts := ev.Facts.Carry(r.epoch).Answered(ans, rec)
	if ev.Last() && len(r.follow) == 0 {
		ans = r.flush(ans, facts.Pending)
		if r.step != nil {
			return Output{Step: *r.step}
		}
		facts = facts.Written()
	}
	return Output{Answer: ans, Facts: facts, Follow: r.follow}
}

// route returns the family of the event name, the method of pass that answers it: session for start, prompt, skill
// and subagent_start (answer_logic_session.go), edit for pre_edit and post_edit (answer_logic_edit.go), command for
// pre_command, post_command, pre_commit and post_commit (answer_logic_command.go), and stop for stop and subagent_stop
// (answer_logic_stop.go); nil for an event polix does not evaluate. Each family stops early when a step is missing; its
// answer is then meaningless.
func route(name string) func(*pass) agent.Answer {
	switch name {
	case workflow.Start, workflow.Prompt, workflow.Skill, workflow.SubagentStart:
		return (*pass).session
	case workflow.PreEdit, workflow.PostEdit:
		return (*pass).edit
	case workflow.PreCommand, workflow.PostCommand, workflow.PreCommit, workflow.PostCommit:
		return (*pass).command
	case workflow.Stop, workflow.SubagentStop:
		return (*pass).stop
	}
	return nil
}
