package answer

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/mlophez/polix-ai/internal/app/state"
	"github.com/mlophez/polix-ai/internal/platform/git"
)

// encodeLine caps the reason, so no line of the log comes near state.MaxLine, and the line decodes back.
func TestEncodeLineCapsReason(t *testing.T) {
	line, err := (&Adapter{}).encodeLine(state.Entry{At: "2026-01-01T00:00:00Z", Kind: state.KindDecision,
		Reason: strings.Repeat("é", 512*1024)})
	if err != nil {
		t.Fatal(err)
	}
	if len(line) > state.MaxReason+512 || strings.Contains(string(line), "\n") {
		t.Fatalf("the line has %d bytes", len(line))
	}
	var l logLine
	if err := json.Unmarshal(line, &l); err != nil {
		t.Fatal(err)
	}
	if e := state.Entry(l); len(e.Reason) > state.MaxReason || !strings.HasSuffix(e.Reason, "[truncated]") {
		t.Fatalf("reason of %d bytes", len(e.Reason))
	}
}

// The fields keep the names of the log, and empty ones are left out.
func TestEncodeLineFields(t *testing.T) {
	a := &Adapter{}
	line, err := a.encodeLine(state.Entry{At: "t", Kind: state.KindDecision, Session: "s", Line: 3})
	if err != nil {
		t.Fatal(err)
	}
	if got := string(line); got != `{"at":"t","kind":"decision","session":"s","line":3}` {
		t.Fatal(got)
	}
	line, err = a.encodeLine(state.Entry{At: "t", Kind: state.KindObserved, Actor: "polix:reviewer", Hash: "h",
		Key: "polix:reviewer", Outcome: state.OutcomeFinished})
	if err != nil {
		t.Fatal(err)
	}
	want := `{"at":"t","kind":"observed","actor":"polix:reviewer","hash":"h","key":"polix:reviewer","outcome":"finished"}`
	if got := string(line); got != want {
		t.Fatal(got)
	}
	if _, err := a.encodeLine(state.Entry{}); err == nil {
		t.Fatal("an entry without kind")
	}
}

// AC6: Append writes the HEAD commit of an entry under the key head, right after the hash, and leaves it out when
// the entry has none.
func TestAppendHead(t *testing.T) {
	a := &Adapter{Repo: &git.Repo{GitDir: t.TempDir()}}
	ctx := context.Background()
	if err := a.Append(ctx, state.Entry{At: "t", Kind: state.KindObserved, Branch: "main", Hash: "h", Head: "c1",
		Key: "just test", Outcome: state.OutcomePassed}); err != nil {
		t.Fatal(err)
	}
	if err := a.Append(ctx, state.Entry{At: "t", Kind: state.KindDecision, Branch: "main", Hash: "h",
		Outcome: "skip"}); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(a.logPath())
	if err != nil {
		t.Fatal(err)
	}
	want := `{"at":"t","kind":"observed","branch":"main","hash":"h","head":"c1","key":"just test","outcome":"passed"}` +
		"\n" + `{"at":"t","kind":"decision","branch":"main","hash":"h","outcome":"skip"}` + "\n"
	if got := string(data); got != want {
		t.Fatal(got)
	}
}
