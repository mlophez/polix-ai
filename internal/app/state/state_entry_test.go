package state

import "testing"

func TestEntryValid(t *testing.T) {
	if err := (Entry{}).Valid(); err == nil {
		t.Fatal("an entry needs a kind")
	}
	if err := (Entry{Kind: KindDecision, Line: -1}).Valid(); err == nil {
		t.Fatal("a negative line")
	}
	if err := (Entry{Kind: "newer_kind"}).Valid(); err != nil {
		t.Fatalf("a kind of a newer polix is kept: %v", err)
	}
	// The fact and the rule of the lines an older polix wrote are only read, never written.
	if err := (Entry{Kind: "fact", Fact: "reviewed"}).Valid(); err == nil {
		t.Fatal("an entry with the fact of an older polix")
	}
	if err := (Entry{Kind: "injected", Rule: "GO-2"}).Valid(); err == nil {
		t.Fatal("an entry with the rule of an older polix")
	}
}

// An observed entry needs a known outcome, and the outcomes about a content need its hash; a finished may go
// without it.
func TestObservedEntryValid(t *testing.T) {
	valid := []Entry{
		{Kind: KindObserved, Outcome: OutcomeFinished, Key: "polix:reviewer", Hash: "h"},
		{Kind: KindObserved, Outcome: OutcomeFinished, Key: "polix:reviewer"},
		{Kind: KindObserved, Outcome: OutcomePassed, Key: "just test", Hash: "h"},
		{Kind: KindObserved, Outcome: OutcomeBlocked, Line: 4, Hash: "h"},
		{Kind: KindObserved, Outcome: OutcomeInjected, Key: "x"},
		{Kind: KindObserved, Outcome: OutcomeStarted},
		{Kind: KindObserved, Outcome: OutcomeSnapshot, Key: "toolu_1", Hash: "h", Head: "c"},
		{Kind: KindObserved, Outcome: OutcomeSnapshot, Key: "toolu_1", Hash: "h", Head: "c", Refs: "s"},
		{Kind: KindObserved, Outcome: OutcomeWatched, Key: "toolu_1"},
	}
	for _, e := range valid {
		if err := e.Valid(); err != nil {
			t.Errorf("%+v: %v", e, err)
		}
	}
	invalid := []Entry{
		{Kind: KindObserved},
		{Kind: KindObserved, Outcome: "fact"},
		{Kind: KindObserved, Outcome: OutcomePassed, Key: "just test"},
		{Kind: KindObserved, Outcome: OutcomeBlocked, Line: 4},
		{Kind: KindObserved, Outcome: OutcomeSnapshot, Key: "toolu_1", Head: "c"},
		{Kind: KindObserved, Outcome: OutcomeWatched, Key: "toolu_1", Refs: "s"},
		{Kind: KindDecision, Outcome: "deny", Refs: "s"},
	}
	for _, e := range invalid {
		if err := e.Valid(); err == nil {
			t.Errorf("%+v is valid", e)
		}
	}
}
