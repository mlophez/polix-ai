package state

import (
	"strings"
	"testing"
)

// Done never holds for an unknown hash, and holds for the fact on its branch and hash.
func TestDone(t *testing.T) {
	s := State{Entries: []Entry{{Kind: KindFact, Fact: "tested", Branch: "main", Hash: "h"}}}
	if s.Done("tested", "main", "") {
		t.Fatal("done holds for an unknown hash")
	}
	if !s.Done("tested", "main", "h") || s.Done("tested", "dev", "h") || s.Done("tested", "main", "g") {
		t.Fatal("done on branch and hash")
	}
}

func TestInjected(t *testing.T) {
	s := State{Entries: []Entry{{Kind: KindInjected, Rule: "GO-2", Session: "s"}}}
	if !s.Injected("GO-2", "s") || s.Injected("GO-2", "t") || s.Injected("GO-3", "s") {
		t.Fatal("injected per rule and session")
	}
}

// StopBlocks counts per session, event, actor, line and hash.
func TestStopBlocks(t *testing.T) {
	b := Entry{Kind: KindStopBlock, Session: "s", Event: "stop", Actor: "session", Line: 4}
	other := b
	other.Actor = "reviewer"
	s := State{Entries: []Entry{b, b, other, {Kind: KindDecision, Session: "s", Event: "stop", Actor: "session", Line: 4}}}
	if n := s.StopBlocks("s", "stop", "session", 4, ""); n != 2 {
		t.Fatalf("got %d", n)
	}
	if n := s.StopBlocks("s", "stop", "session", 5, ""); n != 0 {
		t.Fatalf("other line: %d", n)
	}
}

// Append returns a copy: the original does not see the new entry.
func TestAppend(t *testing.T) {
	s := State{Entries: make([]Entry, 0, 4)}
	got, err := s.Append(Entry{Kind: KindFact, Fact: "a"})
	if err != nil || len(got.Entries) != 1 || len(s.Entries) != 0 {
		t.Fatalf("%v %+v", err, got)
	}
	if _, err := s.Append(Entry{}); err == nil {
		t.Fatal("an entry without kind")
	}
}

func TestStateValid(t *testing.T) {
	if err := (State{Skipped: -1}).Valid(); err == nil || !strings.Contains(err.Error(), "negative") {
		t.Fatal(err)
	}
	if err := (State{Entries: []Entry{{Kind: KindFact}}, Skipped: 1}).Valid(); err != nil {
		t.Fatal(err)
	}
}
