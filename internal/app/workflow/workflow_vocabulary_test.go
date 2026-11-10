package workflow

import (
	"slices"
	"testing"
)

// AC2: every event has its row of verbs, and every verb of a row is a verb of an action, so the table never allows
// what an action cannot hold nor leaves an event without its verbs.
func TestEventVerbs(t *testing.T) {
	if len(EventVerbs) != len(Events) {
		t.Fatalf("%d rows for %d events", len(EventVerbs), len(Events))
	}
	for _, ev := range Events {
		verbs, ok := EventVerbs[ev]
		if !ok || len(verbs) == 0 {
			t.Errorf("%s: no verbs", ev)
		}
		for _, verb := range verbs {
			if !slices.Contains(Actions, verb) {
				t.Errorf("%s: %q is not a verb of Actions", ev, verb)
			}
		}
	}
}
