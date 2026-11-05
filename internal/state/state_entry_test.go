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
}
