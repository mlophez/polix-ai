package state

import (
	"strings"
	"testing"
)

// opened is the started observation of a subagent of type kind and id in the session s.
func opened(s, kind, id string) entry {
	return entry{Outcome: OutcomeStarted, Session: s, Event: "subagent_start", Actor: kind, AgentID: id}
}

// closed is the finished observation of a subagent of type kind and id in the session s.
func closed(s, kind, id string) entry {
	return entry{Outcome: OutcomeFinished, Session: s, Event: "subagent_stop", Actor: kind, AgentID: id, Key: kind}
}

// sessionStarted is the started observation of the session s with the source key.
func sessionStarted(s, key string) entry {
	return entry{Outcome: OutcomeStarted, Session: s, Event: "start", Actor: "session", Key: key}
}

// ids returns the agent ids of the running subagents, or their types when they have none, joined by commas.
func ids(running []Match) string {
	var out []string
	for _, m := range running {
		if m.Record.AgentID != "" {
			out = append(out, m.Record.AgentID)
		} else {
			out = append(out, m.Record.Actor)
		}
	}
	return strings.Join(out, ",")
}

// AC2: two subagents of the same type, started, and only the first finished: the second is still running.
func TestRunningByAgentID(t *testing.T) {
	s := stateOf(opened("s", "builder", "a1"), opened("s", "builder", "a2"), closed("s", "builder", "a1"))
	if got := ids(s.Running("s")); got != "a2" {
		t.Fatalf("running: %q", got)
	}
	s.Records = append(s.Records, closed("s", "builder", "a2").record())
	if got := s.Running("s"); len(got) != 0 {
		t.Fatalf("all finished: %+v", got)
	}
}

// A finished of an id that never started closes nothing of another id, and closes the oldest open of its type
// without id, such as the start an older polix wrote.
func TestRunningUnknownID(t *testing.T) {
	s := stateOf(opened("s", "builder", "a1"), closed("s", "builder", "a9"))
	if got := ids(s.Running("s")); got != "a1" {
		t.Fatalf("another id: %q", got)
	}
	s = stateOf(opened("s", "builder", ""), opened("s", "builder", "a1"), closed("s", "builder", "a9"))
	if got := ids(s.Running("s")); got != "a1" {
		t.Fatalf("the start without id: %q", got)
	}
}

// AC3: observations without agent id pair by type, the oldest first.
func TestRunningByType(t *testing.T) {
	first := opened("s", "builder", "")
	first.At = "1"
	second := opened("s", "builder", "")
	second.At = "2"
	s := stateOf(first, opened("s", "reviewer", ""), second, closed("s", "builder", ""))
	got := s.Running("s")
	if len(got) != 2 || got[0].Record.Actor != "reviewer" || got[1].Record.At != "2" {
		t.Fatalf("running: %+v", got)
	}
	s.Records = append(s.Records, closed("s", "reviewer", "").record(), closed("s", "reviewer", "").record())
	if got := ids(s.Running("s")); got != "builder" {
		t.Fatalf("a finished without a start closes nothing: %q", got)
	}
}

// AC7: a start of the session whose key is startup, resume, clear or empty closes every subagent open in the
// session; one of compact closes none.
func TestRunningSessionStart(t *testing.T) {
	for _, key := range []string{"startup", "resume", "clear", ""} {
		s := stateOf(opened("s", "builder", "a1"), opened("s", "reviewer", ""), sessionStarted("s", key),
			opened("s", "builder", "a2"))
		if got := ids(s.Running("s")); got != "a2" {
			t.Errorf("%q: %q", key, got)
		}
	}
	s := stateOf(opened("s", "builder", "a1"), sessionStarted("s", "compact"))
	if got := ids(s.Running("s")); got != "a1" {
		t.Fatalf("compact: %q", got)
	}
}

// AC6: the subagent that acts is left out, by its id, else by its type as its finished would pair; another subagent
// of its type stays.
func TestRunningOthers(t *testing.T) {
	s := stateOf(opened("s", "builder", "a1"), opened("s", "builder", "a2"))
	if got := ids(s.RunningOthers("s", "a2", "builder")); got != "a1" {
		t.Fatalf("by id: %q", got)
	}
	if got := ids(s.RunningOthers("s", "a9", "builder")); got != "a1,a2" {
		t.Fatalf("an unknown id: %q", got)
	}
	s = stateOf(opened("s", "builder", ""), opened("s", "builder", ""))
	if got := s.RunningOthers("s", "", "builder"); len(got) != 1 {
		t.Fatalf("by type: %+v", got)
	}
	if got := s.RunningOthers("s", "", "reviewer"); len(got) != 2 {
		t.Fatalf("another type: %+v", got)
	}
}

// An ended observation closes the subagent of its agent id, else the oldest of its type, as a finished does: its key
// names them.
func TestRunningEnded(t *testing.T) {
	ended := func(kind, id string) entry {
		return entry{Outcome: OutcomeEnded, Session: "s", Event: "stop", Actor: "session", Key: Match{Record: Record{Actor: kind, AgentID: id}}.EndedKey()}
	}
	s := stateOf(opened("s", "builder", "a1"), opened("s", "builder", "a2"), ended("builder", "a2"))
	if got := ids(s.Running("s")); got != "a1" {
		t.Fatalf("by id: %q", got)
	}
	s = stateOf(opened("s", "builder", ""), opened("s", "reviewer", ""), ended("builder", ""))
	if got := ids(s.Running("s")); got != "reviewer" {
		t.Fatalf("by type: %q", got)
	}
}

// The records of another session, the actions and the observations of other outcomes never count.
func TestRunningOnlyTheSession(t *testing.T) {
	decision := opened("s", "builder", "a3")
	decision.Kind = "decision"
	injected := opened("s", "builder", "a4")
	injected.Outcome = OutcomeInjected
	s := stateOf(opened("t", "builder", "a1"), opened("s", "builder", "a2"), closed("t", "builder", "a2"),
		sessionStarted("t", "startup"), decision, injected)
	if got := ids(s.Running("s")); got != "a2" {
		t.Fatalf("session s: %q", got)
	}
	if got := ids(s.Running("t")); got != "" {
		t.Fatalf("session t: %q", got)
	}
}
