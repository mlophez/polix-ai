package answer

import (
	"strings"

	"github.com/mlophez/polix-ai/internal/app/agent"
	"github.com/mlophez/polix-ai/internal/app/state"
	"github.com/mlophez/polix-ai/internal/app/workflow"
)

const (
	// actionRequirements is the action of the decisions about requirements in the log.
	actionRequirements = "requirements"
	// keyRequirements starts the key of the context of the missing programs, followed by their names.
	keyRequirements = actionRequirements + ": "
)

// session answers start, prompt and subagent_start: the start of a session (or the compaction of its context), a
// prompt of the user and the start of a subagent. They come with no protection, and an unusable workflow only warns.
// Without polix.yaml nothing happens.
//
// On start the built-in guidance of the workflow comes before the entries, so the contexts of the repository have the
// last word. On start and subagent_start, which begin a new context, every inject injects each time it applies; with
// the answer known, they write that the session, or the subagent of the type of the actor, started when the workflow
// observes it (workflow.Workflow.ObservesStart), so its once-only injects are due again (see due). subagent_start
// gets neither the guidance nor the requirements, which are told to the main session. On prompt a refusal rejects the
// prompt, so its contexts never reach the AI: its once-only injects are written as injected only when the answer
// does not refuse. On prompt, before the entries, the programs of requirements the PATH does not run are told to the
// AI once (see requirements).
func (r *pass) session() agent.Answer {
	if r.ev.Invalid != nil {
		return r.warnUnusable()
	}
	if !r.ev.Present {
		return agent.Answer{}
	}
	name := r.ev.Event.Name
	start := name == workflow.Start || name == workflow.SubagentStart
	var ans agent.Answer
	switch name {
	case workflow.Start:
		ans = r.guidance(ans)
	case workflow.Prompt:
		ans = r.requirements(ans)
		if r.waiting() {
			return ans
		}
	}
	r.always = start
	ans = r.entries(ans)
	if r.waiting() {
		return ans
	}
	if !ans.Block {
		r.writeInjected()
	}
	if r.waiting() {
		return ans
	}
	if start && r.ev.Workflow.ObservesStart() {
		_ = r.write(state.KindObserved, "", 0, state.OutcomeStarted, "", "", "")
	}
	if r.waiting() {
		return ans
	}
	return r.warn(ans)
}

// requirements adds to ans the context of the programs of requirements, git included (workflow.Programs), the PATH
// does not run, as a once-only inject of the session: at line 0, keyed by keyRequirements and the missing names, so it
// is due again after a start of the session or when the set of missing programs changes. It is skipped, and the skip logged, while not due; it is kept
// to be written as injected (see writeInjected), and it never blocks. Without requirements it looks nothing up.
func (r *pass) requirements(ans agent.Answer) agent.Answer {
	if len(r.ev.Workflow.Requirements) == 0 {
		return ans
	}
	missing := r.missing(r.ev.Workflow.Programs())
	if r.waiting() || len(missing) == 0 {
		return ans
	}
	names := strings.Join(missing, ", ")
	key := keyRequirements + names
	due := r.due(0, key)
	if r.waiting() {
		return ans
	}
	if !due {
		r.log(actionRequirements, 0, "skip", "the missing programs "+names+
			" were already told to "+r.ev.Event.Actor+" in this session")
		return ans
	}
	r.injected = append(r.injected, passInject{line: 0, key: key})
	return ans.Inform("These programs that polix.yaml requires are not on the PATH: " + names + ". Before doing " +
		"anything else, install them, or ask the user to install them if you cannot.")
}

// guidance adds to ans the built-in guidance of the workflow for the session, when it has any.
func (r *pass) guidance(ans agent.Answer) agent.Answer {
	if text := r.ev.Workflow.SessionContext(); text != "" {
		return ans.Inform(text)
	}
	return ans
}
