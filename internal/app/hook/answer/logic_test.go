package answer

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/mlophez/polix-ai/internal/app/agent"
	"github.com/mlophez/polix-ai/internal/app/diff"
	"github.com/mlophez/polix-ai/internal/app/hook"
	"github.com/mlophez/polix-ai/internal/app/rule"
	"github.com/mlophez/polix-ai/internal/app/state"
	"github.com/mlophez/polix-ai/internal/app/workflow"
	workflowload "github.com/mlophez/polix-ai/internal/app/workflow/load"
)

// The logic is walked step by step with domain values: each step it asks is answered from a table of facts and
// outcomes, as the shell would from the port, and fed back to the evaluation.

// facts are the facts and outcomes a walk answers the steps with.
type facts struct {
	branch    string
	head      string   // the commit HEAD points to, learned with the branch; "" for unknown
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
	found     bool              // whether a file holds the text of a search
	trees     []string          // the paths that differ between two trees
	treeErr   error             // why two trees cannot be compared
	ignored   map[string]bool   // the paths git ignores
	reals     map[string]string // the real path of a path, when it differs from the path
	foreign   map[string]bool   // the folders inside another git repository
	missing   []string          // the programs the PATH does not run

	// checkFails, when set, decides each check instead of runErr: it fails when it covers one of these rules
	checkFails map[string]bool
}

// walk asks the logic for steps until it answers, feeding each one from f, and returns the output and
// the steps asked, in order.
func walk(t *testing.T, ev hook.Evaluation, f facts) (Output, []hook.Step) {
	t.Helper()
	var steps []hook.Step
	for range 100 {
		out := Decide(ev)
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
			ev = ev.KnowBranch(f.branch, f.head)
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
			ev = ev.KnowIgnored(s.Ignored, f.ignored[s.Ignored])
		case s.Real != nil:
			reals := make([]string, len(s.Real))
			for i, p := range s.Real {
				reals[i] = p
				if resolved, ok := f.reals[p]; ok {
					reals[i] = resolved
				}
			}
			ev = ev.KnowReal(s.Real, reals)
		case s.Foreign != "":
			ev = ev.KnowForeign(s.Foreign, f.foreign[s.Foreign])
		case s.Programs != nil:
			ev = ev.KnowMissing(f.missing)
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
	return Output{}, nil
}

// parseWorkflow loads src as the polix.yaml of a temporary folder for the polix of version running, through the
// loader of polix.yaml (internal/app/workflow/load) as cmd/polix builds it: the workflow and the error of loading
// it.
func parseWorkflow(src, running string) (workflow.Workflow, error) {
	root, err := os.MkdirTemp("", "polix-answer-")
	if err != nil {
		return workflow.Workflow{}, err
	}
	defer os.RemoveAll(root)
	if err := os.WriteFile(filepath.Join(root, workflow.File), []byte(src), 0o644); err != nil {
		return workflow.Workflow{}, err
	}
	loaded := workflowload.Command()(context.Background(), filepath.Join(root, workflow.File), running)
	return loaded.Workflow, loaded.Err
}

// eval returns the evaluation of event name of the main session with the workflow of yaml.
func eval(t *testing.T, yaml, name string) hook.Evaluation {
	t.Helper()
	wf, err := parseWorkflow(yaml, "dev")
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.FromSlash("/repo")
	ev := hook.Evaluation{Workflow: wf, Present: true, Root: root, GitDir: filepath.Join(root, ".git"), System: "linux",
		Event: agent.Event{Name: name, Agent: agent.Claude, Session: "s1", Actor: agent.ActorSession, Cwd: root}}
	if err := ev.Valid(); err != nil {
		t.Fatal(err)
	}
	return ev
}

// as returns ev as the event of the subagent actor.
func as(t *testing.T, ev hook.Evaluation, actor string) hook.Evaluation {
	t.Helper()
	e := ev.Event
	e.Actor = actor
	out := hook.Evaluation{Workflow: ev.Workflow, Present: ev.Present, Root: ev.Root, GitDir: ev.GitDir, System: ev.System,
		Event: e}
	if err := out.Valid(); err != nil {
		t.Fatal(err)
	}
	return out
}

// appended returns the entries the steps appended to the log.
func appended(steps []hook.Step) []state.Entry {
	var out []state.Entry
	for _, s := range steps {
		if s.Append != nil {
			out = append(out, *s.Append)
		}
	}
	return out
}

// observed returns the observed entries the steps appended to the log, in order.
func observed(steps []hook.Step) []state.Entry {
	var out []state.Entry
	for _, e := range appended(steps) {
		if e.Kind == state.KindObserved {
			out = append(out, e)
		}
	}
	return out
}

// finished is the log after the subagent polix:reviewer finished on the content hash.
func finished(hash string) state.State {
	return state.State{Entries: []state.Entry{{Kind: state.KindObserved, Outcome: state.OutcomeFinished,
		Session: "s1", Event: workflow.SubagentStop, Actor: "polix:reviewer", Key: "polix:reviewer", Branch: "main",
		Hash: hash}}}
}

// A run that rewrites files changes the content: a later finished is decided on the hash of the new content.
func TestFinishedAfterRunUsesNewContent(t *testing.T) {
	ev := eval(t, `version: 1
on:
  stop:
    - action: { run: [gofmt, -w, .] }
    - unless: { finished: polix:reviewer }
      action: { block: "Falta la revisión" }
`, workflow.Stop)
	reviewed := finished("new")
	out, steps := walk(t, ev, facts{branch: "main", hashes: []string{"old", "new"}, state: reviewed})
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
	out, _ = walk(t, ev, facts{branch: "main", hashes: []string{"new", "old"}, state: reviewed,
		trees: []string{"a.go"}})
	if !out.Answer.Block {
		t.Fatal("finished held on a content it was not observed on")
	}
}

// A block that actually blocks ends the evaluation: the entries after it never run, an inject before it reaches the
// agent with the block, and the decision is logged with its verb and line.
func TestBlockEnds(t *testing.T) {
	ev := eval(t, `version: 1
on:
  pre_command:
    - action: { inject: { text: "cuidado", always: true } }
    - when: { branch: "^main$" }
      action: { block: "primero" }
    - action: { block: "segundo" }
`, workflow.PreCommand)
	out, steps := walk(t, ev, facts{branch: "main"})
	if !out.Answer.Block || !slices.Equal(out.Answer.Reasons, []string{"primero"}) ||
		!slices.Equal(out.Answer.Contexts, []string{"cuidado"}) {
		t.Fatalf("answer %+v", out.Answer)
	}
	logged := appended(steps)
	if len(logged) != 1 || logged[0].Action != workflow.VerbBlock || logged[0].Outcome != "block" || logged[0].Line != 5 {
		t.Fatalf("logged: %+v", logged)
	}
}

// runs returns the commands the steps ran, in order.
func runs(steps []hook.Step) [][]string {
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
func TestFailingRunEnds(t *testing.T) {
	ev := eval(t, `version: 1
on:
  stop:
    - action: { run: just lint }
    - action: { run: just test }
    - action: { block: "nunca" }
`, workflow.Stop)
	out, steps := walk(t, ev, facts{branch: "main", hashes: []string{"h", "h"},
		runErr: errors.New("exit status 1")})
	if runs := runs(steps); len(runs) != 1 || !slices.Equal(runs[0], []string{"just", "lint"}) {
		t.Fatalf("only the first run runs: %q", runs)
	}
	if !out.Answer.Block || len(out.Answer.Reasons) != 1 ||
		!strings.HasPrefix(out.Answer.Reasons[0], "just lint failed: exit status 1") {
		t.Fatalf("answer: %+v", out.Answer)
	}
	logged := appended(steps)
	if last := logged[len(logged)-1]; last.Action != workflow.VerbRun || last.Outcome != "block" || last.Line != 4 {
		t.Fatalf("logged: %+v", logged)
	}
	// A passing run lets the evaluation go on: the second one runs and blocks on its own.
	f := facts{branch: "main", hashes: []string{"h", "h", "h"}}
	out, steps = walk(t, ev, f)
	if runs := runs(steps); len(runs) != 2 || !out.Answer.Block ||
		!slices.Equal(out.Answer.Reasons, []string{"nunca"}) {
		t.Fatalf("passing runs: %q %+v", runs, out.Answer)
	}
}

// AC7: a block or a failing run let through by the loop guard does not end the evaluation: the next entry runs and
// blocks on its own, with the warning of the guard.
func TestLoopGuardGoesOn(t *testing.T) {
	yaml := `version: 1
max_stop_retries: 1
on:
  stop:
    - action: { %s }
    - action: { run: just test }
`
	guarded := state.State{Entries: []state.Entry{{Kind: state.KindObserved, Outcome: state.OutcomeBlocked,
		Session: "s1", Event: workflow.Stop, Actor: agent.ActorSession, Line: 5, Hash: "h"}}}
	for name, verb := range map[string]string{"run": "run: just lint", "block": `block: "revisa"`} {
		t.Run(name, func(t *testing.T) {
			ev := eval(t, strings.Replace(yaml, "%s", verb, 1), workflow.Stop)
			out, steps := walk(t, ev, facts{branch: "main", hashes: []string{"h", "h", "h"},
				state: guarded, runErr: errors.New("exit status 1")})
			runs := runs(steps)
			if len(runs) == 0 || !slices.Equal(runs[len(runs)-1], []string{"just", "test"}) {
				t.Fatalf("the next entry runs: %q", runs)
			}
			if !out.Answer.Block || len(out.Answer.Reasons) != 1 ||
				!strings.HasPrefix(out.Answer.Reasons[0], "just test failed: exit status 1") ||
				len(out.Answer.Warnings) != 1 || !strings.Contains(out.Answer.Warnings[0], "after 1 blocks") {
				t.Fatalf("answer: %+v", out.Answer)
			}
		})
	}
}

// AC7: a failing run on post_edit informs and never ends the evaluation: both failures reach the AI.
func TestInformingRunsGoOn(t *testing.T) {
	ev := eval(t, `version: 1
on:
  post_edit:
    - action: { run: just lint }
    - action: { run: just test }
`, workflow.PostEdit)
	out, steps := walk(t, ev, facts{branch: "main", runErr: errors.New("exit status 1")})
	if runs := runs(steps); len(runs) != 2 {
		t.Fatalf("both runs run: %q", runs)
	}
	if out.Answer.Block || len(out.Answer.Contexts) != 2 ||
		!strings.HasPrefix(out.Answer.Contexts[0], "just lint failed: exit status 1") ||
		!strings.HasPrefix(out.Answer.Contexts[1], "just test failed: exit status 1") {
		t.Fatalf("answer: %+v", out.Answer)
	}
}

// AC6 and AC7 on pre_edit: a block before an inject drops the inject, and an inject before a block reaches the
// agent with the block.
func TestBlockAndInjectOrder(t *testing.T) {
	cases := []struct {
		name, entries string
		contexts      []string
	}{
		{"block first", "    - when: { branch: \"^main$\" }\n      action: { block: \"no\" }\n" +
			"    - action: { inject: \"después\" }\n", nil},
		{"inject first", "    - action: { inject: \"antes\" }\n    - action: { block: \"no\" }\n", []string{"antes"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			base := eval(t, "version: 1\non:\n  pre_edit:\n"+c.entries, workflow.PreEdit)
			ev := hook.Evaluation{Workflow: base.Workflow, Present: true, Root: base.Root, GitDir: base.GitDir,
				System: "linux", Event: agent.Event{Name: workflow.PreEdit, Agent: agent.Claude, Session: "s1",
					Actor: agent.ActorSession, Path: "src/a.go", AbsPath: filepath.Join(base.Root, "src", "a.go")}}
			if err := ev.Valid(); err != nil {
				t.Fatal(err)
			}
			out, _ := walk(t, ev, facts{branch: "main"})
			if !out.Answer.Block || !slices.Equal(out.Answer.Reasons, []string{"no"}) ||
				!slices.Equal(out.Answer.Contexts, c.contexts) {
				t.Fatalf("answer: %+v", out.Answer)
			}
		})
	}
}

// An exception written as a not in when (once allow) skips the block for what it names.
func TestExceptionWithNot(t *testing.T) {
	ev := eval(t, `version: 1
on:
  pre_edit:
    - when: { not: { path: "ai/plans/*.md" } }
      action: { block: "No hay plan" }
`, workflow.PreEdit)
	for path, block := range map[string]bool{"ai/plans/p.md": false, "src/a.go": true} {
		e := hook.Evaluation{Workflow: ev.Workflow, Present: true, Root: ev.Root, GitDir: ev.GitDir, System: "linux",
			Event: agent.Event{Name: workflow.PreEdit, Agent: agent.Claude, Actor: agent.ActorSession, Path: path,
				AbsPath: filepath.Join(ev.Root, filepath.FromSlash(path))}}
		if err := e.Valid(); err != nil {
			t.Fatal(err)
		}
		if out, _ := walk(t, e, facts{branch: "main"}); out.Answer.Block != block {
			t.Errorf("%s: %+v", path, out.Answer)
		}
	}
}

// The loop guard lets the agent stop after max_stop_retries blocks of the entry on the same content.
func TestLoopGuard(t *testing.T) {
	ev := eval(t, "version: 1\nmax_stop_retries: 2\non:\n  stop:\n    - action: { block: \"revisa\" }\n",
		workflow.Stop)
	block := state.Entry{Kind: state.KindObserved, Outcome: state.OutcomeBlocked, Session: "s1",
		Event: workflow.Stop, Actor: agent.ActorSession, Line: 5, Hash: "h"}
	one := state.State{Entries: []state.Entry{block}}
	out, steps := walk(t, ev, facts{branch: "main", hashes: []string{"h"}, state: one})
	if !out.Answer.Block {
		t.Fatal("one block of two: the stop is blocked")
	}
	if logged := appended(steps); len(logged) != 2 || logged[0].Kind != state.KindObserved ||
		logged[0].Outcome != state.OutcomeBlocked || logged[0].Hash != "h" || logged[0].Line != 5 ||
		logged[1].Outcome != "block" {
		t.Fatalf("logged: %+v", logged)
	}
	two := state.State{Entries: []state.Entry{block, block}}
	out, steps = walk(t, ev, facts{branch: "main", hashes: []string{"h"}, state: two})
	if out.Answer.Block || len(out.Answer.Warnings) != 1 || !strings.Contains(out.Answer.Warnings[0], "after 2 blocks") {
		t.Fatalf("answer after max_stop_retries: %+v", out.Answer)
	}
	if logged := appended(steps); len(logged) != 1 || logged[0].Outcome != "loop_guard" {
		t.Fatalf("logged: %+v", logged)
	}
	// Another content has its own count.
	out, _ = walk(t, ev, facts{branch: "main", hashes: []string{"other"}, state: two})
	if !out.Answer.Block {
		t.Fatal("the blocks of another content count")
	}
}

// A log that cannot be read or written lets the agent stop: the guard cannot count.
func TestLoopGuardFailsOpen(t *testing.T) {
	ev := eval(t, "version: 1\non:\n  stop:\n    - action: { block: \"revisa\" }\n", workflow.Stop)
	for name, f := range map[string]facts{
		"unreadable": {branch: "main", hashes: []string{"h"}, stateErr: errors.New("permission denied")},
		"read only":  {branch: "main", hashes: []string{"h"}, appendErr: errors.New("permission denied")},
	} {
		out, _ := walk(t, ev, f)
		if out.Answer.Block || len(out.Answer.Warnings) != 1 ||
			!strings.Contains(out.Answer.Warnings[0], "cannot use its log (permission denied): revisa") {
			t.Errorf("%s: %+v", name, out.Answer)
		}
	}
}

// unshownMessages returns the reasons of the skip decisions of actionMessage the steps logged, in order: the
// messages for the user of a blocked event, which are not shown.
func unshownMessages(steps []hook.Step) []string {
	var out []string
	for _, e := range decided(steps, "skip") {
		if e.Action == actionMessage {
			out = append(out, e.Reason)
		}
	}
	return out
}

// AC7: the warning of the loop guard reaches the messages for the user of an answer that is not blocked, and nothing
// is logged as not shown. A blocked answer keeps its messages, and the logic logs each one as a skip decision that
// says it was not shown: on a stop the guard let an earlier entry through, on pre_commit and on prompt a log that
// cannot be read.
func TestMessagesOfABlockedEvent(t *testing.T) {
	block := state.Entry{Kind: state.KindObserved, Outcome: state.OutcomeBlocked, Session: "s1",
		Event: workflow.Stop, Actor: agent.ActorSession, Line: 5, Hash: "h"}
	guarded := state.State{Entries: []state.Entry{block}}
	ev := eval(t, "version: 1\nmax_stop_retries: 1\non:\n  stop:\n    - action: { block: \"revisa\" }\n",
		workflow.Stop)
	out, steps := walk(t, ev, facts{branch: "main", hashes: []string{"h"}, state: guarded})
	if out.Answer.Block || len(out.Answer.Warnings) != 1 || !strings.Contains(out.Answer.Warnings[0], "after 1 blocks") ||
		len(unshownMessages(steps)) != 0 {
		t.Fatalf("the guard lets the stop through: %+v %v", out.Answer, unshownMessages(steps))
	}
	unreadable := errors.New("permission denied")
	for _, c := range []struct {
		name, yaml, event string
		f                 facts
		want              string
	}{
		{"stop", "version: 1\nmax_stop_retries: 1\non:\n  stop:\n    - action: { block: \"revisa\" }\n" +
			"    - action: { run: just test }\n", workflow.Stop,
			facts{branch: "main", hashes: []string{"h", "h"}, state: guarded, runErr: errors.New("exit status 1")},
			"polix lets the agent stop after 1 blocks: revisa"},
		{"pre_commit", "version: 1\non:\n  pre_commit:\n    - unless: { finished: polix:reviewer }\n" +
			"      action: { block: \"Revisa\" }\n", workflow.PreCommit,
			facts{branch: "main", hashes: []string{"h"}, stateErr: unreadable}, "cannot use its log (permission denied)"},
		{"prompt", "version: 1\non:\n  prompt:\n    - action: { block: { message: \"Espera\", once: true } }\n",
			workflow.Prompt, facts{branch: "main", hashes: []string{"h"}, stateErr: unreadable},
			"cannot use its log (permission denied)"},
	} {
		out, steps := walk(t, eval(t, c.yaml, c.event), c.f)
		unshown := unshownMessages(steps)
		if !out.Answer.Block || len(out.Answer.Warnings) != 1 || !strings.Contains(out.Answer.Warnings[0], c.want) ||
			len(unshown) != 1 || !strings.Contains(unshown[0], "not shown to the user: "+out.Answer.Warnings[0]) {
			t.Errorf("%s: %+v %q", c.name, out.Answer, unshown)
		}
	}
}

// AC7: a protection and a workflow that cannot be used block with the pending warnings of the call, which are logged
// as not shown; a workflow that cannot be used on a stop warns once with no prefix of its own, which the render
// adds.
func TestMessagesOfARefusal(t *testing.T) {
	hashErr := "polix cannot hash the content (boom)"
	ev := eval(t, "version: 1\nguardrails:\n  protected_files: [.env]\n", workflow.PreEdit)
	protected := edit(t, ev, workflow.PreEdit, ".env", agent.ActorSession).KnowHash(0, "", errors.New("boom"))
	invalid := edit(t, ev, workflow.PreEdit, "src/a.go", agent.ActorSession).KnowHash(0, "", errors.New("boom"))
	invalid.Invalid = errors.New("polix.yaml: unknown key")
	for name, ev := range map[string]hook.Evaluation{"protection": protected, "invalid": invalid} {
		out, steps := walk(t, ev, facts{branch: "main"})
		unshown := unshownMessages(steps)
		if !out.Answer.Block || len(unshown) != 1 || !strings.Contains(unshown[0], "not shown to the user: "+hashErr) {
			t.Errorf("%s: %+v %q", name, out.Answer, unshown)
		}
	}
	stop := eval(t, "version: 1\n", workflow.Stop)
	stop.Invalid = workflow.TooOldError{Requires: "0.2.0", Running: "0.1.0"}
	out, steps := walk(t, stop, facts{})
	want := "this repository needs polix 0.2.0 or later and the installed one is 0.1.0: run polix update"
	if out.Answer.Block || !slices.Equal(out.Answer.Warnings, []string{want}) || len(unshownMessages(steps)) != 0 {
		t.Fatalf("a stop with a workflow ahead of polix: %+v", out.Answer)
	}
}

// AC7: the answers of the events of one call are merged into one, so a block of one hides the messages of the
// others: a later event that blocks logs the messages of an earlier one that did not, and a later event that does
// not block logs its own after an earlier one that blocked.
func TestMessagesOfACall(t *testing.T) {
	ev := eval(t, "version: 1\nguardrails:\n  protected_files: [.env]\non:\n  pre_edit:\n"+
		"    - action: { inject: \"Lee el plan\" }\n", workflow.PreEdit)
	code := edit(t, ev, workflow.PreEdit, "src/a.go", agent.ActorSession)
	env := edit(t, ev, workflow.PreEdit, ".env", agent.ActorSession)
	f := facts{branch: "main", stateErr: errors.New("permission denied")}
	const logErr = "cannot use its log (permission denied)"
	first, steps := walk(t, code, f)
	if first.Answer.Block || len(first.Answer.Warnings) != 1 || len(unshownMessages(steps)) != 0 {
		t.Fatalf("the edit of code warns: %+v", first.Answer)
	}
	env.Facts = first.Facts
	second, steps := walk(t, env, f)
	if unshown := unshownMessages(steps); !second.Answer.Block || len(unshown) != 1 ||
		!strings.Contains(unshown[0], logErr) {
		t.Fatalf("a later block logs the earlier message: %+v %q", second.Answer, unshown)
	}
	env = edit(t, ev, workflow.PreEdit, ".env", agent.ActorSession)
	first, steps = walk(t, env, f)
	if !first.Answer.Block || len(unshownMessages(steps)) != 0 || !first.Facts.Blocked {
		t.Fatalf("the edit of .env is denied without messages: %+v", first.Answer)
	}
	code.Facts = first.Facts
	second, steps = walk(t, code, f)
	if unshown := unshownMessages(steps); second.Answer.Block || len(unshown) != 1 ||
		!strings.Contains(unshown[0], logErr) {
		t.Fatalf("a later message after an earlier block is logged: %+v %q", second.Answer, unshown)
	}
}

// A failing run blocks with its output; a passing one writes only that it passed.
func TestRun(t *testing.T) {
	ev := eval(t, "version: 1\non:\n  stop:\n    - action: { run: just test }\n", workflow.Stop)
	out, steps := walk(t, ev, facts{branch: "main", hashes: []string{"h", "h"}})
	if logged := appended(steps); out.Answer.Block || len(logged) != 1 ||
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
	out, _ = walk(t, ev, facts{branch: "main", hashes: []string{"h", "h"},
		runErr: errors.New("exit status 1")})
	if !out.Answer.Block || !strings.HasPrefix(out.Answer.Reasons[0], "just test failed: exit status 1\n  salida") {
		t.Fatalf("a failing run blocks with its output: %+v", out.Answer)
	}
}

// AC9: a failing run with a message answers with the message, a blank line and the output.
func TestRunMessage(t *testing.T) {
	ev := eval(t, "version: 1\non:\n  stop:\n"+
		"    - action: { run: { command: [just, test], message: \"Arregla los tests de {branch}\" } }\n", workflow.Stop)
	out, _ := walk(t, ev, facts{branch: "main", hashes: []string{"h"},
		runErr: errors.New("exit status 1")})
	if !out.Answer.Block || !strings.HasPrefix(out.Answer.Reasons[0],
		"Arregla los tests de main\n\njust test failed: exit status 1\n  salida") {
		t.Fatalf("answer: %+v", out.Answer)
	}
	out, _ = walk(t, ev, facts{branch: "main", hashes: []string{"h"}})
	if out.Answer.Block || len(out.Answer.Contexts) != 0 {
		t.Fatalf("a passing run says nothing: %+v", out.Answer)
	}
}

// AC21: a failing run on start, subagent_start, post_edit or post_command never blocks: its message and output reach
// the AI as context, the evaluation goes on, and the decision is logged as inform.
func TestRunInformsAfterTheFact(t *testing.T) {
	for _, event := range []string{workflow.Start, workflow.SubagentStart, workflow.PostEdit, workflow.PostCommand} {
		t.Run(event, func(t *testing.T) {
			ev := eval(t, "version: 1\nguidance: { commit: false, edit: false, layout: false }\non:\n  "+
				event+":\n    - action: { run: { command: [ruff, check], message: \"Corrige el estilo\" } }\n"+
				"    - action: { inject: { text: \"después\", always: true } }\n", event)
			out, steps := walk(t, ev, facts{branch: "main", runErr: errors.New("exit status 1")})
			if out.Answer.Block || len(out.Answer.Contexts) != 2 ||
				!strings.HasPrefix(out.Answer.Contexts[0], "Corrige el estilo\n\nruff check failed: exit status 1\n  salida") ||
				out.Answer.Contexts[1] != "después" {
				t.Fatalf("answer: %+v", out.Answer)
			}
			if logged := appended(steps); len(logged) != 1 || logged[0].Outcome != "inform" ||
				logged[0].Action != workflow.VerbRun || logged[0].Line != 5 {
				t.Fatalf("logged: %+v", logged)
			}
		})
	}
}

// AC17: a command of the AI, green or not, writes no observed line and never makes finished hold.
func TestCommandsOfTheAINeverCount(t *testing.T) {
	ev := eval(t, `version: 1
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
		post := hook.Evaluation{Workflow: ev.Workflow, Present: true, Root: ev.Root, GitDir: ev.GitDir, System: "linux",
			Event: e}
		if err := post.Valid(); err != nil {
			t.Fatal(err)
		}
		_, steps := walk(t, post, facts{branch: "main", hashes: []string{"h"}})
		if len(appended(steps)) != 0 {
			t.Fatalf("exit %d: %+v", exit, appended(steps))
		}
	}
	commit := eval(t, `version: 1
on:
  pre_commit:
    - unless: { finished: polix:reviewer }
      action: { block: "Revisa" }
`, workflow.PreCommit)
	if out, _ := walk(t, commit, facts{branch: "main", hashes: []string{"h"}}); !out.Answer.Block {
		t.Fatalf("finished holds without a subagent: %+v", out.Answer)
	}
}

// AC16 and AC19: a subagent_stop of an observed type that is not refused writes finished with the hash; a type the
// workflow does not observe, a refused stop and a stop let through by the loop guard write nothing.
func TestObservesFinished(t *testing.T) {
	yaml := `version: 1
max_stop_retries: 1
on:
  pre_commit:
    - unless: { finished: polix:reviewer }
      action: { block: "Revisa" }
  subagent_stop:
    - when: { actor: polix:reviewer, branch: "^refuse$" }
      action: { block: "Termina la revisión" }
`
	stop := eval(t, yaml, workflow.SubagentStop)
	reviewer := as(t, stop, "polix:reviewer")
	_, steps := walk(t, reviewer, facts{branch: "main", hashes: []string{"h"}})
	got := observed(steps)
	if len(got) != 1 || got[0].Outcome != state.OutcomeFinished || got[0].Key != "polix:reviewer" ||
		got[0].Hash != "h" || got[0].Branch != "main" || got[0].Actor != "polix:reviewer" ||
		got[0].Event != workflow.SubagentStop {
		t.Fatalf("finished: %+v", got)
	}
	// Another type is not observed, and no hash is computed for it.
	_, steps = walk(t, as(t, stop, "acme:builder"), facts{branch: "main",
		hashes: []string{"h"}})
	if len(observed(steps)) != 0 || slices.ContainsFunc(steps, func(s hook.Step) bool { return s.Hash }) {
		t.Fatalf("another type: %+v", steps)
	}
	// A refused stop: the subagent goes on, and only the block is observed, for the loop guard.
	out, steps := walk(t, reviewer, facts{branch: "refuse", hashes: []string{"h"}})
	if got := observed(steps); !out.Answer.Block || len(got) != 1 || got[0].Outcome != state.OutcomeBlocked {
		t.Fatalf("refused: %+v %+v", out.Answer, got)
	}
	// Let through by the loop guard: the subagent stops, but it did not finish as asked.
	guard := state.State{Entries: []state.Entry{{Kind: state.KindObserved, Outcome: state.OutcomeBlocked,
		Session: "s1", Event: workflow.SubagentStop, Actor: "polix:reviewer", Line: 8, Hash: "h"}}}
	out, steps = walk(t, reviewer, facts{branch: "refuse", hashes: []string{"h"}, state: guard})
	if out.Answer.Block || len(observed(steps)) != 0 {
		t.Fatalf("loop guard: %+v %+v", out.Answer, observed(steps))
	}
	// An unknown hash writes nothing, with the decision.
	out, steps = walk(t, reviewer, facts{branch: "main", hashErr: errors.New("permission denied")})
	logged := appended(steps)
	if len(observed(steps)) != 0 || len(logged) != 1 || logged[0].Outcome != "warn" ||
		len(out.Answer.Warnings) != 1 || !strings.Contains(out.Answer.Warnings[0], "hash the content (permission denied)") {
		t.Fatalf("unknown hash: %+v %+v", out.Answer, logged)
	}
	// Without a finished in the workflow, nothing is observed.
	plain := as(t, eval(t, "version: 1\n", workflow.SubagentStop), "polix:reviewer")
	if _, steps := walk(t, plain, facts{branch: "main", hashes: []string{"h"}}); len(steps) != 0 {
		t.Fatalf("nothing to observe: %+v", steps)
	}
	// Requirements alone tell nothing to a subagent, so its end is not observed either.
	needs := as(t, eval(t, "version: 1\nrequirements: [go]\n", workflow.SubagentStop), "polix:reviewer")
	if _, steps := walk(t, needs, facts{branch: "main", hashes: []string{"h"}}); len(steps) != 0 {
		t.Fatalf("requirements alone: %+v", steps)
	}
}

// AC16: finished holds after the subagent ended and after an edit outside the scope of the entry, not after an edit
// inside it, nor without a changed after any edit, nor when the trees cannot be compared, nor for another type or
// another branch.
func TestFinished(t *testing.T) {
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
		f          facts
		block      bool
	}{
		{"same content", scoped, facts{hashes: []string{"h"}, state: finished("h")}, false},
		{"edit of README.md", scoped, facts{hashes: []string{"h2"}, state: finished("h"),
			trees: []string{"README.md", "docs/polix.md"}}, false},
		{"edit of a Go file", scoped, facts{hashes: []string{"h2"}, state: finished("h"),
			trees: []string{"README.md", "internal/a.go"}}, true},
		{"no changed, any edit", whole, facts{hashes: []string{"h2"}, state: finished("h"),
			trees: []string{"README.md"}}, true},
		{"no changed, same content", whole, facts{hashes: []string{"h"}, state: finished("h")}, false},
		{"tree missing", scoped, facts{hashes: []string{"h2"}, state: finished("h"),
			treeErr: errors.New("fatal: bad object h")}, true},
		{"unknown hash", scoped, facts{hashErr: errors.New("boom"), state: finished("h")}, true},
		{"never finished", scoped, facts{hashes: []string{"h"}}, true},
		{"another branch", scoped, facts{branch: "dev", hashes: []string{"h"}, state: finished("h")},
			true},
		{"every type of a list", listed, facts{hashes: []string{"h"}, state: finished("h")}, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if c.f.branch == "" {
				c.f.branch = "main"
			}
			c.f.changed = []string{"internal/a.go"}
			out, steps := walk(t, eval(t, c.yaml, workflow.PreCommit), c.f)
			if out.Answer.Block != c.block {
				t.Fatalf("block %v, want %v: %+v", out.Answer.Block, c.block, out.Answer)
			}
			if c.f.treeErr != nil {
				logged := appended(steps)
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
	out, _ := walk(t, eval(t, scoped, workflow.PreCommit), facts{branch: "main",
		hashes: []string{"h"}, state: other, changed: []string{"a.go"}})
	if !out.Answer.Block {
		t.Fatal("finished held for another type")
	}
	// The latest finished counts: a later one on another content replaces an earlier one on this content.
	later := finished("h")
	later.Entries = append(later.Entries, finished("h0").Entries...)
	out, _ = walk(t, eval(t, scoped, workflow.PreCommit), facts{branch: "main",
		hashes: []string{"h"}, state: later, changed: []string{"a.go"}, trees: []string{"a.go"}})
	if !out.Answer.Block {
		t.Fatal("an earlier finished counted over the latest")
	}
}

// run: { rules: changed } runs the checks of the rules of the changed files through the runner of their file.
func TestRunRules(t *testing.T) {
	ev := eval(t, `version: 1
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
	out, steps := walk(t, ev, facts{branch: "main", changed: []string{"src/a.go"}, rules: rules,
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
	if !out.Answer.Block || !strings.HasPrefix(out.Answer.Reasons[0], "Rules not met:\n- GO-1 (tests/go1_test.go): d") {
		t.Fatalf("answer: %+v", out.Answer)
	}
}

// A batch runner checks every matching rule in one step; a failed group is rerun per rule and the block names
// only the rules whose rerun fails.
func TestRunRulesBatch(t *testing.T) {
	ev := eval(t, `version: 1
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
	checksOf := func(steps []hook.Step) []rule.Run {
		var out []rule.Run
		for _, s := range steps {
			if s.Check != nil {
				out = append(out, *s.Check)
			}
		}
		return out
	}
	facts := facts{branch: "main", changed: []string{"src/a.go"}, rules: rules,
		checkFails: map[string]bool{}}
	out, steps := walk(t, ev, facts)
	checks := checksOf(steps)
	if len(checks) != 1 || len(checks[0].Rules) != 3 ||
		!slices.Equal(checks[0].Argv, []string{"go", "test", "./tests", "-run", "^Test(A1|A2|A3)$"}) {
		t.Fatalf("checks: %+v", checks)
	}
	if out.Answer.Block {
		t.Fatalf("a passing group lets the stop through: %+v", out.Answer)
	}
	facts.checkFails = map[string]bool{"A-2": true}
	out, steps = walk(t, ev, facts)
	if checks = checksOf(steps); len(checks) != 4 || len(checks[2].Rules) != 1 || checks[2].Rules[0].ID != "A-2" {
		t.Fatalf("a failed group is rerun per rule: %+v", checks)
	}
	if !out.Answer.Block || !strings.HasPrefix(out.Answer.Reasons[0], "Rules not met:\n- A-2 (tests/a2_test.go): a2") ||
		strings.Contains(out.Answer.Reasons[0], "A-1") || strings.Contains(out.Answer.Reasons[0], "A-3") {
		t.Fatalf("answer: %+v", out.Answer)
	}
}

// Nobody edits the state of polix, and a command that writes it is denied, whatever the workflow says.
func TestProtectsState(t *testing.T) {
	ev := eval(t, "version: 1\n", workflow.PreEdit)
	ev = hook.Evaluation{Workflow: ev.Workflow, Present: true, Root: ev.Root, GitDir: ev.GitDir, System: "linux",
		Event: agent.Event{Name: workflow.PreEdit, Agent: agent.Claude, Actor: "builder",
			AbsPath: filepath.Join(ev.GitDir, "polix", "log.jsonl")}}
	if err := ev.Valid(); err != nil {
		t.Fatal(err)
	}
	out, steps := walk(t, ev, facts{branch: "main"})
	if !out.Answer.Block || out.Answer.Reasons[0] != reasonStateEdit {
		t.Fatalf("answer: %+v", out.Answer)
	}
	if logged := appended(steps); len(logged) != 1 || logged[0].Action != "builtin" {
		t.Fatalf("logged: %+v", logged)
	}
}

// The session context of the guidance turned on is injected on start only, as one context before those of
// the entries of the event.
func TestGuidance(t *testing.T) {
	session := func(yaml string) []string {
		wf, _ := parseWorkflow(yaml, "dev")
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
			out, _ := walk(t, eval(t, c.yaml, c.event), facts{branch: "main"})
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
	ev := eval(t, "version: 1\n", workflow.PreEdit)
	ev = hook.Evaluation{Workflow: ev.Workflow, Present: true, Root: ev.Root, GitDir: ev.GitDir, System: "linux",
		Event: agent.Event{Name: workflow.PreEdit, Agent: agent.Claude, Actor: agent.ActorSession, Path: "src/a.go",
			AbsPath: filepath.Join(ev.Root, "src", "a.go")}}
	if out, _ := walk(t, ev, facts{branch: "main"}); len(out.Answer.Contexts) != 0 {
		t.Fatalf("pre_edit: %q", out.Answer.Contexts)
	}
	// Without polix.yaml, or with an invalid one, nothing of the guidance is injected.
	start := eval(t, "version: 1\n", workflow.Start)
	without := hook.Evaluation{Workflow: start.Workflow, Root: start.Root, GitDir: start.GitDir, Event: start.Event}
	broken := hook.Evaluation{Workflow: start.Workflow, Present: true, Invalid: errors.New("polix.yaml: unknown key"),
		Root: start.Root, GitDir: start.GitDir, Event: start.Event}
	for name, e := range map[string]hook.Evaluation{"without": without, "invalid": broken} {
		if err := e.Valid(); err != nil {
			t.Fatal(err)
		}
		if out, _ := walk(t, e, facts{}); len(out.Answer.Contexts) != 0 {
			t.Fatalf("%s: %q", name, out.Answer.Contexts)
		}
	}
	if out, _ := walk(t, broken, facts{}); len(out.Answer.Warnings) != 1 {
		t.Fatalf("an invalid workflow keeps its warning: %+v", out.Answer)
	}
}

// AC1, AC16: protected_files denies, for every actor, an edit of a protected path, also when git ignores it, and a
// command that writes one, naming the protected path a folder or a wildcard holds, before an earlier entry of on
// that would block; reading passes. The denial is logged as one of the shortcut.
func TestProtectedReasons(t *testing.T) {
	yaml := "version: 1\nguardrails:\n  protected_files: [\".env\", \"AGENTS.md\", \"ai/rules/**\"]\n" +
		"on:\n  pre_edit:\n    - action: { block: \"regular\" }\n  pre_command:\n    - action: { block: \"regular\" }\n"
	base := eval(t, yaml, workflow.PreEdit)
	edit := agent.Event{Name: workflow.PreEdit, Agent: agent.Claude, Path: ".env",
		AbsPath: filepath.Join(base.Root, ".env"), Cwd: base.Root}
	command := func(line string) agent.Event {
		return agent.Event{Name: workflow.PreCommand, Agent: agent.Claude, Command: line, Cwd: base.Root}
	}
	cases := map[string]struct {
		event agent.Event
		want  string // the reason
	}{
		"edit":     {edit, ".env is protected in polix.yaml: only a person changes it."},
		"command":  {command("echo x > AGENTS.md"), "AGENTS.md is protected in polix.yaml: only a person changes it."},
		"folder":   {command("rm -rf ai"), "ai holds ai/rules/**, protected in polix.yaml: only a person changes it."},
		"wildcard": {command("rm -f ai/*"), "ai/* holds ai/rules/**, protected in polix.yaml: only a person changes it."},
		"chained":  {command("ls && rm -f AGENTS.md"), "AGENTS.md is protected in polix.yaml: only a person changes it."},
		"read":     {command("cat AGENTS.md ai/rules/a.yaml"), "regular"},
		"other":    {command("rm -rf ai/plans"), "regular"},
	}
	for name, c := range cases {
		for _, actor := range []string{agent.ActorSession, "builder"} {
			t.Run(name+" "+actor, func(t *testing.T) {
				e := c.event
				e.Actor = actor
				ev := hook.Evaluation{Workflow: base.Workflow, Present: true, Root: base.Root, GitDir: base.GitDir,
					System: "linux", Event: e}
				if err := ev.Valid(); err != nil {
					t.Fatal(err)
				}
				out, steps := walk(t, ev, facts{branch: "main",
					ignored: map[string]bool{".env": true}})
				if !out.Answer.Block || !slices.Equal(out.Answer.Reasons, []string{c.want}) {
					t.Fatalf("reasons %q, want %q", out.Answer.Reasons, c.want)
				}
				from, line := protectedFilesShortcut, 0
				if c.want == "regular" {
					from, line = workflow.VerbBlock, 8
				}
				if logged := appended(steps); len(logged) != 1 || logged[0].Action != from ||
					logged[0].Line != line {
					t.Fatalf("logged: %+v", logged)
				}
			})
		}
	}
}

// AC16: protected_files never applies to an edit outside the repository, and an invalid workflow applies none of its
// protections: it fails closed on its own.
func TestProtectedOutside(t *testing.T) {
	ev := eval(t, "version: 1\nguardrails:\n  protected_files: [\"**\"]\n", workflow.PreEdit)
	outside := with(t, ev, func(e *agent.Event) {
		e.Path, e.Outside, e.AbsPath = "notes.md", true, filepath.FromSlash("/tmp/scratch/notes.md")
	})
	out, steps := walk(t, outside, facts{branch: "main"})
	if out.Answer.Block || len(appended(steps)) != 0 {
		t.Fatalf("outside: %+v %+v", out.Answer, steps)
	}
	inside := edit(t, ev, workflow.PreEdit, "src/a.go", agent.ActorSession)
	if out, _ := walk(t, inside, facts{branch: "main"}); !out.Answer.Block {
		t.Fatalf("inside: %+v", out.Answer)
	}
	invalid := hook.Evaluation{Workflow: inside.Workflow, Present: true, Invalid: errors.New("polix.yaml: broken"),
		Root: inside.Root, GitDir: inside.GitDir, System: "linux", Event: inside.Event}
	if err := invalid.Valid(); err != nil {
		t.Fatal(err)
	}
	if out, _ := walk(t, invalid, facts{branch: "main"}); !out.Answer.Block ||
		!strings.Contains(out.Answer.Reasons[0], "the workflow is not valid") {
		t.Fatalf("invalid: %+v", out.Answer)
	}
}

// AC4, AC16: a forbidden command is denied, for the main session and a subagent, wherever it comes in the line and
// before an earlier entry of on that would block, with the default message or with its reason, followed by the line;
// the denial is logged as one of the shortcut at the line of its item. Any other line goes on to the entries.
func TestForbiddenCommands(t *testing.T) {
	ev := eval(t, `version: 1
guardrails:
  forbidden_commands:
    - pattern: "rm -rf /*"
    - pattern: "git push --force*"
      reason: "Ask a person to force a push."
on:
  pre_command:
    - action: { block: "regular" }
`, workflow.PreCommand)
	cases := map[string]struct {
		want string
		line int
	}{
		"cd tmp && rm -rf /":            {"This command is forbidden by polix.yaml: a person must run it.\ncd tmp && rm -rf /", 4},
		"rm -rf / ; ls":                 {"This command is forbidden by polix.yaml: a person must run it.\nrm -rf / ; ls", 4},
		"git fetch && git push --force": {"Ask a person to force a push.\ngit fetch && git push --force", 5},
		"rm -rf build":                  {"regular", 9},
	}
	for line, c := range cases {
		for _, actor := range []string{agent.ActorSession, "builder"} {
			e := with(t, ev, func(e *agent.Event) { e.Actor, e.Command = actor, line })
			out, steps := walk(t, e, facts{branch: "main"})
			if !out.Answer.Block || !slices.Equal(out.Answer.Reasons, []string{c.want}) {
				t.Errorf("%s by %s: reasons %q, want %q", line, actor, out.Answer.Reasons, c.want)
			}
			from := forbiddenCommandsShortcut
			if c.want == "regular" {
				from = workflow.VerbBlock
			}
			if logged := appended(steps); len(logged) != 1 || logged[0].Action != from ||
				logged[0].Line != c.line {
				t.Errorf("%s by %s: logged %+v", line, actor, logged)
			}
		}
	}
}

// AC6: a commit on a protected branch, also one matched through `*`, is denied; on any other branch it passes.
func TestProtectedBranches(t *testing.T) {
	ev := eval(t, "version: 1\nguardrails:\n  protected_branches: [main, \"release/*\"]\n",
		workflow.PreCommit)
	ev = with(t, ev, func(e *agent.Event) { e.Command = "git commit -m x" })
	for branch, block := range map[string]bool{"main": true, "release/1.2": true, "feature/x": false,
		"main-fix": false} {
		out, _ := walk(t, ev, facts{branch: branch})
		if out.Answer.Block != block {
			t.Errorf("%s: %+v", branch, out.Answer)
			continue
		}
		want := branch + " is protected in polix.yaml: create a branch for this change and commit there."
		if block && !slices.Equal(out.Answer.Reasons, []string{want}) {
			t.Errorf("%s: reasons %q", branch, out.Answer.Reasons)
		}
	}
}

// A commit in another repository (`git -C ../other commit`) is not part of the change: the pre_commit of this
// workflow does not apply to it. A commit in a folder of the repository, or a line that also commits here, does.
func TestCommitOutsideRepository(t *testing.T) {
	ev := eval(t, "version: 1\nguardrails:\n  protected_branches: [main]\n", workflow.PreCommit)
	// /other and /tmp/scratch are other repositories; /tmp/link is a link into this one; /tmp/wt is a linked worktree
	// of it and /tmp/plain a folder in no repository, neither foreign.
	// The folders come joined as written, uncleaned; ../other from /repo is /repo/../other.
	foreign := map[string]bool{filepath.FromSlash("/repo/../other"): true, filepath.FromSlash("/tmp/scratch"): true,
		filepath.FromSlash("/repo/vendor/x"): true}
	reals := map[string]string{filepath.FromSlash("/tmp/link"): filepath.FromSlash("/repo/sub")}
	for line, block := range map[string]bool{
		"git -C ../other commit -m x":                              false,
		"git -C /tmp/scratch add -A && git -C /tmp/scratch commit": false,
		"git -C internal commit -m x":                              true,
		"git -C vendor/x commit -m x":                              true, // a repository nested in this one
		"git -C /tmp/scratch commit && git commit -m x":            true,
		`bash -c "git -C /tmp/scratch commit -m x"`:                true,
		"git --git-dir=/repo/.git -C /tmp/scratch commit":          true,
		"cd internal && git -C .. commit -m x":                     true,
		"git -C /tmp/link commit -m x":                             true,
		"git -C /tmp/wt commit -m x":                               true,
		"git -C /tmp/plain commit -m x":                            true,
	} {
		e := with(t, ev, func(e *agent.Event) { e.Command = line })
		f := facts{branch: "main", reals: reals, foreign: foreign}
		if out, _ := walk(t, e, f); out.Answer.Block != block {
			t.Errorf("%s: block %v, want %v", line, out.Answer.Block, block)
		}
	}
	// The protections still apply to a commit in another repository.
	guarded := eval(t, "version: 1\nguardrails:\n  forbidden_commands:\n    - pattern: \"git*--no-verify*\"\n",
		workflow.PreCommit)
	guarded = with(t, guarded, func(e *agent.Event) { e.Command = "git -C /tmp/scratch commit --no-verify" })
	if out, _ := walk(t, guarded, facts{branch: "main", foreign: foreign}); !out.Answer.Block {
		t.Errorf("a forbidden command in another repository passed: %+v", out.Answer)
	}
}

// AC7: format_on_edit runs the command of each item whose path matches the edited path, or that has none, once per
// path and content, and a failure reaches the agent as context.
func TestFormatOnEdit(t *testing.T) {
	ev := eval(t, `version: 1
guardrails:
  format_on_edit:
    - run: "gofmt -w {path}"
      path: "*.go"
    - run: [lint, "{path}"]
      message: "Lint failed"
`, workflow.PostEdit)
	runs := func(steps []hook.Step) []string {
		var out []string
		for _, s := range steps {
			if s.Run != nil {
				out = append(out, strings.Join(s.Run, " "))
			}
		}
		return out
	}
	hashes := []string{"h", "h", "h"}
	goEdit := edit(t, ev, workflow.PostEdit, "src/a.go", agent.ActorSession)
	out, steps := walk(t, goEdit, facts{branch: "main", hashes: hashes,
		runErr: errors.New("exit status 1")})
	if got := runs(steps); !slices.Equal(got, []string{"gofmt -w src/a.go", "lint src/a.go"}) {
		t.Fatalf("runs %q", got)
	}
	if out.Answer.Block || len(out.Answer.Contexts) != 2 || !strings.HasPrefix(out.Answer.Contexts[1], "Lint failed") {
		t.Fatalf("a failure reaches the agent as context: %+v", out.Answer)
	}
	mdEdit := edit(t, ev, workflow.PostEdit, "docs/a.md", agent.ActorSession)
	_, steps = walk(t, mdEdit, facts{branch: "main", hashes: hashes})
	if got := runs(steps); !slices.Equal(got, []string{"lint docs/a.md"}) {
		t.Fatalf("runs %q", got)
	}
	// A run that passed on the content is not run again for the same path, and runs for another path.
	_, steps = walk(t, goEdit, facts{branch: "main", hashes: hashes})
	passed := logged(t, state.State{}, steps)
	if _, steps = walk(t, goEdit, facts{branch: "main", hashes: hashes, state: passed}); len(runs(steps)) != 0 {
		t.Fatalf("ran again on the same content: %q", runs(steps))
	}
	other := edit(t, ev, workflow.PostEdit, "src/b.go", agent.ActorSession)
	if _, steps = walk(t, other, facts{branch: "main", hashes: hashes, state: passed}); len(runs(steps)) != 2 {
		t.Fatalf("another path: %q", runs(steps))
	}
}

// AC8: run_on_stop blocks the end of the turn while a check fails, and runs a check only when its changed holds.
func TestRunOnStop(t *testing.T) {
	ev := eval(t, `version: 1
guardrails:
  run_on_stop:
    - run: "go test ./..."
      changed: "*.go"
      message: "Tests fail"
`, workflow.Stop)
	out, steps := walk(t, ev, facts{branch: "main", hashes: []string{"h"}, changed: []string{"a.go"},
		runErr: errors.New("exit status 1")})
	if !out.Answer.Block || len(out.Answer.Reasons) != 1 || !strings.HasPrefix(out.Answer.Reasons[0], "Tests fail") {
		t.Fatalf("a failing check blocks the stop: %+v", out.Answer)
	}
	if blocked := observed(steps); len(blocked) != 1 || blocked[0].Line != 4 {
		t.Fatalf("the block is of the line of the item: %+v", blocked)
	}
	out, steps = walk(t, ev, facts{branch: "main", hashes: []string{"h"}, changed: []string{"a.md"}})
	for _, s := range steps {
		if s.Run != nil {
			t.Fatalf("ran without a change of its changed: %q", s.Run)
		}
	}
	if out.Answer.Block {
		t.Fatalf("blocked: %+v", out.Answer)
	}
}

// AC16: protected_files denies the edit of a protected path before an earlier entry of on that would block, whether
// git tracks or ignores it; the entries of on, which govern the change, skip the edit of an ignored file.
func TestIgnoredEdits(t *testing.T) {
	regular := "on:\n  pre_edit:\n    - when: { path: \".env\" }\n      action: { block: \"regular\" }\n"
	protected := eval(t, "version: 1\nguardrails:\n  protected_files: [.env]\n"+regular, workflow.PreEdit)
	plain := eval(t, "version: 1\n"+regular, workflow.PreEdit)
	cases := []struct {
		name    string
		ev      hook.Evaluation
		ignored bool
		want    string // the reason, "" when the edit passes
	}{
		{"protected and tracked", protected, false, ".env is protected in polix.yaml: only a person changes it."},
		{"protected and ignored", protected, true, ".env is protected in polix.yaml: only a person changes it."},
		{"tracked", plain, false, "regular"},
		{"ignored", plain, true, ""},
	}
	for _, c := range cases {
		edit := edit(t, c.ev, workflow.PreEdit, ".env", agent.ActorSession)
		out, _ := walk(t, edit, facts{branch: "main", ignored: map[string]bool{".env": c.ignored}})
		if c.want == "" {
			if out.Answer.Block {
				t.Errorf("%s: %+v", c.name, out.Answer)
			}
			continue
		}
		if !out.Answer.Block || !slices.Equal(out.Answer.Reasons, []string{c.want}) {
			t.Errorf("%s: reasons %q, want %q", c.name, out.Answer.Reasons, c.want)
		}
	}
}

// AC12: the condition path ignores case only where the file names of the system ignore it.
func TestPathFoldsCase(t *testing.T) {
	ev := eval(t, "version: 1\non:\n  pre_edit:\n    - when: { path: \".env\" }\n"+
		"      action: { block: \"no\" }\n", workflow.PreEdit)
	for system, block := range map[string]bool{"linux": false, "darwin": true, "windows": true} {
		e := edit(t, ev, workflow.PreEdit, ".ENV", agent.ActorSession)
		e = hook.Evaluation{Workflow: e.Workflow, Present: true, Root: e.Root, GitDir: e.GitDir, System: system,
			Event: e.Event}
		if err := e.Valid(); err != nil {
			t.Fatal(err)
		}
		if out, _ := walk(t, e, facts{branch: "main"}); out.Answer.Block != block {
			t.Errorf("%s: %+v", system, out.Answer)
		}
	}
}

// Without polix.yaml only the protections apply; an invalid workflow blocks edits but never a stop.
func TestWithoutOrInvalidWorkflow(t *testing.T) {
	ev := eval(t, "version: 1\non:\n  stop:\n    - action: { block: \"x\" }\n", workflow.Stop)
	without := hook.Evaluation{Root: ev.Root, GitDir: ev.GitDir, Event: ev.Event}
	if err := without.Valid(); err != nil {
		t.Fatal(err)
	}
	if out, steps := walk(t, without, facts{}); out.Answer.Block || len(steps) != 0 {
		t.Fatalf("without workflow: %+v %+v", out.Answer, steps)
	}
	broken := hook.Evaluation{Present: true, Invalid: errors.New("polix.yaml: unknown key"), Root: ev.Root,
		GitDir: ev.GitDir, Event: ev.Event}
	if err := broken.Valid(); err != nil {
		t.Fatal(err)
	}
	out, _ := walk(t, broken, facts{})
	if out.Answer.Block || len(out.Answer.Warnings) != 1 {
		t.Fatalf("an invalid workflow never blocks a stop: %+v", out.Answer)
	}
	edit := hook.Evaluation{Present: true, Invalid: broken.Invalid, Root: ev.Root, GitDir: ev.GitDir, Event: agent.Event{
		Name: workflow.PreEdit, Agent: agent.Claude, Actor: agent.ActorSession, Path: "src/a.go",
		AbsPath: filepath.Join(ev.Root, "src", "a.go")}}
	if err := edit.Valid(); err != nil {
		t.Fatal(err)
	}
	out, _ = walk(t, edit, facts{branch: "main"})
	if !out.Answer.Block || !strings.Contains(out.Answer.Reasons[0], "the workflow is not valid") {
		t.Fatalf("an invalid workflow blocks an edit: %+v", out.Answer)
	}
}

// An inject of a file whose name expands to nothing reads no file and asks for nothing: the event is still
// answered, and the step that asks for nothing is never returned.
func TestEmptyRead(t *testing.T) {
	ev := eval(t, `version: 1
on:
  pre_command:
    - action: { inject: { file: "{path}" } }
    - action: { block: "no" }
`, workflow.PreCommand)
	out, steps := walk(t, ev, facts{branch: "main"})
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
func TestFileContains(t *testing.T) {
	ev := eval(t, `version: 1
on:
  pre_command:
    - unless: { file: "ai/plans/*.md", contains: "Ticket: {key}" }
      action: { block: "No hay plan" }
vars:
  key: { branch: "^([A-Z]+-[0-9]+)" }
`, workflow.PreCommand)
	for found, block := range map[bool]bool{true: false, false: true} {
		out, steps := walk(t, ev, facts{branch: "PLAT-7-x", found: found})
		var searches []hook.Step
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

// goDiff is a diff of Go files, tests and docs for the measures: 120 lines in 3 Go files, 300 in all.
var goDiff = diff.Diff{Changes: []diff.Change{
	{Path: "internal/hook/a.go", Added: 50, Deleted: 13},
	{Path: "internal/hook/a_test.go", Added: 40},
	{Path: "main.go", Added: 17},
	{Path: "docs/polix.md", Added: 180},
}}

// blocks walks a pre_commit with the workflow of yaml on the diff and reports whether it blocked.
func blocks(t *testing.T, yaml string, f facts) (Output, []hook.Step) {
	t.Helper()
	ev := eval(t, yaml, workflow.PreCommit)
	if f.branch == "" {
		f.branch = "main"
	}
	if f.diff.Changes == nil {
		f.diff = goDiff
	}
	if f.changed == nil {
		for _, c := range f.diff.Changes {
			f.changed = append(f.changed, c.Path)
		}
	}
	return walk(t, ev, f)
}

// Each measure holds at its number and not below, over the files of the changed next to it or the whole diff.
func TestMeasures(t *testing.T) {
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
		out, _ := blocks(t, "version: 1\non:\n  pre_commit:\n    - when: "+c.cond+"\n      action: { block: \"no\" }\n",
			facts{})
		if out.Answer.Block != c.block {
			t.Errorf("%s: block %v, want %v", c.cond, out.Answer.Block, c.block)
		}
	}
}

// risk_score takes the first matching weight of risk.weights.
func TestRiskWeights(t *testing.T) {
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
		out, _ := blocks(t, strings.Replace(yaml, "%d", strconv.Itoa(n), 1), facts{})
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
func TestAny(t *testing.T) {
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
		out, _ := blocks(t, "version: 1\non:\n  pre_commit:\n    - when: "+c.cond+"\n      action: { block: \"no\" }\n",
			facts{})
		if out.Answer.Block != c.block {
			t.Errorf("%s: block %v, want %v", c.cond, out.Answer.Block, c.block)
		}
	}
}

// A diff that cannot be measured makes the measures hold, with one warning for the call, and shows ? in the text.
func TestMeasuresFailClosed(t *testing.T) {
	yaml := "version: 1\non:\n  pre_commit:\n    - when: { changed_lines: 1000, changed_files: 1000 }\n" +
		"      action: { block: \"{changed_lines} {changed_files} {risk_score}\" }\n"
	out, _ := blocks(t, yaml, facts{diffErr: errors.New("git roto")})
	if !out.Answer.Block || out.Answer.Reasons[0] != "? ? ?" {
		t.Fatalf("answer: %+v", out.Answer)
	}
	want := "polix cannot measure the diff (git roto): changed_lines, changed_files and risk_score hold"
	if len(out.Answer.Warnings) != 1 || out.Answer.Warnings[0] != want {
		t.Fatalf("warnings: %q", out.Answer.Warnings)
	}
}

// A placeholder of a measure measures the changed of the when of the entry; the whole diff without one.
func TestMeasurePlaceholders(t *testing.T) {
	yaml := "version: 1\non:\n  pre_commit:\n    - action: { inject: \"{changed_lines}/{changed_files}\" }\n" +
		"    - when: { changed: \"*.go\" }\n      action: { block: \"{changed_lines}/{changed_files}\" }\n"
	out, _ := blocks(t, yaml, facts{})
	if !out.Answer.Block || out.Answer.Reasons[0] != "120/3" || !slices.Equal(out.Answer.Contexts, []string{"300/4"}) {
		t.Fatalf("answer: %+v", out.Answer)
	}
}

// An event whose entries use no measure never asks for the diff.
func TestNoDiffWithoutMeasures(t *testing.T) {
	yaml := "version: 1\non:\n  pre_commit:\n    - when: { changed: \"*.go\" }\n      action: { block: \"{branch}\" }\n"
	_, steps := blocks(t, yaml, facts{changed: []string{"a.go"}})
	for _, s := range steps {
		if s.Diff {
			t.Fatalf("steps %+v", steps)
		}
	}
}

// The conditions on the event come before those on git: a command that does not match spares the hash.
func TestCheapConditionsFirst(t *testing.T) {
	ev := eval(t, `version: 1
on:
  pre_command:
    - when: { finished: polix:reviewer, command: "git push*", changed: "**", branch: "^main$" }
      action: { block: "no" }
`, workflow.PreCommand)
	out, steps := walk(t, ev, facts{branch: "main"})
	if out.Answer.Block || len(steps) != 0 {
		t.Fatalf("answer %+v, steps %+v", out.Answer, steps)
	}
}

// with returns ev with its event changed by change, as the first event of a new call.
func with(t *testing.T, ev hook.Evaluation, change func(*agent.Event)) hook.Evaluation {
	t.Helper()
	e := ev.Event
	change(&e)
	out := hook.Evaluation{Workflow: ev.Workflow, Present: ev.Present, Root: ev.Root, GitDir: ev.GitDir, System: ev.System,
		Event: e}
	if err := out.Valid(); err != nil {
		t.Fatal(err)
	}
	return out
}

// edit returns ev as the edit name (pre_edit or post_edit) of path by actor.
func edit(t *testing.T, ev hook.Evaluation, name, path, actor string) hook.Evaluation {
	t.Helper()
	return with(t, ev, func(e *agent.Event) {
		e.Name, e.Actor, e.Path, e.AbsPath = name, actor, path, filepath.Join(ev.Root, filepath.FromSlash(path))
	})
}

// logged returns the log st with the entries the steps appended, as the next event reads it.
func logged(t *testing.T, st state.State, steps []hook.Step) state.State {
	t.Helper()
	for _, e := range appended(steps) {
		var err error
		if st, err = st.Append(e); err != nil {
			t.Fatal(err)
		}
	}
	return st
}

// decided returns the decisions the steps logged with outcome, in order.
func decided(steps []hook.Step, outcome string) []state.Entry {
	var out []state.Entry
	for _, e := range appended(steps) {
		if e.Kind == state.KindDecision && e.Outcome == outcome {
			out = append(out, e)
		}
	}
	return out
}

// hashed reports whether the steps asked for the content hash.
func hashed(steps []hook.Step) bool {
	return slices.ContainsFunc(steps, func(s hook.Step) bool { return s.Hash })
}

// AC14 and AC13: a run without always that passed is not run again on the same branch and content, and the skip is
// logged; an edit or another branch makes it run again, and a failing run runs again on the next event.
func TestRunRepeat(t *testing.T) {
	ev := eval(t, `version: 1
on:
  stop:
    - action: { run: just lint }
    - action: { run: { command: [just, test], message: "Arregla los tests" } }
`, workflow.Stop)
	f := facts{branch: "main", hashes: []string{"h", "h", "h"}}
	out, steps := walk(t, ev, f)
	passed := observed(steps)
	if runs := runs(steps); out.Answer.Block || len(runs) != 2 || len(passed) != 2 {
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
	f.state = logged(t, f.state, steps)
	out, steps = walk(t, ev, f)
	skips := decided(steps, "skip")
	if runs := runs(steps); out.Answer.Block || len(runs) != 0 || len(observed(steps)) != 0 ||
		len(skips) != 2 || skips[0].Action != workflow.VerbRun || skips[0].Line != 4 || skips[1].Line != 5 ||
		!strings.Contains(skips[0].Reason, "just lint") {
		t.Fatalf("second turn: %q %+v", runs, appended(steps))
	}
	// An edit, or another branch, makes them run again.
	for name, g := range map[string]facts{
		"edit":   {branch: "main", hashes: []string{"h2", "h2", "h2"}, state: f.state},
		"branch": {branch: "dev", hashes: []string{"h", "h", "h"}, state: f.state},
	} {
		if _, steps := walk(t, ev, g); len(runs(steps)) != 2 {
			t.Errorf("%s: %q", name, runs(steps))
		}
	}
	// A failing run writes no passed and runs again on the next event.
	fail := facts{branch: "main", hashes: []string{"h3", "h3"}, state: f.state,
		runErr: errors.New("exit status 1")}
	for i := range 2 {
		out, steps = walk(t, ev, fail)
		if runs := runs(steps); !out.Answer.Block || len(runs) != 1 {
			t.Fatalf("failing run %d: %q %+v", i, runs, out.Answer)
		}
		for _, o := range observed(steps) {
			if o.Outcome == state.OutcomePassed {
				t.Fatalf("a failing run passed: %+v", o)
			}
		}
		fail.state = logged(t, fail.state, steps)
	}
}

// AC14: the key of a run is what it runs, whoever's entry it is: a {path} repeats it per path, a run of the same
// command on another event is spared, and a run of the rules is keyed by its expanded target.
func TestRunRepeatKeys(t *testing.T) {
	base := eval(t, `version: 1
on:
  post_edit:
    - action: { run: [gofmt, -l, "{path}"] }
    - action: { run: just lint }
  stop:
    - action: { run: just lint }
`, workflow.PostEdit)
	f := facts{branch: "main", hashes: []string{"h", "h", "h"}}
	walkRuns := func(ev hook.Evaluation) [][]string {
		t.Helper()
		_, steps := walk(t, ev, f)
		f.state = logged(t, f.state, steps)
		return runs(steps)
	}
	if runs := walkRuns(edit(t, base, workflow.PostEdit, "a.go", agent.ActorSession)); len(runs) != 2 ||
		!slices.Equal(runs[0], []string{"gofmt", "-l", "a.go"}) {
		t.Fatalf("a.go: %q", runs)
	}
	if runs := walkRuns(edit(t, base, workflow.PostEdit, "b.go", agent.ActorSession)); len(runs) != 1 ||
		!slices.Equal(runs[0], []string{"gofmt", "-l", "b.go"}) {
		t.Fatalf("b.go: %q", runs)
	}
	if runs := walkRuns(edit(t, base, workflow.PostEdit, "a.go", agent.ActorSession)); len(runs) != 0 {
		t.Fatalf("a.go again: %q", runs)
	}
	if runs := walkRuns(with(t, base, func(e *agent.Event) { e.Name = workflow.Stop })); len(runs) != 0 {
		t.Fatalf("the run of another entry: %q", runs)
	}

	rulesEv := eval(t, "version: 1\non:\n  post_edit:\n    - action: { run: { rules: \"{path}\" } }\n",
		workflow.PostEdit)
	g := facts{branch: "main", hashes: []string{"h", "h"}, rules: []rule.Rule{
		{ID: "R-1", Description: "d", Scope: []string{"src/**"}, Deterministic: true, Check: "checks/r.sh"}}}
	checks := func(path string) int {
		t.Helper()
		_, steps := walk(t, edit(t, rulesEv, workflow.PostEdit, path, agent.ActorSession), g)
		g.state = logged(t, g.state, steps)
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
func TestRunAlways(t *testing.T) {
	always := eval(t, "version: 1\non:\n  stop:\n"+
		"    - action: { run: { command: [just, test], always: true } }\n", workflow.Stop)
	f := facts{branch: "main", hashes: []string{"h", "h"}}
	for i := range 2 {
		_, steps := walk(t, always, f)
		f.state = logged(t, f.state, steps)
		if runs := runs(steps); len(runs) != 1 {
			t.Fatalf("run %d: %q", i, runs)
		}
	}
	if got := f.state.Observed(state.Query{Outcome: state.OutcomePassed, Key: "just test", Hash: "h",
		ByHash: true}); len(got) != 2 {
		t.Fatalf("passed: %+v", got)
	}
	// The run rewrote files: the content it checked is gone.
	if _, steps := walk(t, always, facts{branch: "main", hashes: []string{"h", "h2"}}); len(
		appended(steps)) != 0 {
		t.Fatalf("changed during the run: %+v", appended(steps))
	}
	// Without a hash nothing is written, and a run without always runs every time.
	once := eval(t, "version: 1\non:\n  stop:\n    - action: { run: just test }\n", workflow.Stop)
	g := facts{branch: "main", hashErr: errors.New("permission denied")}
	for i := range 2 {
		_, steps := walk(t, once, g)
		if len(runs(steps)) != 1 || len(observed(steps)) != 0 {
			t.Fatalf("unknown hash %d: %+v", i, steps)
		}
		g.state = logged(t, g.state, steps)
	}
}

// AC8 and AC19: a block with once blocks the first attempt of an actor in a session on a content and lets the next
// through, logging the skip; an edit outside the scope of the entry keeps it, one inside it, a comparison that
// fails or an unknown hash repeat it; another actor and a new session are reminded on their own.
func TestBlockOnce(t *testing.T) {
	ev := eval(t, `version: 1
on:
  pre_commit:
    - when: { changed: "*.go" }
      action: { block: { message: "Actualiza la documentación", once: true } }
`, workflow.PreCommit)
	f := facts{branch: "main", hashes: []string{"h"}, changed: []string{"internal/a.go"}}
	out, steps := walk(t, ev, f)
	blocked := observed(steps)
	if !out.Answer.Block || len(blocked) != 1 || blocked[0].Outcome != state.OutcomeBlocked || blocked[0].Hash != "h" ||
		blocked[0].Line != 4 || blocked[0].Session != "s1" || blocked[0].Actor != agent.ActorSession ||
		blocked[0].Branch != "main" {
		t.Fatalf("first commit: %+v %+v", out.Answer, blocked)
	}
	f.state = logged(t, f.state, steps)
	out, steps = walk(t, ev, f)
	if skips := decided(steps, "skip"); out.Answer.Block || len(skips) != 1 ||
		skips[0].Action != workflow.VerbBlock || skips[0].Line != 4 || len(observed(steps)) != 0 {
		t.Fatalf("second commit: %+v %+v", out.Answer, appended(steps))
	}
	cases := []struct {
		name  string
		ev    hook.Evaluation
		f     facts
		block bool
	}{
		{"edit outside the scope", ev, facts{hashes: []string{"h2"}, trees: []string{"README.md", "docs/a.md"}},
			false},
		{"edit inside the scope", ev, facts{hashes: []string{"h2"}, trees: []string{"README.md", "cmd/b.go"}},
			true},
		{"comparison fails", ev, facts{hashes: []string{"h2"}, treeErr: errors.New("fatal: bad object h")}, true},
		{"unknown hash", ev, facts{hashErr: errors.New("permission denied")}, true},
		{"another branch", ev, facts{branch: "dev", hashes: []string{"h"}}, true},
		{"another actor", with(t, ev, func(e *agent.Event) { e.Actor = "acme:builder" }),
			facts{hashes: []string{"h"}}, true},
		{"a new session", with(t, ev, func(e *agent.Event) { e.Session = "s2" }),
			facts{hashes: []string{"h"}}, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if c.f.branch == "" {
				c.f.branch = "main"
			}
			c.f.state, c.f.changed = f.state, f.changed
			out, steps := walk(t, c.ev, c.f)
			if out.Answer.Block != c.block {
				t.Fatalf("block %v, want %v: %+v", out.Answer.Block, c.block, appended(steps))
			}
			if c.f.treeErr != nil {
				if warns := decided(steps, "warn"); len(warns) != 1 ||
					!strings.Contains(warns[0].Reason, "fatal: bad object h") {
					t.Fatalf("the failed comparison is logged: %+v", warns)
				}
			}
		})
	}
	// A plain block blocks every commit and observes nothing.
	plain := eval(t, "version: 1\non:\n  pre_commit:\n    - action: { block: \"No\" }\n", workflow.PreCommit)
	g := facts{branch: "main", hashes: []string{"h"}}
	for i := range 2 {
		out, steps := walk(t, plain, g)
		if !out.Answer.Block || len(observed(steps)) != 0 || hashed(steps) {
			t.Fatalf("plain block %d: %+v %+v", i, out.Answer, steps)
		}
		g.state = logged(t, g.state, steps)
	}
}

// AC5: a block with once that reminded the actor of a session on a branch does not block again on another branch on
// the same HEAD commit while the files in its scope did not change, and blocks on a branch on another commit.
func TestBlockOnceByHead(t *testing.T) {
	ev := eval(t, `version: 1
on:
  pre_commit:
    - when: { changed: "*.go" }
      action: { block: { message: "Actualiza la documentación", once: true } }
`, workflow.PreCommit)
	reminded := state.State{Entries: []state.Entry{{Kind: state.KindObserved, Outcome: state.OutcomeBlocked,
		Session: "s1", Event: workflow.PreCommit, Actor: agent.ActorSession, Line: 4, Branch: "main", Hash: "h",
		Head: "c1"}}}
	f := facts{branch: "feat/x", head: "c1", hashes: []string{"h2"}, state: reminded, changed: []string{"internal/a.go"},
		trees: []string{"ai/reports/r.md"}}
	out, steps := walk(t, ev, f)
	if skips := decided(steps, "skip"); out.Answer.Block || len(skips) != 1 || skips[0].Line != 4 {
		t.Fatalf("same head: %+v %+v", out.Answer, appended(steps))
	}
	f.head = "c2"
	out, steps = walk(t, ev, f)
	if got := observed(steps); !out.Answer.Block || len(got) != 1 || got[0].Outcome != state.OutcomeBlocked ||
		got[0].Head != "c2" {
		t.Fatalf("another head: %+v %+v", out.Answer, got)
	}
}

// AC6: the observed lines about a content that are read by branch, finished with a hash, passed and blocked, carry
// the HEAD commit; a finished without hash, an injected line and the decisions do not.
func TestWritesHead(t *testing.T) {
	yaml := `version: 1
on:
  pre_commit:
    - unless: { finished: polix:reviewer }
      action: { block: "Revisa" }
  pre_edit:
    - action: { inject: "Lee el plan" }
  subagent_stop:
    - when: { actor: polix:reviewer, branch: "^refuse$" }
      action: { block: "Termina la revisión" }
  stop:
    - action: { run: just test }
`
	stop := eval(t, yaml, workflow.SubagentStop)
	f := facts{branch: "main", head: "c1", hashes: []string{"h", "h"}}
	heads := func(steps []hook.Step) map[string]string {
		t.Helper()
		got := map[string]string{}
		for _, e := range appended(steps) {
			if e.Kind == state.KindDecision && e.Head != "" {
				t.Fatalf("a decision carries the head: %+v", e)
			}
			if e.Kind == state.KindObserved {
				got[e.Outcome+" "+e.Hash] = e.Head
			}
		}
		return got
	}
	cases := []struct {
		name string
		ev   hook.Evaluation
		f    facts
		want map[string]string
	}{
		{"finished with a hash", as(t, stop, "polix:reviewer"), f, map[string]string{"finished h": "c1"}},
		{"finished without hash", as(t, stop, "acme:builder"), f, map[string]string{"finished ": ""}},
		{"blocked", as(t, stop, "polix:reviewer"), facts{branch: "refuse", head: "c1", hashes: []string{"h"}},
			map[string]string{"blocked h": "c1"}},
		{"passed", eval(t, yaml, workflow.Stop), f, map[string]string{"passed h": "c1"}},
		{"injected", edit(t, eval(t, yaml, workflow.PreEdit), workflow.PreEdit, "src/a.go", agent.ActorSession), f,
			map[string]string{"injected ": ""}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, steps := walk(t, c.ev, c.f)
			if got := heads(steps); !reflect.DeepEqual(got, c.want) {
				t.Fatalf("heads %v, want %v: %+v", got, c.want, appended(steps))
			}
		})
	}
}

// AC20: the loop guard counts the observed blocked lines of the entry on the whole hash: an edit outside any scope
// starts a new count, and the blocks of an unknown hash are counted together, so a broken git never traps the agent.
func TestLoopGuardCountsBlocked(t *testing.T) {
	ev := eval(t, `version: 1
max_stop_retries: 2
on:
  stop:
    - when: { changed: "*.go" }
      action: { block: "Revisa" }
`, workflow.Stop)
	for name, f := range map[string]facts{
		"known hash":   {hashes: []string{"h"}},
		"unknown hash": {hashErr: errors.New("permission denied")},
	} {
		t.Run(name, func(t *testing.T) {
			f.branch, f.changed = "main", []string{"a.go"}
			for i, block := range []bool{true, true, false} {
				out, steps := walk(t, ev, f)
				if out.Answer.Block != block {
					t.Fatalf("stop %d: %+v", i, out.Answer)
				}
				f.state = logged(t, f.state, steps)
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
	f := facts{branch: "main", hashes: []string{"h"}, changed: []string{"a.go"}}
	for range 2 {
		_, steps := walk(t, ev, f)
		f.state = logged(t, f.state, steps)
	}
	f.hashes = []string{"h2"}
	if out, _ := walk(t, ev, f); !out.Answer.Block {
		t.Fatal("the blocks of another content count")
	}
}

// AC18, AC19 and AC13: a once-only inject reaches each actor once per session, writes injected without computing a
// hash and logs the skip; it is due again after a start of the session and, for a subagent, after that
// subagent type finished, which is written without a hash. A new session is reminded on its own.
func TestInjectOnce(t *testing.T) {
	base := eval(t, `version: 1
guidance: { commit: false, edit: false, layout: false }
on:
  pre_edit:
    - action: { inject: "Lee el plan antes de editar" }
`, workflow.PreEdit)
	const text = "Lee el plan antes de editar"
	var st state.State
	walkInject := func(ev hook.Evaluation) (Output, []hook.Step) {
		t.Helper()
		out, steps := walk(t, ev, facts{branch: "main", hashes: []string{"h"}, state: st})
		if hashed(steps) {
			t.Fatalf("an inject computes no hash: %+v", steps)
		}
		st = logged(t, st, steps)
		return out, steps
	}
	injects := func(ev hook.Evaluation) bool {
		t.Helper()
		out, _ := walkInject(ev)
		return slices.Equal(out.Answer.Contexts, []string{text})
	}
	session := edit(t, base, workflow.PreEdit, "src/a.go", agent.ActorSession)
	out, steps := walkInject(session)
	injected := observed(steps)
	if !slices.Equal(out.Answer.Contexts, []string{text}) || len(injected) != 1 ||
		injected[0].Outcome != state.OutcomeInjected || injected[0].Key != text || injected[0].Line != 5 ||
		injected[0].Hash != "" || injected[0].Session != "s1" || injected[0].Actor != agent.ActorSession {
		t.Fatalf("first edit: %+v %+v", out.Answer, injected)
	}
	out, steps = walkInject(session)
	if skips := decided(steps, "skip"); len(out.Answer.Contexts) != 0 || len(skips) != 1 ||
		skips[0].Action != workflow.VerbInject || skips[0].Line != 5 {
		t.Fatalf("second edit: %+v %+v", out.Answer, appended(steps))
	}
	// A subagent is reminded on its own, once, and again after a subagent of its type finished.
	builder := edit(t, base, workflow.PreEdit, "src/a.go", "acme:builder")
	if !injects(builder) || injects(builder) {
		t.Fatal("a subagent gets the inject once")
	}
	_, steps = walkInject(with(t, base, func(e *agent.Event) {
		e.Name, e.Actor = workflow.SubagentStop, "acme:builder"
	}))
	if got := observed(steps); len(got) != 1 || got[0].Outcome != state.OutcomeFinished ||
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
	_, steps = walkInject(with(t, base, func(e *agent.Event) { e.Name = workflow.Start }))
	if got := observed(steps); len(got) != 1 || got[0].Outcome != state.OutcomeStarted || got[0].Hash != "" {
		t.Fatalf("started: %+v", got)
	}
	if !injects(session) || !injects(builder) || injects(session) {
		t.Fatal("due again once after start")
	}
	// A new session is reminded on its own.
	if !injects(with(t, session, func(e *agent.Event) { e.Session = "s2" })) {
		t.Fatal("a new session")
	}
}

// AC4 and AC5: on subagent_start every inject acts on every call, without the guidance nor the requirements of the
// main session, and polix writes started with the subagent type as actor: a once-only inject of a later event is due
// again for that type only, while the main session and the other types keep theirs. An invalid workflow only warns.
func TestSubagentStart(t *testing.T) {
	const rules, plan = "Lee las reglas antes de revisar", "Lee el plan antes de editar"
	base := eval(t, `version: 1
requirements: [go]
on:
  subagent_start:
    - action: { inject: "`+rules+`" }
  pre_edit:
    - action: { inject: "`+plan+`" }
`, workflow.PreEdit)
	var st state.State
	walkLogged := func(ev hook.Evaluation) (Output, []hook.Step) {
		t.Helper()
		out, steps := walk(t, ev, facts{branch: "main", hashes: []string{"h"}, state: st, missing: []string{"go"}})
		st = logged(t, st, steps)
		return out, steps
	}
	spawn := func(actor string) hook.Evaluation {
		return with(t, base, func(e *agent.Event) { e.Name, e.Actor = workflow.SubagentStart, actor })
	}
	for range 2 {
		out, steps := walkLogged(spawn("polix:reviewer"))
		if out.Answer.Block || !slices.Equal(out.Answer.Contexts, []string{rules}) || len(lookups(steps)) != 0 ||
			hashed(steps) {
			t.Fatalf("every subagent_start injects, without guidance nor requirements: %+v %+v", out.Answer, steps)
		}
		if got := observed(steps); len(got) != 1 || got[0].Outcome != state.OutcomeStarted ||
			got[0].Event != workflow.SubagentStart || got[0].Actor != "polix:reviewer" || got[0].Hash != "" {
			t.Fatalf("started: %+v", got)
		}
	}
	injects := func(actor string) bool {
		t.Helper()
		out, _ := walkLogged(edit(t, base, workflow.PreEdit, "src/a.go", actor))
		return slices.Equal(out.Answer.Contexts, []string{plan})
	}
	for _, actor := range []string{agent.ActorSession, "polix:reviewer", "polix:builder"} {
		if !injects(actor) || injects(actor) {
			t.Fatalf("%s gets the inject once", actor)
		}
	}
	walkLogged(spawn("polix:reviewer"))
	if !injects("polix:reviewer") {
		t.Fatal("a new polix:reviewer gets the inject again")
	}
	if injects(agent.ActorSession) || injects("polix:builder") {
		t.Fatal("the start of a polix:reviewer does not remind the main session nor polix:builder")
	}
	broken := hook.Evaluation{Present: true, Invalid: errors.New("polix.yaml: unknown key"), Root: base.Root,
		GitDir: base.GitDir, Event: spawn("polix:reviewer").Event}
	if err := broken.Valid(); err != nil {
		t.Fatal(err)
	}
	if out, _ := walk(t, broken, facts{}); out.Answer.Block || len(out.Answer.Warnings) != 1 {
		t.Fatalf("an invalid workflow only warns on subagent_start: %+v", out.Answer)
	}
}

// AC18 and AC23: a once-only inject is keyed by the text it expands to: a {path} gives once per path, and a measure
// that changed injects it again. A file is keyed by its path, and with always every event injects.
func TestInjectKeys(t *testing.T) {
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
			base := eval(t, "version: 1\non:\n  pre_edit:\n    - action: { inject: "+c.inject+" }\n",
				workflow.PreEdit)
			var st state.State
			for i, path := range c.edits {
				f := facts{branch: "main", state: st}
				if c.diffs != nil {
					f.diff = diff.Diff{Changes: []diff.Change{{Path: "a.go", Added: c.diffs[i]}}}
				}
				out, steps := walk(t, edit(t, base, workflow.PreEdit, path, agent.ActorSession), f)
				if got := len(out.Answer.Contexts) == 1; got != c.want[i] {
					t.Fatalf("edit %d of %s: %+v", i, path, out.Answer)
				}
				if c.name == "always" && len(appended(steps)) != 0 {
					t.Fatalf("always writes nothing: %+v", appended(steps))
				}
				st = logged(t, st, steps)
			}
		})
	}
}

// AC18: inject: { rules } injects each rule once per actor and session; with nothing left to inject the skip is
// logged.
func TestInjectRulesOnce(t *testing.T) {
	base := eval(t, "version: 1\non:\n  pre_edit:\n    - action: { inject: { rules: \"{path}\" } }\n",
		workflow.PreEdit)
	f := facts{branch: "main", rules: []rule.Rule{
		{ID: "GO-1", Description: "uno", Scope: []string{"src/**"}},
		{ID: "GO-2", Description: "dos", Scope: []string{"src/b.go"}},
	}}
	walk := func(path, actor string) (Output, []hook.Step) {
		t.Helper()
		out, steps := walk(t, edit(t, base, workflow.PreEdit, path, actor), f)
		f.state = logged(t, f.state, steps)
		return out, steps
	}
	out, steps := walk("src/a.go", agent.ActorSession)
	if got := observed(steps); len(out.Answer.Contexts) != 1 ||
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
	if len(out.Answer.Contexts) != 0 || len(decided(steps, "skip")) != 1 {
		t.Fatalf("src/b.go again: %+v %+v", out.Answer, appended(steps))
	}
	if out, _ := walk("src/b.go", "acme:builder"); len(out.Answer.Contexts) != 1 ||
		!strings.Contains(out.Answer.Contexts[0], "GO-1") || !strings.Contains(out.Answer.Contexts[0], "GO-2") {
		t.Fatalf("another actor: %+v", out.Answer)
	}
}

// AC19: polix writes started and an unobserved finished only when the workflow injects once, and never computes a
// hash for them.
func TestObservesOnlyWhatTheWorkflowAsks(t *testing.T) {
	for name, entries := range map[string]string{
		"a block":          "  pre_commit:\n    - action: { block: x }\n",
		"an always inject": "  pre_edit:\n    - action: { inject: { text: x, always: true } }\n",
		"start only":       "  start:\n    - action: { inject: x }\n",
		"subagent_start":   "  subagent_start:\n    - action: { inject: x }\n",
	} {
		t.Run(name, func(t *testing.T) {
			yaml := "version: 1\non:\n" + entries
			start := eval(t, yaml, workflow.Start)
			spawn := as(t, eval(t, yaml, workflow.SubagentStart), "acme:builder")
			stop := as(t, eval(t, yaml, workflow.SubagentStop), "acme:builder")
			for _, ev := range []hook.Evaluation{start, spawn, stop} {
				_, steps := walk(t, ev, facts{branch: "main", hashes: []string{"h"}})
				if len(appended(steps)) != 0 || hashed(steps) {
					t.Fatalf("%s: %+v", ev.Event.Name, steps)
				}
			}
		})
	}
}

// A subagent type both observed by finished and reset by a once-only inject gets one finished line, with the hash.
func TestFinishedObservedAndReset(t *testing.T) {
	ev := as(t, eval(t, `version: 1
on:
  pre_edit:
    - action: { inject: "Lee el plan" }
  pre_commit:
    - unless: { finished: polix:reviewer }
      action: { block: "Revisa" }
`, workflow.SubagentStop), "polix:reviewer")
	_, steps := walk(t, ev, facts{branch: "main", hashes: []string{"h"}})
	if got := observed(steps); len(got) != 1 || got[0].Outcome != state.OutcomeFinished || got[0].Hash != "h" {
		t.Fatalf("finished: %+v", got)
	}
	// Without a hash, the line still makes the injects due again, but the condition never reads it.
	_, steps = walk(t, ev, facts{branch: "main", hashErr: errors.New("permission denied")})
	if got := observed(steps); len(got) != 1 || got[0].Hash != "" || len(decided(steps, "warn")) != 1 {
		t.Fatalf("finished without hash: %+v", appended(steps))
	}
}

// A log that cannot be read makes every verb act every time, with a warning: a run is never spared, an inject is
// always due and a block with once always blocks.
func TestRepeatWithoutLog(t *testing.T) {
	cases := []struct {
		name, yaml, event string
		acted             func(Output, []hook.Step) bool
	}{
		{"run", "version: 1\non:\n  stop:\n    - action: { run: just test }\n", workflow.Stop,
			func(_ Output, steps []hook.Step) bool { return len(runs(steps)) == 1 }},
		{"inject", "version: 1\non:\n  pre_command:\n    - action: { inject: x }\n", workflow.PreCommand,
			func(out Output, _ []hook.Step) bool { return len(out.Answer.Contexts) == 1 }},
		{"block once", "version: 1\non:\n  pre_commit:\n    - action: { block: { message: x, once: true } }\n",
			workflow.PreCommit, func(out Output, _ []hook.Step) bool { return out.Answer.Block }},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			ev := eval(t, c.yaml, c.event)
			for i := range 2 {
				out, steps := walk(t, ev, facts{branch: "main", hashes: []string{"h", "h"},
					stateErr: errors.New("permission denied")})
				if !c.acted(out, steps) || !slices.ContainsFunc(out.Answer.Warnings, func(w string) bool {
					return strings.Contains(w, "cannot use its log (permission denied)")
				}) {
					t.Fatalf("event %d: %+v", i, out.Answer)
				}
			}
		})
	}
}

// A subagent_stop the loop guard let through writes finished without hash when the workflow injects once: the next
// run of the subagent, with a new context, gets the injects again, and finished still never holds for it.
func TestLoopGuardMakesInjectsDue(t *testing.T) {
	yaml := `version: 1
max_stop_retries: 1
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
	f := facts{branch: "main", hashes: []string{"h"}, state: st}
	reviewer := as(t, eval(t, yaml, workflow.SubagentStop), "polix:reviewer")
	out, steps := walk(t, reviewer, f)
	got := observed(steps)
	if out.Answer.Block || len(got) != 1 || got[0].Outcome != state.OutcomeFinished || got[0].Key != "polix:reviewer" ||
		got[0].Hash != "" {
		t.Fatalf("guarded stop: %+v %+v", out.Answer, got)
	}
	f.state = logged(t, f.state, steps)
	edit := edit(t, eval(t, yaml, workflow.PreEdit), workflow.PreEdit, "src/a.go", "polix:reviewer")
	if out, _ := walk(t, edit, f); !slices.Equal(out.Answer.Contexts, []string{"Lee el plan"}) {
		t.Fatalf("the next run of the subagent gets the inject again: %+v", out.Answer)
	}
	if out, _ := walk(t, eval(t, yaml, workflow.PreCommit), f); !out.Answer.Block {
		t.Fatal("finished holds after a stop the loop guard let through")
	}
}

// A once-only inject is observed only when it reaches the AI: on prompt a later block rejects the prompt, so it is
// not written and the next prompt injects it; on the other events it reaches the AI with the block, and it is.
func TestInjectBeforeBlock(t *testing.T) {
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
	prompt := eval(t, yaml, workflow.Prompt)
	out, steps := walk(t, prompt, facts{branch: "main", rules: rules})
	if !out.Answer.Block || len(observed(steps)) != 0 {
		t.Fatalf("a refused prompt: %+v %+v", out.Answer, observed(steps))
	}
	st := logged(t, state.State{}, steps)
	out, steps = walk(t, prompt, facts{branch: "dev", rules: rules, state: st})
	got := observed(steps)
	if out.Answer.Block || len(out.Answer.Contexts) != 2 || len(got) != 2 || got[0].Key != "Lee el plan" ||
		got[0].Line != 4 || got[1].Key != "GO-1" || got[1].Line != 5 {
		t.Fatalf("the next prompt: %+v %+v", out.Answer, got)
	}
	out, steps = walk(t, eval(t, yaml, workflow.PreCommand), facts{branch: "main"})
	got = observed(steps)
	if !out.Answer.Block || !slices.Equal(out.Answer.Contexts, []string{"Cuidado"}) || len(got) != 1 ||
		got[0].Outcome != state.OutcomeInjected || got[0].Key != "Cuidado" {
		t.Fatalf("a refused command: %+v %+v", out.Answer, got)
	}
}

// lookups returns the programs of the Programs steps the steps asked for, in order.
func lookups(steps []hook.Step) [][]string {
	var out [][]string
	for _, s := range steps {
		if s.Programs != nil {
			out = append(out, s.Programs)
		}
	}
	return out
}

// AC3, AC4, AC5 and AC6: a prompt with a program of requirements not on the PATH tells the AI once per session,
// without blocking, after one lookup; it tells it again after a start of the session or when the missing programs
// change, and a refused prompt does not record it. A start never looks the programs up nor tells them; without
// requirements nothing is looked up, and with every program found nothing is told.
func TestRequirementsPrompt(t *testing.T) {
	yaml := `version: 1
requirements: [go, does-not-exist]
guidance: { commit: false, edit: false, layout: false }
on:
  prompt:
    - when: { branch: "^refuse$" }
      action: { block: "No" }
`
	const text = "These programs that polix.yaml requires are not on the PATH: does-not-exist. Before doing anything " +
		"else, install them, or ask the user to install them if you cannot."
	prompt := eval(t, yaml, workflow.Prompt)
	var st state.State
	step := func(ev hook.Evaluation, branch string, missing []string) (Output, []hook.Step) {
		t.Helper()
		out, steps := walk(t, ev, facts{branch: branch, state: st, missing: missing})
		st = logged(t, st, steps)
		return out, steps
	}
	absent := []string{"does-not-exist"}
	out, steps := step(prompt, "main", absent)
	got := observed(steps)
	if out.Answer.Block || !slices.Equal(out.Answer.Contexts, []string{text}) ||
		!reflect.DeepEqual(lookups(steps), [][]string{{"go", "does-not-exist", "git"}}) || len(got) != 1 ||
		got[0].Outcome != state.OutcomeInjected || got[0].Line != 0 || got[0].Key != "requirements: does-not-exist" {
		t.Fatalf("first prompt: %+v %+v %+v", out.Answer, lookups(steps), got)
	}
	out, steps = step(prompt, "main", absent)
	if skips := decided(steps, "skip"); len(out.Answer.Contexts) != 0 || len(observed(steps)) != 0 ||
		len(skips) != 1 || skips[0].Action != "requirements" {
		t.Fatalf("second prompt: %+v %+v", out.Answer, appended(steps))
	}
	// Another set of missing programs is told again.
	if out, _ = step(prompt, "main", []string{"go", "does-not-exist"}); len(out.Answer.Contexts) != 1 ||
		!strings.Contains(out.Answer.Contexts[0], "not on the PATH: go, does-not-exist.") {
		t.Fatalf("another set: %+v", out.Answer)
	}
	// A start never looks the programs up nor tells them, and makes the context due again.
	out, steps = step(with(t, prompt, func(e *agent.Event) { e.Name = workflow.Start }), "main", absent)
	if len(out.Answer.Contexts) != 0 || len(lookups(steps)) != 0 {
		t.Fatalf("start: %+v %+v", out.Answer, lookups(steps))
	}
	if out, _ = step(prompt, "main", absent); !slices.Equal(out.Answer.Contexts, []string{text}) {
		t.Fatalf("a prompt after a start: %+v", out.Answer)
	}
	// A refused prompt never reaches the AI: it is not recorded, and the next prompt tells it.
	st = state.State{}
	out, steps = step(prompt, "refuse", absent)
	if !out.Answer.Block || len(observed(steps)) != 0 {
		t.Fatalf("a refused prompt: %+v %+v", out.Answer, observed(steps))
	}
	if out, _ = step(prompt, "main", absent); !slices.Equal(out.Answer.Contexts, []string{text}) {
		t.Fatalf("the prompt after a refused one: %+v", out.Answer)
	}
	// Every program found tells nothing.
	st = state.State{}
	if out, steps = step(prompt, "main", nil); len(out.Answer.Contexts) != 0 || len(observed(steps)) != 0 ||
		len(lookups(steps)) != 1 {
		t.Fatalf("every program found: %+v %+v", out.Answer, appended(steps))
	}
	// Without requirements nothing is looked up.
	without := eval(t, "version: 1\non:\n  prompt:\n    - action: { inject: \"Hola\" }\n", workflow.Prompt)
	if _, steps = walk(t, without, facts{branch: "main"}); len(lookups(steps)) != 0 {
		t.Fatalf("without requirements: %+v", lookups(steps))
	}
}

// hashes returns the epochs of the content hashes the steps asked for, in order.
func hashes(steps []hook.Step) []int {
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
func TestHashOnlyWhenNeeded(t *testing.T) {
	gate := eval(t, "version: 1\non:\n  pre_commit:\n    - unless: { finished: polix:reviewer }\n"+
		"      action: { block: \"Revisa\" }\n", workflow.PreCommit)
	unhashed := state.State{Entries: []state.Entry{{Kind: state.KindObserved, Outcome: state.OutcomeFinished,
		Key: "polix:reviewer", Branch: "main"}}}
	for name, st := range map[string]state.State{"empty log": {}, "finished without hash": unhashed,
		"another branch": finishedOn("dev", "h"), "another type": {Entries: []state.Entry{{
			Kind: state.KindObserved, Outcome: state.OutcomeFinished, Key: "acme:docs", Branch: "main", Hash: "h"}}}} {
		out, steps := walk(t, gate, facts{branch: "main", hashes: []string{"h"}, state: st})
		if !out.Answer.Block || len(hashes(steps)) != 0 {
			t.Errorf("%s: %+v %v", name, out.Answer, hashes(steps))
		}
	}
	out, steps := walk(t, gate, facts{branch: "main", hashes: []string{"h"},
		stateErr: errors.New("permission denied")})
	if !out.Answer.Block || len(hashes(steps)) != 0 || !slices.ContainsFunc(out.Answer.Warnings,
		func(w string) bool { return strings.Contains(w, "cannot use its log (permission denied)") }) {
		t.Fatalf("a log that cannot be read: %+v", out.Answer)
	}
	run := eval(t, "version: 1\non:\n  stop:\n    - action: { run: just test }\n", workflow.Stop)
	_, steps = walk(t, run, facts{branch: "main", hashErr: errors.New("permission denied")})
	if got := hashes(steps); len(runs(steps)) != 1 || !slices.Equal(got, []int{0}) ||
		len(observed(steps)) != 0 {
		t.Fatalf("an unknown hash before the run: %v", got)
	}
}

// finishedOn is the log after the subagent polix:reviewer finished on branch with the content hash.
func finishedOn(branch, hash string) state.State {
	st := finished(hash)
	st.Entries[0].Branch = branch
	return st
}

// On a detached HEAD the branch is empty: what was observed on a branch never counts there, so a run that passed
// runs again, a block with once blocks again and finished does not hold.
func TestDetachedHead(t *testing.T) {
	onMain := state.State{Entries: []state.Entry{
		{Kind: state.KindObserved, Outcome: state.OutcomePassed, Branch: "main", Key: "just test", Line: 4, Hash: "h"},
		{Kind: state.KindObserved, Outcome: state.OutcomeBlocked, Branch: "main", Session: "s1",
			Actor: agent.ActorSession, Line: 4, Hash: "h"},
		finished("h").Entries[0],
	}}
	cases := map[string]struct {
		yaml, event string
		acted       func(Output, []hook.Step) bool
	}{
		"run": {"version: 1\non:\n  stop:\n    - action: { run: just test }\n", workflow.Stop,
			func(_ Output, steps []hook.Step) bool { return len(runs(steps)) == 1 }},
		"block once": {"version: 1\non:\n  pre_commit:\n    - action: { block: { message: x, once: true } }\n",
			workflow.PreCommit, func(out Output, _ []hook.Step) bool { return out.Answer.Block }},
		"finished": {"version: 1\non:\n  pre_commit:\n    - unless: { finished: polix:reviewer }\n" +
			"      action: { block: x }\n", workflow.PreCommit,
			func(out Output, _ []hook.Step) bool { return out.Answer.Block }},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			ev := eval(t, c.yaml, c.event)
			for _, branch := range []string{"", "main"} {
				out, steps := walk(t, ev, facts{branch: branch, hashes: []string{"h", "h"},
					state: onMain})
				if acted := c.acted(out, steps); acted != (branch == "") {
					t.Fatalf("branch %q: acted %v: %+v", branch, acted, out.Answer)
				}
			}
		})
	}
}

// Decide routes each event of the workflow to its family: session for start, prompt and subagent_start, edit for
// pre_edit and post_edit, command for pre_command, post_command and pre_commit, and stop for stop and subagent_stop;
// an event polix does not evaluate reaches none.
func TestRoutesByFamily(t *testing.T) {
	families := map[string]func(*pass) agent.Answer{
		workflow.Start: (*pass).session, workflow.Prompt: (*pass).session, workflow.SubagentStart: (*pass).session,
		workflow.PreEdit: (*pass).edit, workflow.PostEdit: (*pass).edit,
		workflow.PreCommand: (*pass).command, workflow.PostCommand: (*pass).command, workflow.PreCommit: (*pass).command,
		workflow.Stop: (*pass).stop, workflow.SubagentStop: (*pass).stop,
	}
	if len(families) != len(workflow.Events) {
		t.Fatalf("%d families for %d events", len(families), len(workflow.Events))
	}
	name := func(f func(*pass) agent.Answer) string {
		if f == nil {
			return "none"
		}
		return runtime.FuncForPC(reflect.ValueOf(f).Pointer()).Name()
	}
	for _, event := range workflow.Events {
		want, ok := families[event]
		if got := route(event); !ok || name(got) != name(want) {
			t.Errorf("%s reaches %s, want %s", event, name(got), name(want))
		}
	}
	if got := route("turn_stop"); got != nil {
		t.Errorf("an unknown event reaches %s", name(got))
	}
}
