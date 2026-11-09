package hook

import (
	"testing"

	"github.com/mlophez/polix-ai/internal/app/state"
)

// The next event starts after the runs of this one, with the log as it left it.
func TestFactsCarry(t *testing.T) {
	s := state.State{Entries: []state.Entry{{Kind: state.KindObserved, Outcome: state.OutcomeStarted}}}
	f := Facts{HasState: true, StateAt: 3}.Carry(2, s)
	if f.Epoch != 2 || len(f.State.Entries) != 1 || f.StateAt != 0 {
		t.Fatalf("%+v", f)
	}
	if g := (Facts{}).Carry(1, s); len(g.State.Entries) != 0 {
		t.Fatal("a log not read stays unread")
	}
}
