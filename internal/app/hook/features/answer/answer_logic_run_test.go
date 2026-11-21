package answer

import (
	"testing"

	"github.com/mlophez/polix-ai/internal/app/state"
	"github.com/mlophez/polix-ai/internal/app/workflow"
)

// indent indents every line, and nothing for an empty text.
func TestIndent(t *testing.T) {
	if indent("a\nb\n") != "  a\n  b" || indent("") != "" {
		t.Fatal("indent")
	}
}

// AC4: a run that passed on a branch is skipped on another branch on the same HEAD commit and content hash, and
// runs on a branch on another commit; the passed line it writes carries the head.
func TestRunRepeatByHead(t *testing.T) {
	ev := eval(t, "version: 1\non:\n  stop:\n    - action: { run: just test }\n", workflow.Stop)
	passed := state.State{Entries: []state.Entry{{Kind: state.KindObserved, Outcome: state.OutcomePassed,
		Session: "s1", Event: workflow.Stop, Key: "just test", Line: 4, Branch: "main", Hash: "h", Head: "c1"}}}
	out, steps := walk(t, ev, facts{branch: "feat/x", head: "c1", hashes: []string{"h", "h"}, state: passed})
	if skips := decided(steps, "skip"); out.Answer.Block || len(runs(steps)) != 0 || len(skips) != 1 {
		t.Fatalf("same head: %q %+v", runs(steps), appended(steps))
	}
	_, steps = walk(t, ev, facts{branch: "feat/x", head: "c2", hashes: []string{"h", "h"}, state: passed})
	if len(runs(steps)) != 1 {
		t.Fatalf("another head: %q", runs(steps))
	}
	if got := observed(steps); len(got) != 1 || got[0].Outcome != state.OutcomePassed || got[0].Head != "c2" ||
		got[0].Branch != "feat/x" {
		t.Fatalf("passed: %+v", got)
	}
}
