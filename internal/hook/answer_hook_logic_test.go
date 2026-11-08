package hook

import (
	"errors"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/mlophez/polix-ai/internal/agent"
	"github.com/mlophez/polix-ai/internal/diff"
	"github.com/mlophez/polix-ai/internal/rule"
	"github.com/mlophez/polix-ai/internal/state"
	"github.com/mlophez/polix-ai/internal/workflow"
)

// The logic is walked step by step with domain values: each step it asks is answered from a table of facts and
// outcomes, as the shell would from the port, and fed back to the evaluation.

// answerHookFacts are the facts and outcomes a walk answers the steps with.
type answerHookFacts struct {
	branch    string
	hashes    []string // the content hash at each epoch
	hashErr   error
	state     state.State
	stateErr  error
	appendErr error
	runErr    error // the outcome of every run and check
	changed   []string
	diff      diff.Diff
	diffErr   error
	rules     []rule.Rule
	found     bool     // whether a file holds the text of a search
	trees     []string // the paths that differ between two trees
	treeErr   error    // why two trees cannot be compared
	// checkFails, when set, decides each check instead of runErr: it fails when it covers one of these rules
	checkFails map[string]bool
}

// answerHookWalk asks the logic for steps until it answers, feeding each one from f, and returns the output and
// the steps asked, in order.
func answerHookWalk(t *testing.T, ev Evaluation, f answerHookFacts) (AnswerHookOutput, []Step) {
	t.Helper()
	var steps []Step
	for range 100 {
		out := AnswerHook(ev)
		s := out.Step
		if s.None() {
			return out, steps
		}
		if err := s.Valid(); err != nil {
			t.Fatalf("step %+v: %v", s, err)
		}
		steps = append(steps, s)
		switch {
		case s.Branch:
			ev = ev.KnowBranch(f.branch)
		case s.Hash:
			h := ""
			if s.Epoch < len(f.hashes) {
				h = f.hashes[s.Epoch]
			}
			ev = ev.KnowHash(s.Epoch, h, f.hashErr)
		case s.Compare != "":
			ev = ev.KnowTreeDiff(s.Compare, s.Tree, f.trees, f.treeErr)
		case s.Changed:
			ev = ev.KnowChanged(f.changed)
		case s.Diff:
			ev = ev.KnowDiff(f.diff, f.diffErr)
		case s.Files:
			ev = ev.KnowFiles(nil)
		case s.Rules:
			ev = ev.KnowRules(f.rules, nil)
		case s.State:
			ev = ev.KnowState(f.state, f.stateErr)
		case s.Ignored != "":
			ev = ev.KnowIgnored(s.Ignored, false)
		case s.Real != nil:
			ev = ev.KnowReal(s.Real, s.Real)
		case s.Contains != nil:
			ev = ev.KnowFound(s.Epoch, s.Contains, s.Text, f.found)
		case s.Read != "":
			ev = ev.KnowContent(s.Epoch, s.Read, "", errors.New("no file"))
		case s.Check != nil && f.checkFails != nil:
			var err error
			for _, r := range s.Check.Rules {
				if f.checkFails[r.ID] {
					err = errors.New("exit status 1")
				}
			}
			ev = ev.Note("salida", err)
		case s.Run != nil, s.Check != nil:
			ev = ev.Note("salida", f.runErr)
		case s.Append != nil:
			ev = ev.Note("", f.appendErr)
		}
	}
	t.Fatal("the evaluation never answers")
	return AnswerHookOutput{}, nil
}

// answerHookEval returns the evaluation of event name of the main session with the workflow of yaml.
func answerHookEval(t *testing.T, yaml, name string) Evaluation {
	t.Helper()
	wf, err := workflow.Parse([]byte(yaml), "dev")
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.FromSlash("/repo")
	ev := Evaluation{Workflow: wf, Present: true, Root: root, GitDir: filepath.Join(root, ".git"), System: "linux",
		Event: agent.Event{Name: name, Agent: agent.Claude, Session: "s1", Actor: agent.ActorSession, Cwd: root}}
	if err := ev.Valid(); err != nil {
		t.Fatal(err)
	}
	return ev
}

// answerHookAs returns ev as the event of the subagent actor.
func answerHookAs(t *testing.T, ev Evaluation, actor string) Evaluation {
	t.Helper()
	e := ev.Event
	e.Actor = actor
	out := Evaluation{Workflow: ev.Workflow, Present: ev.Present, Root: ev.Root, GitDir: ev.GitDir, System: ev.System,
		Event: e}
	if err := out.Valid(); err != nil {
		t.Fatal(err)
	}
	return out
}

// answerHookAppended returns the entries the steps appended to the log.
func answerHookAppended(steps []Step) []state.Entry {
	var out []state.Entry
	for _, s := range steps {
		if s.Append != nil {
			out = append(out, *s.Append)
		}
	}
	return out
}

// answerHookObserved returns the observed entries the steps appended to the log, in order.
func answerHookObserved(steps []Step) []state.Entry {
	var out []state.Entry
	for _, e := range answerHookAppended(steps) {
		if e.Kind == state.KindObserved {
			out = append(out, e)
		}
	}
	return out
}

// answerHookFinished is the log after the subagent polix:reviewer finished on the content hash.
func answerHookFinished(hash string) state.State {
	return state.State{Entries: []state.Entry{{Kind: state.KindObserved, Outcome: state.OutcomeFinished,
		Session: "s1", Event: workflow.SubagentStop, Actor: "polix:reviewer", Key: "polix:reviewer", Branch: "main",
		Hash: hash}}}
}

// A run that rewrites files changes the content: a later finished is decided on the hash of the new content.
func TestAnswerHookFinishedAfterRunUsesNewContent(t *testing.T) {
	ev := answerHookEval(t, `version: 1
on:
  stop:
    - action: { run: [gofmt, -w, .] }
    - unless: { finished: polix:reviewer }
      action: { block: "Falta la revisión" }
`, workflow.Stop)
	reviewed := answerHookFinished("new")
	out, steps := answerHookWalk(t, ev, answerHookFacts{branch: "main", hashes: []string{"old", "new"}, state: reviewed})
	if out.Answer.Block {
		t.Fatalf("finished was decided on the old content: %+v", out.Answer)
	}
	// The hash of epoch 0 is the one the run is keyed by; finished asks for the one of epoch 1, after the run.
	ran := false
	for _, s := range steps {
		ran = ran || s.Run != nil
		if s.Hash && s.Epoch != 0 && !ran || s.Hash && s.Epoch != 1 && ran {
			t.Fatalf("the hash of epoch %d was asked with the run done: %v", s.Epoch, ran)
		}
	}
	if out.Facts.Epoch != 1 {
		t.Fatalf("the next event starts after the run: epoch %d", out.Facts.Epoch)
	}
	// The run changed the content it reviewed: without a changed, any differing path counts.
	out, _ = answerHookWalk(t, ev, answerHookFacts{branch: "main", hashes: []string{"new", "old"}, state: reviewed,
		trees: []string{"a.go"}})
	if !out.Answer.Block {
		t.Fatal("finished held on a content it was not observed on")
	}
}

// A block that actually blocks ends the evaluation: the entries after it never run, an inject before it reaches the
// agent with the block, and the decision is logged with its verb and line.
func TestAnswerHookBlockEnds(t *testing.T) {
	ev := answerHookEval(t, `version: 1
on:
  pre_command:
    - action: { inject: { text: "cuidado", always: true } }
    - action: { block: "primero" }
    - action: { block: "segundo" }
`, workflow.PreCommand)
	out, steps := answerHookWalk(t, ev, answerHookFacts{branch: "main"})
	if !out.Answer.Block || !slices.Equal(out.Answer.Reasons, []string{"primero"}) ||
		!slices.Equal(out.Answer.Contexts, []string{"cuidado"}) {
		t.Fatalf("answer %+v", out.Answer)
	}
	logged := answerHookAppended(steps)
	if len(logged) != 1 || logged[0].Action != workflow.VerbBlock || logged[0].Outcome != "block" || logged[0].Line != 5 {
		t.Fatalf("logged: %+v", logged)
	}
}

// answerHookRuns returns the commands the steps ran, in order.
func answerHookRuns(steps []Step) [][]string {
	var out [][]string
	for _, s := range steps {
		if s.Run != nil {
			out = append(out, s.Run)
		}
	}
	return out
}

// AC6: a failing run that actually blocks ends the evaluation as a block does: a stop with two failing runs
// blocks with the output of the first one only and never runs the second.
func TestAnswerHookFailingRunEnds(t *testing.T) {
	ev := answerHookEval(t, `version: 1
on:
  stop:
    - action: { run: just lint }
    - action: { run: just test }
    - action: { block: "nunca" }
`, workflow.Stop)
	out, steps := answerHookWalk(t, ev, answerHookFacts{branch: "main", hashes: []string{"h", "h"},
		runErr: errors.New("exit status 1")})
	if runs := answerHookRuns(steps); len(runs) != 1 || !slices.Equal(runs[0], []string{"just", "lint"}) {
		t.Fatalf("only the first run runs: %q", runs)
	}
	if !out.Answer.Block || len(out.Answer.Reasons) != 1 ||
		!strings.HasPrefix(out.Answer.Reasons[0], "Falla just lint: exit status 1") {
		t.Fatalf("answer: %+v", out.Answer)
	}
	logged := answerHookAppended(steps)
	if last := logged[len(logged)-1]; last.Action != workflow.VerbRun || last.Outcome != "block" || last.Line != 4 {
		t.Fatalf("logged: %+v", logged)
	}
	// A passing run lets the evaluation go on: the second one runs and blocks on its own.
	f := answerHookFacts{branch: "main", hashes: []string{"h", "h", "h"}}
	out, steps = answerHookWalk(t, ev, f)
	if runs := answerHookRuns(steps); len(runs) != 2 || !out.Answer.Block ||
		!slices.Equal(out.Answer.Reasons, []string{"nunca"}) {
		t.Fatalf("passing runs: %q %+v", runs, out.Answer)
	}
}

// AC7: a block or a failing run let through by the loop guard does not end the evaluation: the next entry runs and
// blocks on its own, with the warning of the guard.
func TestAnswerHookLoopGuardGoesOn(t *testing.T) {
	yaml := `version: 1
max_stop_blocks: 1
on:
  stop:
    - action: { %s }
    - action: { run: just test }
`
	guarded := state.State{Entries: []state.Entry{{Kind: state.KindObserved, Outcome: state.OutcomeBlocked,
		Session: "s1", Event: workflow.Stop, Actor: agent.ActorSession, Line: 5, Hash: "h"}}}
	for name, verb := range map[string]string{"run": "run: just lint", "block": `block: "revisa"`} {
		t.Run(name, func(t *testing.T) {
			ev := answerHookEval(t, strings.Replace(yaml, "%s", verb, 1), workflow.Stop)
			out, steps := answerHookWalk(t, ev, answerHookFacts{branch: "main", hashes: []string{"h", "h", "h"},
				state: guarded, runErr: errors.New("exit status 1")})
			runs := answerHookRuns(steps)
			if len(runs) == 0 || !slices.Equal(runs[len(runs)-1], []string{"just", "test"}) {
				t.Fatalf("the next entry runs: %q", runs)
			}
			if !out.Answer.Block || len(out.Answer.Reasons) != 1 ||
				!strings.HasPrefix(out.Answer.Reasons[0], "Falla just test: exit status 1") ||
				len(out.Answer.Warnings) != 1 || !strings.Contains(out.Answer.Warnings[0], "tras 1 bloqueos") {
				t.Fatalf("answer: %+v", out.Answer)
			}
		})
	}
}

// AC7: a failing run on post_edit informs and never ends the evaluation: both failures reach the AI.
func TestAnswerHookInformingRunsGoOn(t *testing.T) {
	ev := answerHookEval(t, `version: 1
on:
  post_edit:
    - action: { run: just lint }
    - action: { run: just test }
`, workflow.PostEdit)
	out, steps := answerHookWalk(t, ev, answerHookFacts{branch: "main", runErr: errors.New("exit status 1")})
	if runs := answerHookRuns(steps); len(runs) != 2 {
		t.Fatalf("both runs run: %q", runs)
	}
	if out.Answer.Block || len(out.Answer.Contexts) != 2 ||
		!strings.HasPrefix(out.Answer.Contexts[0], "Falla just lint: exit status 1") ||
		!strings.HasPrefix(out.Answer.Contexts[1], "Falla just test: exit status 1") {
		t.Fatalf("answer: %+v", out.Answer)
	}
}

// AC6 and AC7 on pre_edit: a block before an inject drops the inject, and an inject before a block reaches the
// agent with the block.
func TestAnswerHookBlockAndInjectOrder(t *testing.T) {
	cases := []struct {
		name, entries string
		contexts      []string
	}{
		{"block first", "    - action: { block: \"no\" }\n    - action: { inject: \"después\" }\n", nil},
		{"inject first", "    - action: { inject: \"antes\" }\n    - action: { block: \"no\" }\n", []string{"antes"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			base := answerHookEval(t, "version: 1\non:\n  pre_edit:\n"+c.entries, workflow.PreEdit)
			ev := Evaluation{Workflow: base.Workflow, Present: true, Root: base.Root, GitDir: base.GitDir,
				System: "linux", Event: agent.Event{Name: workflow.PreEdit, Agent: agent.Claude, Session: "s1",
					Actor: agent.ActorSession, Path: "src/a.go", AbsPath: filepath.Join(base.Root, "src", "a.go")}}
			if err := ev.Valid(); err != nil {
				t.Fatal(err)
			}
			out, _ := answerHookWalk(t, ev, answerHookFacts{branch: "main"})
			if !out.Answer.Block || !slices.Equal(out.Answer.Reasons, []string{"no"}) ||
				!slices.Equal(out.Answer.Contexts, c.contexts) {
				t.Fatalf("answer: %+v", out.Answer)
			}
		})
	}
}

// An exception written as a not in when (once allow) skips the block for what it names.
func TestAnswerHookExceptionWithNot(t *testing.T) {
	ev := answerHookEval(t, `version: 1
on:
  pre_edit:
    - when: { not: { path: "ai/plans/*.md" } }
      action: { block: "No hay plan" }
`, workflow.PreEdit)
	for path, block := range map[string]bool{"ai/plans/p.md": false, "src/a.go": true} {
		e := Evaluation{Workflow: ev.Workflow, Present: true, Root: ev.Root, GitDir: ev.GitDir, System: "linux",
			Event: agent.Event{Name: workflow.PreEdit, Agent: agent.Claude, Actor: agent.ActorSession, Path: path,
				AbsPath: filepath.Join(ev.Root, filepath.FromSlash(path))}}
		if err := e.Valid(); err != nil {
			t.Fatal(err)
		}
		if out, _ := answerHookWalk(t, e, answerHookFacts{branch: "main"}); out.Answer.Block != block {
			t.Errorf("%s: %+v", path, out.Answer)
		}
	}
}

// The loop guard lets the agent stop after max_stop_blocks blocks of the entry on the same content.
func TestAnswerHookLoopGuard(t *testing.T) {
	ev := answerHookEval(t, "version: 1\nmax_stop_blocks: 2\non:\n  stop:\n    - action: { block: \"revisa\" }\n",
		workflow.Stop)
	block := state.Entry{Kind: state.KindObserved, Outcome: state.OutcomeBlocked, Session: "s1",
		Event: workflow.Stop, Actor: agent.ActorSession, Line: 5, Hash: "h"}
	one := state.State{Entries: []state.Entry{block}}
	out, steps := answerHookWalk(t, ev, answerHookFacts{branch: "main", hashes: []string{"h"}, state: one})
	if !out.Answer.Block {
		t.Fatal("one block of two: the stop is blocked")
	}
	if logged := answerHookAppended(steps); len(logged) != 2 || logged[0].Kind != state.KindObserved ||
		logged[0].Outcome != state.OutcomeBlocked || logged[0].Hash != "h" || logged[0].Line != 5 ||
		logged[1].Outcome != "block" {
		t.Fatalf("logged: %+v", logged)
	}
	two := state.State{Entries: []state.Entry{block, block}}
	out, steps = answerHookWalk(t, ev, answerHookFacts{branch: "main", hashes: []string{"h"}, state: two})
	if out.Answer.Block || len(out.Answer.Warnings) != 1 || !strings.Contains(out.Answer.Warnings[0], "tras 2 bloqueos") {
		t.Fatalf("answer after max_stop_blocks: %+v", out.Answer)
	}
	if logged := answerHookAppended(steps); len(logged) != 1 || logged[0].Outcome != "loop_guard" {
		t.Fatalf("logged: %+v", logged)
	}
	// Another content has its own count.
	out, _ = answerHookWalk(t, ev, answerHookFacts{branch: "main", hashes: []string{"other"}, state: two})
	if !out.Answer.Block {
		t.Fatal("the blocks of another content count")
	}
}

// A log that cannot be read or written lets the agent stop: the guard cannot count.
func TestAnswerHookLoopGuardFailsOpen(t *testing.T) {
	ev := answerHookEval(t, "version: 1\non:\n  stop:\n    - action: { block: \"revisa\" }\n", workflow.Stop)
	for name, f := range map[string]answerHookFacts{
		"unreadable": {branch: "main", hashes: []string{"h"}, stateErr: errors.New("permission denied")},
		"read only":  {branch: "main", hashes: []string{"h"}, appendErr: errors.New("permission denied")},
	} {
		out, _ := answerHookWalk(t, ev, f)
		if out.Answer.Block || len(out.Answer.Warnings) != 1 ||
			!strings.Contains(out.Answer.Warnings[0], "no puede usar su log (permission denied): revisa") {
			t.Errorf("%s: %+v", name, out.Answer)
		}
	}
}

// A failing run blocks with its output; a passing one writes only that it passed.
func TestAnswerHookRun(t *testing.T) {
	ev := answerHookEval(t, "version: 1\non:\n  stop:\n    - action: { run: just test }\n", workflow.Stop)
	out, steps := answerHookWalk(t, ev, answerHookFacts{branch: "main", hashes: []string{"h", "h"}})
	if logged := answerHookAppended(steps); out.Answer.Block || len(logged) != 1 ||
		logged[0].Outcome != state.OutcomePassed {
		t.Fatalf("a passing run: %+v %+v", out.Answer, logged)
	}
	var runs [][]string
	for _, s := range steps {
		if s.Run != nil {
			runs = append(runs, s.Run)
		}
	}
	if len(runs) != 1 || !slices.Equal(runs[0], []string{"just", "test"}) {
		t.Fatalf("the run string is split on spaces: %q", runs)
	}
	out, _ = answerHookWalk(t, ev, answerHookFacts{branch: "main", hashes: []string{"h", "h"},
		runErr: errors.New("exit status 1")})
	if !out.Answer.Block || !strings.HasPrefix(out.Answer.Reasons[0], "Falla just test: exit status 1\n  salida") {
		t.Fatalf("a failing run blocks with its output: %+v", out.Answer)
	}
}

// AC9: a failing run with a message answers with the message, a blank line and the output.
func TestAnswerHookRunMessage(t *testing.T) {
	ev := answerHookEval(t, "version: 1\non:\n  stop:\n"+
		"    - action: { run: { command: [just, test], message: \"Arregla los tests de {branch}\" } }\n", workflow.Stop)
	out, _ := answerHookWalk(t, ev, answerHookFacts{branch: "main", hashes: []string{"h"},
		runErr: errors.New("exit status 1")})
	if !out.Answer.Block || !strings.HasPrefix(out.Answer.Reasons[0],
		"Arregla los tests de main\n\nFalla just test: exit status 1\n  salida") {
		t.Fatalf("answer: %+v", out.Answer)
	}
	out, _ = answerHookWalk(t, ev, answerHookFacts{branch: "main", hashes: []string{"h"}})
	if out.Answer.Block || len(out.Answer.Contexts) != 0 {
		t.Fatalf("a passing run says nothing: %+v", out.Answer)
	}
}

// AC21: a failing run on start, post_edit or post_command never blocks: its message and output reach the
// AI as context, the evaluation goes on, and the decision is logged as inform.
func TestAnswerHookRunInformsAfterTheFact(t *testing.T) {
	for _, event := range []string{workflow.Start, workflow.PostEdit, workflow.PostCommand} {
		t.Run(event, func(t *testing.T) {
			ev := answerHookEval(t, "version: 1\nguidance: { commit: false, edit: false, layout: false }\non:\n  "+
				event+":\n    - action: { run: { command: [ruff, check], message: \"Corrige el estilo\" } }\n"+
				"    - action: { inject: { text: \"después\", always: true } }\n", event)
			out, steps := answerHookWalk(t, ev, answerHookFacts{branch: "main", runErr: errors.New("exit status 1")})
			if out.Answer.Block || len(out.Answer.Contexts) != 2 ||
				!strings.HasPrefix(out.Answer.Contexts[0], "Corrige el estilo\n\nFalla ruff check: exit status 1\n  salida") ||
				out.Answer.Contexts[1] != "después" {
				t.Fatalf("answer: %+v", out.Answer)
			}
			if logged := answerHookAppended(steps); len(logged) != 1 || logged[0].Outcome != "inform" ||
				logged[0].Action != workflow.VerbRun || logged[0].Line != 5 {
				t.Fatalf("logged: %+v", logged)
			}
		})
	}
}

// AC17: a command of the AI, green or not, writes no observed line and never makes finished hold.
func TestAnswerHookCommandsOfTheAINeverCount(t *testing.T) {
	ev := answerHookEval(t, `version: 1
on:
  post_command:
    - when: { command: "just test*", exit: 1 }
      action: { inject: { text: "Los tests fallan", always: true } }
  pre_commit:
    - unless: { finished: polix:reviewer }
      action: { block: "Revisa" }
`, workflow.PostCommand)
	for _, exit := range []int{0, 1} {
		e := ev.Event
		e.Command, e.ExitCode = "just test", exit
		post := Evaluation{Workflow: ev.Workflow, Present: true, Root: ev.Root, GitDir: ev.GitDir, System: "linux",
			Event: e}
		if err := post.Valid(); err != nil {
			t.Fatal(err)
		}
		_, steps := answerHookWalk(t, post, answerHookFacts{branch: "main", hashes: []string{"h"}})
		if len(answerHookAppended(steps)) != 0 {
			t.Fatalf("exit %d: %+v", exit, answerHookAppended(steps))
		}
	}
	commit := answerHookEval(t, `version: 1
on:
  pre_commit:
    - unless: { finished: polix:reviewer }
      action: { block: "Revisa" }
`, workflow.PreCommit)
	if out, _ := answerHookWalk(t, commit, answerHookFacts{branch: "main", hashes: []string{"h"}}); !out.Answer.Block {
		t.Fatalf("finished holds without a subagent: %+v", out.Answer)
	}
}

// AC16 and AC19: a subagent_stop of an observed type that is not refused writes finished with the hash; a type the
// workflow does not observe, a refused stop and a stop let through by the loop guard write nothing.
func TestAnswerHookObservesFinished(t *testing.T) {
	yaml := `version: 1
max_stop_blocks: 1
on:
  pre_commit:
    - unless: { finished: polix:reviewer }
      action: { block: "Revisa" }
  subagent_stop:
    - when: { actor: polix:reviewer, branch: "^refuse$" }
      action: { block: "Termina la revisión" }
`
	stop := answerHookEval(t, yaml, workflow.SubagentStop)
	reviewer := answerHookAs(t, stop, "polix:reviewer")
	_, steps := answerHookWalk(t, reviewer, answerHookFacts{branch: "main", hashes: []string{"h"}})
	got := answerHookObserved(steps)
	if len(got) != 1 || got[0].Outcome != state.OutcomeFinished || got[0].Key != "polix:reviewer" ||
		got[0].Hash != "h" || got[0].Branch != "main" || got[0].Actor != "polix:reviewer" ||
		got[0].Event != workflow.SubagentStop {
		t.Fatalf("finished: %+v", got)
	}
	// Another type is not observed, and no hash is computed for it.
	_, steps = answerHookWalk(t, answerHookAs(t, stop, "acme:builder"), answerHookFacts{branch: "main",
		hashes: []string{"h"}})
	if len(answerHookObserved(steps)) != 0 || slices.ContainsFunc(steps, func(s Step) bool { return s.Hash }) {
		t.Fatalf("another type: %+v", steps)
	}
	// A refused stop: the subagent goes on, and only the block is observed, for the loop guard.
	out, steps := answerHookWalk(t, reviewer, answerHookFacts{branch: "refuse", hashes: []string{"h"}})
	if got := answerHookObserved(steps); !out.Answer.Block || len(got) != 1 || got[0].Outcome != state.OutcomeBlocked {
		t.Fatalf("refused: %+v %+v", out.Answer, got)
	}
	// Let through by the loop guard: the subagent stops, but it did not finish as asked.
	guard := state.State{Entries: []state.Entry{{Kind: state.KindObserved, Outcome: state.OutcomeBlocked,
		Session: "s1", Event: workflow.SubagentStop, Actor: "polix:reviewer", Line: 8, Hash: "h"}}}
	out, steps = answerHookWalk(t, reviewer, answerHookFacts{branch: "refuse", hashes: []string{"h"}, state: guard})
	if out.Answer.Block || len(answerHookObserved(steps)) != 0 {
		t.Fatalf("loop guard: %+v %+v", out.Answer, answerHookObserved(steps))
	}
	// An unknown hash writes nothing, with the decision.
	out, steps = answerHookWalk(t, reviewer, answerHookFacts{branch: "main", hashErr: errors.New("permission denied")})
	logged := answerHookAppended(steps)
	if len(answerHookObserved(steps)) != 0 || len(logged) != 1 || logged[0].Outcome != "warn" ||
		len(out.Answer.Warnings) != 1 || !strings.Contains(out.Answer.Warnings[0], "hash del contenido (permission denied)") {
		t.Fatalf("unknown hash: %+v %+v", out.Answer, logged)
	}
	// Without a finished in the workflow, nothing is observed.
	plain := answerHookAs(t, answerHookEval(t, "version: 1\n", workflow.SubagentStop), "polix:reviewer")
	if _, steps := answerHookWalk(t, plain, answerHookFacts{branch: "main", hashes: []string{"h"}}); len(steps) != 0 {
		t.Fatalf("nothing to observe: %+v", steps)
	}
}

// AC16: finished holds after the subagent ended and after an edit outside the scope of the entry, not after an edit
// inside it, nor without a changed after any edit, nor when the trees cannot be compared, nor for another type or
// another branch.
func TestAnswerHookFinished(t *testing.T) {
	scoped := `version: 1
on:
  pre_commit:
    - when: { changed: "*.go" }
      unless: { finished: polix:reviewer }
      action: { block: "Revisa" }
`
	whole := "version: 1\non:\n  pre_commit:\n    - unless: { finished: polix:reviewer }\n      action: { block: \"Revisa\" }\n"
	listed := "version: 1\non:\n  pre_commit:\n    - unless: { finished: [polix:reviewer, acme:docs] }\n" +
		"      action: { block: \"Revisa\" }\n"
	cases := []struct {
		name, yaml string
		f          answerHookFacts
		block      bool
	}{
		{"same content", scoped, answerHookFacts{hashes: []string{"h"}, state: answerHookFinished("h")}, false},
		{"edit of README.md", scoped, answerHookFacts{hashes: []string{"h2"}, state: answerHookFinished("h"),
			trees: []string{"README.md", "docs/polix.md"}}, false},
		{"edit of a Go file", scoped, answerHookFacts{hashes: []string{"h2"}, state: answerHookFinished("h"),
			trees: []string{"README.md", "internal/a.go"}}, true},
		{"no changed, any edit", whole, answerHookFacts{hashes: []string{"h2"}, state: answerHookFinished("h"),
			trees: []string{"README.md"}}, true},
		{"no changed, same content", whole, answerHookFacts{hashes: []string{"h"}, state: answerHookFinished("h")}, false},
		{"tree missing", scoped, answerHookFacts{hashes: []string{"h2"}, state: answerHookFinished("h"),
			treeErr: errors.New("fatal: bad object h")}, true},
		{"unknown hash", scoped, answerHookFacts{hashErr: errors.New("boom"), state: answerHookFinished("h")}, true},
		{"never finished", scoped, answerHookFacts{hashes: []string{"h"}}, true},
		{"another branch", scoped, answerHookFacts{branch: "dev", hashes: []string{"h"}, state: answerHookFinished("h")},
			true},
		{"every type of a list", listed, answerHookFacts{hashes: []string{"h"}, state: answerHookFinished("h")}, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if c.f.branch == "" {
				c.f.branch = "main"
			}
			c.f.changed = []string{"internal/a.go"}
			out, steps := answerHookWalk(t, answerHookEval(t, c.yaml, workflow.PreCommit), c.f)
			if out.Answer.Block != c.block {
				t.Fatalf("block %v, want %v: %+v", out.Answer.Block, c.block, out.Answer)
			}
			if c.f.treeErr != nil {
				logged := answerHookAppended(steps)
				if len(logged) == 0 || logged[0].Outcome != "warn" || logged[0].Line != 4 ||
					!strings.Contains(logged[0].Reason, "fatal: bad object h") {
					t.Fatalf("the failed comparison is logged: %+v", logged)
				}
			}
		})
	}
	// Another type that finished on the same content does not count.
	other := state.State{Entries: []state.Entry{{Kind: state.KindObserved, Outcome: state.OutcomeFinished,
		Key: "acme:builder", Branch: "main", Hash: "h"}}}
	out, _ := answerHookWalk(t, answerHookEval(t, scoped, workflow.PreCommit), answerHookFacts{branch: "main",
		hashes: []string{"h"}, state: other, changed: []string{"a.go"}})
	if !out.Answer.Block {
		t.Fatal("finished held for another type")
	}
	// The latest finished counts: a later one on another content replaces an earlier one on this content.
	later := answerHookFinished("h")
	later.Entries = append(later.Entries, answerHookFinished("h0").Entries...)
	out, _ = answerHookWalk(t, answerHookEval(t, scoped, workflow.PreCommit), answerHookFacts{branch: "main",
		hashes: []string{"h"}, state: later, changed: []string{"a.go"}, trees: []string{"a.go"}})
	if !out.Answer.Block {
		t.Fatal("an earlier finished counted over the latest")
	}
}

// run: { rules: changed } runs the checks of the rules of the changed files through the runner of their file.
func TestAnswerHookRunRules(t *testing.T) {
	ev := answerHookEval(t, `version: 1
rules:
  run:
    "*_test.go": [go, test, "{dir}", -run, "^Test{name}$"]
on:
  stop:
    - action: { run: { rules: changed } }
`, workflow.Stop)
	rules := []rule.Rule{
		{ID: "GO-1", Description: "d", Scope: []string{"src/**"}, Deterministic: true, Check: "tests/go1_test.go"},
		{ID: "SH-1", Description: "s", Scope: []string{"src/**"}, Deterministic: true, Check: "checks/sh1.sh"},
		{ID: "DOC-1", Description: "x", Scope: []string{"docs/**"}, Deterministic: true, Check: "checks/doc.sh"},
	}
	out, steps := answerHookWalk(t, ev, answerHookFacts{branch: "main", changed: []string{"src/a.go"}, rules: rules,
		runErr: errors.New("exit status 1")})
	var checks []rule.Run
	for _, s := range steps {
		if s.Check != nil {
			checks = append(checks, *s.Check)
		}
	}
	if len(checks) != 2 || !slices.Equal(checks[0].Argv, []string{"go", "test", "./tests", "-run", "^TestGO1$"}) ||
		checks[1].Argv != nil {
		t.Fatalf("checks: %+v", checks)
	}
	if !out.Answer.Block || !strings.HasPrefix(out.Answer.Reasons[0], "Rules que no se cumplen:\n- GO-1 (tests/go1_test.go): d") {
		t.Fatalf("answer: %+v", out.Answer)
	}
}

// A batch runner checks every matching rule in one step; a failed group is rerun per rule and the block names
// only the rules whose rerun fails.
func TestAnswerHookRunRulesBatch(t *testing.T) {
	ev := answerHookEval(t, `version: 1
rules:
  run:
    "*_test.go": [go, test, "{dir}", -run, "^Test({names})$"]
on:
  stop:
    - action: { run: { rules: changed } }
`, workflow.Stop)
	rules := []rule.Rule{
		{ID: "A-1", Description: "a1", Scope: []string{"src/**"}, Deterministic: true, Check: "tests/a1_test.go"},
		{ID: "A-2", Description: "a2", Scope: []string{"src/**"}, Deterministic: true, Check: "tests/a2_test.go"},
		{ID: "A-3", Description: "a3", Scope: []string{"src/**"}, Deterministic: true, Check: "tests/a3_test.go"},
	}
	checksOf := func(steps []Step) []rule.Run {
		var out []rule.Run
		for _, s := range steps {
			if s.Check != nil {
				out = append(out, *s.Check)
			}
		}
		return out
	}
	facts := answerHookFacts{branch: "main", changed: []string{"src/a.go"}, rules: rules,
		checkFails: map[string]bool{}}
	out, steps := answerHookWalk(t, ev, facts)
	checks := checksOf(steps)
	if len(checks) != 1 || len(checks[0].Rules) != 3 ||
		!slices.Equal(checks[0].Argv, []string{"go", "test", "./tests", "-run", "^Test(A1|A2|A3)$"}) {
		t.Fatalf("checks: %+v", checks)
	}
	if out.Answer.Block {
		t.Fatalf("a passing group lets the stop through: %+v", out.Answer)
	}
	facts.checkFails = map[string]bool{"A-2": true}
	out, steps = answerHookWalk(t, ev, facts)
	if checks = checksOf(steps); len(checks) != 4 || len(checks[2].Rules) != 1 || checks[2].Rules[0].ID != "A-2" {
		t.Fatalf("a failed group is rerun per rule: %+v", checks)
	}
	if !out.Answer.Block || !strings.HasPrefix(out.Answer.Reasons[0], "Rules que no se cumplen:\n- A-2 (tests/a2_test.go): a2") ||
		strings.Contains(out.Answer.Reasons[0], "A-1") || strings.Contains(out.Answer.Reasons[0], "A-3") {
		t.Fatalf("answer: %+v", out.Answer)
	}
}

// Nobody edits the state of polix, and a command that writes it is denied, whatever the workflow says.
func TestAnswerHookProtectsState(t *testing.T) {
	ev := answerHookEval(t, "version: 1\n", workflow.PreEdit)
	ev = Evaluation{Workflow: ev.Workflow, Present: true, Root: ev.Root, GitDir: ev.GitDir, System: "linux",
		Event: agent.Event{Name: workflow.PreEdit, Agent: agent.Claude, Actor: "builder",
			AbsPath: filepath.Join(ev.GitDir, "polix", "log.jsonl")}}
	if err := ev.Valid(); err != nil {
		t.Fatal(err)
	}
	out, steps := answerHookWalk(t, ev, answerHookFacts{branch: "main"})
	if !out.Answer.Block || out.Answer.Reasons[0] != ReasonStateEdit {
		t.Fatalf("answer: %+v", out.Answer)
	}
	if logged := answerHookAppended(steps); len(logged) != 1 || logged[0].Action != "builtin" {
		t.Fatalf("logged: %+v", logged)
	}
}

// The session context of the guidance turned on is injected on start only, as one context before those of
// the entries of the event.
func TestAnswerHookGuidance(t *testing.T) {
	session := func(yaml string) []string {
		wf, _ := workflow.Parse([]byte(yaml), "dev")
		return []string{wf.SessionContext()}
	}
	defaults := session("version: 1\n")
	cases := []struct {
		name, yaml, event string
		want              []string
	}{
		{"defaults", "version: 1\n", workflow.Start, defaults},
		{"only edit", "version: 1\nguidance: { commit: false, layout: false }\n", workflow.Start,
			session("version: 1\nguidance: { commit: false, layout: false }\n")},
		{"all off", "version: 1\nguidance: { commit: false, edit: false, layout: false }\n", workflow.Start, nil},
		{"before the entries", "version: 1\non:\n  start:\n    - action: { inject: repo }\n",
			workflow.Start, append(slices.Clone(defaults), "repo")},
		{"not on prompt", "version: 1\n", workflow.Prompt, nil},
		{"not on stop", "version: 1\n", workflow.Stop, nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			out, _ := answerHookWalk(t, answerHookEval(t, c.yaml, c.event), answerHookFacts{branch: "main"})
			if out.Answer.Block || !slices.Equal(out.Answer.Contexts, c.want) {
				t.Fatalf("contexts %q, want %q", out.Answer.Contexts, c.want)
			}
		})
	}
	edit := session("version: 1\nguidance: { commit: false, layout: false }\n")[0]
	if !strings.HasPrefix(edit, "This project is managed by polix.\nFollow these rules:\n- Create and change files") ||
		strings.Contains(edit, "git commit") {
		t.Fatalf("only the guidance on edits: %q", edit)
	}
	ev := answerHookEval(t, "version: 1\n", workflow.PreEdit)
	ev = Evaluation{Workflow: ev.Workflow, Present: true, Root: ev.Root, GitDir: ev.GitDir, System: "linux",
		Event: agent.Event{Name: workflow.PreEdit, Agent: agent.Claude, Actor: agent.ActorSession, Path: "src/a.go",
			AbsPath: filepath.Join(ev.Root, "src", "a.go")}}
	if out, _ := answerHookWalk(t, ev, answerHookFacts{branch: "main"}); len(out.Answer.Contexts) != 0 {
		t.Fatalf("pre_edit: %q", out.Answer.Contexts)
	}
	// Without polix.yaml, or with an invalid one, nothing of the guidance is injected.
	start := answerHookEval(t, "version: 1\n", workflow.Start)
	without := Evaluation{Workflow: start.Workflow, Root: start.Root, GitDir: start.GitDir, Event: start.Event}
	broken := Evaluation{Workflow: start.Workflow, Present: true, Invalid: errors.New("polix.yaml: unknown key"),
		Root: start.Root, GitDir: start.GitDir, Event: start.Event}
	for name, e := range map[string]Evaluation{"without": without, "invalid": broken} {
		if err := e.Valid(); err != nil {
			t.Fatal(err)
		}
		if out, _ := answerHookWalk(t, e, answerHookFacts{}); len(out.Answer.Contexts) != 0 {
			t.Fatalf("%s: %q", name, out.Answer.Contexts)
		}
	}
	if out, _ := answerHookWalk(t, broken, answerHookFacts{}); len(out.Answer.Warnings) != 1 {
		t.Fatalf("an invalid workflow keeps its warning: %+v", out.Answer)
	}
}

// A block by protected tells, in English, the path, the glob a folder holds, and who changes it.
func TestAnswerHookProtectedReasons(t *testing.T) {
	yaml := "version: 1\nprotected: [\".env\", \"AGENTS.md\", \"ai/rules/**\"]\n"
	base := answerHookEval(t, yaml, workflow.PreEdit)
	edit := agent.Event{Name: workflow.PreEdit, Agent: agent.Claude, Actor: agent.ActorSession, Path: ".env",
		AbsPath: filepath.Join(base.Root, ".env"), Cwd: base.Root}
	write := agent.Event{Name: workflow.PreCommand, Agent: agent.Claude, Actor: agent.ActorSession,
		Command: "echo x > AGENTS.md", Cwd: base.Root}
	remove := agent.Event{Name: workflow.PreCommand, Agent: agent.Claude, Actor: agent.ActorSession,
		Command: "rm -rf ai", Cwd: base.Root}
	cases := map[string]struct {
		event agent.Event
		want  string
	}{
		"edit":    {edit, ".env is protected in polix.yaml: only a person changes it."},
		"command": {write, "AGENTS.md is protected in polix.yaml: only a person changes it."},
		"folder":  {remove, "ai holds ai/rules/**, protected in polix.yaml: only a person changes it."},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			ev := Evaluation{Workflow: base.Workflow, Present: true, Root: base.Root, GitDir: base.GitDir,
				System: "linux", Event: c.event}
			if err := ev.Valid(); err != nil {
				t.Fatal(err)
			}
			out, _ := answerHookWalk(t, ev, answerHookFacts{branch: "main"})
			if !out.Answer.Block || !slices.Equal(out.Answer.Reasons, []string{c.want}) {
				t.Fatalf("reasons %q, want %q", out.Answer.Reasons, c.want)
			}
		})
	}
}

// Without polix.yaml only the protections apply; an invalid workflow blocks edits but never a stop.
func TestAnswerHookWithoutOrInvalidWorkflow(t *testing.T) {
	ev := answerHookEval(t, "version: 1\non:\n  stop:\n    - action: { block: \"x\" }\n", workflow.Stop)
	without := Evaluation{Root: ev.Root, GitDir: ev.GitDir, Event: ev.Event}
	if err := without.Valid(); err != nil {
		t.Fatal(err)
	}
	if out, steps := answerHookWalk(t, without, answerHookFacts{}); out.Answer.Block || len(steps) != 0 {
		t.Fatalf("without workflow: %+v %+v", out.Answer, steps)
	}
	broken := Evaluation{Present: true, Invalid: errors.New("polix.yaml: unknown key"), Root: ev.Root,
		GitDir: ev.GitDir, Event: ev.Event}
	if err := broken.Valid(); err != nil {
		t.Fatal(err)
	}
	out, _ := answerHookWalk(t, broken, answerHookFacts{})
	if out.Answer.Block || len(out.Answer.Warnings) != 1 {
		t.Fatalf("an invalid workflow never blocks a stop: %+v", out.Answer)
	}
	edit := Evaluation{Present: true, Invalid: broken.Invalid, Root: ev.Root, GitDir: ev.GitDir, Event: agent.Event{
		Name: workflow.PreEdit, Agent: agent.Claude, Actor: agent.ActorSession, Path: "src/a.go",
		AbsPath: filepath.Join(ev.Root, "src", "a.go")}}
	if err := edit.Valid(); err != nil {
		t.Fatal(err)
	}
	out, _ = answerHookWalk(t, edit, answerHookFacts{branch: "main"})
	if !out.Answer.Block || !strings.Contains(out.Answer.Reasons[0], "el workflow no es válido") {
		t.Fatalf("an invalid workflow blocks an edit: %+v", out.Answer)
	}
}

// An inject of a file whose name expands to nothing reads no file and asks for nothing: the event is still
// answered, and the step that asks for nothing is never returned.
func TestAnswerHookEmptyRead(t *testing.T) {
	ev := answerHookEval(t, `version: 1
on:
  pre_command:
    - action: { inject: { file: "{path}" } }
    - action: { block: "no" }
`, workflow.PreCommand)
	out, steps := answerHookWalk(t, ev, answerHookFacts{branch: "main"})
	if !out.Answer.Block || len(out.Answer.Contexts) != 0 {
		t.Fatalf("answer: %+v", out.Answer)
	}
	for _, s := range steps {
		if s.Read != "" || s.None() {
			t.Fatalf("step %+v", s)
		}
	}
}

// file with contains is one search per epoch, not one read per file.
func TestAnswerHookFileContains(t *testing.T) {
	ev := answerHookEval(t, `version: 1
on:
  pre_command:
    - unless: { file: "ai/plans/*.md", contains: "Ticket: {key}" }
      action: { block: "No hay plan" }
vars:
  key: { branch: "^([A-Z]+-[0-9]+)" }
`, workflow.PreCommand)
	for found, block := range map[bool]bool{true: false, false: true} {
		out, steps := answerHookWalk(t, ev, answerHookFacts{branch: "PLAT-7-x", found: found})
		var searches []Step
		for _, s := range steps {
			if s.Contains != nil {
				searches = append(searches, s)
			}
		}
		if out.Answer.Block != block || len(searches) != 1 || searches[0].Text != "Ticket: PLAT-7" {
			t.Errorf("found %v: %+v %+v", found, out.Answer, searches)
		}
	}
}

// answerHookDiff is a diff of Go files, tests and docs for the measures: 120 lines in 3 Go files, 300 in all.
var answerHookDiff = diff.Diff{Changes: []diff.Change{
	{Path: "internal/hook/a.go", Added: 50, Deleted: 13},
	{Path: "internal/hook/a_test.go", Added: 40},
	{Path: "main.go", Added: 17},
	{Path: "docs/polix.md", Added: 180},
}}

// answerHookBlocks walks a pre_commit with the workflow of yaml on the diff and reports whether it blocked.
func answerHookBlocks(t *testing.T, yaml string, f answerHookFacts) (AnswerHookOutput, []Step) {
	t.Helper()
	ev := answerHookEval(t, yaml, workflow.PreCommit)
	if f.branch == "" {
		f.branch = "main"
	}
	if f.diff.Changes == nil {
		f.diff = answerHookDiff
	}
	if f.changed == nil {
		for _, c := range f.diff.Changes {
			f.changed = append(f.changed, c.Path)
		}
	}
	return answerHookWalk(t, ev, f)
}

// Each measure holds at its number and not below, over the files of the changed next to it or the whole diff.
func TestAnswerHookMeasures(t *testing.T) {
	cases := []struct {
		cond  string
		block bool
	}{
		{`{ changed: "*.go", changed_lines: 120 }`, true},
		{`{ changed: "*.go", changed_lines: 121 }`, false},
		{`{ changed_lines: 300 }`, true},
		{`{ changed_lines: 301 }`, false},
		{`{ changed: "*.go", changed_files: 3 }`, true},
		{`{ changed: "*.go", changed_files: 4 }`, false},
		{`{ changed_files: 4 }`, true},
		{`{ changed_files: 0 }`, true},
		// log2(64) + log2(41) + log2(18) = 6 + 5.36 + 4.17 = 15.5, rounded 16
		{`{ changed: "*.go", risk_score: 16 }`, true},
		{`{ changed: "*.go", risk_score: 17 }`, false},
	}
	for _, c := range cases {
		out, _ := answerHookBlocks(t, "version: 1\non:\n  pre_commit:\n    - when: "+c.cond+"\n      action: { block: \"no\" }\n",
			answerHookFacts{})
		if out.Answer.Block != c.block {
			t.Errorf("%s: block %v, want %v", c.cond, out.Answer.Block, c.block)
		}
	}
}

// risk_score takes the first matching weight of risk.weights.
func TestAnswerHookRiskWeights(t *testing.T) {
	yaml := `version: 1
risk:
  weights:
    "*_test.go": 0
    "internal/hook/**": 3
on:
  pre_commit:
    - when: { changed: "*.go", risk_score: %d }
      action: { block: "riesgo {risk_score}" }
`
	// 3 * 6 + 0 + log2(18) = 22.17, rounded 22
	for n, block := range map[int]bool{22: true, 23: false} {
		out, _ := answerHookBlocks(t, strings.Replace(yaml, "%d", strconv.Itoa(n), 1), answerHookFacts{})
		if out.Answer.Block != block {
			t.Errorf("risk_score %d: %+v", n, out.Answer)
		}
		if block && out.Answer.Reasons[0] != "riesgo 22" {
			t.Errorf("reason %q", out.Answer.Reasons[0])
		}
	}
}

// any holds when one of its conditions holds; a measure inside it takes the changed of the enclosing map unless
// its own map has one.
func TestAnswerHookAny(t *testing.T) {
	cases := []struct {
		cond  string
		block bool
	}{
		{`{ changed: "*.go", any: { changed_lines: 150, changed_files: 3 } }`, true},
		{`{ changed: "*.go", any: { changed_lines: 150, changed_files: 4 } }`, false},
		{`{ changed: "*.go", any: { changed: "**", changed_lines: 300 } }`, true},
		{`{ changed: "*.go", not: { any: { changed_lines: 150 } } }`, true},
		{`{ any: { not: { branch: "^main$" }, changed_files: 9 } }`, false},
	}
	for _, c := range cases {
		out, _ := answerHookBlocks(t, "version: 1\non:\n  pre_commit:\n    - when: "+c.cond+"\n      action: { block: \"no\" }\n",
			answerHookFacts{})
		if out.Answer.Block != c.block {
			t.Errorf("%s: block %v, want %v", c.cond, out.Answer.Block, c.block)
		}
	}
}

// A diff that cannot be measured makes the measures hold, with one warning for the call, and shows ? in the text.
func TestAnswerHookMeasuresFailClosed(t *testing.T) {
	yaml := "version: 1\non:\n  pre_commit:\n    - when: { changed_lines: 1000, changed_files: 1000 }\n" +
		"      action: { block: \"{changed_lines} {changed_files} {risk_score}\" }\n"
	out, _ := answerHookBlocks(t, yaml, answerHookFacts{diffErr: errors.New("git roto")})
	if !out.Answer.Block || out.Answer.Reasons[0] != "? ? ?" {
		t.Fatalf("answer: %+v", out.Answer)
	}
	want := "polix no puede medir el diff (git roto): changed_lines, changed_files y risk_score se cumplen"
	if len(out.Answer.Warnings) != 1 || out.Answer.Warnings[0] != want {
		t.Fatalf("warnings: %q", out.Answer.Warnings)
	}
}

// A placeholder of a measure measures the changed of the when of the entry; the whole diff without one.
func TestAnswerHookMeasurePlaceholders(t *testing.T) {
	yaml := "version: 1\non:\n  pre_commit:\n    - action: { inject: \"{changed_lines}/{changed_files}\" }\n" +
		"    - when: { changed: \"*.go\" }\n      action: { block: \"{changed_lines}/{changed_files}\" }\n"
	out, _ := answerHookBlocks(t, yaml, answerHookFacts{})
	if !out.Answer.Block || out.Answer.Reasons[0] != "120/3" || !slices.Equal(out.Answer.Contexts, []string{"300/4"}) {
		t.Fatalf("answer: %+v", out.Answer)
	}
}

// An event whose entries use no measure never asks for the diff.
func TestAnswerHookNoDiffWithoutMeasures(t *testing.T) {
	yaml := "version: 1\non:\n  pre_commit:\n    - when: { changed: \"*.go\" }\n      action: { block: \"{branch}\" }\n"
	_, steps := answerHookBlocks(t, yaml, answerHookFacts{changed: []string{"a.go"}})
	for _, s := range steps {
		if s.Diff {
			t.Fatalf("steps %+v", steps)
		}
	}
}

// The conditions on the event come before those on git: a command that does not match spares the hash.
func TestAnswerHookCheapConditionsFirst(t *testing.T) {
	ev := answerHookEval(t, `version: 1
on:
  pre_command:
    - when: { finished: polix:reviewer, command: "git push*", changed: "**", branch: "^main$" }
      action: { block: "no" }
`, workflow.PreCommand)
	out, steps := answerHookWalk(t, ev, answerHookFacts{branch: "main"})
	if out.Answer.Block || len(steps) != 0 {
		t.Fatalf("answer %+v, steps %+v", out.Answer, steps)
	}
}

// answerHookWith returns ev with its event changed by change, as the first event of a new call.
func answerHookWith(t *testing.T, ev Evaluation, change func(*agent.Event)) Evaluation {
	t.Helper()
	e := ev.Event
	change(&e)
	out := Evaluation{Workflow: ev.Workflow, Present: ev.Present, Root: ev.Root, GitDir: ev.GitDir, System: ev.System,
		Event: e}
	if err := out.Valid(); err != nil {
		t.Fatal(err)
	}
	return out
}

// answerHookEdit returns ev as the edit name (pre_edit or post_edit) of path by actor.
func answerHookEdit(t *testing.T, ev Evaluation, name, path, actor string) Evaluation {
	t.Helper()
	return answerHookWith(t, ev, func(e *agent.Event) {
		e.Name, e.Actor, e.Path, e.AbsPath = name, actor, path, filepath.Join(ev.Root, filepath.FromSlash(path))
	})
}

// answerHookLogged returns the log st with the entries the steps appended, as the next event reads it.
func answerHookLogged(t *testing.T, st state.State, steps []Step) state.State {
	t.Helper()
	for _, e := range answerHookAppended(steps) {
		var err error
		if st, err = st.Append(e); err != nil {
			t.Fatal(err)
		}
	}
	return st
}

// answerHookDecided returns the decisions the steps logged with outcome, in order.
func answerHookDecided(steps []Step, outcome string) []state.Entry {
	var out []state.Entry
	for _, e := range answerHookAppended(steps) {
		if e.Kind == state.KindDecision && e.Outcome == outcome {
			out = append(out, e)
		}
	}
	return out
}

// answerHookHashed reports whether the steps asked for the content hash.
func answerHookHashed(steps []Step) bool {
	return slices.ContainsFunc(steps, func(s Step) bool { return s.Hash })
}

// AC14 and AC13: a run without always that passed is not run again on the same branch and content, and the skip is
// logged; an edit or another branch makes it run again, and a failing run runs again on the next event.
func TestAnswerHookRunRepeat(t *testing.T) {
	ev := answerHookEval(t, `version: 1
on:
  stop:
    - action: { run: just lint }
    - action: { run: { command: [just, test], message: "Arregla los tests" } }
`, workflow.Stop)
	f := answerHookFacts{branch: "main", hashes: []string{"h", "h", "h"}}
	out, steps := answerHookWalk(t, ev, f)
	passed := answerHookObserved(steps)
	if runs := answerHookRuns(steps); out.Answer.Block || len(runs) != 2 || len(passed) != 2 {
		t.Fatalf("first turn: %q %+v", runs, passed)
	}
	for i, want := range []struct {
		key  string
		line int
	}{{"just lint", 4}, {"just test", 5}} {
		if p := passed[i]; p.Outcome != state.OutcomePassed || p.Key != want.key || p.Line != want.line ||
			p.Hash != "h" || p.Branch != "main" || p.Session != "s1" {
			t.Fatalf("passed %d: %+v", i, p)
		}
	}
	// A second stop on the same content runs nothing and logs a skip per entry.
	f.state = answerHookLogged(t, f.state, steps)
	out, steps = answerHookWalk(t, ev, f)
	skips := answerHookDecided(steps, "skip")
	if runs := answerHookRuns(steps); out.Answer.Block || len(runs) != 0 || len(answerHookObserved(steps)) != 0 ||
		len(skips) != 2 || skips[0].Action != workflow.VerbRun || skips[0].Line != 4 || skips[1].Line != 5 ||
		!strings.Contains(skips[0].Reason, "just lint") {
		t.Fatalf("second turn: %q %+v", runs, answerHookAppended(steps))
	}
	// An edit, or another branch, makes them run again.
	for name, g := range map[string]answerHookFacts{
		"edit":   {branch: "main", hashes: []string{"h2", "h2", "h2"}, state: f.state},
		"branch": {branch: "dev", hashes: []string{"h", "h", "h"}, state: f.state},
	} {
		if _, steps := answerHookWalk(t, ev, g); len(answerHookRuns(steps)) != 2 {
			t.Errorf("%s: %q", name, answerHookRuns(steps))
		}
	}
	// A failing run writes no passed and runs again on the next event.
	fail := answerHookFacts{branch: "main", hashes: []string{"h3", "h3"}, state: f.state,
		runErr: errors.New("exit status 1")}
	for i := range 2 {
		out, steps = answerHookWalk(t, ev, fail)
		if runs := answerHookRuns(steps); !out.Answer.Block || len(runs) != 1 {
			t.Fatalf("failing run %d: %q %+v", i, runs, out.Answer)
		}
		for _, o := range answerHookObserved(steps) {
			if o.Outcome == state.OutcomePassed {
				t.Fatalf("a failing run passed: %+v", o)
			}
		}
		fail.state = answerHookLogged(t, fail.state, steps)
	}
}

// AC14: the key of a run is what it runs, whoever's entry it is: a {path} repeats it per path, a run of the same
// command on another event is spared, and a run of the rules is keyed by its expanded target.
func TestAnswerHookRunRepeatKeys(t *testing.T) {
	base := answerHookEval(t, `version: 1
on:
  post_edit:
    - action: { run: [gofmt, -l, "{path}"] }
    - action: { run: just lint }
  stop:
    - action: { run: just lint }
`, workflow.PostEdit)
	f := answerHookFacts{branch: "main", hashes: []string{"h", "h", "h"}}
	walk := func(ev Evaluation) [][]string {
		t.Helper()
		_, steps := answerHookWalk(t, ev, f)
		f.state = answerHookLogged(t, f.state, steps)
		return answerHookRuns(steps)
	}
	if runs := walk(answerHookEdit(t, base, workflow.PostEdit, "a.go", agent.ActorSession)); len(runs) != 2 ||
		!slices.Equal(runs[0], []string{"gofmt", "-l", "a.go"}) {
		t.Fatalf("a.go: %q", runs)
	}
	if runs := walk(answerHookEdit(t, base, workflow.PostEdit, "b.go", agent.ActorSession)); len(runs) != 1 ||
		!slices.Equal(runs[0], []string{"gofmt", "-l", "b.go"}) {
		t.Fatalf("b.go: %q", runs)
	}
	if runs := walk(answerHookEdit(t, base, workflow.PostEdit, "a.go", agent.ActorSession)); len(runs) != 0 {
		t.Fatalf("a.go again: %q", runs)
	}
	if runs := walk(answerHookWith(t, base, func(e *agent.Event) { e.Name = workflow.Stop })); len(runs) != 0 {
		t.Fatalf("the run of another entry: %q", runs)
	}

	rulesEv := answerHookEval(t, "version: 1\non:\n  post_edit:\n    - action: { run: { rules: \"{path}\" } }\n",
		workflow.PostEdit)
	g := answerHookFacts{branch: "main", hashes: []string{"h", "h"}, rules: []rule.Rule{
		{ID: "R-1", Description: "d", Scope: []string{"src/**"}, Deterministic: true, Check: "checks/r.sh"}}}
	checks := func(path string) int {
		t.Helper()
		_, steps := answerHookWalk(t, answerHookEdit(t, rulesEv, workflow.PostEdit, path, agent.ActorSession), g)
		g.state = answerHookLogged(t, g.state, steps)
		n := 0
		for _, s := range steps {
			if s.Check != nil {
				n++
			}
		}
		return n
	}
	if n := checks("src/a.go"); n != 1 {
		t.Fatalf("src/a.go: %d checks", n)
	}
	if got := g.state.Observed(state.Query{Outcome: state.OutcomePassed}); len(got) != 1 || got[0].Key != "rules src/a.go" {
		t.Fatalf("passed: %+v", got)
	}
	if n := checks("src/b.go"); n != 1 {
		t.Fatalf("src/b.go: %d checks", n)
	}
	if n := checks("src/a.go"); n != 0 {
		t.Fatalf("src/a.go again: %d checks", n)
	}
}

// AC15: a run with always runs on every event and writes passed every time it passes on unchanged content; a run
// during which the content changed, or with an unknown hash, writes none, and the latter runs every time.
func TestAnswerHookRunAlways(t *testing.T) {
	always := answerHookEval(t, "version: 1\non:\n  stop:\n"+
		"    - action: { run: { command: [just, test], always: true } }\n", workflow.Stop)
	f := answerHookFacts{branch: "main", hashes: []string{"h", "h"}}
	for i := range 2 {
		_, steps := answerHookWalk(t, always, f)
		f.state = answerHookLogged(t, f.state, steps)
		if runs := answerHookRuns(steps); len(runs) != 1 {
			t.Fatalf("run %d: %q", i, runs)
		}
	}
	if got := f.state.Observed(state.Query{Outcome: state.OutcomePassed, Key: "just test", Hash: "h",
		ByHash: true}); len(got) != 2 {
		t.Fatalf("passed: %+v", got)
	}
	// The run rewrote files: the content it checked is gone.
	if _, steps := answerHookWalk(t, always, answerHookFacts{branch: "main", hashes: []string{"h", "h2"}}); len(
		answerHookAppended(steps)) != 0 {
		t.Fatalf("changed during the run: %+v", answerHookAppended(steps))
	}
	// Without a hash nothing is written, and a run without always runs every time.
	once := answerHookEval(t, "version: 1\non:\n  stop:\n    - action: { run: just test }\n", workflow.Stop)
	g := answerHookFacts{branch: "main", hashErr: errors.New("permission denied")}
	for i := range 2 {
		_, steps := answerHookWalk(t, once, g)
		if len(answerHookRuns(steps)) != 1 || len(answerHookObserved(steps)) != 0 {
			t.Fatalf("unknown hash %d: %+v", i, steps)
		}
		g.state = answerHookLogged(t, g.state, steps)
	}
}

// AC8 and AC19: a block with once blocks the first attempt of an actor in a session on a content and lets the next
// through, logging the skip; an edit outside the scope of the entry keeps it, one inside it, a comparison that
// fails or an unknown hash repeat it; another actor and a new session are reminded on their own.
func TestAnswerHookBlockOnce(t *testing.T) {
	ev := answerHookEval(t, `version: 1
on:
  pre_commit:
    - when: { changed: "*.go" }
      action: { block: { message: "Actualiza la documentación", once: true } }
`, workflow.PreCommit)
	f := answerHookFacts{branch: "main", hashes: []string{"h"}, changed: []string{"internal/a.go"}}
	out, steps := answerHookWalk(t, ev, f)
	blocked := answerHookObserved(steps)
	if !out.Answer.Block || len(blocked) != 1 || blocked[0].Outcome != state.OutcomeBlocked || blocked[0].Hash != "h" ||
		blocked[0].Line != 4 || blocked[0].Session != "s1" || blocked[0].Actor != agent.ActorSession ||
		blocked[0].Branch != "main" {
		t.Fatalf("first commit: %+v %+v", out.Answer, blocked)
	}
	f.state = answerHookLogged(t, f.state, steps)
	out, steps = answerHookWalk(t, ev, f)
	if skips := answerHookDecided(steps, "skip"); out.Answer.Block || len(skips) != 1 ||
		skips[0].Action != workflow.VerbBlock || skips[0].Line != 4 || len(answerHookObserved(steps)) != 0 {
		t.Fatalf("second commit: %+v %+v", out.Answer, answerHookAppended(steps))
	}
	cases := []struct {
		name  string
		ev    Evaluation
		f     answerHookFacts
		block bool
	}{
		{"edit outside the scope", ev, answerHookFacts{hashes: []string{"h2"}, trees: []string{"README.md", "docs/a.md"}},
			false},
		{"edit inside the scope", ev, answerHookFacts{hashes: []string{"h2"}, trees: []string{"README.md", "cmd/b.go"}},
			true},
		{"comparison fails", ev, answerHookFacts{hashes: []string{"h2"}, treeErr: errors.New("fatal: bad object h")}, true},
		{"unknown hash", ev, answerHookFacts{hashErr: errors.New("permission denied")}, true},
		{"another branch", ev, answerHookFacts{branch: "dev", hashes: []string{"h"}}, true},
		{"another actor", answerHookWith(t, ev, func(e *agent.Event) { e.Actor = "acme:builder" }),
			answerHookFacts{hashes: []string{"h"}}, true},
		{"a new session", answerHookWith(t, ev, func(e *agent.Event) { e.Session = "s2" }),
			answerHookFacts{hashes: []string{"h"}}, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if c.f.branch == "" {
				c.f.branch = "main"
			}
			c.f.state, c.f.changed = f.state, f.changed
			out, steps := answerHookWalk(t, c.ev, c.f)
			if out.Answer.Block != c.block {
				t.Fatalf("block %v, want %v: %+v", out.Answer.Block, c.block, answerHookAppended(steps))
			}
			if c.f.treeErr != nil {
				if warns := answerHookDecided(steps, "warn"); len(warns) != 1 ||
					!strings.Contains(warns[0].Reason, "fatal: bad object h") {
					t.Fatalf("the failed comparison is logged: %+v", warns)
				}
			}
		})
	}
	// A plain block blocks every commit and observes nothing.
	plain := answerHookEval(t, "version: 1\non:\n  pre_commit:\n    - action: { block: \"No\" }\n", workflow.PreCommit)
	g := answerHookFacts{branch: "main", hashes: []string{"h"}}
	for i := range 2 {
		out, steps := answerHookWalk(t, plain, g)
		if !out.Answer.Block || len(answerHookObserved(steps)) != 0 || answerHookHashed(steps) {
			t.Fatalf("plain block %d: %+v %+v", i, out.Answer, steps)
		}
		g.state = answerHookLogged(t, g.state, steps)
	}
}

// AC20: the loop guard counts the observed blocked lines of the entry on the whole hash: an edit outside any scope
// starts a new count, and the blocks of an unknown hash are counted together, so a broken git never traps the agent.
func TestAnswerHookLoopGuardCountsBlocked(t *testing.T) {
	ev := answerHookEval(t, `version: 1
max_stop_blocks: 2
on:
  stop:
    - when: { changed: "*.go" }
      action: { block: "Revisa" }
`, workflow.Stop)
	for name, f := range map[string]answerHookFacts{
		"known hash":   {hashes: []string{"h"}},
		"unknown hash": {hashErr: errors.New("permission denied")},
	} {
		t.Run(name, func(t *testing.T) {
			f.branch, f.changed = "main", []string{"a.go"}
			for i, block := range []bool{true, true, false} {
				out, steps := answerHookWalk(t, ev, f)
				if out.Answer.Block != block {
					t.Fatalf("stop %d: %+v", i, out.Answer)
				}
				f.state = answerHookLogged(t, f.state, steps)
			}
			want := "h"
			if f.hashErr != nil {
				want = state.UnknownHash
			}
			if got := f.state.Observed(state.Query{Outcome: state.OutcomeBlocked, Line: 5, Hash: want,
				ByHash: true}); len(got) != 2 {
				t.Fatalf("blocked: %+v", got)
			}
		})
	}
	// Another content, even outside the scope of the entry, has its own count.
	f := answerHookFacts{branch: "main", hashes: []string{"h"}, changed: []string{"a.go"}}
	for range 2 {
		_, steps := answerHookWalk(t, ev, f)
		f.state = answerHookLogged(t, f.state, steps)
	}
	f.hashes = []string{"h2"}
	if out, _ := answerHookWalk(t, ev, f); !out.Answer.Block {
		t.Fatal("the blocks of another content count")
	}
}

// AC18, AC19 and AC13: a once-only inject reaches each actor once per session, writes injected without computing a
// hash and logs the skip; it is due again after a start of the session and, for a subagent, after that
// subagent type finished, which is written without a hash. A new session is reminded on its own.
func TestAnswerHookInjectOnce(t *testing.T) {
	base := answerHookEval(t, `version: 1
guidance: { commit: false, edit: false, layout: false }
on:
  pre_edit:
    - action: { inject: "Lee el plan antes de editar" }
`, workflow.PreEdit)
	const text = "Lee el plan antes de editar"
	var st state.State
	walk := func(ev Evaluation) (AnswerHookOutput, []Step) {
		t.Helper()
		out, steps := answerHookWalk(t, ev, answerHookFacts{branch: "main", hashes: []string{"h"}, state: st})
		if answerHookHashed(steps) {
			t.Fatalf("an inject computes no hash: %+v", steps)
		}
		st = answerHookLogged(t, st, steps)
		return out, steps
	}
	injects := func(ev Evaluation) bool {
		t.Helper()
		out, _ := walk(ev)
		return slices.Equal(out.Answer.Contexts, []string{text})
	}
	session := answerHookEdit(t, base, workflow.PreEdit, "src/a.go", agent.ActorSession)
	out, steps := walk(session)
	injected := answerHookObserved(steps)
	if !slices.Equal(out.Answer.Contexts, []string{text}) || len(injected) != 1 ||
		injected[0].Outcome != state.OutcomeInjected || injected[0].Key != text || injected[0].Line != 5 ||
		injected[0].Hash != "" || injected[0].Session != "s1" || injected[0].Actor != agent.ActorSession {
		t.Fatalf("first edit: %+v %+v", out.Answer, injected)
	}
	out, steps = walk(session)
	if skips := answerHookDecided(steps, "skip"); len(out.Answer.Contexts) != 0 || len(skips) != 1 ||
		skips[0].Action != workflow.VerbInject || skips[0].Line != 5 {
		t.Fatalf("second edit: %+v %+v", out.Answer, answerHookAppended(steps))
	}
	// A subagent is reminded on its own, once, and again after a subagent of its type finished.
	builder := answerHookEdit(t, base, workflow.PreEdit, "src/a.go", "acme:builder")
	if !injects(builder) || injects(builder) {
		t.Fatal("a subagent gets the inject once")
	}
	_, steps = walk(answerHookWith(t, base, func(e *agent.Event) {
		e.Name, e.Actor = workflow.SubagentStop, "acme:builder"
	}))
	if got := answerHookObserved(steps); len(got) != 1 || got[0].Outcome != state.OutcomeFinished ||
		got[0].Key != "acme:builder" || got[0].Hash != "" {
		t.Fatalf("finished without hash: %+v", got)
	}
	if !injects(builder) {
		t.Fatal("the next run of the subagent gets the inject again")
	}
	if injects(session) {
		t.Fatal("the end of a subagent does not remind the main session")
	}
	// A start (a new or compacted context) makes it due again, for the main session and its subagents.
	_, steps = walk(answerHookWith(t, base, func(e *agent.Event) { e.Name = workflow.Start }))
	if got := answerHookObserved(steps); len(got) != 1 || got[0].Outcome != state.OutcomeStarted || got[0].Hash != "" {
		t.Fatalf("started: %+v", got)
	}
	if !injects(session) || !injects(builder) || injects(session) {
		t.Fatal("due again once after start")
	}
	// A new session is reminded on its own.
	if !injects(answerHookWith(t, session, func(e *agent.Event) { e.Session = "s2" })) {
		t.Fatal("a new session")
	}
}

// AC18 and AC23: a once-only inject is keyed by the text it expands to: a {path} gives once per path, and a measure
// that changed injects it again. A file is keyed by its path, and with always every event injects.
func TestAnswerHookInjectKeys(t *testing.T) {
	cases := []struct {
		name, inject string
		edits        []string
		diffs        []int // the added lines of the diff at each edit
		want         []bool
	}{
		{"path", `"Revisa {path}"`, []string{"a.go", "b.go", "a.go"}, nil, []bool{true, true, false}},
		{"measure", `"Llevas {changed_lines} líneas"`, []string{"a.go", "a.go", "a.go"}, []int{10, 10, 12},
			[]bool{true, false, true}},
		{"always", `{ text: "Cuidado", always: true }`, []string{"a.go", "a.go"}, nil, []bool{true, true}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			base := answerHookEval(t, "version: 1\non:\n  pre_edit:\n    - action: { inject: "+c.inject+" }\n",
				workflow.PreEdit)
			var st state.State
			for i, path := range c.edits {
				f := answerHookFacts{branch: "main", state: st}
				if c.diffs != nil {
					f.diff = diff.Diff{Changes: []diff.Change{{Path: "a.go", Added: c.diffs[i]}}}
				}
				out, steps := answerHookWalk(t, answerHookEdit(t, base, workflow.PreEdit, path, agent.ActorSession), f)
				if got := len(out.Answer.Contexts) == 1; got != c.want[i] {
					t.Fatalf("edit %d of %s: %+v", i, path, out.Answer)
				}
				if c.name == "always" && len(answerHookAppended(steps)) != 0 {
					t.Fatalf("always writes nothing: %+v", answerHookAppended(steps))
				}
				st = answerHookLogged(t, st, steps)
			}
		})
	}
}

// AC18: inject: { rules } injects each rule once per actor and session; with nothing left to inject the skip is
// logged.
func TestAnswerHookInjectRulesOnce(t *testing.T) {
	base := answerHookEval(t, "version: 1\non:\n  pre_edit:\n    - action: { inject: { rules: \"{path}\" } }\n",
		workflow.PreEdit)
	f := answerHookFacts{branch: "main", rules: []rule.Rule{
		{ID: "GO-1", Description: "uno", Scope: []string{"src/**"}},
		{ID: "GO-2", Description: "dos", Scope: []string{"src/b.go"}},
	}}
	walk := func(path, actor string) (AnswerHookOutput, []Step) {
		t.Helper()
		out, steps := answerHookWalk(t, answerHookEdit(t, base, workflow.PreEdit, path, actor), f)
		f.state = answerHookLogged(t, f.state, steps)
		return out, steps
	}
	out, steps := walk("src/a.go", agent.ActorSession)
	if got := answerHookObserved(steps); len(out.Answer.Contexts) != 1 ||
		!strings.Contains(out.Answer.Contexts[0], "- GO-1: uno") || len(got) != 1 || got[0].Key != "GO-1" {
		t.Fatalf("src/a.go: %+v %+v", out.Answer, got)
	}
	// Only the rule not injected yet.
	out, _ = walk("src/b.go", agent.ActorSession)
	if len(out.Answer.Contexts) != 1 || strings.Contains(out.Answer.Contexts[0], "GO-1") ||
		!strings.Contains(out.Answer.Contexts[0], "- GO-2: dos") {
		t.Fatalf("src/b.go: %+v", out.Answer)
	}
	out, steps = walk("src/b.go", agent.ActorSession)
	if len(out.Answer.Contexts) != 0 || len(answerHookDecided(steps, "skip")) != 1 {
		t.Fatalf("src/b.go again: %+v %+v", out.Answer, answerHookAppended(steps))
	}
	if out, _ := walk("src/b.go", "acme:builder"); len(out.Answer.Contexts) != 1 ||
		!strings.Contains(out.Answer.Contexts[0], "GO-1") || !strings.Contains(out.Answer.Contexts[0], "GO-2") {
		t.Fatalf("another actor: %+v", out.Answer)
	}
}

// AC19: polix writes started and an unobserved finished only when the workflow injects once, and never computes a
// hash for them.
func TestAnswerHookObservesOnlyWhatTheWorkflowAsks(t *testing.T) {
	for name, entries := range map[string]string{
		"a block":          "  pre_commit:\n    - action: { block: x }\n",
		"an always inject": "  pre_edit:\n    - action: { inject: { text: x, always: true } }\n",
		"start only":       "  start:\n    - action: { inject: x }\n",
	} {
		t.Run(name, func(t *testing.T) {
			yaml := "version: 1\non:\n" + entries
			start := answerHookEval(t, yaml, workflow.Start)
			stop := answerHookAs(t, answerHookEval(t, yaml, workflow.SubagentStop), "acme:builder")
			for _, ev := range []Evaluation{start, stop} {
				_, steps := answerHookWalk(t, ev, answerHookFacts{branch: "main", hashes: []string{"h"}})
				if len(answerHookAppended(steps)) != 0 || answerHookHashed(steps) {
					t.Fatalf("%s: %+v", ev.Event.Name, steps)
				}
			}
		})
	}
}

// A subagent type both observed by finished and reset by a once-only inject gets one finished line, with the hash.
func TestAnswerHookFinishedObservedAndReset(t *testing.T) {
	ev := answerHookAs(t, answerHookEval(t, `version: 1
on:
  pre_edit:
    - action: { inject: "Lee el plan" }
  pre_commit:
    - unless: { finished: polix:reviewer }
      action: { block: "Revisa" }
`, workflow.SubagentStop), "polix:reviewer")
	_, steps := answerHookWalk(t, ev, answerHookFacts{branch: "main", hashes: []string{"h"}})
	if got := answerHookObserved(steps); len(got) != 1 || got[0].Outcome != state.OutcomeFinished || got[0].Hash != "h" {
		t.Fatalf("finished: %+v", got)
	}
	// Without a hash, the line still makes the injects due again, but the condition never reads it.
	_, steps = answerHookWalk(t, ev, answerHookFacts{branch: "main", hashErr: errors.New("permission denied")})
	if got := answerHookObserved(steps); len(got) != 1 || got[0].Hash != "" || len(answerHookDecided(steps, "warn")) != 1 {
		t.Fatalf("finished without hash: %+v", answerHookAppended(steps))
	}
}

// A log that cannot be read makes every verb act every time, with a warning: a run is never spared, an inject is
// always due and a block with once always blocks.
func TestAnswerHookRepeatWithoutLog(t *testing.T) {
	cases := []struct {
		name, yaml, event string
		acted             func(AnswerHookOutput, []Step) bool
	}{
		{"run", "version: 1\non:\n  stop:\n    - action: { run: just test }\n", workflow.Stop,
			func(_ AnswerHookOutput, steps []Step) bool { return len(answerHookRuns(steps)) == 1 }},
		{"inject", "version: 1\non:\n  pre_command:\n    - action: { inject: x }\n", workflow.PreCommand,
			func(out AnswerHookOutput, _ []Step) bool { return len(out.Answer.Contexts) == 1 }},
		{"block once", "version: 1\non:\n  pre_commit:\n    - action: { block: { message: x, once: true } }\n",
			workflow.PreCommit, func(out AnswerHookOutput, _ []Step) bool { return out.Answer.Block }},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			ev := answerHookEval(t, c.yaml, c.event)
			for i := range 2 {
				out, steps := answerHookWalk(t, ev, answerHookFacts{branch: "main", hashes: []string{"h", "h"},
					stateErr: errors.New("permission denied")})
				if !c.acted(out, steps) || !slices.ContainsFunc(out.Answer.Warnings, func(w string) bool {
					return strings.Contains(w, "no puede usar su log (permission denied)")
				}) {
					t.Fatalf("event %d: %+v", i, out.Answer)
				}
			}
		})
	}
}

// A subagent_stop the loop guard let through writes finished without hash when the workflow injects once: the next
// run of the subagent, with a new context, gets the injects again, and finished still never holds for it.
func TestAnswerHookLoopGuardMakesInjectsDue(t *testing.T) {
	yaml := `version: 1
max_stop_blocks: 1
on:
  pre_edit:
    - action: { inject: "Lee el plan" }
  pre_commit:
    - unless: { finished: polix:reviewer }
      action: { block: "Revisa" }
  subagent_stop:
    - action: { block: "Termina" }
`
	st := state.State{Entries: []state.Entry{
		{Kind: state.KindObserved, Outcome: state.OutcomeInjected, Session: "s1", Actor: "polix:reviewer", Line: 5,
			Key: "Lee el plan"},
		{Kind: state.KindObserved, Outcome: state.OutcomeBlocked, Session: "s1", Event: workflow.SubagentStop,
			Actor: "polix:reviewer", Line: 10, Hash: "h", Branch: "main"},
	}}
	f := answerHookFacts{branch: "main", hashes: []string{"h"}, state: st}
	reviewer := answerHookAs(t, answerHookEval(t, yaml, workflow.SubagentStop), "polix:reviewer")
	out, steps := answerHookWalk(t, reviewer, f)
	got := answerHookObserved(steps)
	if out.Answer.Block || len(got) != 1 || got[0].Outcome != state.OutcomeFinished || got[0].Key != "polix:reviewer" ||
		got[0].Hash != "" {
		t.Fatalf("guarded stop: %+v %+v", out.Answer, got)
	}
	f.state = answerHookLogged(t, f.state, steps)
	edit := answerHookEdit(t, answerHookEval(t, yaml, workflow.PreEdit), workflow.PreEdit, "src/a.go", "polix:reviewer")
	if out, _ := answerHookWalk(t, edit, f); !slices.Equal(out.Answer.Contexts, []string{"Lee el plan"}) {
		t.Fatalf("the next run of the subagent gets the inject again: %+v", out.Answer)
	}
	if out, _ := answerHookWalk(t, answerHookEval(t, yaml, workflow.PreCommit), f); !out.Answer.Block {
		t.Fatal("finished holds after a stop the loop guard let through")
	}
}

// A once-only inject is observed only when it reaches the AI: on prompt a later block rejects the prompt, so it is
// not written and the next prompt injects it; on the other events it reaches the AI with the block, and it is.
func TestAnswerHookInjectBeforeBlock(t *testing.T) {
	yaml := `version: 1
on:
  prompt:
    - action: { inject: "Lee el plan" }
    - action: { inject: { rules: src/a.go } }
    - when: { branch: "^main$" }
      action: { block: "No en main" }
  pre_command:
    - action: { inject: "Cuidado" }
    - action: { block: "No" }
`
	rules := []rule.Rule{{ID: "GO-1", Description: "uno", Scope: []string{"src/**"}}}
	prompt := answerHookEval(t, yaml, workflow.Prompt)
	out, steps := answerHookWalk(t, prompt, answerHookFacts{branch: "main", rules: rules})
	if !out.Answer.Block || len(answerHookObserved(steps)) != 0 {
		t.Fatalf("a refused prompt: %+v %+v", out.Answer, answerHookObserved(steps))
	}
	st := answerHookLogged(t, state.State{}, steps)
	out, steps = answerHookWalk(t, prompt, answerHookFacts{branch: "dev", rules: rules, state: st})
	got := answerHookObserved(steps)
	if out.Answer.Block || len(out.Answer.Contexts) != 2 || len(got) != 2 || got[0].Key != "Lee el plan" ||
		got[0].Line != 4 || got[1].Key != "GO-1" || got[1].Line != 5 {
		t.Fatalf("the next prompt: %+v %+v", out.Answer, got)
	}
	out, steps = answerHookWalk(t, answerHookEval(t, yaml, workflow.PreCommand), answerHookFacts{branch: "main"})
	got = answerHookObserved(steps)
	if !out.Answer.Block || !slices.Equal(out.Answer.Contexts, []string{"Cuidado"}) || len(got) != 1 ||
		got[0].Outcome != state.OutcomeInjected || got[0].Key != "Cuidado" {
		t.Fatalf("a refused command: %+v %+v", out.Answer, got)
	}
}

// answerHookHashes returns the epochs of the content hashes the steps asked for, in order.
func answerHookHashes(steps []Step) []int {
	var out []int
	for _, s := range steps {
		if s.Hash {
			out = append(out, s.Epoch)
		}
	}
	return out
}

// finished reads the log before the content hash: without a finished line with hash of its type on the branch the
// hash is never computed, and a log that cannot be read warns; a run whose hash before it is unknown never asks for
// the hash after it.
func TestAnswerHookHashOnlyWhenNeeded(t *testing.T) {
	gate := answerHookEval(t, "version: 1\non:\n  pre_commit:\n    - unless: { finished: polix:reviewer }\n"+
		"      action: { block: \"Revisa\" }\n", workflow.PreCommit)
	unhashed := state.State{Entries: []state.Entry{{Kind: state.KindObserved, Outcome: state.OutcomeFinished,
		Key: "polix:reviewer", Branch: "main"}}}
	for name, st := range map[string]state.State{"empty log": {}, "finished without hash": unhashed,
		"another branch": answerHookFinishedOn("dev", "h"), "another type": {Entries: []state.Entry{{
			Kind: state.KindObserved, Outcome: state.OutcomeFinished, Key: "acme:docs", Branch: "main", Hash: "h"}}}} {
		out, steps := answerHookWalk(t, gate, answerHookFacts{branch: "main", hashes: []string{"h"}, state: st})
		if !out.Answer.Block || len(answerHookHashes(steps)) != 0 {
			t.Errorf("%s: %+v %v", name, out.Answer, answerHookHashes(steps))
		}
	}
	out, steps := answerHookWalk(t, gate, answerHookFacts{branch: "main", hashes: []string{"h"},
		stateErr: errors.New("permission denied")})
	if !out.Answer.Block || len(answerHookHashes(steps)) != 0 || !slices.ContainsFunc(out.Answer.Warnings,
		func(w string) bool { return strings.Contains(w, "no puede usar su log (permission denied)") }) {
		t.Fatalf("a log that cannot be read: %+v", out.Answer)
	}
	run := answerHookEval(t, "version: 1\non:\n  stop:\n    - action: { run: just test }\n", workflow.Stop)
	_, steps = answerHookWalk(t, run, answerHookFacts{branch: "main", hashErr: errors.New("permission denied")})
	if got := answerHookHashes(steps); len(answerHookRuns(steps)) != 1 || !slices.Equal(got, []int{0}) ||
		len(answerHookObserved(steps)) != 0 {
		t.Fatalf("an unknown hash before the run: %v", got)
	}
}

// answerHookFinishedOn is the log after the subagent polix:reviewer finished on branch with the content hash.
func answerHookFinishedOn(branch, hash string) state.State {
	st := answerHookFinished(hash)
	st.Entries[0].Branch = branch
	return st
}

// On a detached HEAD the branch is empty: what was observed on a branch never counts there, so a run that passed
// runs again, a block with once blocks again and finished does not hold.
func TestAnswerHookDetachedHead(t *testing.T) {
	onMain := state.State{Entries: []state.Entry{
		{Kind: state.KindObserved, Outcome: state.OutcomePassed, Branch: "main", Key: "just test", Line: 4, Hash: "h"},
		{Kind: state.KindObserved, Outcome: state.OutcomeBlocked, Branch: "main", Session: "s1",
			Actor: agent.ActorSession, Line: 4, Hash: "h"},
		answerHookFinished("h").Entries[0],
	}}
	cases := map[string]struct {
		yaml, event string
		acted       func(AnswerHookOutput, []Step) bool
	}{
		"run": {"version: 1\non:\n  stop:\n    - action: { run: just test }\n", workflow.Stop,
			func(_ AnswerHookOutput, steps []Step) bool { return len(answerHookRuns(steps)) == 1 }},
		"block once": {"version: 1\non:\n  pre_commit:\n    - action: { block: { message: x, once: true } }\n",
			workflow.PreCommit, func(out AnswerHookOutput, _ []Step) bool { return out.Answer.Block }},
		"finished": {"version: 1\non:\n  pre_commit:\n    - unless: { finished: polix:reviewer }\n" +
			"      action: { block: x }\n", workflow.PreCommit,
			func(out AnswerHookOutput, _ []Step) bool { return out.Answer.Block }},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			ev := answerHookEval(t, c.yaml, c.event)
			for _, branch := range []string{"", "main"} {
				out, steps := answerHookWalk(t, ev, answerHookFacts{branch: branch, hashes: []string{"h", "h"},
					state: onMain})
				if acted := c.acted(out, steps); acted != (branch == "") {
					t.Fatalf("branch %q: acted %v: %+v", branch, acted, out.Answer)
				}
			}
		})
	}
}
