package hook

import (
	"errors"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/mlophez/polix-ai/internal/app/agent"
	"github.com/mlophez/polix-ai/internal/app/state"
	"github.com/mlophez/polix-ai/internal/app/workflow"
)

// An evaluation needs the repository and a neutral event.
func TestEvaluationValid(t *testing.T) {
	stop := agent.Event{Name: workflow.Stop, Agent: agent.Claude}
	if err := (Evaluation{Root: "/r", GitDir: "/r/.git", Event: stop}).Valid(); err != nil {
		t.Fatal(err)
	}
	if (Evaluation{Event: stop}).Valid() == nil || (Evaluation{Root: "/r", GitDir: "/g"}).Valid() == nil {
		t.Fatal("an evaluation without repository or event is valid")
	}
	if (Evaluation{Root: "/r", GitDir: "/g", Event: stop, Facts: Facts{Epoch: -1}}).Valid() == nil {
		t.Fatal("negative facts are valid")
	}
	ev := Evaluation{GitDir: filepath.FromSlash("/r/.git"), System: "darwin"}
	if ev.StateDir() != filepath.FromSlash("/r/.git/polix") || !ev.Fold() || ev.Windows() {
		t.Fatal("StateDir, Fold, Windows")
	}
}

// The facts fed back are kept without touching the evaluation they came from.
func TestEvaluationKnow(t *testing.T) {
	var ev Evaluation
	got := ev.KnowBranch("main").KnowChanged([]string{"a"}).KnowFiles([]string{"b"}).
		KnowRules(nil, errors.New("rules")).KnowIgnored("x", true).KnowReal([]string{"/a"}, []string{"/real/a"}).
		KnowContent(0, "f", "data", nil).KnowContent(1, "f", "", errors.New("gone")).KnowHash(0, "h", nil)
	f := got.Facts
	if f.Branch != "main" || !f.HasBranch || f.Changed[0] != "a" || f.Files[0] != "b" || f.RulesErr == nil ||
		!f.Ignored["x"] || f.Reals["/a"] != "/real/a" {
		t.Fatalf("facts: %+v", f)
	}
	if data, readable, read := f.Content(0, "f"); data != "data" || !readable || !read {
		t.Fatal("content at epoch 0")
	}
	if _, readable, read := f.Content(1, "f"); readable || !read {
		t.Fatal("content at epoch 1 cannot be read")
	}
	if _, _, read := f.Content(2, "f"); read {
		t.Fatal("content at epoch 2 was not read")
	}
	if h, known, computed := f.Hash(0); h != "h" || !known || !computed {
		t.Fatal("hash at epoch 0")
	}
	if ev.Facts.HasBranch || ev.Facts.Reals != nil {
		t.Fatal("the original changed")
	}
}

// A hash that cannot be computed is unknown and warned once.
func TestEvaluationKnowHashError(t *testing.T) {
	ev := Evaluation{}.KnowHash(0, "x", errors.New("boom")).KnowHash(1, "", errors.New("boom"))
	if h, known, computed := ev.Facts.Hash(0); h != "" || known || !computed {
		t.Fatal("an unknown hash")
	}
	if len(ev.Facts.Warnings) != 1 || !strings.Contains(ev.Facts.Warnings[0], "hash del contenido (boom)") {
		t.Fatalf("warnings: %q", ev.Facts.Warnings)
	}
}

// The log is read after the steps of the journal so far, and Note adds to the journal.
func TestEvaluationKnowStateAndNote(t *testing.T) {
	ev := Evaluation{}.Note("out", nil).KnowState(state.State{Skipped: 1}, nil).Note("", errors.New("x"))
	if ev.Facts.StateAt != 1 || ev.Facts.State.Skipped != 1 || len(ev.Journal) != 2 || ev.Journal[1].Err == nil {
		t.Fatalf("%+v", ev)
	}
	broken := Evaluation{}.KnowState(state.State{Skipped: 1}, errors.New("denied"))
	if broken.Facts.StateErr == nil || broken.Facts.State.Skipped != 0 {
		t.Fatalf("an unreadable log is empty: %+v", broken.Facts)
	}
}

// The comparison of two trees is kept per pair, whatever the epoch: its paths, or why it could not be done.
func TestEvaluationKnowTreeDiff(t *testing.T) {
	var ev Evaluation
	got := ev.KnowTreeDiff("a", "b", []string{"x.go"}, nil).KnowTreeDiff("c", "b", nil, errors.New("bad object c"))
	if paths, compared, err := got.Facts.TreeDiff("a", "b"); !compared || err != nil || !slices.Equal(paths, []string{"x.go"}) {
		t.Fatalf("a b: %q %v %v", paths, compared, err)
	}
	if _, compared, err := got.Facts.TreeDiff("c", "b"); !compared || err == nil {
		t.Fatalf("c b: %v %v", compared, err)
	}
	if _, compared, _ := got.Facts.TreeDiff("b", "a"); compared {
		t.Fatal("b a was not compared")
	}
	if ev.Facts.TreeDiffs != nil || ev.Facts.TreeErrs != nil {
		t.Fatal("the original changed")
	}
}

// The problems of a workflow hint an update only when a newer polix may know them; TooOldError stays itself.
func TestEvaluationRunning(t *testing.T) {
	unusable := func(err error, running string) error { return Evaluation{Invalid: err}.Running(running).Invalid }
	if unusable(nil, "dev") != nil {
		t.Fatal("no error")
	}
	for problem, hint := range map[string]bool{
		"polix.yaml:3: unknown key \"x\"":                      true,
		"polix.yaml: version must be 1, got 2":                 true,
		"polix.yaml: requires polix 0.3.0":                     true,
		"polix.yaml: max_stop_blocks must be positive, got -1": false,
		"polix.yaml:3: unknown event \"turn_stop\": use stop\npolix.yaml:4: unknown key \"deny\": use action: { block: <message> }":  false,
		"polix.yaml:3: unknown event \"session_start\": use start":                                                                   false,
		"polix.yaml:4: unknown condition \"done\": use finished: <subagent>\npolix.yaml:5: unknown key \"x\" (when, unless, action)": true,
		"polix.yaml:4: unknown key \"context\" in action: use inject":                                                                false,
	} {
		err := unusable(errors.New(problem), "0.1.0")
		if got := strings.Contains(err.Error(), "no las quites: ejecuta polix update"); got != hint {
			t.Errorf("%s: hint %v", problem, got)
		}
		if !strings.HasPrefix(err.Error(), problem) {
			t.Errorf("%s: %v", problem, err)
		}
	}
	// The problems of a polix.yaml in the old vocabulary, as the workflow reports them, never hint an update.
	// version: 1
	// on:
	//   turn_stop:
	//     - if: { done: x }
	//       action: { deny: x }
	//     - context: x
	oldVocabulary := workflow.Workflow{Version: 1, EventLines: map[string]int{"turn_stop": 3},
		On: map[string][]workflow.Entry{"turn_stop": {
			{Line: 4, Keys: []string{"if", "action"}, OldIf: workflow.Cond{"done": "x"},
				Action: workflow.Action{Keys: []string{"deny"}}},
			{Line: 6, Keys: []string{"context"}},
		}}}
	err := oldVocabulary.Valid()
	if err == nil || strings.Contains(unusable(err, "0.1.0").Error(), "no las quites") {
		t.Fatalf("the old vocabulary hints an update: %v", err)
	}
	old := workflow.TooOldError{Requires: "0.2.0", Running: "0.1.0"}
	var got workflow.TooOldError
	if err := unusable(old, "0.1.0"); !errors.As(err, &got) || strings.Contains(err.Error(), "no las quites") {
		t.Fatalf("TooOldError: %v", err)
	}
}

// The file names of macOS and Windows ignore case.
func TestEvaluationFold(t *testing.T) {
	fold := func(goos string) bool { return Evaluation{System: goos}.Fold() }
	if !fold("darwin") || !fold("windows") || fold("linux") {
		t.Fatal("Fold")
	}
}
