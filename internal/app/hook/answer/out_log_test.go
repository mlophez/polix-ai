package answer

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/mlophez/polix-ai/internal/app/state"
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
	if e := state.Entry(l); len(e.Reason) > state.MaxReason || !strings.HasSuffix(e.Reason, "[recortado]") {
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
