package log

import (
	"reflect"
	"testing"

	"github.com/mlophez/polix-ai/internal/app/state"
)

// session is the state of the file of the session id, with one entry at each time of ats, in that order.
func session(id string, ats ...string) state.State {
	var st state.State
	for _, at := range ats {
		st.Entries = append(st.Entries, state.Entry{At: at, Kind: state.KindDecision, Session: id})
	}
	return st
}

// ids returns the id of each session of the output, in order, and checks they are the sessions of the output.
func ids(t *testing.T, out Output) []string {
	t.Helper()
	if len(out.IDs) != len(out.Sessions) {
		t.Fatalf("%d ids for %d sessions", len(out.IDs), len(out.Sessions))
	}
	for i, st := range out.Sessions {
		if st.Entries[0].Session != out.IDs[i] {
			t.Fatalf("id %q for the session %q", out.IDs[i], st.Entries[0].Session)
		}
	}
	return out.IDs
}

// AC7, AC8: the sessions are ordered by the latest time of their entries, most recent last, whatever the offset of
// the clock and the order of their lines; the last tail are kept whole, all of them with 0 or less.
func TestDecideOrdersTheSessions(t *testing.T) {
	sessions := []state.State{
		// 10:00+01:00 is 09:00Z, after 10:30+02:00, 08:30Z, though it reads before.
		session("b", "2026-11-12T08:00:00Z", "2026-11-12T10:00:00+01:00"),
		session("c", "2026-11-12T09:30:00Z", "2026-11-12T07:00:00Z"),
		session("a", "2026-11-12T10:30:00+02:00"),
	}
	for tail, want := range map[int][]string{0: {"a", "b", "c"}, -1: {"a", "b", "c"}, 2: {"b", "c"}, 1: {"c"},
		3: {"a", "b", "c"}, 9: {"a", "b", "c"}} {
		out := Decide(sessions, tail)
		if got := ids(t, out); !reflect.DeepEqual(got, want) {
			t.Fatalf("tail %d: %v", tail, got)
		}
		for _, st := range out.Sessions {
			if st.Entries[0].Session == "b" && len(st.Entries) != 2 {
				t.Fatalf("tail %d: the session b is not whole: %+v", tail, st)
			}
		}
	}
	if out := Decide(nil, 5); len(out.Sessions) != 0 || len(out.IDs) != 0 || out.Skipped != 0 {
		t.Fatalf("no sessions: %+v", out)
	}
}

// A session without a time that parses sorts first, sessions of the same latest time sort by id, and a file without
// entries shows no session but counts its skipped lines, summed with those of every file.
func TestDecideUnknownTimesAndTies(t *testing.T) {
	empty := state.State{Skipped: 2}
	late := session("late", "2026-11-12T09:00:00Z")
	late.Skipped = 1
	sessions := []state.State{
		late,
		session("y", "2026-11-12T08:00:00Z"),
		empty,
		session("x", "2026-11-12T09:00:00+01:00"),
		session("old", "t0", "not a time"),
	}
	out := Decide(sessions, 0)
	if got, want := ids(t, out), []string{"old", "x", "y", "late"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("order: %v", got)
	}
	if out.Skipped != 3 {
		t.Fatalf("skipped %d", out.Skipped)
	}
}
