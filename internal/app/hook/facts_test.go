package hook

import (
	"errors"
	"slices"
	"testing"

	"github.com/mlophez/polix-ai/internal/app/agent"
	"github.com/mlophez/polix-ai/internal/app/diff"
	"github.com/mlophez/polix-ai/internal/app/state"
)

// The next event starts after the runs of this one, with the log as the file holds it and the facts it learned.
func TestFactsCarry(t *testing.T) {
	s := state.State{Records: []state.Record{{Session: "s", Observations: []state.Observation{{Outcome: state.OutcomeStarted}}}}}
	f := Facts{HasState: true, State: s}.Carry(2)
	if f.Epoch != 2 || len(f.State.Records) != 1 || !f.HasState {
		t.Fatalf("%+v", f)
	}
	if g := (Facts{}).Carry(1); len(g.State.Records) != 0 || g.HasState {
		t.Fatal("a log not read stays unread")
	}
	if g := (Facts{Epoch: 3}).Carry(2); g.Epoch != 3 {
		t.Fatalf("the epochs never go back: %d", g.Epoch)
	}
}

// Written leaves no record pending, on a copy.
func TestFactsWritten(t *testing.T) {
	f := Facts{Pending: []state.Record{{Session: "s", Observations: []state.Observation{{Outcome: state.OutcomeStarted}}}}}
	if g := f.Written(); len(g.Pending) != 0 || len(f.Pending) != 1 {
		t.Fatalf("%+v %+v", g.Pending, f.Pending)
	}
}

// The next event knows the answer of this one: the messages of an answer that does not block become warnings of the
// call, once each, and a block marks the call as blocked without adding its messages.
func TestFactsAnswered(t *testing.T) {
	f := Facts{Warnings: []string{"a"}}.Answered(agent.Answer{}.Warn("a").Warn("b"), state.Record{})
	if f.Blocked || !slices.Equal(f.Warnings, []string{"a", "b"}) {
		t.Fatalf("%+v", f)
	}
	g := f.Answered(agent.Answer{}.Refuse("r").Warn("c"), state.Record{})
	if !g.Blocked || !slices.Equal(g.Warnings, []string{"a", "b"}) {
		t.Fatalf("%+v", g)
	}
	if h := g.Answered(agent.Answer{}, state.Record{}); !h.Blocked {
		t.Fatal("a call stays blocked")
	}
}

// The records of the events of the call wait in the facts, in order, without touching the ones the caller holds; a record with
// nothing in it is not kept; a block of the call drops the notified observations of all of them, and the records
// left with nothing; and a pending record that cannot be appended is invalid.
func TestFactsPending(t *testing.T) {
	notice := state.Observation{Line: 4, Outcome: state.OutcomeNotified, Key: "Aviso"}
	passed := state.Observation{Line: 5, Outcome: state.OutcomePassed, Key: "just test", Hash: "h"}
	only := state.Record{Session: "s", Event: "pre_edit", Observations: []state.Observation{notice}}
	both := state.Record{Session: "s", Event: "pre_commit", Observations: []state.Observation{notice, passed}}
	acted := state.Record{Session: "s", Event: "prompt", Actions: []state.Action{{Action: "block", Outcome: "block"}},
		Observations: []state.Observation{notice}}
	earlier := Facts{Pending: []state.Record{only}}

	f := earlier.Carry(1).Answered(agent.Answer{}.Warn("Aviso"), both)
	if len(f.Pending) != 2 || f.Pending[0].Event != "pre_edit" || f.Pending[1].Observations[0].Key != "Aviso" ||
		f.Valid() != nil || len(earlier.Pending) != 1 {
		t.Fatalf("an answer that does not block keeps them: %+v", f.Pending)
	}
	if g := earlier.Answered(agent.Answer{}, state.Record{}); len(g.Pending) != 1 {
		t.Fatalf("a record without anything is not kept: %+v", g.Pending)
	}
	g := Facts{Pending: []state.Record{only, acted}}.Answered(agent.Answer{}.Refuse("r"), both)
	if !g.Blocked || len(g.Pending) != 2 || g.Pending[0].Event != "prompt" || len(g.Pending[0].Observations) != 0 ||
		len(g.Pending[0].Actions) != 1 || g.Pending[1].Event != "pre_commit" || len(g.Pending[1].Observations) != 1 ||
		g.Pending[1].Observations[0].Outcome != state.OutcomePassed {
		t.Fatalf("a block drops the notices and the records left empty: %+v", g.Pending)
	}
	if len(acted.Observations) != 1 {
		t.Fatal("the block changed a record of the caller")
	}
	if (Facts{Pending: []state.Record{{Session: "s", Observations: []state.Observation{{Outcome: "x"}}}}}).Valid() == nil {
		t.Fatal("a pending record with an unknown outcome is valid")
	}
}

// The branch and the HEAD commit are learned together, an unknown commit included, and both carry to the next event.
func TestFactsLearnBranch(t *testing.T) {
	f := Facts{}.learnBranch("feat/x", "c0ffee").Carry(1)
	if f.Branch != "feat/x" || f.Head != "c0ffee" || !f.HasBranch {
		t.Fatalf("%+v", f)
	}
	if g := (Facts{}).learnBranch("", ""); g.Branch != "" || g.Head != "" || !g.HasBranch {
		t.Fatalf("a detached HEAD without commit: %+v", g)
	}
}

// The commit just made carries to the next event with its files, its hash and its subject; one that cannot be read
// is empty and adds the warning once, with the text the user reads.
func TestFactsLearnCommit(t *testing.T) {
	c := Commit{Diff: diff.Diff{Changes: []diff.Change{{Path: "a.go", Added: 3}, {Path: "docs/b.md", Added: 1}}},
		Hash: "abc1234", Subject: "feat: x"}
	f := Facts{}.learnCommit(c, nil).Carry(0)
	if !f.HasCommit || f.CommitErr != nil || f.Commit.Hash != "abc1234" || f.Commit.Subject != "feat: x" ||
		!slices.Equal(f.Commit.Paths(), []string{"a.go", "docs/b.md"}) || len(f.Warnings) != 0 {
		t.Fatalf("%+v", f)
	}
	g := Facts{}.learnCommit(c, errors.New("bad object")).learnCommit(c, errors.New("bad object"))
	want := "polix cannot read the commit (bad object): changed, changed_lines, changed_files and risk_score hold"
	if !g.HasCommit || g.CommitErr == nil || g.Commit.Hash != "" || len(g.Commit.Paths()) != 0 ||
		!slices.Equal(g.Warnings, []string{want}) {
		t.Fatalf("%+v", g)
	}
	if e := (Evaluation{}).KnowCommit(c, nil); !e.Facts.HasCommit || e.Facts.Commit.Subject != "feat: x" {
		t.Fatalf("%+v", e.Facts)
	}
}
