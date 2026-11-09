package answer

import (
	"github.com/mlophez/polix-ai/internal/app/agent"
	"github.com/mlophez/polix-ai/internal/app/state"
	"github.com/mlophez/polix-ai/internal/app/workflow"
)

// session answers start and prompt, the start of a session (or the compaction of its context) and a prompt of the
// user. They come with no protection, and an unusable workflow only warns. Without polix.yaml nothing happens.
//
// On start the built-in guidance of the workflow comes before the entries, so the contexts of the repository have the
// last word, and every inject injects each time it applies; with the answer known, it writes that the session
// started when the workflow injects once, so its once-only injects are due again. On prompt a refusal rejects the
// prompt, so its contexts never reach the AI: its once-only injects are written as injected only when the answer
// does not refuse.
func (r *pass) session() agent.Answer {
	if r.ev.Invalid != nil {
		return r.warnUnusable()
	}
	if !r.ev.Present {
		return agent.Answer{}
	}
	start := r.ev.Event.Name == workflow.Start
	var ans agent.Answer
	if start {
		ans = r.guidance(ans)
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
	if start && r.ev.Workflow.InjectsOnce() {
		_ = r.write(state.KindObserved, "", 0, state.OutcomeStarted, "", "", "")
	}
	if r.waiting() {
		return ans
	}
	return r.warn(ans)
}

// guidance adds to ans the built-in guidance of the workflow for the session, when it has any.
func (r *pass) guidance(ans agent.Answer) agent.Answer {
	if text := r.ev.Workflow.SessionContext(); text != "" {
		return ans.Inform(text)
	}
	return ans
}
