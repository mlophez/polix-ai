package hook

import (
	"errors"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/mlophez/polix-ai/internal/agent"
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
	rules     []rule.Rule
	found     bool // whether a file holds the text of a search
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
		case s.Changed:
			ev = ev.KnowChanged(f.changed)
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

// A run that rewrites files changes the content: a later done is decided on the hash of the new content.
func TestAnswerHookDoneAfterRunUsesNewContent(t *testing.T) {
	ev := answerHookEval(t, `version: 1
on:
  stop:
    - run: { command: [gofmt, -w, .] }
    - unless: { done: formatted }
      require: "Falta formatear"
`, workflow.Stop)
	formatted := state.State{Entries: []state.Entry{{Kind: state.KindFact, Fact: "formatted", Branch: "main", Hash: "new"}}}
	out, steps := answerHookWalk(t, ev, answerHookFacts{branch: "main", hashes: []string{"old", "new"}, state: formatted})
	if out.Answer.Block {
		t.Fatalf("done was decided on the old content: %+v", out.Answer)
	}
	for _, s := range steps {
		if s.Hash && s.Epoch != 1 {
			t.Fatalf("the hash was asked at epoch %d, before the run", s.Epoch)
		}
	}
	if out.Facts.Epoch != 1 {
		t.Fatalf("the next event starts after the run: epoch %d", out.Facts.Epoch)
	}
	// The same content before and after: the fact of the old content does not hold.
	out, _ = answerHookWalk(t, ev, answerHookFacts{branch: "main", hashes: []string{"new", "old"}, state: formatted})
	if !out.Answer.Block {
		t.Fatal("done held on a content it was not recorded on")
	}
}

// A record makes a later done of the same event hold: the log read before it gets the fact appended.
func TestAnswerHookRecordThenDone(t *testing.T) {
	ev := answerHookEval(t, `version: 1
on:
  pre_commit:
    - unless: { done: other }
      context: { text: "x" }
    - record: tested
    - unless: { done: tested }
      deny: "Faltan los tests"
`, workflow.PreCommit)
	out, steps := answerHookWalk(t, ev, answerHookFacts{branch: "main", hashes: []string{"h"}})
	if out.Answer.Block {
		t.Fatalf("the recorded fact does not hold: %+v", out.Answer)
	}
	facts := answerHookAppended(steps)
	if len(facts) != 1 || facts[0].Kind != state.KindFact || facts[0].Fact != "tested" || facts[0].Hash != "h" {
		t.Fatalf("appended: %+v", facts)
	}
	if !out.Facts.State.Done("tested", "main", "h") {
		t.Fatal("the next event of the call does not see the fact")
	}
}

// allow ends the evaluation of the event, and is logged.
func TestAnswerHookAllowEnds(t *testing.T) {
	ev := answerHookEval(t, `version: 1
on:
  pre_command:
    - allow: true
    - deny: "nunca"
`, workflow.PreCommand)
	out, steps := answerHookWalk(t, ev, answerHookFacts{branch: "main"})
	if out.Answer.Block {
		t.Fatalf("allow did not end the evaluation: %+v", out.Answer)
	}
	logged := answerHookAppended(steps)
	if len(logged) != 1 || logged[0].Outcome != "allow" || logged[0].Line != 4 {
		t.Fatalf("logged: %+v", logged)
	}
}

// A deny that blocks ends the evaluation; a deny in warn mode does not, and gives a context and a warning.
func TestAnswerHookDenyEndsUnlessWarn(t *testing.T) {
	ev := answerHookEval(t, `version: 1
on:
  pre_command:
    - mode: warn
      deny: "cuidado"
    - deny: "primero"
    - deny: "segundo"
`, workflow.PreCommand)
	out, steps := answerHookWalk(t, ev, answerHookFacts{branch: "main"})
	want := agent.Answer{Block: true, Reasons: []string{"primero"}, Contexts: []string{"Aviso de polix: cuidado"},
		Warnings: []string{"cuidado"}}
	if !slices.Equal(out.Answer.Reasons, want.Reasons) || !slices.Equal(out.Answer.Contexts, want.Contexts) ||
		!slices.Equal(out.Answer.Warnings, want.Warnings) || !out.Answer.Block {
		t.Fatalf("answer %+v, want %+v", out.Answer, want)
	}
	var outcomes []string
	for _, e := range answerHookAppended(steps) {
		outcomes = append(outcomes, e.Outcome)
	}
	if !slices.Equal(outcomes, []string{"warn", "block"}) {
		t.Fatalf("logged outcomes %q", outcomes)
	}
}

// The loop guard lets the agent stop after max_stop_blocks blocks of the entry on the same content.
func TestAnswerHookLoopGuard(t *testing.T) {
	ev := answerHookEval(t, "version: 1\nmax_stop_blocks: 2\non:\n  stop:\n    - require: \"revisa\"\n", workflow.Stop)
	block := state.Entry{Kind: state.KindStopBlock, Session: "s1", Event: workflow.Stop, Actor: agent.ActorSession,
		Line: 5, Hash: "h"}
	one := state.State{Entries: []state.Entry{block}}
	out, steps := answerHookWalk(t, ev, answerHookFacts{branch: "main", hashes: []string{"h"}, state: one})
	if !out.Answer.Block {
		t.Fatal("one block of two: the stop is blocked")
	}
	if logged := answerHookAppended(steps); len(logged) != 2 || logged[0].Kind != state.KindStopBlock ||
		logged[0].Hash != "h" || logged[1].Outcome != "block" {
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
	ev := answerHookEval(t, "version: 1\non:\n  stop:\n    - require: \"revisa\"\n", workflow.Stop)
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

// A run with record records only when it passes on stable content, and a failing run blocks with its output.
func TestAnswerHookRunRecord(t *testing.T) {
	ev := answerHookEval(t, "version: 1\non:\n  stop:\n    - run: { command: [just, test] }\n      record: tested\n",
		workflow.Stop)
	out, steps := answerHookWalk(t, ev, answerHookFacts{branch: "main", hashes: []string{"h", "h"}})
	if out.Answer.Block || len(answerHookAppended(steps)) != 1 || answerHookAppended(steps)[0].Fact != "tested" {
		t.Fatalf("a passing run on stable content records: %+v %+v", out.Answer, answerHookAppended(steps))
	}
	out, steps = answerHookWalk(t, ev, answerHookFacts{branch: "main", hashes: []string{"h", "h2"}})
	if len(out.Answer.Warnings) != 1 || !strings.Contains(out.Answer.Warnings[0], "el contenido cambió durante el run") ||
		answerHookAppended(steps)[0].Kind != state.KindDecision {
		t.Fatalf("a run that changed the content records nothing: %+v", out.Answer)
	}
	out, _ = answerHookWalk(t, ev, answerHookFacts{branch: "main", hashes: []string{"h", "h"},
		runErr: errors.New("exit status 1")})
	if !out.Answer.Block || !strings.HasPrefix(out.Answer.Reasons[0], "Falla just test: exit status 1\n  salida") {
		t.Fatalf("a failing run blocks with its output: %+v", out.Answer)
	}
}

// answerHookRecorded returns the facts the steps appended to the log, in order.
func answerHookRecorded(steps []Step) []string {
	var out []string
	for _, e := range answerHookAppended(steps) {
		if e.Kind == state.KindFact {
			out = append(out, e.Fact)
		}
	}
	return out
}

// A require with record blocks once and records that it asked, so the next stop on the same content passes.
func TestAnswerHookRequireRecordsOnce(t *testing.T) {
	ev := answerHookEval(t, `version: 1
on:
  stop:
    - if: { not: { done: asked } }
      require: "revisa"
      record: asked
`, workflow.Stop)
	out, steps := answerHookWalk(t, ev, answerHookFacts{branch: "main", hashes: []string{"h"}})
	if !out.Answer.Block || !slices.Equal(answerHookRecorded(steps), []string{"asked"}) {
		t.Fatalf("the first stop blocks and records: %+v %v", out.Answer, answerHookRecorded(steps))
	}
	asked := state.State{Entries: []state.Entry{{Kind: state.KindFact, Fact: "asked", Branch: "main", Hash: "h"}}}
	out, steps = answerHookWalk(t, ev, answerHookFacts{branch: "main", hashes: []string{"h"}, state: asked})
	if out.Answer.Block || len(answerHookRecorded(steps)) != 0 {
		t.Fatalf("the next stop on the same content passes: %+v", out.Answer)
	}
}

// Every action records the fact of its entry once it applied: a deny in enforce records and stops the evaluation,
// in warn records and goes on; allow and context record too.
func TestAnswerHookEveryActionRecords(t *testing.T) {
	cases := []struct {
		name, yaml, event string
		block             bool
		want              []string
	}{
		{"deny enforce", "    - deny: no\n      record: denied\n    - record: after\n", workflow.PreCommit, true,
			[]string{"denied"}},
		{"deny warn", "    - deny: no\n      mode: warn\n      record: denied\n    - record: after\n", workflow.PreCommit,
			false, []string{"denied", "after"}},
		{"allow", "    - allow: true\n      record: allowed\n    - record: after\n", workflow.PreCommit, false,
			[]string{"allowed"}},
		{"context", "    - context: { text: x }\n      record: told\n", workflow.PreCommit, false, []string{"told"}},
	}
	for _, c := range cases {
		ev := answerHookEval(t, "version: 1\non:\n  "+c.event+":\n"+c.yaml, c.event)
		out, steps := answerHookWalk(t, ev, answerHookFacts{branch: "main", hashes: []string{"h"}})
		if out.Answer.Block != c.block || !slices.Equal(answerHookRecorded(steps), c.want) {
			t.Errorf("%s: block %v, recorded %v", c.name, out.Answer.Block, answerHookRecorded(steps))
		}
	}
}

// An entry in audit mode records nothing: not a require, not a passing run, not a lone record.
func TestAnswerHookAuditRecordsNothing(t *testing.T) {
	ev := answerHookEval(t, `version: 1
on:
  stop:
    - require: "revisa"
      mode: audit
      record: asked
    - run: { command: [just, test] }
      mode: audit
      record: tested
    - record: alone
      mode: audit
`, workflow.Stop)
	out, steps := answerHookWalk(t, ev, answerHookFacts{branch: "main", hashes: []string{"h", "h"}})
	if out.Answer.Block || len(answerHookRecorded(steps)) != 0 {
		t.Fatalf("audit records nothing: %+v %v", out.Answer, answerHookRecorded(steps))
	}
}

// A require let through by the loop guard applied, with a warning: it records its fact.
func TestAnswerHookLoopGuardRecords(t *testing.T) {
	ev := answerHookEval(t, "version: 1\nmax_stop_blocks: 1\non:\n  stop:\n    - require: \"revisa\"\n      record: asked\n",
		workflow.Stop)
	block := state.Entry{Kind: state.KindStopBlock, Session: "s1", Event: workflow.Stop, Actor: agent.ActorSession,
		Line: 5, Hash: "h"}
	out, steps := answerHookWalk(t, ev, answerHookFacts{branch: "main", hashes: []string{"h"},
		state: state.State{Entries: []state.Entry{block}}})
	if out.Answer.Block || !slices.Equal(answerHookRecorded(steps), []string{"asked"}) {
		t.Fatalf("the loop guard lets the stop through and records: %+v %v", out.Answer, answerHookRecorded(steps))
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
    - run: { rules: changed }
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
    - run: { rules: changed }
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

// The session context of the guidance turned on is injected on session_start only, as one context before those of
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
		{"defaults", "version: 1\n", workflow.SessionStart, defaults},
		{"only edit", "version: 1\nguidance: { commit: false, layout: false }\n", workflow.SessionStart,
			session("version: 1\nguidance: { commit: false, layout: false }\n")},
		{"all off", "version: 1\nguidance: { commit: false, edit: false, layout: false }\n", workflow.SessionStart, nil},
		{"before the entries", "version: 1\non:\n  session_start:\n    - context: { text: repo }\n",
			workflow.SessionStart, append(slices.Clone(defaults), "repo")},
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
	start := answerHookEval(t, "version: 1\n", workflow.SessionStart)
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
	ev := answerHookEval(t, "version: 1\non:\n  stop:\n    - require: \"x\"\n", workflow.Stop)
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

// A hash that cannot be computed is warned once, and nothing is recorded.
func TestAnswerHookUnknownHash(t *testing.T) {
	ev := answerHookEval(t, "version: 1\non:\n  post_command:\n    - record: tested\n", workflow.PostCommand)
	out, steps := answerHookWalk(t, ev, answerHookFacts{branch: "main", hashErr: errors.New("permission denied")})
	if len(out.Answer.Warnings) != 1 || !strings.Contains(out.Answer.Warnings[0], "hash del contenido (permission denied)") {
		t.Fatalf("answer: %+v", out.Answer)
	}
	if logged := answerHookAppended(steps); len(logged) != 1 || logged[0].Kind != state.KindDecision ||
		logged[0].Outcome != "warn" {
		t.Fatalf("logged: %+v", logged)
	}
}

// A context of a file whose name expands to nothing reads no file and asks for nothing: the event is still
// answered, and the step that asks for nothing is never returned.
func TestAnswerHookEmptyRead(t *testing.T) {
	ev := answerHookEval(t, `version: 1
on:
  pre_command:
    - context: { file: "{path}" }
    - deny: "no"
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
      deny: "No hay plan"
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

// The conditions on the event come before those on git: a command that does not match spares the hash.
func TestAnswerHookCheapConditionsFirst(t *testing.T) {
	ev := answerHookEval(t, `version: 1
on:
  pre_command:
    - if: { done: tested, command: "git push*", changed: "**", branch: "^main$" }
      deny: "no"
`, workflow.PreCommand)
	out, steps := answerHookWalk(t, ev, answerHookFacts{branch: "main"})
	if out.Answer.Block || len(steps) != 0 {
		t.Fatalf("answer %+v, steps %+v", out.Answer, steps)
	}
}
