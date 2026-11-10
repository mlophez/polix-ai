package hook

import (
	"slices"
	"testing"

	"github.com/mlophez/polix-ai/internal/app/agent"
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

// The next event knows the answer of this one: the messages of an answer that does not block become warnings of the
// call, once each, and a block marks the call as blocked without adding its messages.
func TestFactsAnswered(t *testing.T) {
	f := Facts{Warnings: []string{"a"}}.Answered(agent.Answer{}.Warn("a").Warn("b"), nil)
	if f.Blocked || !slices.Equal(f.Warnings, []string{"a", "b"}) {
		t.Fatalf("%+v", f)
	}
	g := f.Answered(agent.Answer{}.Refuse("r").Warn("c"), nil)
	if !g.Blocked || !slices.Equal(g.Warnings, []string{"a", "b"}) {
		t.Fatalf("%+v", g)
	}
	if h := g.Answered(agent.Answer{}, nil); !h.Blocked {
		t.Fatal("a call stays blocked")
	}
}

// The lines written only when the whole call does not block carry to the next event in place of the earlier ones,
// unshared with the caller; a block of the call drops them, and a pending line that cannot be appended is invalid.
func TestFactsPending(t *testing.T) {
	notice := state.Entry{Kind: state.KindObserved, Outcome: state.OutcomeNotified, Session: "s", Line: 4, Key: "Aviso"}
	entries := []state.Entry{notice}
	earlier := Facts{Pending: []state.Entry{{Kind: state.KindObserved, Outcome: state.OutcomeNotified}}}
	f := earlier.Carry(1, state.State{}).Answered(agent.Answer{}.Warn("Aviso"), entries)
	entries[0].Key = "otro"
	if len(f.Pending) != 1 || f.Pending[0].Key != "Aviso" || f.Valid() != nil {
		t.Fatalf("an answer that does not block keeps them: %+v", f.Pending)
	}
	if g := earlier.Answered(agent.Answer{}.Refuse("r"), entries); len(g.Pending) != 0 || !g.Blocked {
		t.Fatalf("a block drops them: %+v", g)
	}
	if (Facts{Pending: []state.Entry{{Kind: state.KindObserved, Outcome: "x"}}}).Valid() == nil {
		t.Fatal("a pending line with an unknown outcome is valid")
	}
}

// The branch and the HEAD commit are learned together, an unknown commit included, and both carry to the next event.
func TestFactsLearnBranch(t *testing.T) {
	f := Facts{}.learnBranch("feat/x", "c0ffee").Carry(1, state.State{})
	if f.Branch != "feat/x" || f.Head != "c0ffee" || !f.HasBranch {
		t.Fatalf("%+v", f)
	}
	if g := (Facts{}).learnBranch("", ""); g.Branch != "" || g.Head != "" || !g.HasBranch {
		t.Fatalf("a detached HEAD without commit: %+v", g)
	}
}
