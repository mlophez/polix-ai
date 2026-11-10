package workflow

import (
	"strings"
	"testing"
)

// AC3: an entry that follows a block without when, unless nor once in its event never runs, a problem at its line
// that names the line of the block; a block with a condition, or a reminder, lets the later entries run, and the
// loop guard of stop and subagent_stop lets them run too.
func TestOrderUnreachable(t *testing.T) {
	block := func(b Block) Action { return Action{Block: &b, Keys: []string{VerbBlock}} }
	run := Action{Run: &Run{Command: []string{"ls"}}, Keys: []string{VerbRun}}
	inject := Action{Inject: &Inject{Text: "x"}, Keys: []string{VerbInject}}
	entry := func(line int, a Action) Entry { return Entry{Action: a, Line: line, Keys: []string{"action"}} }
	cases := []struct {
		name  string
		event string
		first Entry
		want  string
	}{
		{"unconditional", PreCommit, entry(4, block(Block{Message: "x"})),
			"polix.yaml:6: never runs: the entry of line 4 always blocks the event\n" +
				"polix.yaml:8: never runs: the entry of line 4 always blocks the event"},
		{"on prompt", Prompt, entry(4, block(Block{Message: "x"})),
			"polix.yaml:6: never runs: the entry of line 4 always blocks the event\n" +
				"polix.yaml:8: never runs: the entry of line 4 always blocks the event"},
		{"with when", PreCommit, Entry{When: Cond{"branch": "main"}, Action: block(Block{Message: "x"}), Line: 4,
			Keys: []string{"when", "action"}}, ""},
		{"with unless", PreEdit, Entry{Unless: Cond{"actor": "polix:builder"}, Action: block(Block{Message: "x"}),
			Line: 4, Keys: []string{"unless", "action"}}, ""},
		{"with once", PreCommand, entry(4, block(Block{Message: "x", Once: true, Keys: []string{"message", "once"}})),
			""},
		{"on stop", Stop, entry(4, block(Block{Message: "x"})), ""},
		{"on subagent_stop", SubagentStop, entry(4, block(Block{Message: "x"})), ""},
		{"after a run", PreCommit, entry(4, run), ""},
	}
	for _, c := range cases {
		w := Workflow{Version: 1, On: map[string][]Entry{c.event: {c.first, entry(6, run), entry(8, inject)}}}
		if c.event == Stop || c.event == SubagentStop {
			w.On[c.event] = []Entry{c.first, entry(6, run), entry(8, block(Block{Message: "y"}))}
		}
		if got := strings.Join(w.Problems(), "\n"); got != c.want {
			t.Errorf("%s:\n%s\nwant:\n%s", c.name, got, c.want)
		}
	}
}

// AC10: a notify followed in its event by an entry that may block it, a block or a run on an event that has not
// happened yet, is a problem at the line of the notify naming the first such entry; after the gates, before a run
// on an event that already happened, and on stop after its gates it is valid.
func TestOrderNotices(t *testing.T) {
	block := Action{Block: &Block{Message: "x", Once: true}, Keys: []string{VerbBlock}}
	run := Action{Run: &Run{Command: []string{"ls"}}, Keys: []string{VerbRun}}
	notify := Action{Notify: &Notify{Message: "aviso"}, Keys: []string{VerbNotify}}
	inject := Action{Inject: &Inject{Text: "x"}, Keys: []string{VerbInject}}
	entry := func(line int, a Action) Entry { return Entry{Action: a, Line: line, Keys: []string{"action"}} }
	cases := []struct {
		name    string
		event   string
		entries []Entry
		want    string
	}{
		{"before a block", PreCommit, []Entry{entry(4, notify), entry(6, inject), entry(8, block), entry(10, run)},
			"polix.yaml:4: the notice is not shown when the entry of line 8 blocks the event: move it after that " +
				"entry or into its message"},
		{"before a run on pre_commit", PreCommit, []Entry{entry(4, notify), entry(6, run)},
			"polix.yaml:4: the notice is not shown when the entry of line 6 blocks the event: move it after that " +
				"entry or into its message"},
		{"before a block on stop", Stop, []Entry{entry(4, notify), entry(6, block)},
			"polix.yaml:4: the notice is not shown when the entry of line 6 blocks the event: move it after that " +
				"entry or into its message"},
		{"after the gates", PreCommit, []Entry{entry(4, block), entry(6, run), entry(8, notify)}, ""},
		{"before a run on post_edit", PostEdit, []Entry{entry(4, notify), entry(6, run)}, ""},
		{"on stop after the gates", Stop, []Entry{entry(4, run), entry(6, block), entry(8, notify)}, ""},
		{"before an inject", Prompt, []Entry{entry(4, notify), entry(6, inject)}, ""},
	}
	for _, c := range cases {
		w := Workflow{Version: 1, On: map[string][]Entry{c.event: c.entries}}
		if got := strings.Join(w.Problems(), "\n"); got != c.want {
			t.Errorf("%s:\n%s\nwant:\n%s", c.name, got, c.want)
		}
	}
}

// A block expanded from a shortcut is named by its shortcut, and so is an entry of a shortcut that never runs, as
// the other problems of a shortcut start with it.
func TestOrderNamesTheShortcut(t *testing.T) {
	block := Action{Block: &Block{Message: "x"}, Keys: []string{VerbBlock}}
	run := Action{Run: &Run{Command: []string{"ls"}}, Keys: []string{VerbRun}}
	w := Workflow{Version: 1, On: map[string][]Entry{PreCommit: {
		{Action: block, From: "guardrails." + ShortcutProtectedBranches, Line: 3, Keys: []string{"action"}},
		{Action: run, From: "guardrails." + ShortcutProtectedBranches, Line: 4, Keys: []string{"action"}},
		{Action: run, Line: 9, Keys: []string{"action"}},
	}}}
	want := "polix.yaml:4: guardrails.protected_branches: never runs: guardrails.protected_branches of line 3 " +
		"always blocks the event\n" +
		"polix.yaml:9: never runs: guardrails.protected_branches of line 3 always blocks the event"
	if got := strings.Join(w.Problems(), "\n"); got != want {
		t.Fatalf("got:\n%s\nwant:\n%s", got, want)
	}
}
