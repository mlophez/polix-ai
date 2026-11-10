package answer

import (
	"testing"

	"github.com/mlophez/polix-ai/internal/app/state"
	"github.com/mlophez/polix-ai/internal/app/workflow"
)

// branch holds on a match of its expression, never on one that does not compile.
func TestBranch(t *testing.T) {
	if !branchMatches("^main$", "main") || branchMatches("^main$", "dev") || branchMatches("(", "(") {
		t.Fatal("branchMatches")
	}
}

// AC1, AC2, AC3 and AC7: a finished recorded on a branch holds on another branch on the same HEAD commit while the
// files in scope did not change, not on another commit; on its own branch it holds after a commit moved HEAD; a
// line without head, or an unknown current head, matches only by branch.
func TestFinishedByHead(t *testing.T) {
	scoped := `version: 1
on:
  pre_commit:
    - when: { changed: "*.go" }
      unless: { finished: polix:reviewer }
      action: { block: "Revisa" }
`
	reviewed := func(head string) state.State {
		return state.State{Entries: []state.Entry{{Kind: state.KindObserved, Outcome: state.OutcomeFinished,
			Session: "s1", Event: workflow.SubagentStop, Actor: "polix:reviewer", Key: "polix:reviewer",
			Branch: "main", Hash: "h", Head: head}}}
	}
	cases := []struct {
		name, branch, head, logged string
		block                      bool
	}{
		{"another branch, same head", "feat/x", "c1", "c1", false},
		{"another branch, another head", "feat/x", "c2", "c1", true},
		{"same branch after a commit", "main", "c2", "c1", false},
		{"a line without head", "feat/x", "c1", "", true},
		{"unknown head, another branch", "feat/x", "", "c1", true},
		{"unknown head, same branch", "main", "", "c1", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			// Only a markdown file changed since the review: outside the scope of the entry.
			f := facts{branch: c.branch, head: c.head, hashes: []string{"h2"}, state: reviewed(c.logged),
				trees: []string{"ai/reports/r.md"}, changed: []string{"internal/a.go"}}
			out, steps := walk(t, eval(t, scoped, workflow.PreCommit), f)
			if out.Answer.Block != c.block {
				t.Fatalf("block %v, want %v: %+v", out.Answer.Block, c.block, appended(steps))
			}
		})
	}
}
