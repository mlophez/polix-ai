package githook

import (
	"errors"
	"testing"

	"github.com/mlophez/polix-ai/internal/app/workflow"
)

// A call of git is valid on an event of githooks, with staged files that are paths.
func TestGithookValid(t *testing.T) {
	cases := []struct {
		name string
		g    Githook
		ok   bool
	}{
		{"pre_commit", Githook{Event: workflow.PreCommit, Branch: "main", Staged: []string{"a.go"}}, true},
		{"commit_msg on a detached HEAD", Githook{Event: workflow.CommitMsg}, true},
		{"an event of the AI", Githook{Event: workflow.Stop}, false},
		{"no event", Githook{}, false},
		{"an empty staged file", Githook{Event: workflow.PreCommit, Staged: []string{""}}, false},
		{"commit_msg with its message", Githook{Event: workflow.CommitMsg, Message: "fix: x\n", Comment: "#"}, true},
		{"a message on pre_commit", Githook{Event: workflow.PreCommit, Message: "x"}, false},
		{"a comment string on pre_commit", Githook{Event: workflow.PreCommit, Comment: "#"}, false},
	}
	for _, c := range cases {
		if err := c.g.Valid(); (err == nil) != c.ok {
			t.Errorf("%s: %v", c.name, err)
		}
	}
}

// Ran returns a copy that knows the outcome of one more run, in order, and leaves the original as it was.
func TestGithookRan(t *testing.T) {
	boom := errors.New("boom")
	g := Githook{Event: workflow.PreCommit}
	once := g.Ran(nil)
	twice := once.Ran(boom)
	if len(g.Outcomes) != 0 || len(once.Outcomes) != 1 || len(twice.Outcomes) != 2 ||
		!once.Outcomes[0].Passed() || twice.Outcomes[1].Passed() || !errors.Is(twice.Outcomes[1].Err, boom) {
		t.Fatalf("got %+v %+v %+v", g, once, twice)
	}
	if err := (Outcome{}).Valid(); err != nil {
		t.Fatal(err)
	}
}
