package hook

import (
	"testing"

	"github.com/mlophez/polix-ai/internal/app/state"
)

// The zero step is none; a step asks for one thing at a time.
func TestStepValid(t *testing.T) {
	if !(Step{}).None() || (Step{}).Valid() != nil {
		t.Fatal("the zero step is none and valid")
	}
	for _, s := range []Step{{Branch: true}, {Hash: true, Epoch: 2}, {Ignored: "a"}, {Real: []string{"/a"}}, {Foreign: "/a"},
		{Read: "a", Epoch: 1}, {Run: []string{"ls"}}, {Append: &state.Entry{Kind: state.KindObserved}},
		{Compare: "old", Tree: "new"}} {
		if s.None() || s.Valid() != nil {
			t.Errorf("%+v: none %v, valid %v", s, s.None(), s.Valid())
		}
	}
	for _, s := range []Step{{Branch: true, Hash: true}, {Hash: true, Epoch: -1}, {Run: []string{}},
		{Compare: "old"}, {Tree: "new"}, {Compare: "old", Tree: "new", Branch: true}} {
		if s.Valid() == nil {
			t.Errorf("%+v is valid", s)
		}
	}
	if (Outcome{}).Valid() != nil {
		t.Fatal("any outcome is valid")
	}
}
