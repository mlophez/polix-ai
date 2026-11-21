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

// Every line that decodes is kept, even without kind; the lines that do not decode are left out, and the lines
// over state.MaxLine are skipped and counted. An old fact line is kept with its kind and never selected by a query.
func TestLogReadLog(t *testing.T) {
	lines := []string{`{"kind":"fact","fact":"reviewed","branch":"main","hash":"h"}`, `not json`,
		strings.Repeat("x", state.MaxLine+1), `{"key":"x"}`,
		`{"kind":"observed","outcome":"finished","key":"polix:reviewer","branch":"main"}`}
	p := filepath.Join(t.TempDir(), "log.jsonl")
	if err := os.WriteFile(p, []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := (&Log{}).read(p)
	entries, skipped := got.Entries, got.Skipped
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 3 || entries[0].Kind != "fact" || entries[0].Hash != "h" ||
		entries[0].Fact != "reviewed" || entries[1].Key != "x" ||
		entries[1].Kind != "" || entries[2].Key != "polix:reviewer" || skipped != 1 {
		t.Fatalf("entries %+v, skipped %d", entries, skipped)
	}
	s := state.State{Entries: entries, Skipped: skipped}
	q := state.Query{Outcome: state.OutcomeFinished, Branch: "main"}
	if got := s.Observed(q); len(got) != 1 || got[0].Kind != state.KindObserved {
		t.Fatalf("the old fact line is ignored: %+v", got)
	}
}

// The key head decodes into Entry.Head; a line without it, such as one of an older polix, leaves it empty.
func TestLogReadLogHead(t *testing.T) {
	lines := []string{`{"kind":"observed","outcome":"passed","key":"k","branch":"main","hash":"h","head":"c1"}`,
		`{"kind":"observed","outcome":"passed","key":"k","branch":"main","hash":"h"}`}
	p := filepath.Join(t.TempDir(), "log.jsonl")
	if err := os.WriteFile(p, []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := (&Log{}).read(p)
	entries := got.Entries
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 || entries[0].Head != "c1" || entries[0].Hash != "h" || entries[1].Head != "" {
		t.Fatalf("entries %+v", entries)
	}
}

// The key agent_id decodes into Entry.AgentID; a line without it, such as one of an older polix, leaves it empty
// and reads as before.
func TestLogReadLogAgentID(t *testing.T) {
	lines := []string{
		`{"kind":"observed","outcome":"started","session":"s","event":"subagent_start","actor":"builder","agent_id":"a1"}`,
		`{"kind":"observed","outcome":"started","session":"s","event":"subagent_start","actor":"builder"}`}
	p := filepath.Join(t.TempDir(), "log.jsonl")
	if err := os.WriteFile(p, []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := (&Log{}).read(p)
	entries := got.Entries
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 || entries[0].AgentID != "a1" || entries[0].Actor != "builder" || entries[1].AgentID != "" ||
		entries[1].Actor != "builder" || entries[1].Outcome != state.OutcomeStarted {
		t.Fatalf("entries %+v", entries)
	}
}

// A missing log has no lines and no error.
func TestLogReadLogWithoutFile(t *testing.T) {
	got, err := (&Log{}).read(filepath.Join(t.TempDir(), "log.jsonl"))
	entries, skipped := got.Entries, got.Skipped
	if len(entries) != 0 || skipped != 0 || err != nil {
		t.Fatalf("got %+v %d %v", entries, skipped, err)
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
	p := filepath.Join(t.TempDir(), "log.jsonl")
	if err := os.WriteFile(p, []byte(`{"kind":"decision","session":"s"}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
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
	if len(first.State.Entries) != 1 || len(second.State.Entries) != 2 || first.Err != nil || second.Err != nil {
		t.Fatalf("first %+v, second %+v", first, second)
	}
}

// Load never fails: a log that cannot be read, a folder in its place, is an empty state with Err set.
func TestLogLoadNeverFails(t *testing.T) {
	got := (&Log{}).Load(context.Background(), t.TempDir())
	if got.Err == nil || len(got.State.Entries) != 0 || got.State.Skipped != 0 {
		t.Fatalf("got %+v", got)
	}
	if got := (&Log{}).Load(context.Background(), filepath.Join(t.TempDir(), "log.jsonl")); got.Err != nil ||
		len(got.State.Entries) != 0 {
		t.Fatalf("no file: %+v", got)
	}
}
