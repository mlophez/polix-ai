package answer

import (
	"slices"

	"github.com/mlophez/polix-ai/internal/app/agent"
	"github.com/mlophez/polix-ai/internal/app/hook"
	"github.com/mlophez/polix-ai/internal/app/state"
	"github.com/mlophez/polix-ai/internal/app/workflow"
)

// Output is the decision of the logic of AnswerHook for one event: the next step the shell carries out
// or, once it is none, the answer to the event and the facts the next event of the call starts from.
type Output struct {
	Step   hook.Step
	Answer agent.Answer
	Facts  hook.Facts
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
	// informed tells that the event already happened (workflow.Informed): a failing run reaches the AI as context
	// instead of blocking.
	informed bool
	// always tells that every inject of the event injects each time it applies, as on start (see session).
	always bool
	// loop tells that every block of the event goes through the loop guard of max_stop_retries (see stop).
	loop bool
	// guarded tells that the loop guard let a block of the event through: the event did not end as asked.
	guarded bool
	// warnings are the warnings of the pass for the user, such as a log that cannot be read.
	warnings []string
	// injected are the once-only injects that reached the answer, written as observed `injected` lines once the
	// answer is known (see writeInjected).
	injected []passInject
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
// none, the event is answered.
func Decide(ev hook.Evaluation) Output {
	r := &pass{ev: ev, epoch: ev.Facts.Epoch, state: ev.Facts.State,
		informed: slices.Contains(workflow.Informed, ev.Event.Name)}
	var ans agent.Answer
	if family := route(ev.Event.Name); family != nil {
		ans = family(r)
	}
	if r.step != nil {
		return Output{Step: *r.step}
	}
	return Output{Answer: ans, Facts: ev.Facts.Carry(r.epoch, r.state)}
}

// route returns the family of the event name, the method of pass that answers it: session for start and prompt
// (logic_session.go), edit for pre_edit and post_edit (logic_edit.go), command for pre_command, post_command and
// pre_commit (logic_command.go), and stop for stop and subagent_stop (logic_stop.go); nil for an event polix does
// not evaluate. Each family stops early when a step is missing; its answer is then meaningless.
func route(name string) func(*pass) agent.Answer {
	switch name {
	case workflow.Start, workflow.Prompt:
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
