package answer

import (
	"fmt"
	"slices"
	"strings"

	"github.com/mlophez/polix-ai/internal/app/state"
	"github.com/mlophez/polix-ai/internal/app/workflow"
)

// condFinished holds when every subagent type of the finished of all finished on the content of the scope of the
// entry: the nearest changed, else the one of its when (see finished). An empty list never holds (see subagentTypes).
func (r *pass) condFinished(all workflow.Cond, scope []string) bool {
	names, ok := r.subagentTypes(all, "finished")
	if !ok {
		return false
	}
	if scope == nil {
		scope = r.whenScope()
	}
	for _, name := range names {
		if !r.finished(name, scope) || r.waiting() {
			return false
		}
	}
	return true
}

// condFinishedSinceCommit holds when every subagent type of the finished_since_commit of all finished on the current
// HEAD (see finishedOnHead), whatever changed in the files since. An empty list never holds (see subagentTypes).
func (r *pass) condFinishedSinceCommit(all workflow.Cond) bool {
	names, ok := r.subagentTypes(all, "finished_since_commit")
	if !ok {
		return false
	}
	for _, name := range names {
		if !r.finishedOnHead(name) || r.waiting() {
			return false
		}
	}
	return true
}

// subagentTypes reads the subagent types of the condition k of all (finished or finished_since_commit), expanded. It
// reports false for an empty list or a type that is blank once expanded, which names no subagent: an empty Key would
// select the lines of any type, so the condition never holds on them.
func (r *pass) subagentTypes(all workflow.Cond, k string) ([]string, bool) {
	names := r.list(all, k)
	if len(names) == 0 {
		return nil, false
	}
	for _, name := range names {
		if strings.TrimSpace(name) == "" {
			return nil, false
		}
	}
	return names, true
}

// finished reports whether the subagent type name finished in the session, whose log alone the state holds, on the
// current branch or on the same HEAD commit, on a content whose files in scope did not change since: the tree of its
// latest observed finished with a hash is compared with the current one (see sameScope). The log is read first, so
// without such a line the content hash is never computed. A log that cannot be read, with a warning, an unknown hash, a
// tree git no longer has or a comparison that fails never holds, the last two with the error in the log.
func (r *pass) finished(name string, scope []string) bool {
	branch, head := r.branch(), r.head()
	q := state.Query{Outcome: state.OutcomeFinished, Branch: branch, Head: head, ByBranch: true, Key: name}
	if q.Valid() != nil {
		return false
	}
	_, last, ok := r.latestFinished(q)
	if !ok {
		return false
	}
	hash, known := r.hash()
	if r.waiting() || !known {
		return false
	}
	same, err := r.sameScope(last.Hash, hash, scope)
	if err != nil {
		r.log("finished", r.entry.Line, "warn", fmt.Sprintf(
			"the content %s finished on cannot be compared with the current one (%v): finished does not hold", name, err))
		return false
	}
	return same
}

// finishedOnHead reports whether the latest observed finished line with a hash of the subagent type name in the
// session, whose log alone the state holds, on any branch, is one the query by HEAD alone (state.Query.ByHead) on the
// current HEAD selects, so a later end on another commit stops it even after HEAD went back. It never computes the
// content. An unknown HEAD, no line with a hash, one without HEAD or a log that cannot be read (warned) never holds.
func (r *pass) finishedOnHead(name string) bool {
	head := r.head()
	all := state.Query{Outcome: state.OutcomeFinished, Key: name}
	q := state.Query{Outcome: state.OutcomeFinished, Head: head, ByHead: true, Key: name}
	if all.Valid() != nil || q.Valid() != nil {
		return false
	}
	st, last, ok := r.latestFinished(all)
	return ok && slices.ContainsFunc(st.Observed(q), func(m state.Match) bool { return m.Seq == last.Seq })
}

// latestFinished reads the log of the session and returns it with the latest observation with a hash that q selects;
// ok is false when the pass waits for a step, the log cannot be read (warned) or q selects no observation with a
// hash.
func (r *pass) latestFinished(q state.Query) (state.State, state.Match, bool) {
	st, err := r.currentState()
	if r.waiting() {
		return st, state.Match{}, false
	}
	if err != nil {
		r.warnLog(err)
		return st, state.Match{}, false
	}
	var last state.Match
	for _, m := range st.Observed(q) {
		if m.Hash != "" {
			last = m
		}
	}
	return st, last, last.Hash != ""
}
