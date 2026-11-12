package answer

import (
	"testing"

	"github.com/mlophez/polix-ai/internal/app/hook"
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

// commandEval returns the evaluation of the event name for the command line with the exit code, with the workflow
// of yaml.
func commandEval(t *testing.T, yaml, name, line string, exit int) hook.Evaluation {
	t.Helper()
	ev := eval(t, yaml, name)
	e := ev.Event
	e.Command, e.ExitCode = line, exit
	out := hook.Evaluation{Workflow: ev.Workflow, Present: true, Root: ev.Root, GitDir: ev.GitDir, System: ev.System,
		Event: e}
	if err := out.Valid(); err != nil {
		t.Fatal(err)
	}
	return out
}

// AC8: the exit code counts for a command nested in a -c script only when it is the last one of a script that
// propagates; a command inside $(...) never propagates.
func TestConditionExitNested(t *testing.T) {
	yaml := `version: 1
on:
  post_command:
    - when: { command: "just test*", exit: 1 }
      action: { inject: { text: "Los tests fallan", always: true } }
`
	for line, want := range map[string]bool{
		"bash -c 'true && just test'":        true,
		"echo $(just test)":                  false,
		"bash -c 'just test; true'":          false,
		"bash -c 'true && just test' | tail": false,
		"echo $(just test && true)":          false,
		"! just test":                        false,
	} {
		out, _ := walk(t, commandEval(t, yaml, workflow.PostCommand, line, 1), facts{branch: "main",
			hashes: []string{"h"}})
		if got := len(out.Answer.Contexts) > 0; got != want {
			t.Errorf("%q: exit holds %v, want %v", line, got, want)
		}
	}
}

// AC4: a command pattern holds on a command as written or normalized, past assignments, wrappers and the global
// options of git, nested commands included; never on the text of an argument.
func TestConditionCommandNormalized(t *testing.T) {
	yaml := `version: 1
on:
  pre_command:
    - when: { command: ["git add*", "git stage*"] }
      action: { block: "Formatea antes" }
`
	for line, want := range map[string]bool{
		"git add .": true, "cd frontend && git add -A": true, "git -C x add .": true,
		"git -c core.x=y add .": true, "git --no-pager add .": true, "sudo env FOO=1 git add .": true,
		"bash -c 'git add .'": true, "git stage x": true,
		`git commit -m "add foo"`: false, `echo "git add"`: false, "git status": false,
	} {
		out, _ := walk(t, commandEval(t, yaml, workflow.PreCommand, line, 0), facts{branch: "main",
			hashes: []string{"h"}})
		if out.Answer.Block != want {
			t.Errorf("%q: command holds %v, want %v", line, out.Answer.Block, want)
		}
	}
}
