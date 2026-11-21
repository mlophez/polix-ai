package state

import (
	"strings"
	"testing"
)

// opened is the started line of a subagent of type kind and id in the session s.
func opened(s, kind, id string) Entry {
	return Entry{Kind: KindObserved, Outcome: OutcomeStarted, Session: s, Event: "subagent_start", Actor: kind,
		AgentID: id}
}

// closed is the finished line of a subagent of type kind and id in the session s.
func closed(s, kind, id string) Entry {
	return Entry{Kind: KindObserved, Outcome: OutcomeFinished, Session: s, Event: "subagent_stop", Actor: kind,
		AgentID: id, Key: kind}
}

// sessionStarted is the started line of the session s with the source key.
func sessionStarted(s, key string) Entry {
	return Entry{Kind: KindObserved, Outcome: OutcomeStarted, Session: s, Event: "start", Actor: "session", Key: key}
}

// ids returns the agent ids of the running subagents, or their types when they have none, joined by commas.
func ids(running []Entry) string {
	var out []string
	for _, e := range running {
		if e.AgentID != "" {
			out = append(out, e.AgentID)
		} else {
			out = append(out, e.Actor)
		}
	}
	return strings.Join(out, ",")
}

// AC2: two subagents of the same type, started, and only the first finished: the second is still running.
func TestRunningByAgentID(t *testing.T) {
	s := State{Entries: []Entry{opened("s", "builder", "a1"), opened("s", "builder", "a2"),
		closed("s", "builder", "a1")}}
	if got := ids(s.Running("s")); got != "a2" {
		t.Fatalf("running: %q", got)
	}
	s.Entries = append(s.Entries, closed("s", "builder", "a2"))
	if got := s.Running("s"); len(got) != 0 {
		t.Fatalf("all finished: %+v", got)
	}
}

// A finished of an id that never started closes nothing of another id, and closes the oldest open of its type
// without id, such as the start an older polix wrote.
func TestRunningUnknownID(t *testing.T) {
	s := State{Entries: []Entry{opened("s", "builder", "a1"), closed("s", "builder", "a9")}}
	if got := ids(s.Running("s")); got != "a1" {
		t.Fatalf("another id: %q", got)
	}
	s = State{Entries: []Entry{opened("s", "builder", ""), opened("s", "builder", "a1"), closed("s", "builder", "a9")}}
	if got := ids(s.Running("s")); got != "a1" {
		t.Fatalf("the start without id: %q", got)
	}
}

// AC3: lines without agent_id pair by type, the oldest first.
func TestRunningByType(t *testing.T) {
	first := opened("s", "builder", "")
	first.At = "1"
	second := opened("s", "builder", "")
	second.At = "2"
	s := State{Entries: []Entry{first, opened("s", "reviewer", ""), second, closed("s", "builder", "")}}
	got := s.Running("s")
	if len(got) != 2 || got[0].Actor != "reviewer" || got[1].At != "2" {
		t.Fatalf("running: %+v", got)
	}
	s.Entries = append(s.Entries, closed("s", "reviewer", ""), closed("s", "reviewer", ""))
	if got := ids(s.Running("s")); got != "builder" {
		t.Fatalf("a finished without a start closes nothing: %q", got)
	}
}

// AC7: a start of the session whose key is startup, resume, clear or empty closes every subagent open in the
// session; one of compact closes none.
func TestRunningSessionStart(t *testing.T) {
	for _, key := range []string{"startup", "resume", "clear", ""} {
		s := State{Entries: []Entry{opened("s", "builder", "a1"), opened("s", "reviewer", ""), sessionStarted("s", key),
			opened("s", "builder", "a2")}}
		if got := ids(s.Running("s")); got != "a2" {
			t.Errorf("%q: %q", key, got)
		}
	}
	s := State{Entries: []Entry{opened("s", "builder", "a1"), sessionStarted("s", "compact")}}
	if got := ids(s.Running("s")); got != "a1" {
		t.Fatalf("compact: %q", got)
	}
}

// AC6: the subagent that acts is left out, by its id, else by its type as its finished would pair; another subagent
// of its type stays.
func TestRunningOthers(t *testing.T) {
	s := State{Entries: []Entry{opened("s", "builder", "a1"), opened("s", "builder", "a2")}}
	if got := ids(s.RunningOthers("s", "a2", "builder")); got != "a1" {
		t.Fatalf("by id: %q", got)
	}
	if got := ids(s.RunningOthers("s", "a9", "builder")); got != "a1,a2" {
		t.Fatalf("an unknown id: %q", got)
	}
	s = State{Entries: []Entry{opened("s", "builder", ""), opened("s", "builder", "")}}
	if got := s.RunningOthers("s", "", "builder"); len(got) != 1 {
		t.Fatalf("by type: %+v", got)
	}
	if got := s.RunningOthers("s", "", "reviewer"); len(got) != 2 {
		t.Fatalf("another type: %+v", got)
	}
}

// An ended line closes the subagent of its agent_id, else the oldest of its type, as a finished does.
func TestRunningEnded(t *testing.T) {
	ended := func(kind, id string) Entry {
		return Entry{Kind: KindObserved, Outcome: OutcomeEnded, Session: "s", Event: "stop", Actor: kind, AgentID: id}
	}
	s := State{Entries: []Entry{opened("s", "builder", "a1"), opened("s", "builder", "a2"), ended("builder", "a2")}}
	if got := ids(s.Running("s")); got != "a1" {
		t.Fatalf("by id: %q", got)
	}
	s = State{Entries: []Entry{opened("s", "builder", ""), opened("s", "reviewer", ""), ended("builder", "")}}
	if got := ids(s.Running("s")); got != "reviewer" {
		t.Fatalf("by type: %q", got)
	}
}

// The lines of another session, the decisions and the lines of other outcomes never count.
func TestRunningOnlyTheSession(t *testing.T) {
	decision := opened("s", "builder", "a3")
	decision.Kind = KindDecision
	injected := opened("s", "builder", "a4")
	injected.Outcome = OutcomeInjected
	s := State{Entries: []Entry{opened("t", "builder", "a1"), opened("s", "builder", "a2"), closed("t", "builder", "a2"),
		sessionStarted("t", "startup"), decision, injected}}
	if got := ids(s.Running("s")); got != "a2" {
		t.Fatalf("session s: %q", got)
	}
	if got := ids(s.Running("t")); got != "" {
		t.Fatalf("session t: %q", got)
	}
}
