package answer

import (
	"fmt"
	"path/filepath"
	"slices"

	"github.com/mlophez/polix-ai/internal/app/agent"
	"github.com/mlophez/polix-ai/internal/app/hook"
	"github.com/mlophez/polix-ai/internal/app/state"
	"github.com/mlophez/polix-ai/internal/app/workflow"
)

// maxCommandEdits is how many paths a command may change, the protected ones aside, and still have each evaluated
// as an edit; past it the session would be flooded, so the agent only gets the count.
const maxCommandEdits = 25

// watches reports whether the workflow applies something to an edit, guardrails.protected_files or an entry on
// pre_edit or post_edit (format_on_edit already expanded into one), so the files a command changes are worth
// finding: only then does a command cost the content hash before and after it.
func watches(wf workflow.Workflow) bool {
	return len(wf.ProtectedFiles) > 0 || len(wf.On[workflow.PreEdit]) > 0 || len(wf.On[workflow.PostEdit]) > 0
}

// commandKey is the key that pairs the snapshot of a command with its post_command: the id the agent gives the call
// of the tool, the same before and after it, else the command line.
func commandKey(ev agent.Event) string {
	if ev.ToolUse != "" {
		return ev.ToolUse
	}
	return ev.Command
}

// snapshot records the content right before a command runs, on the last event before it of the call (see
// lastBefore), when the valid workflow watches and nothing blocked the line: an observed `snapshot` line with the
// content hash of the epoch the entries left, the HEAD commit, the refs git moves without HEAD (hook.Facts.Refs)
// and the key of the command. An unknown hash writes none, with the warning of the call.
func (r *pass) snapshot() {
	key := commandKey(r.ev.Event)
	if key == "" || !watches(r.guardrails()) {
		return
	}
	hash, known := r.hash()
	refs := r.refs()
	if r.waiting() || !known {
		return
	}
	_ = r.writeEntry(state.KindObserved, "", 0, state.OutcomeSnapshot, "", key, hash, refs)
}

// watch finds the files the command of a post_command changed, when the valid workflow watches: the latest
// `snapshot` line of the session, the actor and the key of the command is compared with the current content, unless
// a `watched` line of the same key came after it. Any post_command that finds it writes that `watched` line, so a
// snapshot is compared once: a later post_command of the same key without a snapshot of its own (a pre_command that
// did not reach polix) finds none. When HEAD, the stash or an operation stopped halfway moved since the snapshot (a
// pull, a checkout, a commit, a git stash, a merge stopped on its conflicts) the changes are git's: nothing is done,
// and the skip is logged as builtin. The paths of
// guardrails.protected_files the command changed are put back as the snapshot holds them, removed when it did not
// hold them, and the command is refused once per path, logged as a denial of guardrails.protected_files; any restore
// may rewrite files, so it starts a new epoch. The other paths that still exist become the edits the shell evaluates
// next in the same call (see derive). Without a snapshot, with an unknown hash or with a log that cannot be read
// nothing happens; two trees that cannot be compared warn the user.
func (r *pass) watch() agent.Answer {
	ev := r.ev.Event
	key := commandKey(ev)
	if key == "" || !watches(r.guardrails()) {
		return agent.Answer{}
	}
	st, err := r.currentState()
	if r.waiting() || err != nil {
		return agent.Answer{}
	}
	q := state.Query{Outcome: state.OutcomeSnapshot, Session: ev.Session, Actor: ev.Actor, Key: key}
	consumed := state.Query{Outcome: state.OutcomeWatched, Session: ev.Session, Actor: ev.Actor, Key: key}
	if q.Valid() != nil || consumed.Valid() != nil {
		return agent.Answer{}
	}
	at := st.Index(q)
	if at < 0 || st.Index(consumed) > at || st.Entries[at].Hash == "" {
		return agent.Answer{}
	}
	snap := st.Entries[at]
	_ = r.write(state.KindObserved, "", 0, state.OutcomeWatched, "", key, "")
	head, refs := r.head(), r.refs()
	if r.waiting() {
		return agent.Answer{}
	}
	if moved := gitMoved(snap, head, refs); moved != "" {
		r.log(builtinProtection, 0, "skip", moved+" while the command ran: its changes are git's, polix does not "+
			"check them")
		return agent.Answer{}
	}
	paths, removed := r.commandChanges(snap.Hash)
	if r.waiting() {
		return agent.Answer{}
	}
	protected, others := r.protected(paths)
	ans := r.restore(snap.Hash, protected, removed)
	if r.waiting() {
		return agent.Answer{}
	}
	return r.derive(slices.DeleteFunc(others, func(p string) bool { return slices.Contains(removed, p) }), ans)
}

// gitMoved says what git moved since the snapshot snap, given the current HEAD commit and the refs git moves
// without HEAD (the stash and an operation stopped halfway, see hook.Facts.Refs): "" when neither moved.
func gitMoved(snap state.Entry, head, refs string) string {
	switch {
	case head != snap.Head:
		return fmt.Sprintf("HEAD moved from %q to %q", snap.Head, head)
	case refs != snap.Refs:
		return fmt.Sprintf("the stash or the operation of git in progress went from %q to %q", snap.Refs, refs)
	}
	return ""
}

// commandChanges returns the paths that differ between the tree of the snapshot and the current content, and those
// of them the current content no longer holds: none with an unknown hash, and none with a warning for the user when
// the trees cannot be compared.
func (r *pass) commandChanges(snapshot string) (paths, removed []string) {
	hash, known := r.hash()
	if r.waiting() || !known {
		return nil, nil
	}
	paths, removed, err := r.treeDiff(snapshot, hash)
	if r.waiting() {
		return nil, nil
	}
	if err != nil {
		r.warnings = append(r.warnings, fmt.Sprintf(
			"polix cannot compare the content before and after the command (%v): it did not check the files it changed",
			err))
		return nil, nil
	}
	return paths, removed
}

// protected splits paths into those of guardrails.protected_files and the others, each in their order.
func (r *pass) protected(paths []string) (protected, others []string) {
	for _, p := range paths {
		if anyPathFold(r.guardrails().ProtectedFiles, p, r.ev.Fold()) {
			protected = append(protected, p)
		} else {
			others = append(others, p)
		}
	}
	return protected, others
}

// derive turns the paths a command changed that are not protected and still exist into the edits the shell evaluates next in the
// same call, from ans: a pre_edit and then a post_edit of each path, in order, made by the command
// (agent.Event.ByCommand) in the session, by the actor and the agent of the post_command, located inside the root.
// Past maxCommandEdits paths, as after an install or a generator, none is derived: the agent gets the count as
// context instead, logged as an inform of builtin.
func (r *pass) derive(paths []string, ans agent.Answer) agent.Answer {
	if len(paths) > maxCommandEdits {
		text := fmt.Sprintf("A command changed %d files, more than %d: polix did not evaluate them as edits, so the "+
			"entries of %s and %s did not apply to them.", len(paths), maxCommandEdits, workflow.PreEdit,
			workflow.PostEdit)
		r.log(builtinProtection, 0, "inform", text)
		return ans.Inform(text)
	}
	ev := r.ev.Event
	for _, p := range paths {
		for _, name := range []string{workflow.PreEdit, workflow.PostEdit} {
			e := agent.Event{Name: name, Agent: ev.Agent, Session: ev.Session, Actor: ev.Actor, Cwd: ev.Cwd,
				AbsPath: absolute(r.ev.Root, "", filepath.FromSlash(p)), ByCommand: true}
			e = e.Locate(p, true)
			if e.Valid() == nil {
				r.follow = append(r.follow, e)
			}
		}
	}
	return ans
}

// restore puts the protected paths back as the tree source holds them, one step per path so each gets its own
// outcome, those the command removed last: on a file system that folds case, a path the command renamed only in case
// is removed before its own name is written back. Then it starts a new epoch, since the files changed, and refuses
// the command once per path, each logged as a denial of guardrails.protected_files. A path that could not be put back
// is refused with its error, and the user is warned to put it back by hand.
func (r *pass) restore(source string, paths, removed []string) agent.Answer {
	if len(paths) == 0 {
		return agent.Answer{}
	}
	order := slices.Clone(paths)
	slices.SortStableFunc(order, func(a, b string) int {
		return boolOrder(slices.Contains(removed, a), slices.Contains(removed, b))
	})
	errs := make(map[string]error, len(order))
	for _, p := range order {
		s := hook.Step{Restore: []string{p}, Source: source}
		o := r.act(s, s.Valid())
		if r.waiting() {
			return agent.Answer{}
		}
		errs[p] = o.Err
	}
	r.epoch++
	var ans agent.Answer
	for _, p := range order {
		reason := fmt.Sprintf("%s is protected in %s: a command changed it and polix put it back.", p, workflow.File)
		if err := errs[p]; err != nil {
			reason = fmt.Sprintf("%s is protected in %s: a command changed it and polix could not put it back (%v).",
				p, workflow.File, err)
			r.warnings = append(r.warnings, fmt.Sprintf(
				"polix could not put back %s, protected in %s, after a command changed it (%v): put it back by hand",
				p, workflow.File, err))
		}
		denied, _ := r.deny(protectedFilesShortcut, 0, reason)
		ans = ans.Merge(denied)
	}
	return ans
}

// boolOrder orders false before true.
func boolOrder(a, b bool) int {
	switch {
	case a == b:
		return 0
	case a:
		return 1
	}
	return -1
}
