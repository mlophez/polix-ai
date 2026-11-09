package setup

import "testing"

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
