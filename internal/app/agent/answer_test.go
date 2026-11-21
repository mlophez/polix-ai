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
