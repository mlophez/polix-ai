package state

import (
	"strings"
	"testing"
)

// entry is what the tests of the queries write for a line of the log: one observation (kind observed, the default),
// one action (decision) or the one of a legacy line of any other kind, with the fields of the record it is in.
type entry struct {
	Kind    string
	At      string
	Session string
	Event   string
	Actor   string
	AgentID string
	Branch  string
	Head    string
	Line    int
	Key     string
	Outcome string
	Hash    string
	Before  string
	Refs    string
	Rule    string
}

// record returns the record of the entry: an event line with its observation or its action, or a legacy record with
// the kind of the entry and its observation.
func (e entry) record() Record {
	r := Record{At: e.At, Session: e.Session, Event: e.Event, Actor: e.Actor, AgentID: e.AgentID, Branch: e.Branch,
		Head: e.Head, Rule: e.Rule}
	o := Observation{Line: e.Line, Outcome: e.Outcome, Key: e.Key, Hash: e.Hash, Refs: e.Refs, Before: e.Before}
	switch e.Kind {
	case "", "observed":
		r.Observations = []Observation{o}
	case "decision":
		r.Actions = []Action{{Line: e.Line, Action: "block", Outcome: e.Outcome, Key: e.Key}}
	default:
		r.Legacy, r.Observations = e.Kind, []Observation{o}
	}
	return r
}

// stateOf returns the state of the records of the entries, in order.
func stateOf(entries ...entry) State {
	var s State
	for _, e := range entries {
		s.Records = append(s.Records, e.record())
	}
	return s
}

// A record needs a session, which names its file, and an event line an action or an observation; a legacy line is as
// it was written. Writable adds what this polix writes: no legacy line, and actions and observations that are valid,
// the observation with every invariant of a line of the log.
func TestRecordValid(t *testing.T) {
	passed := Observation{Outcome: OutcomePassed, Key: "just test", Hash: "h"}
	valid := []Record{
		{Session: "s", Observations: []Observation{passed}},
		{Session: "s", Actions: []Action{{Line: 4, Action: "block", Outcome: "block", Reason: "no"}}},
		{Session: "s", Actions: []Action{{Action: "run", Outcome: "passed", Key: "just test", DurationMS: new(int64)}},
			Observations: []Observation{passed}, Input: Input{Command: "just test"}},
	}
	for _, r := range valid {
		if err := r.Valid(); err != nil {
			t.Errorf("%+v: %v", r, err)
		}
		if err := r.Writable(); err != nil {
			t.Errorf("%+v: %v", r, err)
		}
	}
	err := (Record{Observations: []Observation{passed}}).Valid()
	if err == nil || !strings.Contains(err.Error(), "session") {
		t.Fatalf("a record without session: %v", err)
	}
	if (Record{Session: "s"}).Valid() == nil || (Record{Session: "s", Input: Input{Command: "ls"}}).Valid() == nil {
		t.Fatal("an event line with neither actions nor observations is valid")
	}
	// A line of a newer polix, or an old one, is kept as read: a legacy line with nothing, an unknown outcome.
	for _, r := range []Record{{Session: "s", Legacy: "stop_block"}, {Session: "s", Legacy: LegacyObserved,
		Observations: []Observation{{Outcome: "newer"}}}, {Session: "s", Observations: []Observation{{Outcome: "newer"}}}} {
		if err := r.Valid(); err != nil {
			t.Errorf("%+v: %v", r, err)
		}
	}
	negative := int64(-1)
	unwritable := map[string]Record{
		"nothing":         {Session: "s"},
		"no session":      {Observations: []Observation{passed}},
		"legacy":          {Session: "s", Legacy: LegacyObserved, Observations: []Observation{passed}},
		"legacy rule":     {Session: "s", Rule: "GO-2", Observations: []Observation{passed}},
		"legacy fact":     {Session: "s", Fact: "reviewed", Observations: []Observation{passed}},
		"no action":       {Session: "s", Actions: []Action{{Outcome: "block"}}},
		"no outcome":      {Session: "s", Actions: []Action{{Action: "block"}}},
		"negative line":   {Session: "s", Actions: []Action{{Line: -1, Action: "block", Outcome: "block"}}},
		"negative time":   {Session: "s", Actions: []Action{{Action: "run", Outcome: "passed", DurationMS: &negative}}},
		"no outcome seen": {Session: "s", Observations: []Observation{{}}},
		"unknown outcome": {Session: "s", Observations: []Observation{{Outcome: "fact"}}},
		"passed no hash":  {Session: "s", Observations: []Observation{{Outcome: OutcomePassed, Key: "just test"}}},
		"blocked no hash": {Session: "s", Observations: []Observation{{Outcome: OutcomeBlocked, Line: 4}}},
		"snapshot no hash": {Session: "s", Observations: []Observation{
			{Outcome: OutcomeSnapshot, Key: "toolu_1"}}},
		"refs on watched": {Session: "s", Observations: []Observation{
			{Outcome: OutcomeWatched, Key: "toolu_1", Refs: "s"}}},
		"before on passed": {Session: "s", Observations: []Observation{
			{Outcome: OutcomePassed, Key: "just test", Hash: "h", Before: "g"}}},
		"negative observed line": {Session: "s", Observations: []Observation{{Outcome: OutcomeStarted, Line: -1}}},
	}
	for name, r := range unwritable {
		if err := r.Writable(); err == nil {
			t.Errorf("%s: %+v is writable", name, r)
		}
	}
	for _, o := range []Observation{
		{Outcome: OutcomeFinished, Key: "polix:reviewer", Hash: "h"},
		{Outcome: OutcomeFinished, Key: "polix:reviewer"},
		{Outcome: OutcomeBlocked, Line: 4, Hash: "h", Before: "g"},
		{Outcome: OutcomeInjected, Key: "x"},
		{Outcome: OutcomeStarted},
		{Outcome: OutcomeSnapshot, Key: "toolu_1", Hash: "h", Refs: "s"},
		{Outcome: OutcomeWatched, Key: "toolu_1"},
		{Outcome: OutcomeEnded, Key: "polix:reviewer a1"},
		{Outcome: OutcomeNotified, Key: "hola"},
	} {
		if err := o.Valid(); err != nil {
			t.Errorf("%+v: %v", o, err)
		}
	}
	if (Match{Seq: -1}).Valid() == nil || (Match{}).Valid() != nil {
		t.Fatal("a match needs a position that is not negative")
	}
}

// Acts tells whether something acted or was observed; Unnotified drops the notices a blocked call never shows, and
// shares no list with the record it comes from; Stamped sets the time only when the record has none.
func TestRecordMethods(t *testing.T) {
	notice := Observation{Line: 4, Outcome: OutcomeNotified, Key: "Aviso"}
	passed := Observation{Outcome: OutcomePassed, Key: "just test", Hash: "h"}
	r := Record{Session: "s", At: "t0", Actions: []Action{{Action: "notify", Outcome: "notify"}},
		Observations: []Observation{notice, passed}}
	got := r.Unnotified()
	if len(got.Observations) != 1 || got.Observations[0] != passed || len(got.Actions) != 1 || len(r.Observations) != 2 {
		t.Fatalf("Unnotified: %+v from %+v", got, r)
	}
	got.Actions[0].Outcome, got.Observations[0].Key = "changed", "changed"
	if r.Actions[0].Outcome != "notify" || r.Observations[1].Key != "just test" {
		t.Fatalf("the copy shares a list with the record: %+v", r)
	}
	if !r.Acts() || (Record{Session: "s", Input: Input{Command: "ls"}}).Acts() || (Record{}).Acts() ||
		!(Record{Observations: []Observation{passed}}).Acts() {
		t.Fatal("Acts")
	}
	if r.Stamped("t1").At != "t0" || (Record{Session: "s"}).Stamped("t1").At != "t1" {
		t.Fatal("Stamped")
	}
}

// Chained keeps the content the event started from only on a blocked observation whose event changed a known
// content.
func TestObservationChained(t *testing.T) {
	blocked := Observation{Outcome: OutcomeBlocked, Line: 4, Hash: "h"}
	if got := blocked.Chained("g"); got.Before != "g" {
		t.Fatalf("a run that changed the content: %+v", got)
	}
	unknown := blocked
	unknown.Hash = UnknownHash
	passed := Observation{Outcome: OutcomePassed, Hash: "h"}
	for _, c := range []struct {
		o      Observation
		before string
	}{{blocked, "h"}, {blocked, ""}, {blocked, UnknownHash}, {unknown, "g"}, {passed, "g"}} {
		if got := c.o.Chained(c.before); got.Before != "" {
			t.Errorf("%+v with %q carries before: %+v", c.o, c.before, got)
		}
	}
}

// The key of an ended observation names the type of the subagent and, when it has one, its id; Ended reads them back.
func TestEndedKey(t *testing.T) {
	for _, c := range []struct{ kind, id, key string }{
		{"polix:reviewer", "", "polix:reviewer"}, {"polix:reviewer", "a1", "polix:reviewer a1"}} {
		started := Match{Record: Record{Actor: c.kind, AgentID: c.id}}
		if got := started.EndedKey(); got != c.key {
			t.Errorf("EndedKey of %q %q = %q", c.kind, c.id, got)
		}
		if kind, id := (Match{Observation: Observation{Key: c.key}}).Ended(); kind != c.kind || id != c.id {
			t.Errorf("Ended of %q = %q, %q", c.key, kind, id)
		}
	}
}

// Every query selects nothing from a legacy record, which the same record would match as an event line.
func TestQueriesSkipLegacy(t *testing.T) {
	obs := []Observation{
		{Outcome: OutcomeFinished, Key: "polix:reviewer", Hash: "h"},
		{Outcome: OutcomeBlocked, Line: 5, Hash: "h"},
		{Outcome: OutcomePassed, Key: "just test", Hash: "h"},
		{Outcome: OutcomeStarted, Key: "startup"},
	}
	record := func(legacy string, o Observation) Record {
		return Record{Session: "s1", Event: "subagent_start", Actor: "polix:reviewer", Branch: "main", Legacy: legacy,
			Observations: []Observation{o}}
	}
	for _, o := range obs {
		q := Query{Outcome: o.Outcome, Session: "s1"}
		event := State{Records: []Record{record("", o)}}
		legacy := State{Records: []Record{record(LegacyObserved, o)}}
		if len(event.Observed(q)) != 1 || event.Index(q) != 0 {
			t.Fatalf("%s as an event line: %+v", o.Outcome, event.Observed(q))
		}
		if got := legacy.Observed(q); len(got) != 0 {
			t.Errorf("Observed selects the legacy %s: %+v", o.Outcome, got)
		}
		if _, ok := legacy.Latest(q); ok {
			t.Errorf("Latest selects the legacy %s", o.Outcome)
		}
		if legacy.Index(q) != -1 {
			t.Errorf("Index selects the legacy %s", o.Outcome)
		}
		if o.Hash != "" {
			chain := Query{Outcome: o.Outcome, Session: "s1", Hash: "h"}
			if len(event.Chain(chain)) != 1 || len(legacy.Chain(chain)) != 0 {
				t.Errorf("Chain over the legacy %s", o.Outcome)
			}
		}
	}
	start := Record{Session: "s1", Event: "subagent_start", Actor: "polix:reviewer",
		Observations: []Observation{{Outcome: OutcomeStarted}}}
	legacy := start
	legacy.Legacy = LegacyObserved
	if got := (State{Records: []Record{start}}).Running("s1"); len(got) != 1 {
		t.Fatalf("the event line runs: %+v", got)
	}
	if got := (State{Records: []Record{legacy}}).Running("s1"); len(got) != 0 {
		t.Errorf("Running counts a legacy line: %+v", got)
	}
	if got := (State{Records: []Record{legacy}}).RunningOthers("s1", "", "polix:builder"); len(got) != 0 {
		t.Errorf("RunningOthers counts a legacy line: %+v", got)
	}
}
