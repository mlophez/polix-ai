package adapters

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mlophez/polix-ai/internal/app/state"
)

// Log is tested against a temporary folder: the real disk.

// writeLog writes the lines to a log in a temporary folder and returns its path.
func writeLog(t *testing.T, lines ...string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "log.jsonl")
	if err := os.WriteFile(p, []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

// An event line decodes into a record with its input, its actions and its observations, and keeps the line as it was
// read; the lines that do not decode are left out, and the lines over state.MaxLine are skipped and counted.
func TestLogReadsEventLines(t *testing.T) {
	event := `{"at":"2026-11-22T13:50:05+01:00","agent":"claude","session":"s1","event":"pre_commit","actor":"session",` +
		`"agent_id":"a1","branch":"main","head":"c0ffee","hash":"h","input":{"command":"git commit -m \"wip\"",` +
		`"tool_use":"toolu_1"},"actions":[{"line":4,"action":"inject","outcome":"injected","key":"ai/rules/go.yaml"},` +
		`{"line":5,"action":"run","outcome":"passed","key":"just test","duration_ms":0},` +
		`{"line":6,"action":"block","outcome":"block","reason":"Subagents never commit."}],` +
		`"observed":[{"line":5,"outcome":"passed","key":"just test","hash":"h"},` +
		`{"outcome":"snapshot","key":"toolu_1","hash":"h","refs":"r"},{"line":6,"outcome":"blocked","hash":"h","before":"g"}]}`
	p := writeLog(t, event, `not json`, `[1]`, `null`, strings.Repeat("x", state.MaxLine+1), `{"session":"s1"}`,
		`{"session":"s1","event":"stop","observed":[{"outcome":"newer"}]}`, `{"kind":"decision"}`)
	got, err := (&Log{}).read(p)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Records) != 2 || got.Skipped != 1 {
		t.Fatalf("records %+v, skipped %d", got.Records, got.Skipped)
	}
	r := got.Records[0]
	if r.Legacy != "" || r.Raw != event || r.At != "2026-11-22T13:50:05+01:00" || r.Agent != "claude" ||
		r.Session != "s1" || r.Event != "pre_commit" || r.Actor != "session" || r.AgentID != "a1" ||
		r.Branch != "main" || r.Head != "c0ffee" || r.Hash != "h" {
		t.Fatalf("record %+v", r)
	}
	if r.Input != (state.Input{Command: `git commit -m "wip"`, ToolUse: "toolu_1"}) {
		t.Fatalf("input %+v", r.Input)
	}
	if len(r.Actions) != 3 || r.Actions[0] != (state.Action{Line: 4, Action: "inject", Outcome: "injected",
		Key: "ai/rules/go.yaml"}) || r.Actions[2].Reason != "Subagents never commit." {
		t.Fatalf("actions %+v", r.Actions)
	}
	if d := r.Actions[1].DurationMS; d == nil || *d != 0 || r.Actions[0].DurationMS != nil {
		t.Fatalf("durations %+v", r.Actions)
	}
	want := []state.Observation{{Line: 5, Outcome: "passed", Key: "just test", Hash: "h"},
		{Outcome: "snapshot", Key: "toolu_1", Hash: "h", Refs: "r"}, {Line: 6, Outcome: "blocked", Hash: "h", Before: "g"}}
	if len(r.Observations) != 3 || r.Observations[0] != want[0] || r.Observations[1] != want[1] ||
		r.Observations[2] != want[2] {
		t.Fatalf("observations %+v", r.Observations)
	}
	s := state.State{Records: got.Records, Skipped: got.Skipped}
	q := state.Query{Outcome: state.OutcomePassed, Branch: "main", ByBranch: true, Key: "just test"}
	if m := s.Observed(q); len(m) != 1 || m[0].Record.Head != "c0ffee" {
		t.Fatalf("the memory of the session reads the event line: %+v", m)
	}
}

// Legacy lines of every kind decode into legacy records with their fields, the rule and the fact included, and are
// never selected by a query.
func TestLogReadsLegacyLines(t *testing.T) {
	lines := []string{
		`{"at":"t0","kind":"fact","fact":"reviewed","session":"s","branch":"main","hash":"h"}`,
		`{"at":"t1","kind":"observed","session":"s","event":"subagent_stop","actor":"polix:reviewer","agent_id":"a1",` +
			`"branch":"main","head":"c1","hash":"h","before":"g","key":"polix:reviewer","outcome":"finished","line":3}`,
		`{"at":"t2","kind":"decision","session":"s","event":"pre_edit","actor":"builder","action":"block","line":4,` +
			`"outcome":"block","reason":"first\nsecond","hash":"h"}`,
		`{"at":"t3","kind":"injected","session":"s","rule":"GO-2"}`,
		`{"at":"t4","kind":"stop_block","session":"s","event":"stop","actor":"session","line":4,"hash":"h","reason":"revisa"}`,
		`{"at":"t5","kind":"newer","session":"s","outcome":"x"}`,
	}
	got, err := (&Log{}).read(writeLog(t, lines...))
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Records) != 6 || got.Skipped != 0 {
		t.Fatalf("records %+v", got.Records)
	}
	for i, r := range got.Records {
		if r.Raw != lines[i] || r.Legacy == "" || r.Session != "s" {
			t.Errorf("record %d: %+v", i, r)
		}
	}
	fact, observed, decision := got.Records[0], got.Records[1], got.Records[2]
	if fact.Legacy != "fact" || fact.Fact != "reviewed" || fact.Hash != "h" || fact.Branch != "main" {
		t.Errorf("fact %+v", fact)
	}
	if observed.Legacy != state.LegacyObserved || observed.AgentID != "a1" || observed.Head != "c1" ||
		len(observed.Observations) != 1 || observed.Observations[0] != (state.Observation{Line: 3,
		Outcome: "finished", Key: "polix:reviewer", Hash: "h", Before: "g"}) {
		t.Errorf("observed %+v", observed)
	}
	if decision.Legacy != state.LegacyDecision || len(decision.Actions) != 1 || decision.Actions[0] != (state.Action{
		Line: 4, Action: "block", Outcome: "block", Reason: "first\nsecond"}) || decision.Hash != "h" {
		t.Errorf("decision %+v", decision)
	}
	if got.Records[3].Rule != "GO-2" || got.Records[4].Actions[0].Reason != "revisa" || got.Records[4].Hash != "h" {
		t.Errorf("old kinds %+v %+v", got.Records[3], got.Records[4])
	}
	s := state.State{Records: got.Records}
	for _, o := range state.Outcomes {
		if m := s.Observed(state.Query{Outcome: o}); len(m) != 0 {
			t.Errorf("%s: a legacy line is selected: %+v", o, m)
		}
	}
	if len(s.Running("s")) != 0 {
		t.Error("a legacy line is running")
	}
}

// A missing log has no lines and no error.
func TestLogReadLogWithoutFile(t *testing.T) {
	got, err := (&Log{}).read(filepath.Join(t.TempDir(), "log.jsonl"))
	if len(got.Records) != 0 || got.Skipped != 0 || err != nil {
		t.Fatalf("got %+v %v", got, err)
	}
}

// A log that cannot be read, a folder in its place, is the error.
func TestLogReadLogUnreadable(t *testing.T) {
	if _, err := (&Log{}).read(t.TempDir()); err == nil {
		t.Fatal("want the error of a log that cannot be read")
	}
}

// Load reads the log at every call: the process that reads it also appends to it.
func TestLogLoadReadsEveryCall(t *testing.T) {
	p := writeLog(t, `{"kind":"decision","session":"s"}`)
	l := &Log{}
	ctx := context.Background()
	first := l.Load(ctx, p)
	f, err := os.OpenFile(p, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString(`{"kind":"decision","session":"s"}` + "\n"); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	second := l.Load(ctx, p)
	if len(first.State.Records) != 1 || len(second.State.Records) != 2 || first.Err != nil || second.Err != nil {
		t.Fatalf("first %+v, second %+v", first, second)
	}
}

// Load never fails: a log that cannot be read, a folder in its place, is an empty state with Err set.
func TestLogLoadNeverFails(t *testing.T) {
	got := (&Log{}).Load(context.Background(), t.TempDir())
	if got.Err == nil || len(got.State.Records) != 0 || got.State.Skipped != 0 {
		t.Fatalf("got %+v", got)
	}
	if got := (&Log{}).Load(context.Background(), filepath.Join(t.TempDir(), "log.jsonl")); got.Err != nil ||
		len(got.State.Records) != 0 {
		t.Fatalf("no file: %+v", got)
	}
}
