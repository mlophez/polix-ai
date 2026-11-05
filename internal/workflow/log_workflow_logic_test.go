package workflow

import (
	"reflect"
	"testing"

	"github.com/mlophez/polix-ai/internal/state"
)

func TestLogWorkflowPicksTheLastEntries(t *testing.T) {
	st := state.State{Entries: []state.Entry{{Kind: "a"}, {Kind: "b"}, {Kind: "c"}}}
	for tail, want := range map[int][]string{0: {"a", "b", "c"}, -1: {"a", "b", "c"}, 2: {"b", "c"}, 3: {"a", "b", "c"}, 9: {"a", "b", "c"}} {
		var got []string
		for _, e := range LogWorkflow(st, tail).Entries {
			got = append(got, e.Kind)
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("tail %d: %v", tail, got)
		}
	}
	if out := LogWorkflow(state.State{}, 5); len(out.Entries) != 0 {
		t.Fatalf("empty log: %+v", out)
	}
}
