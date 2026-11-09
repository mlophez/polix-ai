package agent

import (
	"slices"
	"testing"
)

// Merge joins two answers without repeating a text, and a block wins.
func TestAnswerMerge(t *testing.T) {
	a := Answer{}.Refuse("no").Inform("c").Warn("w")
	b := Answer{}.Inform("c").Inform("d").Warn("w")
	got := a.Merge(b)
	if !got.Block || !slices.Equal(got.Reasons, []string{"no"}) || !slices.Equal(got.Contexts, []string{"c", "d"}) ||
		!slices.Equal(got.Warnings, []string{"w"}) {
		t.Fatalf("Merge = %+v", got)
	}
	if len(a.Contexts) != 1 {
		t.Fatal("Merge changed the original")
	}
}

// Refuse, Inform and Warn append, without touching the original.
func TestAnswerAppends(t *testing.T) {
	a := Answer{}.Warn("x")
	b := a.Warn("x")
	if len(a.Warnings) != 1 || len(b.Warnings) != 2 {
		t.Fatalf("%+v %+v", a, b)
	}
}

// A block needs a reason.
func TestAnswerValid(t *testing.T) {
	if (Answer{Block: true}).Valid() == nil || (Answer{}).Valid() != nil || (Answer{}).Refuse("x").Valid() != nil {
		t.Fatal("Valid")
	}
}

// A block goes to stderr with exit 2, its contexts after the reasons; otherwise warnings go to stderr and the
// contexts to the JSON of the events that take it.
func TestAnswerRender(t *testing.T) {
	stdout, stderr, code := Answer{}.Refuse("a").Refuse("b").Inform("c").render("PreToolUse", claudeContextEvents)
	if code != ExitBlock || stdout != "" || stderr != "a\n\nb\n\nc\n" {
		t.Fatalf("block: %q %q %d", stdout, stderr, code)
	}
	stdout, stderr, code = Answer{}.Inform("x<y").Inform("z").Warn("w").render("PreToolUse", claudeContextEvents)
	want := `{"hookSpecificOutput":{"additionalContext":"x\u003cy\n\nz","hookEventName":"PreToolUse"}}` + "\n"
	if code != ExitOK || stdout != want || stderr != "polix: w\n" {
		t.Fatalf("context: %q %q %d", stdout, stderr, code)
	}
	if stdout, _, _ := (Answer{}).Inform("x").render("Stop", claudeContextEvents); stdout != "" {
		t.Fatalf("Stop takes no context: %q", stdout)
	}
}
