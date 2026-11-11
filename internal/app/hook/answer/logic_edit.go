package answer

import (
	"fmt"

	"github.com/mlophez/polix-ai/internal/app/agent"
	"github.com/mlophez/polix-ai/internal/app/workflow"
)

// edit answers pre_edit and post_edit, an edit about to happen and one that happened. Before an edit come its
// protections (see protectEdit), but for an edit a command made: post_command already put its protected files back
// (see watch), and the state of polix lives in the git directory, which no tree holds. An unusable workflow fails
// closed before an edit, but for what fixing it needs (see unusableEdit), and only warns after one. Without
// polix.yaml nothing else happens, and neither for an edit outside the change (see outsideEdit). A failing run after
// an edit reaches the AI as context (workflow.Informed), and so do a block and a failing run on an edit a command
// made (see apply).
func (r *pass) edit() agent.Answer {
	pre := r.ev.Event.Name == workflow.PreEdit
	if pre && !r.byCommand {
		if ans, denied := r.protectEdit(); r.waiting() || denied {
			return ans
		}
	}
	if r.ev.Invalid != nil {
		if pre {
			return r.unusableEdit()
		}
		return r.warnUnusable()
	}
	if !r.ev.Present {
		return agent.Answer{}
	}
	if outside := r.outsideEdit(); r.waiting() || outside {
		return agent.Answer{}
	}
	return r.evaluate(agent.Answer{})
}

// protectEdit applies the protections of an edit, which come before the entries of the workflow and before the skip
// of the edits outside the change, and reports whether one denied it, logged with where it comes from: nobody edits
// the state of polix, whatever the workflow says, also without one or with an invalid one, its folder and the path
// compared by their real paths; and, with a valid workflow, no actor, the main session included, edits a path of
// guardrails.protected_files, also when git ignores it.
func (r *pass) protectEdit() (agent.Answer, bool) {
	ev, fold := r.ev.Event, r.ev.Fold()
	if abs := absolute(r.ev.Root, "", ev.AbsPath); abs != "" {
		reals := r.reals(r.ev.StateDir(), abs)
		if r.waiting() {
			return agent.Answer{}, false
		}
		if within(reals[r.ev.StateDir()], reals[abs], fold) {
			return r.deny(builtinProtection, 0, reasonStateEdit)
		}
	}
	if !ev.Outside && ev.Path != "" && anyPathFold(r.guardrails().ProtectedFiles, ev.Path, fold) {
		return r.deny(protectedFilesShortcut, 0, fmt.Sprintf("%s is protected in %s: only a person changes it.",
			ev.Path, workflow.File))
	}
	return agent.Answer{}, false
}

// unusableEdit answers an edit about to happen with a workflow that cannot be used: an edit outside the change never
// depended on the workflow, and the main session may edit the workflow to fix it, unless it requires a newer polix,
// so the agent does not take options out of it to please an old binary. Any other edit is refused.
func (r *pass) unusableEdit() agent.Answer {
	ev := r.ev.Event
	_, _, tooOld := r.unusable()
	outside := r.outsideEdit()
	if r.waiting() || outside || (!tooOld && ev.Actor == agent.ActorSession && ev.Path == workflow.File) {
		return agent.Answer{}
	}
	return r.refuseUnusable()
}

// outsideEdit reports an edit outside the repository (scratchpad, memory, another repository) or of a file git
// ignores: it is not part of the change, so the workflow does not apply to it.
func (r *pass) outsideEdit() bool {
	ev := r.ev.Event
	return ev.Outside || r.ignored(ev.Path)
}
