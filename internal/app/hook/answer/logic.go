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
	epoch int         // runs carried out so far, which the content hash and the files read depend on
	state state.State // the log as this pass left it
	next  int         // the next outcome of the journal
	step  *hook.Step  // the step the pass waits for; nil while it goes on
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
	// loop tells that every block of the event goes through the loop guard of max_stop_retries (see stop).
	loop bool
	// guarded tells that the loop guard let a block of the event through: the event did not end as asked.
	guarded bool
	// released are the blocks the loop guard let through, whose observed `blocked` lines are written once the
	// entries of the event are done, with the content they ended on (see writeReleased).
	released []passReleased
	// warnings are the warnings of the pass for the user, such as a log that cannot be read.
	warnings []string
	// injected are the once-only injects that reached the answer, written as observed `injected` lines once the
	// answer is known (see writeInjected).
	injected []passInject
	// notices are the notices of notify without always that reached the answer of the event, written as observed
	// `notified` lines only when the whole call does not block (see settle).
	notices []passNotice
	// pending are the observed lines written only when the whole call does not block: those of the earlier events
	// of the call (hook.Facts.Pending), then those of this one, carried to the next event until the last one writes
	// them (see settle).
	pending []state.Entry
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
// none, the event is answered, and the facts the next event starts from know the answer (hook.Facts.Answered), so
// the messages for the user a block of the call hides are logged as not shown (see warn), and the lines written
// only when the whole call does not block are carried to its last event (see settle). An edit a command made
// already happened, so it is evaluated as informed.
func Decide(ev hook.Evaluation) Output {
	byCommand := ev.Event.ByCommand
	r := &pass{ev: ev, epoch: ev.Facts.Epoch, state: ev.Facts.State, byCommand: byCommand,
		informed: byCommand || slices.Contains(workflow.Informed, ev.Event.Name), pending: ev.Facts.Pending}
	var ans agent.Answer
	if family := route(ev.Event.Name); family != nil {
		ans = family(r)
	}
	if !r.waiting() {
		r.settle(ans)
	}
	if r.step != nil {
		return Output{Step: *r.step}
	}
	return Output{Answer: ans, Facts: ev.Facts.Carry(r.epoch, r.state).Answered(ans, r.pending), Follow: r.follow}
}

// route returns the family of the event name, the method of pass that answers it: session for start, prompt and
// subagent_start (logic_session.go), edit for pre_edit and post_edit (logic_edit.go), command for pre_command, post_command and
// pre_commit (logic_command.go), and stop for stop and subagent_stop (logic_stop.go); nil for an event polix does
// not evaluate. Each family stops early when a step is missing; its answer is then meaningless.
func route(name string) func(*pass) agent.Answer {
	switch name {
	case workflow.Start, workflow.Prompt, workflow.SubagentStart:
		return (*pass).session
	case workflow.PreEdit, workflow.PostEdit:
		return (*pass).edit
	case workflow.PreCommand, workflow.PostCommand, workflow.PreCommit:
		return (*pass).command
	case workflow.Stop, workflow.SubagentStop:
		return (*pass).stop
	}
	return nil
}
