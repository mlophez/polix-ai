package answer

import (
	"fmt"

	"github.com/mlophez/polix-ai/internal/app/agent"
	"github.com/mlophez/polix-ai/internal/app/workflow"
)

// command answers pre_command, post_command, pre_commit and post_commit, a command line about to run, one that ran
// and, with pre_command, a line about to commit and, with post_command, one whose commit succeeded. Before a command come its protections (see protectCommand). An unusable
// workflow fails closed before a command, but for the commands of polix that fixing it needs (see unusableCommand),
// and only warns after one. Without polix.yaml nothing else happens. The last event before a command line runs, the
// pre_command or the pre_commit of a line that commits, records the content before it runs when nothing blocked the
// line (see lastBefore), and a post_command first finds what the command changed and puts back the protected files
// (see watch), before its entries. The entries do not apply to a pre_commit whose commits all happen in another
// repository (see commitsOutside), nor to a post_commit whose commits do. A post_commit comes after the command: it
// has no protections, no snapshot and no watch of the files, and every inject and notify of it acts each time it
// applies. A failing run after a command reaches the AI as context (workflow.Informed).
func (r *pass) command() agent.Answer {
	name := r.ev.Event.Name
	pre := !r.after()
	if pre {
		if ans, denied := r.protectCommand(); r.waiting() || denied {
			return ans
		}
	}
	if r.ev.Invalid != nil {
		if pre {
			return r.unusableCommand()
		}
		return r.warnUnusable()
	}
	if !r.ev.Present {
		return agent.Answer{}
	}
	var ans agent.Answer
	if name == workflow.PostCommand {
		ans = r.watch()
	}
	if r.waiting() {
		return ans
	}
	if name == workflow.PreCommit || name == workflow.PostCommit {
		if outside := r.commitsOutside(); r.waiting() || outside {
			return r.lastBefore(agent.Answer{})
		}
	}
	if name == workflow.PostCommit {
		r.always, r.every = true, true
	}
	return r.lastBefore(r.evaluate(ans))
}

// lastBefore returns ans, the answer of an event about a command line, after recording the content before the line
// runs (see snapshot) when the event is the last before it of the call (hook.Evaluation.Last), the pre_commit of a
// line that commits, and neither ans nor an earlier event of the call (hook.Facts.Blocked) blocks it: the snapshot
// then holds what every entry of the line left, and a line some entry refused leaves none.
func (r *pass) lastBefore(ans agent.Answer) agent.Answer {
	if !r.after() && r.ev.Last() && !ans.Block && !r.ev.Facts.Blocked && !r.waiting() {
		r.snapshot()
	}
	return ans
}

// protectCommand applies the protections of a command line about to run, which come before the entries of the
// workflow and before the skip of the commits outside the change, and reports whether one denied it, logged with
// where it comes from: reading is always allowed (the log is the evidence of the process), but nobody runs a command
// that could write and names the state of polix, whatever the workflow says; and, with a valid workflow, no actor,
// the main session included, writes a path of guardrails.protected_files, also when git ignores it, nor runs a
// command of guardrails.forbidden_commands, wherever it comes in the line.
func (r *pass) protectCommand() (agent.Answer, bool) {
	ev, wf := r.ev.Event, r.guardrails()
	if mayWrite(ev.Command, r.ev.Windows()) {
		reals := r.reals(r.ev.StateDir())
		if r.waiting() {
			return agent.Answer{}, false
		}
		if namesState(ev.Command, r.ev.StateDir(), reals[r.ev.StateDir()], r.ev.Fold()) {
			return r.deny(builtinProtection, 0, reasonStateCommand)
		}
	}
	p, held, ok := r.writes(wf.ProtectedFiles)
	switch {
	case r.waiting():
		return agent.Answer{}, false
	case ok && held != "":
		return r.deny(protectedFilesShortcut, 0, fmt.Sprintf("%s holds %s, protected in %s: only a person changes it.",
			p, held, workflow.File))
	case ok:
		return r.deny(protectedFilesShortcut, 0, fmt.Sprintf("%s is protected in %s: only a person changes it.", p,
			workflow.File))
	}
	if f, ok := forbidden(wf.ForbiddenCommands, ev.Command, r.ev.Windows()); ok {
		reason := f.Reason
		if reason == "" {
			reason = reasonForbidden
		}
		return r.deny(forbiddenCommandsShortcut, f.Line, reason+"\n"+ev.Command)
	}
	return agent.Answer{}, false
}

// unusableCommand answers a command line about to run with a workflow that cannot be used: the commands of polix
// that check or update it pass (see onlyPolix), any other is refused.
func (r *pass) unusableCommand() agent.Answer {
	if onlyPolix(r.ev.Event.Command, r.ev.Windows()) {
		return agent.Answer{}
	}
	return r.refuseUnusable()
}

// commitsOutside reports whether every commit of the command of the event happens in another repository (`git -C
// ../other commit`): commitDirs places the line, and each folder is outside the root, symlinks resolved, and inside
// an existing git repository other than this one, a linked worktree of this one excluded. Such commits are not part
// of the change, so the workflow does not apply to them; anything less keeps it.
func (r *pass) commitsOutside() bool {
	root := r.ev.Root
	dirs, placed := commitDirs(r.ev.Event.Command, r.ev.Event.Cwd, root, r.ev.Windows())
	if !placed {
		return false
	}
	reals := r.reals(append([]string{root}, dirs...)...)
	if r.waiting() {
		return false
	}
	for _, d := range dirs {
		if within(reals[root], reals[d], r.ev.Fold()) {
			return false
		}
		if foreign := r.foreign(d); r.waiting() || !foreign {
			return false
		}
	}
	return true
}

// after reports whether the event comes after its command line ran: post_command and post_commit.
func (r *pass) after() bool {
	name := r.ev.Event.Name
	return name == workflow.PostCommand || name == workflow.PostCommit
}
