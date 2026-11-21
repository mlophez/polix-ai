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
	// post_commit is an informed event that comes right after pre_commit, with the verbs of post_command.
	if want := []string{VerbRun, VerbInject, VerbNotify}; !slices.Equal(EventVerbs[PostCommit], want) {
		t.Errorf("post_commit: %q, want %q", EventVerbs[PostCommit], want)
	}
	if !slices.Contains(Informed, PostCommit) {
		t.Error("post_commit is not informed")
	}
	if i := slices.Index(Events, PreCommit); i < 0 || Events[i+1] != PostCommit {
		t.Errorf("post_commit does not follow pre_commit: %q", Events)
	}
}
