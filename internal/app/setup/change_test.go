package setup

import (
	"slices"
	"testing"
)

func TestChange(t *testing.T) {
	for _, c := range []Change{{Path: "", Action: "created"}, {Path: "x", Action: "moved"}} {
		if err := c.Valid(); err == nil {
			t.Errorf("%+v is not a change", c)
		}
	}
	todo := Change{Path: "claude plugin install", Action: "todo"}
	removed := Change{Path: "x", Action: "unregistered"}
	if todo.Valid() != nil || !todo.Todo() || todo.Removal() || !removed.Removal() {
		t.Fatal("todo and removal")
	}
}

func TestChanges(t *testing.T) {
	got, err := Changes{}.Synced([]string{"a"}, nil, []string{"b"})
	if err != nil || !slices.Equal(got, Changes{{Path: "a", Action: "created"}, {Path: "b", Action: "removed"}}) {
		t.Fatal(got, err)
	}
	if more, err := got.Add("todo", "c"); err != nil || len(more) != 3 || len(got) != 2 {
		t.Fatalf("a copy with the change: %v %v", more, err)
	}
	if none, err := (Changes{}).Add("created"); err != nil || none != nil {
		t.Fatalf("no path, no change: %v %v", none, err)
	}
	if _, err := (Changes{}).Add("moved", "x"); err == nil {
		t.Fatal("an unknown action")
	}
}
