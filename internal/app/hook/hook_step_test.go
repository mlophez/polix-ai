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
	for _, s := range []Step{{Branch: true}, {Refs: true}, {Hash: true, Epoch: 2}, {Ignored: "a"}, {Real: []string{"/a"}}, {Foreign: "/a"},
		{Read: "a", Epoch: 1}, {Run: []string{"ls"}}, {Append: &state.Entry{Kind: state.KindObserved}},
		{Commit: true}, {Compare: "old", Tree: "new"}, {Programs: []string{"go"}}, {Restore: []string{"AGENTS.md"}, Source: "old"}} {
		if s.None() || s.Valid() != nil {
			t.Errorf("%+v: none %v, valid %v", s, s.None(), s.Valid())
		}
	}
	for _, s := range []Step{{Branch: true, Hash: true}, {Branch: true, Refs: true}, {Commit: true, Diff: true}, {Hash: true, Epoch: -1}, {Run: []string{}},
		{Compare: "old"}, {Tree: "new"}, {Compare: "old", Tree: "new", Branch: true}, {Programs: []string{}},
		{Programs: []string{"go"}, Branch: true}, {Restore: []string{"AGENTS.md"}}, {Source: "old"},
		{Restore: []string{}, Source: "old"}, {Restore: []string{"AGENTS.md"}, Source: "old", Run: []string{"ls"}}} {
		if s.Valid() == nil {
			t.Errorf("%+v is valid", s)
		}
	}
	if n := (Step{Programs: []string{"go", "golangci-lint"}}).asks(); n != 1 {
		t.Fatalf("a step with programs asks %d things", n)
	}
	if n := (Step{Restore: []string{"AGENTS.md"}, Source: "old", Run: []string{"ls"}}).asks(); n != 2 {
		t.Fatalf("a step with a restore and a run asks %d things", n)
	}
	if (Outcome{}).Valid() != nil {
		t.Fatal("any outcome is valid")
	}
}
