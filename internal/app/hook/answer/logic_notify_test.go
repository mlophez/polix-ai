package answer

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/mlophez/polix-ai/internal/app/agent"
	"github.com/mlophez/polix-ai/internal/app/hook"
	"github.com/mlophez/polix-ai/internal/app/state"
	"github.com/mlophez/polix-ai/internal/app/workflow"
)

// notifiedLines returns the observed notified lines the steps appended to the log, in order.
func notifiedLines(steps []hook.Step) []state.Entry {
	var out []state.Entry
	for _, e := range observed(steps) {
		if e.Outcome == state.OutcomeNotified {
			out = append(out, e)
		}
	}
	return out
}

// AC9: a notice reaches the user, never the agent, once per session and text: the first prompt shows it, logs a
// notify decision and writes it as notified, without hash; a later prompt, a subagent and a start (a new context) of
// the same session do not show it again, each skip logged, while a new session shows it.
func TestNotifyOnce(t *testing.T) {
	const text = "Recuerda abrir el ticket"
	base := eval(t, `version: 1
on:
  prompt:
    - action: { notify: "`+text+`" }
  pre_edit:
    - action: { notify: "`+text+`" }
    - action: { inject: "Lee el plan" }
`, workflow.Prompt)
	var st state.State
	walkLogged := func(ev hook.Evaluation) (Output, []hook.Step) {
		t.Helper()
		out, steps := walk(t, ev, facts{branch: "main", hashes: []string{"h"}, state: st})
		st = logged(t, st, steps)
		return out, steps
	}
	out, steps := walkLogged(base)
	notified := notifiedLines(steps)
	if !slices.Equal(out.Answer.Warnings, []string{text}) || len(out.Answer.Contexts) != 0 || out.Answer.Block ||
		len(notified) != 1 || notified[0].Line != 4 || notified[0].Key != text || notified[0].Hash != "" ||
		notified[0].Session != "s1" {
		t.Fatalf("first prompt: %+v %+v", out.Answer, notified)
	}
	if shown := decided(steps, workflow.VerbNotify); len(shown) != 1 || shown[0].Action != workflow.VerbNotify ||
		shown[0].Reason != text || shown[0].Line != 4 {
		t.Fatalf("the notify decision: %+v", shown)
	}
	out, steps = walkLogged(base)
	if skips := decided(steps, "skip"); len(out.Answer.Warnings) != 0 || len(skips) != 1 ||
		skips[0].Action != workflow.VerbNotify || len(notifiedLines(steps)) != 0 {
		t.Fatalf("second prompt: %+v %+v", out.Answer, appended(steps))
	}
	// Another entry of the same text is another notice: the edit shows it once, and to no other actor again.
	out, _ = walkLogged(edit(t, base, workflow.PreEdit, "src/a.go", "polix:builder"))
	if !slices.Equal(out.Answer.Warnings, []string{text}) || !slices.Equal(out.Answer.Contexts, []string{"Lee el plan"}) {
		t.Fatalf("the notice of pre_edit: %+v", out.Answer)
	}
	session := edit(t, base, workflow.PreEdit, "src/a.go", agent.ActorSession)
	if out, _ = walkLogged(session); len(out.Answer.Warnings) != 0 {
		t.Fatalf("whatever the actor: %+v", out.Answer)
	}
	if out, _ = walkLogged(as(t, base, "polix:reviewer")); len(out.Answer.Warnings) != 0 {
		t.Fatalf("a subagent: %+v", out.Answer)
	}
	_, steps = walkLogged(with(t, base, func(e *agent.Event) { e.Name = workflow.Start }))
	if got := observed(steps); len(got) != 1 || got[0].Outcome != state.OutcomeStarted {
		t.Fatalf("started: %+v", got)
	}
	if out, _ = walkLogged(base); len(out.Answer.Warnings) != 0 {
		t.Fatalf("after a start: %+v", out.Answer)
	}
	if out, _ = walkLogged(with(t, base, func(e *agent.Event) { e.Session = "s2" })); !slices.Equal(
		out.Answer.Warnings, []string{text}) {
		t.Fatalf("a new session: %+v", out.Answer)
	}
}

// AC9: with always a notice shows every time and writes no notified line; a placeholder that changes its text makes
// a new notice; a log that cannot be read shows it every time, with the warning of the log.
func TestNotifyRepeats(t *testing.T) {
	always := eval(t, "version: 1\non:\n  prompt:\n    - action: { notify: { message: \"Aviso\", always: true } }\n",
		workflow.Prompt)
	var st state.State
	for i := range 2 {
		out, steps := walk(t, always, facts{branch: "main", state: st})
		st = logged(t, st, steps)
		if !slices.Equal(out.Answer.Warnings, []string{"Aviso"}) || len(notifiedLines(steps)) != 0 ||
			len(decided(steps, workflow.VerbNotify)) != 1 {
			t.Fatalf("always, prompt %d: %+v %+v", i, out.Answer, appended(steps))
		}
	}
	branch := eval(t, "version: 1\non:\n  prompt:\n    - action: { notify: \"Rama {branch}\" }\n", workflow.Prompt)
	st = state.State{}
	for _, c := range []struct {
		branch string
		want   []string
	}{{"main", []string{"Rama main"}}, {"main", nil}, {"feat", []string{"Rama feat"}}, {"feat", nil}} {
		out, steps := walk(t, branch, facts{branch: c.branch, state: st})
		st = logged(t, st, steps)
		if !slices.Equal(out.Answer.Warnings, c.want) {
			t.Fatalf("on %s: %+v, want %q", c.branch, out.Answer, c.want)
		}
	}
	once := eval(t, "version: 1\non:\n  prompt:\n    - action: { notify: \"Aviso\" }\n", workflow.Prompt)
	for i := range 2 {
		out, _ := walk(t, once, facts{branch: "main", stateErr: errors.New("permission denied")})
		if len(out.Answer.Warnings) != 2 || out.Answer.Warnings[0] != "Aviso" ||
			!strings.Contains(out.Answer.Warnings[1], "cannot use its log (permission denied)") {
			t.Fatalf("an unreadable log, prompt %d: %+v", i, out.Answer)
		}
	}
}

// AC7 and AC9: the events of one call give one answer, so a notice of an earlier event is written as notified only by
// the last event, once the whole call does not block. A command whose commit is blocked shows the notice of its
// pre_command to nobody: it is logged as not shown, never written, and shows on the next call; a commit that goes
// through writes it, with the event that showed it, and the next call skips it.
func TestNotifyCall(t *testing.T) {
	base := eval(t, `version: 1
on:
  pre_command:
    - action: { notify: "Aviso" }
  pre_commit:
    - when: { branch: "^main$" }
      action: { block: "No en main" }
`, workflow.PreCommand)
	var st state.State
	call := func(branch string) (first, second Output, steps []hook.Step) {
		t.Helper()
		command := with(t, base, func(e *agent.Event) { e.Command = "git commit -m x" })
		command.Remaining = 1
		first, s1 := walk(t, command, facts{branch: branch, state: st})
		commit := with(t, base, func(e *agent.Event) { e.Name, e.Command = workflow.PreCommit, "git commit -m x" })
		commit.Facts = first.Facts
		second, s2 := walk(t, commit, facts{branch: branch, state: st})
		if notified := notifiedLines(s1); len(notified) != 0 {
			t.Fatalf("an event before the last writes no notified line: %+v", notified)
		}
		st = logged(t, logged(t, st, s1), s2)
		return first, second, s2
	}
	for range 2 {
		first, second, steps := call("main")
		unshown := unshownMessages(steps)
		if !slices.Equal(first.Answer.Warnings, []string{"Aviso"}) || len(first.Facts.Pending) != 1 ||
			!second.Answer.Block || len(notifiedLines(steps)) != 0 || len(second.Facts.Pending) != 0 ||
			len(unshown) != 1 || !strings.HasSuffix(unshown[0], "not shown to the user: Aviso") {
			t.Fatalf("a blocked commit: %+v %+v %q", first.Answer, second.Answer, unshown)
		}
	}
	first, second, steps := call("feat")
	notified := notifiedLines(steps)
	if !slices.Equal(first.Answer.Warnings, []string{"Aviso"}) || second.Answer.Block || len(notified) != 1 ||
		notified[0].Event != workflow.PreCommand || notified[0].Line != 4 || notified[0].Key != "Aviso" ||
		len(unshownMessages(steps)) != 0 {
		t.Fatalf("a commit that goes through: %+v %+v %+v", first.Answer, second.Answer, notified)
	}
	if first, _, _ := call("feat"); len(first.Answer.Warnings) != 0 {
		t.Fatalf("the next call: %+v", first.Answer)
	}
}

// AC9: an edit of several paths is one call: its first pre_edit shows the notice, the next ones skip it, and the last
// writes it once.
func TestNotifyCallOfSeveralEdits(t *testing.T) {
	base := eval(t, "version: 1\non:\n  pre_edit:\n    - action: { notify: \"Aviso\" }\n", workflow.PreEdit)
	a := edit(t, base, workflow.PreEdit, "src/a.go", agent.ActorSession)
	a.Remaining = 1
	first, s1 := walk(t, a, facts{branch: "main"})
	b := edit(t, base, workflow.PreEdit, "src/b.go", agent.ActorSession)
	b.Facts = first.Facts
	second, s2 := walk(t, b, facts{branch: "main"})
	notified := notifiedLines(s2)
	if !slices.Equal(first.Answer.Warnings, []string{"Aviso"}) || len(notifiedLines(s1)) != 0 ||
		len(second.Answer.Warnings) != 1 || len(decided(s2, workflow.VerbNotify)) != 0 || len(notified) != 1 ||
		notified[0].Key != "Aviso" {
		t.Fatalf("%+v %+v %+v", first.Answer, second.Answer, notified)
	}
}

// AC7 and AC9: a notice of a prompt a later entry blocks is not shown, so it is not written as notified and stays
// due: it is logged as not shown. polix config check rejects such a workflow, so it is built by hand.
func TestNotifyBlocked(t *testing.T) {
	ev := eval(t, "version: 1\non:\n  prompt:\n    - action: { notify: \"Aviso\" }\n", workflow.Prompt)
	ev.Workflow.On[workflow.Prompt] = append(ev.Workflow.On[workflow.Prompt], workflow.Entry{
		Action: workflow.Action{Block: &workflow.Block{Message: "Espera"}}, Line: 5})
	out, steps := walk(t, ev, facts{branch: "main"})
	unshown := unshownMessages(steps)
	if !out.Answer.Block || len(notifiedLines(steps)) != 0 || len(unshown) != 1 ||
		!strings.HasSuffix(unshown[0], "not shown to the user: Aviso") {
		t.Fatalf("a blocked prompt: %+v %q %+v", out.Answer, unshown, appended(steps))
	}
}
