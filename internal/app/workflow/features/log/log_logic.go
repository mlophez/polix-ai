package log

import (
	"cmp"
	"slices"
	"strings"
	"time"

	"github.com/mlophez/polix-ai/internal/app/state"
)

// Output is the decision of the logic of LogWorkflow: the sessions of the log to show, the most recently active
// last, each whole and in file order, with the id of each at the same index, and the lines skipped in every file.
// A session asked for by its id that cannot be chosen is refused as data: NoMatch, or Matches.
type Output struct {
	Sessions []state.State
	IDs      []string // the id of each session of Sessions, the session of its first entry
	NoMatch  bool     // the session asked for matches no session
	Matches  []string // the ids, sorted, of the several sessions the session asked for matches; else empty
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
// last, a session without a time that parses first and the ties by id.
// With session it keeps only the session whose id equals it, else the only one whose id starts with it; with none
// Output.NoMatch is set, with several Output.Matches holds their ids sorted, and no session is shown.
// Else it keeps the last tail of them, or all of them when all is set, tail is 0 or less or there are no more.
// A file without entries shows no session, but its skipped lines count.
func Decide(sessions []state.State, tail int, all bool, session string) Output {
	var out Output
	var ranking []ranked
	for _, st := range sessions {
		out.Skipped += st.Skipped
		if len(st.Entries) == 0 {
			continue
		}
		r := ranked{st: st, id: st.Entries[0].Session}
		r.latest, r.known = latest(st)
		ranking = append(ranking, r)
	}
	slices.SortStableFunc(ranking, compare)
	if session != "" {
		ranking = selectSession(&out, ranking, session)
	} else if !all && tail > 0 && len(ranking) > tail {
		ranking = ranking[len(ranking)-tail:]
	}
	for _, r := range ranking {
		out.Sessions = append(out.Sessions, r.st)
		out.IDs = append(out.IDs, r.id)
	}
	return out
}

// selectSession returns the session of ranking whose id equals id, else the only one whose id starts with it. With
// none it sets out.NoMatch, with several it sets out.Matches to their ids sorted; both return no session.
func selectSession(out *Output, ranking []ranked, id string) []ranked {
	var prefixed []ranked
	for _, r := range ranking {
		if r.id == id {
			return []ranked{r}
		}
		if strings.HasPrefix(r.id, id) {
			prefixed = append(prefixed, r)
		}
	}
	switch len(prefixed) {
	case 0:
		out.NoMatch = true
	case 1:
		return prefixed
	default:
		for _, r := range prefixed {
			out.Matches = append(out.Matches, r.id)
		}
		slices.Sort(out.Matches)
	}
	return nil
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
