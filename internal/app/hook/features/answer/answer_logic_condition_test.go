package answer

import (
	"path/filepath"
	"testing"

	"github.com/mlophez/polix-ai/internal/app/agent"
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
		return stateOf(entry{Kind: kindObserved, Outcome: state.OutcomeFinished,
			Session: "s1", Event: workflow.SubagentStop, Actor: "polix:reviewer", Key: "polix:reviewer",
			Branch: "main", Hash: "h", Head: head})
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

// CUJ-04 and CUJ-05: finished_since_commit holds only while the latest hashed finished line of every type of its list,
// on any branch, is on the current HEAD, whatever changed since, and never reads the content.
func TestFinishedSinceCommit(t *testing.T) {
	gated := func(types string) string {
		return `version: 1
vars:
  key: { branch: "^([A-Z]+-[0-9]+)?" }
on:
  pre_commit:
    - when: { changed: "*.go" }
      unless: { finished_since_commit: ` + types + ` }
      action: { block: "Falta la revisión" }
`
	}
	line := func(key, branch, head, hash string) entry {
		return entry{Kind: kindObserved, Outcome: state.OutcomeFinished, Session: "s1",
			Event: workflow.SubagentStop, Actor: key, Key: key, Branch: branch, Hash: hash, Head: head}
	}
	cases := []struct {
		name, types, branch, head string
		lines                     []entry
		block                     bool
	}{
		{"same head and branch", "polix:builder", "main", "c1",
			[]entry{line("polix:builder", "main", "c1", "h")}, false},
		{"another branch on the same head", "polix:builder", "feat/x", "c1",
			[]entry{line("polix:builder", "main", "c1", "h")}, false},
		{"a detached head", "polix:builder", "", "c1",
			[]entry{line("polix:builder", "main", "c1", "h")}, false},
		{"unknown current head", "polix:builder", "main", "",
			[]entry{line("polix:builder", "main", "c1", "h")}, true},
		{"another head, same branch", "polix:builder", "main", "c2",
			[]entry{line("polix:builder", "main", "c1", "h")}, true},
		{"a line without hash", "polix:builder", "main", "c1",
			[]entry{line("polix:builder", "main", "", "")}, true},
		{"a hashed line without head", "polix:builder", "main", "c1",
			[]entry{line("polix:builder", "main", "", "h")}, true},
		{"a later line on another head", "polix:builder", "main", "c1",
			[]entry{line("polix:builder", "main", "c1", "h"), line("polix:builder", "main", "c2", "h2")}, true},
		{"a later line on another head and branch", "polix:builder", "main", "c1",
			[]entry{line("polix:builder", "main", "c1", "h"), line("polix:builder", "feat/x", "c2", "h2")}, true},
		{"a later line on the same head", "polix:builder", "main", "c2",
			[]entry{line("polix:builder", "main", "c1", "h"), line("polix:builder", "main", "c2", "h2")}, false},
		{"a later line without hash", "polix:builder", "main", "c1",
			[]entry{line("polix:builder", "main", "c1", "h"), line("polix:builder", "main", "", "")}, false},
		{"no line", "polix:builder", "main", "c1", nil, true},
		{"a line of another type", "polix:builder", "main", "c1",
			[]entry{line("polix:reviewer", "main", "c1", "h")}, true},
		{"only one type of the list", "[polix:builder, polix:verifier]", "main", "c1",
			[]entry{line("polix:builder", "main", "c1", "h")}, true},
		{"every type of the list", "[polix:builder, polix:verifier]", "main", "c1",
			[]entry{line("polix:builder", "main", "c1", "h"), line("polix:verifier", "main", "c1", "h2")}, false},
		{"a type of the list on another head", "[polix:builder, polix:verifier]", "main", "c2",
			[]entry{line("polix:builder", "main", "c2", "h"), line("polix:verifier", "main", "c1", "h2")}, true},
		// FND-03: an empty list, or a type that expands to an empty text, never holds, even on a line of another type.
		{"an empty list", "[]", "main", "c1",
			[]entry{line("polix:reviewer", "main", "c1", "h")}, true},
		{"a placeholder empty on the event", `"{path}"`, "main", "c1",
			[]entry{line("polix:reviewer", "main", "c1", "h")}, true},
		{"a var empty on the branch", `"{key}"`, "main", "c1",
			[]entry{line("polix:reviewer", "main", "c1", "h")}, true},
		{"an empty type in a list", `[polix:builder, "{path}"]`, "main", "c1",
			[]entry{line("polix:builder", "main", "c1", "h")}, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			// Go files changed since the lines, and no content hash is known: the condition never reads it.
			f := facts{branch: c.branch, head: c.head, state: stateOf(c.lines...),
				changed: []string{"internal/a.go"}}
			out, steps := walk(t, emptyListed(t, gated, c.types, "finished_since_commit"), f)
			if out.Answer.Block != c.block {
				t.Fatalf("block %v, want %v: %+v", out.Answer.Block, c.block, appended(steps))
			}
			for _, s := range steps {
				if s.Hash || s.Compare != "" {
					t.Fatalf("the condition read the content: %+v", s)
				}
			}
		})
	}
}

// emptyListed returns the evaluation on pre_commit of the workflow gated gives for types; for "[]", which polix config
// check rejects, the workflow of a valid type with the list of the unless of its entry emptied after the load.
func emptyListed(t *testing.T, gated func(string) string, types, key string) hook.Evaluation {
	t.Helper()
	if types != "[]" {
		return eval(t, gated(types), workflow.PreCommit)
	}
	ev := eval(t, gated("polix:builder"), workflow.PreCommit)
	ev.Workflow.On[workflow.PreCommit][0].Unless[key] = []any{}
	return ev
}

// FND-03: finished never holds with an empty list or a type that expands to an empty text, even when a line of
// another type finished on the current content.
func TestFinishedEmptyType(t *testing.T) {
	gated := func(types string) string {
		return `version: 1
vars:
  key: { branch: "^([A-Z]+-[0-9]+)?" }
on:
  pre_commit:
    - when: { changed: "*.go" }
      unless: { finished: ` + types + ` }
      action: { block: "Falta la revisión" }
`
	}
	reviewed := stateOf(entry{Kind: kindObserved, Outcome: state.OutcomeFinished,
		Session: "s1", Event: workflow.SubagentStop, Actor: "polix:reviewer", Key: "polix:reviewer",
		Branch: "main", Hash: "h", Head: "c1"})
	for _, types := range []string{"polix:reviewer", "[]", `"{path}"`, `"{key}"`, `[polix:reviewer, "{path}"]`} {
		t.Run(types, func(t *testing.T) {
			f := facts{branch: "main", head: "c1", hashes: []string{"h"}, state: reviewed,
				changed: []string{"internal/a.go"}}
			out, steps := walk(t, emptyListed(t, gated, types, "finished"), f)
			if want := types != "polix:reviewer"; out.Answer.Block != want {
				t.Fatalf("block %v, want %v: %+v", out.Answer.Block, want, appended(steps))
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

// skillEval returns the evaluation of the event skill of the main session for the skill name, with the workflow of
// yaml.
func skillEval(t *testing.T, yaml, name string) hook.Evaluation {
	t.Helper()
	wf, err := parseWorkflow(yaml, "dev")
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.FromSlash("/repo")
	ev := hook.Evaluation{Workflow: wf, Present: true, Root: root, GitDir: filepath.Join(root, ".git"), System: "linux",
		Event: agent.Event{Name: workflow.Skill, Agent: agent.Claude, Session: "s1", Actor: agent.ActorSession,
			Cwd: root, Skill: name}}
	if err := ev.Valid(); err != nil {
		t.Fatal(err)
	}
	return ev
}

// AC-03: the condition skill holds when one of its names equals the name of the skill or its bare name, the part
// after its last colon; never on another name, on a name that only starts with one of them, nor with a prefixed name
// on a skill without that prefix.
func TestConditionSkill(t *testing.T) {
	cases := []struct {
		cond, skill string
		want        bool
	}{
		{`"polix:spec"`, "polix:spec", true},
		{"spec", "polix:spec", true},
		{"spec", "spec", true},
		{"[polix:build, spec]", "polix:spec", true},
		{"[polix:build, spec]", "other:spec", true},
		{`"polix:spec"`, "spec", false},
		{"spec", "polix:spectre", false},
		{"spec", "polix:build", false},
		{"spe", "polix:spec", false},
		{"polix", "polix:spec", false},
		{"[polix:build, polix:plan]", "polix:spec", false},
	}
	for _, c := range cases {
		yaml := "version: 1\non:\n  skill:\n    - when: { skill: " + c.cond + " }\n" +
			"      action: { inject: \"Skill {skill} started.\" }\n"
		out, _ := walk(t, skillEval(t, yaml, c.skill), facts{branch: "main"})
		if got := len(out.Answer.Contexts) > 0; got != c.want {
			t.Errorf("skill %s on %s: holds %v, want %v", c.cond, c.skill, got, c.want)
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
