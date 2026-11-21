package state

import (
	"strings"
	"testing"
)

func TestEntryValid(t *testing.T) {
	if err := (Entry{}).Valid(); err == nil {
		t.Fatal("an entry needs a kind")
	}
	if err := (Entry{Kind: KindDecision, Session: "s", Line: -1}).Valid(); err == nil {
		t.Fatal("a negative line")
	}
	if err := (Entry{Kind: "newer_kind", Session: "s"}).Valid(); err != nil {
		t.Fatalf("a kind of a newer polix is kept: %v", err)
	}
	// The fact and the rule of the lines an older polix wrote are only read, never written.
	if err := (Entry{Kind: "fact", Session: "s", Fact: "reviewed"}).Valid(); err == nil {
		t.Fatal("an entry with the fact of an older polix")
	}
	if err := (Entry{Kind: "injected", Session: "s", Rule: "GO-2"}).Valid(); err == nil {
		t.Fatal("an entry with the rule of an older polix")
	}
}

// An entry without a session is never written: there is no file for it.
func TestEntryValidWithoutSession(t *testing.T) {
	err := (Entry{Kind: KindDecision, Outcome: "allow"}).Valid()
	if err == nil || !strings.Contains(err.Error(), "session") {
		t.Fatalf("an entry without session: %v", err)
	}
	if err := (Entry{Kind: KindDecision, Session: "s"}).Valid(); err != nil {
		t.Fatal(err)
	}
}

// Chained keeps the content the event started from only on a blocked line whose event changed a known content.
func TestEntryChained(t *testing.T) {
	blocked := Entry{Kind: KindObserved, Outcome: OutcomeBlocked, Line: 4, Hash: "h"}
	if got := blocked.Chained("g"); got.Before != "g" {
		t.Fatalf("a run that changed the content: %+v", got)
	}
	unknown := blocked
	unknown.Hash = UnknownHash
	passed := Entry{Kind: KindObserved, Outcome: OutcomePassed, Hash: "h"}
	for _, c := range []struct {
		e      Entry
		before string
	}{{blocked, "h"}, {blocked, ""}, {blocked, UnknownHash}, {unknown, "g"}, {passed, "g"}} {
		if got := c.e.Chained(c.before); got.Before != "" {
			t.Errorf("%+v with %q carries before: %+v", c.e, c.before, got)
		}
	}
}

// An observed entry needs a known outcome, and the outcomes about a content need its hash; a finished may go
// without it. Only a blocked line carries the hash its event started from.
func TestObservedEntryValid(t *testing.T) {
	valid := []Entry{
		{Kind: KindObserved, Outcome: OutcomeFinished, Key: "polix:reviewer", Hash: "h"},
		{Kind: KindObserved, Outcome: OutcomeFinished, Key: "polix:reviewer"},
		{Kind: KindObserved, Outcome: OutcomePassed, Key: "just test", Hash: "h"},
		{Kind: KindObserved, Outcome: OutcomeBlocked, Line: 4, Hash: "h"},
		{Kind: KindObserved, Outcome: OutcomeBlocked, Line: 4, Hash: "h", Before: "g"},
		{Kind: KindObserved, Outcome: OutcomeInjected, Key: "x"},
		{Kind: KindObserved, Outcome: OutcomeStarted},
		{Kind: KindObserved, Outcome: OutcomeSnapshot, Key: "toolu_1", Hash: "h", Head: "c"},
		{Kind: KindObserved, Outcome: OutcomeSnapshot, Key: "toolu_1", Hash: "h", Head: "c", Refs: "s"},
		{Kind: KindObserved, Outcome: OutcomeWatched, Key: "toolu_1"},
	}
	for _, e := range valid {
		e.Session = "s"
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
		{Kind: KindObserved, Outcome: OutcomePassed, Key: "just test", Hash: "h", Before: "g"},
		{Kind: KindDecision, Outcome: "block", Before: "g"},
	}
	for _, e := range invalid {
		e.Session = "s"
		if err := e.Valid(); err == nil {
			t.Errorf("%+v is valid", e)
		}
	}
}
