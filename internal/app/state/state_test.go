package state

import (
	"strings"
	"testing"
)

// The query selects the observed entries of its outcome and of every field it sets, in file order; never another
// kind, another branch or another hash, and never an empty hash.
func TestObserved(t *testing.T) {
	finished := Entry{Kind: KindObserved, Outcome: OutcomeFinished, Session: "s", Actor: "polix:reviewer",
		Event: "subagent_stop", Branch: "main", Key: "polix:reviewer", Hash: "h1"}
	later := finished
	later.Hash = "h2"
	other := finished
	other.Branch, other.Hash = "dev", "h3"
	blocked := Entry{Kind: KindObserved, Outcome: OutcomeBlocked, Session: "s", Branch: "main", Line: 7, Hash: "h1"}
	fact := Entry{Kind: "fact", Outcome: OutcomeFinished, Branch: "main", Key: "polix:reviewer", Hash: "h1"}
	decision := Entry{Kind: KindDecision, Outcome: OutcomeFinished, Branch: "main", Key: "polix:reviewer"}
	s := State{Entries: []Entry{finished, fact, blocked, later, other, decision}}

	got := s.Observed(Query{Outcome: OutcomeFinished, Branch: "main", Key: "polix:reviewer"})
	if len(got) != 2 || got[0].Hash != "h1" || got[1].Hash != "h2" {
		t.Fatalf("finished on main: %+v", got)
	}
	if got := s.Observed(Query{Outcome: OutcomeFinished, Branch: "dev"}); len(got) != 1 || got[0].Hash != "h3" {
		t.Fatalf("another branch: %+v", got)
	}
	if got := s.Observed(Query{Outcome: OutcomeFinished, Key: "polix:builder"}); len(got) != 0 {
		t.Fatalf("another key: %+v", got)
	}
	if got := s.Observed(Query{Outcome: OutcomeBlocked, Line: 7, Hash: "h1", ByHash: true}); len(got) != 1 {
		t.Fatalf("by line and hash: %+v", got)
	}
	if got := s.Observed(Query{Outcome: OutcomeBlocked, Hash: "h2", ByHash: true}); len(got) != 0 {
		t.Fatalf("another hash: %+v", got)
	}
	if got := s.Observed(Query{Outcome: OutcomeFinished, ByHash: true}); len(got) != 0 {
		t.Fatalf("an empty hash never matches: %+v", got)
	}
	if got := s.Observed(Query{Outcome: "fact"}); got != nil {
		t.Fatalf("a query that is not valid selects nothing: %+v", got)
	}
	if e, ok := s.Latest(Query{Outcome: OutcomeFinished, Branch: "main"}); !ok || e.Hash != "h2" {
		t.Fatalf("latest: %+v %v", e, ok)
	}
	if _, ok := s.Latest(Query{Outcome: OutcomePassed}); ok {
		t.Fatal("latest of nothing")
	}
}

// A query needs a known outcome and a line that is not negative.
func TestQueryValid(t *testing.T) {
	if err := (Query{Outcome: OutcomePassed}).Valid(); err != nil {
		t.Fatal(err)
	}
	if (Query{}).Valid() == nil || (Query{Outcome: "fact"}).Valid() == nil ||
		(Query{Outcome: OutcomeBlocked, Line: -1}).Valid() == nil {
		t.Fatal("a query without a known outcome or with a negative line")
	}
}

// The query selects by each field it sets, and leaves out the entries that differ in that field only.
func TestObservedByEachField(t *testing.T) {
	base := Entry{Kind: KindObserved, Outcome: OutcomeBlocked, Session: "s", Actor: "session", Event: "stop",
		Branch: "main", Key: "k", Line: 4, Hash: "h"}
	cases := []struct {
		name   string
		q      Query
		change func(*Entry)
	}{
		{"outcome", Query{Outcome: OutcomeBlocked}, func(e *Entry) { e.Outcome = OutcomePassed }},
		{"session", Query{Outcome: OutcomeBlocked, Session: "s"}, func(e *Entry) { e.Session = "t" }},
		{"actor", Query{Outcome: OutcomeBlocked, Actor: "session"}, func(e *Entry) { e.Actor = "polix:reviewer" }},
		{"event", Query{Outcome: OutcomeBlocked, Event: "stop"}, func(e *Entry) { e.Event = "subagent_stop" }},
		{"branch", Query{Outcome: OutcomeBlocked, Branch: "main"}, func(e *Entry) { e.Branch = "dev" }},
		{"exact branch", Query{Outcome: OutcomeBlocked, Branch: "main", ByBranch: true}, func(e *Entry) { e.Branch = "" }},
		{"key", Query{Outcome: OutcomeBlocked, Key: "k"}, func(e *Entry) { e.Key = "other" }},
		{"line", Query{Outcome: OutcomeBlocked, Line: 4}, func(e *Entry) { e.Line = 5 }},
		{"hash", Query{Outcome: OutcomeBlocked, Hash: "h", ByHash: true}, func(e *Entry) { e.Hash = "h2" }},
		{"kind", Query{Outcome: OutcomeBlocked}, func(e *Entry) { e.Kind = "stop_block" }},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			other := base
			c.change(&other)
			s := State{Entries: []Entry{other, base, other}}
			if got := s.Observed(c.q); len(got) != 1 || got[0] != base {
				t.Fatalf("selected %+v", got)
			}
			if i := s.Index(c.q); i != 1 {
				t.Fatalf("index %d", i)
			}
		})
	}
}

// With ByBranch an empty branch, that of a detached HEAD, selects only the entries without branch; without it, an
// empty branch selects any.
func TestObservedByBranchDetached(t *testing.T) {
	detached := Entry{Kind: KindObserved, Outcome: OutcomePassed, Key: "k", Hash: "h"}
	onMain := detached
	onMain.Branch = "main"
	s := State{Entries: []Entry{onMain, detached, onMain}}
	exact := Query{Outcome: OutcomePassed, ByBranch: true}
	if got := s.Observed(exact); len(got) != 1 || got[0] != detached {
		t.Fatalf("a detached HEAD selects the entries of a branch: %+v", got)
	}
	if i := s.Index(exact); i != 1 {
		t.Fatalf("index %d", i)
	}
	if e, ok := s.Latest(exact); !ok || e != detached {
		t.Fatalf("latest: %+v", e)
	}
	if got := s.Observed(Query{Outcome: OutcomePassed}); len(got) != 3 {
		t.Fatalf("without ByBranch any branch: %+v", got)
	}
}

// With ByBranch and a head, an entry is selected when it is of the same branch, whatever its head, or when it
// carries the same head, whatever its branch; an entry of another branch with another head or without head is not.
// A query without head selects only by branch, and on a detached HEAD the entries of a branch on the same commit
// are selected too.
func TestObservedByBranchOrHead(t *testing.T) {
	sameBranch := Entry{Kind: KindObserved, Outcome: OutcomePassed, Branch: "main", Head: "c2", Key: "a", Hash: "h"}
	sameHead := Entry{Kind: KindObserved, Outcome: OutcomePassed, Branch: "dev", Head: "c1", Key: "b", Hash: "h"}
	otherHead := Entry{Kind: KindObserved, Outcome: OutcomePassed, Branch: "dev", Head: "c2", Key: "c", Hash: "h"}
	noHead := Entry{Kind: KindObserved, Outcome: OutcomePassed, Branch: "dev", Key: "d", Hash: "h"}
	s := State{Entries: []Entry{sameBranch, sameHead, otherHead, noHead}}
	keys := func(q Query) string {
		var out []string
		for _, e := range s.Observed(q) {
			out = append(out, e.Key)
		}
		return strings.Join(out, "")
	}
	if got := keys(Query{Outcome: OutcomePassed, Branch: "main", Head: "c1", ByBranch: true}); got != "ab" {
		t.Fatalf("by branch or head: %q", got)
	}
	if i := s.Index(Query{Outcome: OutcomePassed, Branch: "main", Head: "c1", ByBranch: true}); i != 1 {
		t.Fatalf("index %d", i)
	}
	if got := keys(Query{Outcome: OutcomePassed, Branch: "main", ByBranch: true}); got != "a" {
		t.Fatalf("without head only the branch: %q", got)
	}
	if got := keys(Query{Outcome: OutcomePassed, Head: "c1", ByBranch: true}); got != "b" {
		t.Fatalf("a detached HEAD on the same commit: %q", got)
	}
	if got := keys(Query{Outcome: OutcomePassed, Head: "c1"}); got != "abcd" {
		t.Fatalf("without ByBranch the head is ignored: %q", got)
	}
}

// Index is the position of the last entry the query selects, so two observations can be ordered; -1 for none, for
// an empty hash and for a query that is not valid.
func TestIndex(t *testing.T) {
	injected := Entry{Kind: KindObserved, Outcome: OutcomeInjected, Session: "s", Actor: "session", Line: 3, Key: "x"}
	started := Entry{Kind: KindObserved, Outcome: OutcomeStarted, Session: "s", Actor: "session"}
	s := State{Entries: []Entry{injected, started, {Kind: KindDecision, Outcome: OutcomeInjected}, injected}}
	if i := s.Index(Query{Outcome: OutcomeInjected, Session: "s", Line: 3, Key: "x"}); i != 3 {
		t.Fatalf("the last injected: %d", i)
	}
	if i := s.Index(Query{Outcome: OutcomeStarted, Session: "s"}); i != 1 {
		t.Fatalf("started: %d", i)
	}
	if s.Index(Query{Outcome: OutcomeStarted, Session: "t"}) != -1 || s.Index(Query{Outcome: OutcomePassed}) != -1 ||
		s.Index(Query{Outcome: OutcomeInjected, ByHash: true}) != -1 || s.Index(Query{Outcome: "injected_rule"}) != -1 {
		t.Fatal("nothing selected is -1")
	}
}

// Chain selects the entries on the content of the query or of also, and walks back through the Before of each one
// it selects, so the blocks of a run that changes the content on every stop count together; an edit of the agent
// between two stops breaks the chain, and without any Before it selects what Observed selects by hash.
func TestChain(t *testing.T) {
	blocked := func(hash, before string) Entry {
		return Entry{Kind: KindObserved, Outcome: OutcomeBlocked, Session: "s", Event: "stop", Line: 4, Hash: hash,
			Before: before}
	}
	q := Query{Outcome: OutcomeBlocked, Session: "s", Event: "stop", Line: 4, ByHash: true}
	// h0 -> h1 by the run; the agent edits to e1 -> e2 by the run; then it does nothing: e2 -> e3, e3 -> e4.
	other := blocked("e2", "e1")
	other.Line = 9
	s := State{Entries: []Entry{blocked("h1", "h0"), blocked("e2", "e1"), other, blocked("e3", "e2"),
		{Kind: KindDecision, Outcome: OutcomeBlocked, Line: 4, Hash: "e3"}, blocked("e4", "e3")}}

	q.Hash = "e5"
	if got := s.Chain(q, "e4"); len(got) != 3 || got[0].Hash != "e2" || got[2].Hash != "e4" {
		t.Fatalf("the chain from e4, without h1 nor line 9: %+v", got)
	}
	q.Hash = "h1"
	if got := s.Chain(q); len(got) != 1 || got[0].Hash != "h1" {
		t.Fatalf("the chain from h1: %+v", got)
	}
	q.Hash = "e1"
	if got := s.Chain(q); len(got) != 0 {
		t.Fatalf("an older content never reaches a later one: %+v", got)
	}

	plain := State{Entries: []Entry{blocked("a", ""), blocked("b", ""), blocked("a", "")}}
	q.Hash = "a"
	if got, want := plain.Chain(q), plain.Observed(q); len(got) != 2 || len(want) != 2 {
		t.Fatalf("without Before as Observed: %+v %+v", got, want)
	}
	if got := plain.Chain(q, "b", ""); len(got) != 3 {
		t.Fatalf("also b: %+v", got)
	}
	q.Hash = ""
	if got := plain.Chain(q, "a"); got != nil {
		t.Fatalf("without the hash of the query: %+v", got)
	}
	if got := plain.Chain(Query{Outcome: "fact", Hash: "a"}); got != nil {
		t.Fatalf("a query that is not valid: %+v", got)
	}
}

// The lines of the kinds an older polix wrote (injected, stop_block) are kept as read and never selected.
func TestOldKindsIgnored(t *testing.T) {
	s := State{Entries: []Entry{
		{Kind: "injected", Rule: "GO-2", Session: "s"},
		{Kind: "stop_block", Session: "s", Event: "stop", Actor: "session", Line: 4},
	}}
	for _, o := range Outcomes {
		if got := s.Observed(Query{Outcome: o}); len(got) != 0 {
			t.Fatalf("%s: %+v", o, got)
		}
	}
}

// Append returns a copy: the original does not see the new entry.
func TestAppend(t *testing.T) {
	s := State{Entries: make([]Entry, 0, 4)}
	got, err := s.Append(Entry{Kind: KindObserved, Outcome: OutcomeFinished, Key: "a"})
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
	if err := (State{Entries: []Entry{{Kind: KindObserved}}, Skipped: 1}).Valid(); err != nil {
		t.Fatal(err)
	}
}
