package agent

import (
	"testing"

	"github.com/mlophez/polix-ai/internal/app/workflow"
)

// Locate places the path of an edit in the repository, or outside it.
func TestEventLocate(t *testing.T) {
	ev := Event{Name: workflow.PreEdit, Agent: Claude}
	if got := ev.Locate("src/a.go", true); got.Path != "src/a.go" || got.Outside {
		t.Fatalf("inside: %+v", got)
	}
	if got := ev.Locate("../x", false); got.Path != "" || !got.Outside {
		t.Fatalf("outside: %+v", got)
	}
	if !ev.Edits() || (Event{Name: workflow.Stop}).Edits() {
		t.Fatal("Edits")
	}
}

// An event needs a neutral name and its agent; the old names of the start and the stop of a turn are not ones.
func TestEventValid(t *testing.T) {
	if err := (Event{Name: workflow.Stop, Agent: Claude}).Valid(); err != nil {
		t.Fatal(err)
	}
	if (Event{Name: "Stop", Agent: Claude}).Valid() == nil || (Event{Name: workflow.Stop}).Valid() == nil ||
		(Event{Name: "turn_stop", Agent: Claude}).Valid() == nil ||
		(Event{Name: "session_start", Agent: Claude}).Valid() == nil {
		t.Fatal("a native name, an old name or no agent is valid")
	}
}
