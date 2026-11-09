package load

import (
	"errors"
	"testing"

	"github.com/mlophez/polix-ai/internal/app/state"
)

// The entries and the count of skipped lines make the state, kept as read.
func TestDecide(t *testing.T) {
	entries := []state.Entry{{Kind: "fact", Fact: "reviewed"}, {Key: "x"}}
	out := Decide(entries, 1, nil)
	if len(out.Loaded.State.Entries) != 2 || out.Loaded.State.Entries[1].Key != "x" ||
		out.Loaded.State.Skipped != 1 || out.Loaded.Err != nil {
		t.Fatalf("got %+v", out.Loaded)
	}
	if out := Decide(nil, 0, nil); len(out.Loaded.State.Entries) != 0 || out.Loaded.Err != nil {
		t.Fatalf("an empty log: %+v", out.Loaded)
	}
}

// A log that cannot be read loads an empty state with why.
func TestDecideUnreadable(t *testing.T) {
	boom := errors.New("boom")
	out := Decide([]state.Entry{{Kind: state.KindDecision}}, 1, boom)
	if len(out.Loaded.State.Entries) != 0 || out.Loaded.State.Skipped != 0 || !errors.Is(out.Loaded.Err, boom) {
		t.Fatalf("got %+v", out.Loaded)
	}
}

// A state that is not valid loads an empty state with why.
func TestDecideInvalid(t *testing.T) {
	out := Decide([]state.Entry{{Kind: state.KindDecision}}, -1, nil)
	if len(out.Loaded.State.Entries) != 0 || out.Loaded.Err == nil {
		t.Fatalf("got %+v", out.Loaded)
	}
}
