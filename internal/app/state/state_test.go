package state

import (
	"strings"
	"testing"
)

// The query selects the observations of its outcome and of every field it sets, in file order; never another
// kind of record, another branch or another hash, and never an empty hash.
func TestObserved(t *testing.T) {
	finished := entry{Outcome: OutcomeFinished, Session: "s", Actor: "polix:reviewer", Event: "subagent_stop",
		Branch: "main", Key: "polix:reviewer", Hash: "h1"}
	later := finished
	later.Hash = "h2"
	other := finished
	other.Branch, other.Hash = "dev", "h3"
	blocked := entry{Outcome: OutcomeBlocked, Session: "s", Branch: "main", Line: 7, Hash: "h1"}
	fact := entry{Kind: "fact", Outcome: OutcomeFinished, Branch: "main", Key: "polix:reviewer", Hash: "h1"}
	decision := entry{Kind: "decision", Outcome: OutcomeFinished, Branch: "main", Key: "polix:reviewer"}
	s := stateOf(finished, fact, blocked, later, other, decision)

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
	if m, ok := s.Latest(Query{Outcome: OutcomeFinished, Branch: "main"}); !ok || m.Hash != "h2" {
		t.Fatalf("latest: %+v %v", m, ok)
	}
	if _, ok := s.Latest(Query{Outcome: OutcomePassed}); ok {
		t.Fatal("latest of nothing")
	}
	if got := s.Observed(Query{Outcome: OutcomeFinished, Branch: "main", Key: "polix:reviewer"}); got[0].Record.Actor !=
		"polix:reviewer" || got[0].Record.Event != "subagent_stop" {
		t.Fatalf("a match carries its record: %+v", got[0])
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

// The query selects by each field it sets, and leaves out the observations that differ in that field only.
func TestObservedByEachField(t *testing.T) {
	base := entry{Outcome: OutcomeBlocked, Session: "s", Actor: "session", Event: "stop", Branch: "main", Key: "k",
		Line: 4, Hash: "h"}
	cases := []struct {
		name   string
		q      Query
		change func(*entry)
	}{
		{"outcome", Query{Outcome: OutcomeBlocked}, func(e *entry) { e.Outcome = OutcomePassed }},
		{"session", Query{Outcome: OutcomeBlocked, Session: "s"}, func(e *entry) { e.Session = "t" }},
		{"actor", Query{Outcome: OutcomeBlocked, Actor: "session"}, func(e *entry) { e.Actor = "polix:reviewer" }},
		{"event", Query{Outcome: OutcomeBlocked, Event: "stop"}, func(e *entry) { e.Event = "subagent_stop" }},
		{"branch", Query{Outcome: OutcomeBlocked, Branch: "main"}, func(e *entry) { e.Branch = "dev" }},
		{"exact branch", Query{Outcome: OutcomeBlocked, Branch: "main", ByBranch: true}, func(e *entry) { e.Branch = "" }},
		{"key", Query{Outcome: OutcomeBlocked, Key: "k"}, func(e *entry) { e.Key = "other" }},
		{"line", Query{Outcome: OutcomeBlocked, Line: 4}, func(e *entry) { e.Line = 5 }},
		{"hash", Query{Outcome: OutcomeBlocked, Hash: "h", ByHash: true}, func(e *entry) { e.Hash = "h2" }},
		{"kind", Query{Outcome: OutcomeBlocked}, func(e *entry) { e.Kind = "stop_block" }},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			other := base
			c.change(&other)
			s := stateOf(other, base, other)
			// A legacy record has no position: the base is the first observation then, else the second.
			want := 1
			if c.name == "kind" {
				want = 0
			}
			if got := s.Observed(c.q); len(got) != 1 || got[0].Seq != want || got[0].Record.Session != "s" {
				t.Fatalf("selected %+v", got)
			}
			if i := s.Index(c.q); i != want {
				t.Fatalf("index %d", i)
			}
		})
	}
}

// With ByBranch an empty branch, that of a detached HEAD, selects only the observations of records without branch;
// without it, an empty branch selects any.
func TestObservedByBranchDetached(t *testing.T) {
	detached := entry{Outcome: OutcomePassed, Key: "k", Hash: "h"}
	onMain := detached
	onMain.Branch = "main"
	s := stateOf(onMain, detached, onMain)
	exact := Query{Outcome: OutcomePassed, ByBranch: true}
	if got := s.Observed(exact); len(got) != 1 || got[0].Seq != 1 {
		t.Fatalf("a detached HEAD selects the observations of a branch: %+v", got)
	}
	if i := s.Index(exact); i != 1 {
		t.Fatalf("index %d", i)
	}
	if m, ok := s.Latest(exact); !ok || m.Seq != 1 || m.Record.Branch != "" {
		t.Fatalf("latest: %+v", m)
	}
	if got := s.Observed(Query{Outcome: OutcomePassed}); len(got) != 3 {
		t.Fatalf("without ByBranch any branch: %+v", got)
	}
}

// With ByBranch and a head, an observation is selected when its record is of the same branch, whatever its head, or
// when it carries the same head, whatever its branch; one of another branch with another head or without head is not.
// A query without head selects only by branch, and on a detached HEAD the records of a branch on the same commit
// are selected too.
func TestObservedByBranchOrHead(t *testing.T) {
	sameBranch := entry{Outcome: OutcomePassed, Branch: "main", Head: "c2", Key: "a", Hash: "h"}
	sameHead := entry{Outcome: OutcomePassed, Branch: "dev", Head: "c1", Key: "b", Hash: "h"}
	otherHead := entry{Outcome: OutcomePassed, Branch: "dev", Head: "c2", Key: "c", Hash: "h"}
	noHead := entry{Outcome: OutcomePassed, Branch: "dev", Key: "d", Hash: "h"}
	s := stateOf(sameBranch, sameHead, otherHead, noHead)
	keys := func(q Query) string {
		var out []string
		for _, m := range s.Observed(q) {
			out = append(out, m.Key)
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

// ByHead selects the observations of records of exactly the head of the query on any branch, none of another head or
// without head, and none when the head of the query is empty.
func TestObservedByHead(t *testing.T) {
	onMain := entry{Outcome: OutcomeFinished, Branch: "main", Head: "c1", Key: "a", Hash: "h"}
	onDev := entry{Outcome: OutcomeFinished, Branch: "dev", Head: "c1", Key: "b", Hash: "h"}
	detached := entry{Outcome: OutcomeFinished, Head: "c1", Key: "c", Hash: "h"}
	other := entry{Outcome: OutcomeFinished, Branch: "main", Head: "c2", Key: "d", Hash: "h"}
	noHead := entry{Outcome: OutcomeFinished, Branch: "main", Key: "e", Hash: "h"}
	s := stateOf(onMain, onDev, detached, other, noHead)
	keys := func(q Query) string {
		var out []string
		for _, m := range s.Observed(q) {
			out = append(out, m.Key)
		}
		return strings.Join(out, "")
	}
	if got := keys(Query{Outcome: OutcomeFinished, Branch: "feat", Head: "c1", ByHead: true}); got != "abc" {
		t.Fatalf("by head on any branch: %q", got)
	}
	if got := keys(Query{Outcome: OutcomeFinished, Head: "c2", ByHead: true}); got != "d" {
		t.Fatalf("another head: %q", got)
	}
	if got := keys(Query{Outcome: OutcomeFinished, Branch: "main", ByHead: true}); got != "" {
		t.Fatalf("an empty head selects none: %q", got)
	}
	if i := s.Index(Query{Outcome: OutcomeFinished, Head: "c1", ByHead: true}); i != 2 {
		t.Fatalf("index %d", i)
	}
}

// Index is the position of the last observation the query selects, so two observations can be ordered; -1 for none,
// for an empty hash and for a query that is not valid. The observations of one record keep the order they were noted
// in.
func TestIndex(t *testing.T) {
	injected := entry{Outcome: OutcomeInjected, Session: "s", Actor: "session", Line: 3, Key: "x"}
	started := entry{Outcome: OutcomeStarted, Session: "s", Actor: "session"}
	s := stateOf(injected, started, entry{Kind: "decision", Outcome: OutcomeInjected}, injected)
	if i := s.Index(Query{Outcome: OutcomeInjected, Session: "s", Line: 3, Key: "x"}); i != 2 {
		t.Fatalf("the last injected: %d", i)
	}
	if i := s.Index(Query{Outcome: OutcomeStarted, Session: "s"}); i != 1 {
		t.Fatalf("started: %d", i)
	}
	if s.Index(Query{Outcome: OutcomeStarted, Session: "t"}) != -1 || s.Index(Query{Outcome: OutcomePassed}) != -1 ||
		s.Index(Query{Outcome: OutcomeInjected, ByHash: true}) != -1 || s.Index(Query{Outcome: "injected_rule"}) != -1 {
		t.Fatal("nothing selected is -1")
	}
	one := State{Records: []Record{{Session: "s", Observations: []Observation{
		{Outcome: OutcomeInjected, Line: 3, Key: "x"}, {Outcome: OutcomeStarted}, {Outcome: OutcomeInjected, Line: 4}}}}}
	if a, b := one.Index(Query{Outcome: OutcomeInjected, Line: 3}), one.Index(Query{Outcome: OutcomeStarted}); a != 0 ||
		b != 1 || one.Index(Query{Outcome: OutcomeInjected, Line: 4}) != 2 {
		t.Fatalf("within a record: %d %d", a, b)
	}
}

// Chain selects the observations on the content of the query or of also, and walks back through the Before of each
// one it selects, so the blocks of a run that changes the content on every stop count together; an edit of the agent
// between two stops breaks the chain, and without any Before it selects what Observed selects by hash.
func TestChain(t *testing.T) {
	blocked := func(hash, before string) entry {
		return entry{Outcome: OutcomeBlocked, Session: "s", Event: "stop", Line: 4, Hash: hash, Before: before}
	}
	q := Query{Outcome: OutcomeBlocked, Session: "s", Event: "stop", Line: 4, ByHash: true}
	// h0 -> h1 by the run; the agent edits to e1 -> e2 by the run; then it does nothing: e2 -> e3, e3 -> e4.
	other := blocked("e2", "e1")
	other.Line = 9
	s := stateOf(blocked("h1", "h0"), blocked("e2", "e1"), other, blocked("e3", "e2"),
		entry{Kind: "decision", Outcome: OutcomeBlocked, Line: 4}, blocked("e4", "e3"))

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

	plain := stateOf(blocked("a", ""), blocked("b", ""), blocked("a", ""))
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
	s := stateOf(
		entry{Kind: "injected", Rule: "GO-2", Session: "s"},
		entry{Kind: "stop_block", Session: "s", Event: "stop", Actor: "session", Line: 4},
	)
	for _, o := range Outcomes {
		if got := s.Observed(Query{Outcome: o}); len(got) != 0 {
			t.Fatalf("%s: %+v", o, got)
		}
	}
}

// Append returns a copy: the original does not see the new record.
func TestAppend(t *testing.T) {
	s := State{Records: make([]Record, 0, 4)}
	got, err := s.Append(entry{Outcome: OutcomeFinished, Session: "s", Key: "a"}.record())
	if err != nil || len(got.Records) != 1 || len(s.Records) != 0 {
		t.Fatalf("%v %+v", err, got)
	}
	if _, err := s.Append(Record{}); err == nil {
		t.Fatal("a record without session nor observations")
	}
	if _, err := s.Append(entry{Outcome: OutcomeFinished}.record()); err == nil {
		t.Fatal("a record without session")
	}
}

func TestStateValid(t *testing.T) {
	if err := (State{Skipped: -1}).Valid(); err == nil || !strings.Contains(err.Error(), "negative") {
		t.Fatal(err)
	}
	if err := (State{Records: []Record{{Observations: []Observation{{}}}}, Skipped: 1}).Valid(); err != nil {
		t.Fatal(err)
	}
}

// A session id made only of safe characters, at most 128 long and not only dots, names its own file.
func TestSessionFileAsIs(t *testing.T) {
	long := strings.Repeat("a", 128)
	for _, id := range []string{"0b8e1c5a-7f3d-4c0e-9a41-2d6f8e1b7c35", long, "a.b_c-D9", ".hidden", "a..b", "..a", "-"} {
		got, err := Session(id).File()
		if err != nil || got != id+".jsonl" {
			t.Errorf("%q: %q %v", id, got, err)
		}
	}
}

// Any other id is hashed, so the id of an agent never reaches a path as it is: separators, spaces, a dot or dots
// alone, a long id.
func TestSessionFileHashed(t *testing.T) {
	ids := []string{"a/b", `a\b`, "../x", "a b", ".", "..", "...", strings.Repeat("a", 129), "ñ", "a\x00b"}
	seen := map[string]string{}
	for _, id := range ids {
		got, err := Session(id).File()
		if err != nil {
			t.Fatalf("%q: %v", id, err)
		}
		name, ok := strings.CutSuffix(got, ".jsonl")
		if !ok || len(name) != 64 || strings.Trim(name, "0123456789abcdef") != "" {
			t.Errorf("%q: %q is not a lowercase sha256 and .jsonl", id, got)
		}
		if again, _ := Session(id).File(); again != got {
			t.Errorf("%q: %q then %q", id, got, again)
		}
		if other, dup := seen[got]; dup {
			t.Errorf("%q and %q share %q", id, other, got)
		}
		seen[got] = id
	}
}

// A session without id has no file.
func TestSessionFileEmpty(t *testing.T) {
	if got, err := Session("").File(); err == nil || got != "" {
		t.Fatalf("%q %v", got, err)
	}
}
