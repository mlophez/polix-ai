package load

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mlophez/polix-ai/internal/app/state"
)

// The adapter is tested against a temporary folder: the real disk, as the port promises.

// Every line that decodes is kept, even without kind; the lines that do not decode are left out, and the lines
// over state.MaxLine are skipped and counted. An old fact line is kept with its kind and never selected by a query.
func TestAdapterReadLog(t *testing.T) {
	lines := []string{`{"kind":"fact","fact":"reviewed","branch":"main","hash":"h"}`, `not json`,
		strings.Repeat("x", state.MaxLine+1), `{"key":"x"}`,
		`{"kind":"observed","outcome":"finished","key":"polix:reviewer","branch":"main"}`}
	p := filepath.Join(t.TempDir(), "log.jsonl")
	if err := os.WriteFile(p, []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	entries, skipped, err := Adapter{}.ReadLog(context.Background(), p)
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
func TestAdapterReadLogHead(t *testing.T) {
	lines := []string{`{"kind":"observed","outcome":"passed","key":"k","branch":"main","hash":"h","head":"c1"}`,
		`{"kind":"observed","outcome":"passed","key":"k","branch":"main","hash":"h"}`}
	p := filepath.Join(t.TempDir(), "log.jsonl")
	if err := os.WriteFile(p, []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	entries, _, err := Adapter{}.ReadLog(context.Background(), p)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 || entries[0].Head != "c1" || entries[0].Hash != "h" || entries[1].Head != "" {
		t.Fatalf("entries %+v", entries)
	}
}

// A missing log has no lines and no error.
func TestAdapterReadLogWithoutFile(t *testing.T) {
	entries, skipped, err := Adapter{}.ReadLog(context.Background(), filepath.Join(t.TempDir(), "log.jsonl"))
	if len(entries) != 0 || skipped != 0 || err != nil {
		t.Fatalf("got %+v %d %v", entries, skipped, err)
	}
}

// A log that cannot be read, a folder in its place, is the error.
func TestAdapterReadLogUnreadable(t *testing.T) {
	if _, _, err := (Adapter{}).ReadLog(context.Background(), t.TempDir()); err == nil {
		t.Fatal("want the error of a log that cannot be read")
	}
}
