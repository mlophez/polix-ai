package answer

import (
	"errors"
	"fmt"
	"slices"

	"github.com/mlophez/polix-ai/internal/app/agent"
	"github.com/mlophez/polix-ai/internal/app/state"
	"github.com/mlophez/polix-ai/internal/app/workflow"
)

// actionMessage is the action of the decisions about the messages for the user in the log.
const actionMessage = "message"

// evaluate answers the event from ans with its entries (see entries), then writes the once-only injects that reached
// the AI (see writeInjected) and adds the warnings of the call (see warn): the whole evaluation of the events whose
// family adds nothing after the entries.
func (r *pass) evaluate(ans agent.Answer) agent.Answer {
	ans = r.entries(ans)
	if r.waiting() {
		return ans
	}
	r.writeInjected()
	if r.waiting() {
		return ans
	}
	return r.warn(ans)
}

// entries evaluates the entries of the event in order, from ans, until the first one that actually blocks, a block
// or a failing run, whatever its verb (see apply); the entries after it never run, and the contexts of those before
// it reach the agent with the block. The notices they show are kept to be written as notified once the call is known
// not to block (see settle).
func (r *pass) entries(ans agent.Answer) agent.Answer {
	for _, e := range r.ev.Workflow.On[r.ev.Event.Name] {
		r.entry = e
		if !r.matches(e.When, nil) || len(e.Unless) > 0 && r.matches(e.Unless, nil) {
			if r.waiting() {
				return ans
			}
			continue
		}
		if r.waiting() {
			return ans
		}
		var stop bool
		before := len(ans.Warnings)
		ans, stop = r.apply(e, ans)
		if !r.every {
			r.notices = shown(r.notices, e, ans.Warnings[before:])
		}
		if r.waiting() || stop {
			break
		}
	}
	return ans
}

// warn adds to ans the warnings of the pass and of the call it does not hold yet, those of the earlier events of the
// call included (hook.Facts.Answered): the messages for the user, which the agent shows only when the call is not
// blocked (a systemMessage on exit 0). When ans is blocked, or an earlier event of the call blocked, each of its
// messages is logged as a skip decision of actionMessage that says it was not shown (see unshown).
func (r *pass) warn(ans agent.Answer) agent.Answer {
	for _, w := range append(slices.Clone(r.warnings), r.ev.Facts.Warnings...) {
		if !slices.Contains(ans.Warnings, w) {
			ans = ans.Warn(w)
		}
	}
	if ans.Block || r.ev.Facts.Blocked {
		r.unshown(ans)
	}
	return ans
}

// unshown logs each message for the user of ans, in a blocked call, as a skip decision: the agent reads no output of
// a blocked event but its reasons, so the user never sees them.
func (r *pass) unshown(ans agent.Answer) {
	for _, w := range ans.Warnings {
		r.log(actionMessage, 0, "skip", "the event was blocked, so this message was not shown to the user: "+w)
		if r.waiting() {
			return
		}
	}
}

// apply carries out the verb of the action of a matching entry and reports whether the evaluation of the event
// stops there. One rule, whatever the verb: the first entry that actually blocks ends the evaluation, a `block` or
// a failing `run` on an event where it blocks. A block let through by the loop guard, an inject, a notice, a skipped
// verb, a passing run and a failing run on an event where it only informs let it go on, and so does a block on an
// edit a command made, which already happened: its reason reaches the AI as context, logged as inform.
func (r *pass) apply(e workflow.Entry, ans agent.Answer) (agent.Answer, bool) {
	a := e.Action
	switch a.Verb() {
	case workflow.VerbBlock:
		if a.Block.Once {
			reminded := r.reminded(e)
			if r.waiting() {
				return ans, false
			}
			if reminded {
				r.log(workflow.VerbBlock, e.Line, "skip", "already blocked "+r.ev.Event.Actor+
					" in this session on the same content")
				return ans, false
			}
		}
		reason := r.expand(a.Block.Message)
		if r.waiting() {
			return ans, false
		}
		if r.byCommand {
			r.log(workflow.VerbBlock, e.Line, "inform", reason)
			return ans.Inform(reason), false
		}
		return r.block(e, ans, reason, "")
	case workflow.VerbRun:
		return r.runEntry(e, ans)
	case workflow.VerbInject:
		return r.inject(e, ans), false
	case workflow.VerbNotify:
		return r.notify(e, ans), false
	}
	return ans, false
}

// block blocks the event for the entry e and reports whether it is actually blocked; key is what a failing run ran,
// "" for a block. A block with once writes an observed `blocked` line, which reminded reads. On the events of the
// loop guard (see stop) every block (a block or a failing run) writes it too, with the content the event started from
// when its runs changed it (see startOf), and the guard may let the event through instead (see loopGuard); when the
// log cannot be written the guard cannot count, so it fails open. A stop the guard lets through after runs that
// changed the content writes its line too (see releaseChain). A block with once outside them blocks without writing
// when the hash is unknown or the log cannot be read, so it blocks again.
func (r *pass) block(e workflow.Entry, ans agent.Answer, reason, key string) (agent.Answer, bool) {
	verb := e.Action.Verb()
	once := e.Action.Block != nil && e.Action.Block.Once
	if !r.loop && !once {
		r.log(verb, e.Line, "block", reason)
		return ans.Refuse(reason), true
	}
	hash, known := r.hash()
	st, err := r.currentState()
	if r.waiting() {
		return ans, false
	}
	before := ""
	if r.loop {
		before = r.startOf(hash, known)
		var warning string
		if hash, warning = r.loopGuard(e, st, err, hash, before, known, reason); warning != "" {
			return r.releaseChain(e, ans, warning, reason, key, err == nil), false
		}
		known = true
	}
	if known && err == nil {
		werr := r.writeBlocked(e.Line, key, hash, before)
		if r.waiting() {
			return ans, false
		}
		if werr != nil && r.loop {
			return r.release(e, ans, unusableLog(werr, reason), reason), false
		}
	}
	r.log(verb, e.Line, "block", reason)
	return ans.Refuse(reason), true
}

// startOf returns the content hash at the start of the event, the content the agent left, when the runs of the event
// changed it since and hash, the content now, is known; "" otherwise. The first run of the event computes the hash of
// epoch 0 before it runs (see runEntry), so it is known whenever a run moved the epoch. A block written with it chains
// to the blocks that ended on the content the agent left (see state.State.Chain), so a run that writes files the tree
// does not ignore, the blocking one or any earlier one of the event, does not make every stop a new content.
func (r *pass) startOf(hash string, known bool) string {
	if !known || r.epoch == 0 {
		return ""
	}
	if start, ok, _ := r.ev.Facts.Hash(0); ok && start != hash {
		return start
	}
	return ""
}

// releaseChain lets the event through for the loop guard as release does and, when the log is usable, keeps the
// block of the entry e, with key, for its `blocked` line, written once the entries of the event are done (see
// writeReleased): the evaluation goes on, and a later run of the event may change the content again.
func (r *pass) releaseChain(e workflow.Entry, ans agent.Answer, warning, reason, key string,
	usable bool) agent.Answer {
	if usable {
		r.released = append(r.released, passReleased{line: e.Line, key: key})
	}
	return r.release(e, ans, warning, reason)
}

// writeReleased writes the observed `blocked` line of each block the loop guard let through in the event, with the
// content hash the entries of the event ended on, the one the next stop starts from when the agent changes nothing,
// and the content the event started from when its runs changed it. So the chain of the blocks of each entry goes on
// (see state.State.Chain) and the next stop on a content the agent left alone is let through too, a block with once
// included, whose line reminded reads as already blocked on that content. It writes nothing when the runs of the
// event left the content as it started, since the next stop then finds the blocks on that same content, nor
// without a known hash.
func (r *pass) writeReleased() {
	if len(r.released) == 0 || r.epoch == 0 {
		return
	}
	hash, known := r.hash()
	if r.waiting() {
		return
	}
	start := r.startOf(hash, known)
	if start == "" {
		return
	}
	for _, b := range r.released {
		_ = r.writeBlocked(b.line, b.key, hash, start)
		if r.waiting() {
			return
		}
	}
}

// release lets the event through for the loop guard, with warning for the user: the block of the entry e is logged
// as loop_guard with its reason, and the event did not end as asked.
func (r *pass) release(e workflow.Entry, ans agent.Answer, warning, reason string) agent.Answer {
	r.guarded = true
	r.log(e.Action.Verb(), e.Line, "loop_guard", reason)
	return ans.Warn(warning)
}

// reminded reports whether the block with once of the entry e already blocked the actor of the event in its
// session on the current branch or on the same HEAD commit, on a content whose files in the scope of the entry did
// not change since: the tree of its latest observed `blocked` line is compared with the current one. An unknown
// hash, a log that cannot be read and two trees that cannot be compared never count as reminded, the last with the
// error in the log: the reminder blocks again.
func (r *pass) reminded(e workflow.Entry) bool {
	ev := r.ev.Event
	branch, head := r.branch(), r.head()
	hash, known := r.hash()
	st, err := r.currentState()
	if r.waiting() || !known {
		return false
	}
	if err != nil {
		r.warnLog(err)
		return false
	}
	q := state.Query{Outcome: state.OutcomeBlocked, Session: ev.Session, Actor: ev.Actor, Line: e.Line, Branch: branch,
		Head: head, ByBranch: true}
	if q.Valid() != nil {
		return false
	}
	last, ok := st.Latest(q)
	if !ok || last.Hash == "" || last.Hash == state.UnknownHash {
		return false
	}
	same, err := r.sameScope(last.Hash, hash, r.whenScope())
	if err != nil {
		r.log(workflow.VerbBlock, e.Line, "warn", fmt.Sprintf(
			"the content it reminded on cannot be compared with the current one (%v): the block blocks again", err))
		return false
	}
	return same
}

// unusable returns what the answer says of a workflow that cannot be used: its problems, the reason of a refusal and
// whether it requires a newer polix. Such a workflow is not broken but ahead of this binary: the answer asks to
// update polix instead of fixing it, and not to change it.
func (r *pass) unusable() (problems, reason string, tooOld bool) {
	problems = r.ev.Invalid.Error()
	reason = "polix: the workflow is not valid; fix it and run polix config check.\n" + problems
	var old workflow.TooOldError
	if !errors.As(r.ev.Invalid, &old) {
		return problems, reason, false
	}
	problems = fmt.Sprintf("this repository needs polix %s or later and the installed one is %s", old.Requires,
		old.Running)
	reason = "polix: " + problems + ". Run polix update (or ask the user to) and do not change " +
		workflow.File + "."
	return problems, reason, true
}

// warnUnusable answers an event with a workflow that cannot be used only with a warning, for the events whose block
// would not help to fix it: a blocked stop would keep the agent in a loop it cannot leave. The warning, as every
// message for the user, goes without the prefix polix:, which the render adds; the pending warnings of the call
// follow it (see warn).
func (r *pass) warnUnusable() agent.Answer {
	problems, _, tooOld := r.unusable()
	if tooOld {
		return r.warn(agent.Answer{}.Warn(problems + ": run polix update"))
	}
	return r.warn(agent.Answer{}.Warn("the workflow is not valid: " + problems))
}

// refuseUnusable refuses the event for a workflow that cannot be used, logged as invalid: edits and commands fail
// closed, so a broken file is noticed at once instead of silently disabling the workflow. The pending warnings of
// the call are logged as not shown (see warn).
func (r *pass) refuseUnusable() agent.Answer {
	problems, reason, _ := r.unusable()
	r.log("invalid", 0, "block", problems)
	return r.warn(agent.Answer{}.Refuse(reason))
}
