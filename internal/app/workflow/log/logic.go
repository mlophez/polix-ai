package log

import (
	"cmp"
	"slices"
	"time"

	"github.com/mlophez/polix-ai/internal/app/state"
)

// Output is the decision of the logic of LogWorkflow: the sessions of the log to show, the most recently active
// last, each whole and in file order, with the id of each at the same index, and the lines skipped in every file.
type Output struct {
	Sessions []state.State
	IDs      []string // the id of each session of Sessions, the session of its first entry
	Skipped  int      // the lines over state.MaxLine skipped in the file of every session, shown or not
}

// ranked is a session with what orders it: its id and the latest time of its entries, when one parses.
type ranked struct {
	st     state.State
	id     string
	latest time.Time
	known  bool
}

// Decide picks the sessions of the log to show, given the state of the file of each session: those with entries,
// ordered by the latest time of their entries (RFC 3339, so the offsets of the clock compare right), the most recent
// last, a session without a time that parses first and the ties by id; then the last tail of them, or all when tail
// is 0 or less or there are no more. A file without entries shows no session, but its skipped lines count.
func Decide(sessions []state.State, tail int) Output {
	var out Output
	var all []ranked
	for _, st := range sessions {
		out.Skipped += st.Skipped
		if len(st.Entries) == 0 {
			continue
		}
		r := ranked{st: st, id: st.Entries[0].Session}
		r.latest, r.known = latest(st)
		all = append(all, r)
	}
	slices.SortStableFunc(all, compare)
	if tail > 0 && len(all) > tail {
		all = all[len(all)-tail:]
	}
	for _, r := range all {
		out.Sessions = append(out.Sessions, r.st)
		out.IDs = append(out.IDs, r.id)
	}
	return out
}

// latest returns the latest time of the entries of st that parses as RFC 3339, and whether one does.
func latest(st state.State) (time.Time, bool) {
	var last time.Time
	known := false
	for _, e := range st.Entries {
		at, err := time.Parse(time.RFC3339, e.At)
		if err != nil {
			continue
		}
		if !known || at.After(last) {
			last, known = at, true
		}
	}
	return last, known
}

// compare orders two sessions by their last activity: one without a known time first, then by the latest time,
// then by id.
func compare(a, b ranked) int {
	if a.known != b.known {
		if a.known {
			return 1
		}
		return -1
	}
	if c := a.latest.Compare(b.latest); a.known && c != 0 {
		return c
	}
	return cmp.Compare(a.id, b.id)
}
