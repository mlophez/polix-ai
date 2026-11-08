package state

import (
	"strings"
	"testing"
)

// Every line that decodes is kept, even without kind; the lines that do not decode are left out, and the count
// of skipped lines is kept. An old fact line is kept with its kind and never selected by a query.
func TestParse(t *testing.T) {
	lines := [][]byte{[]byte(`{"kind":"fact","fact":"reviewed","branch":"main","hash":"h"}`), []byte(`not json`),
		[]byte(`{"key":"x"}`), []byte(`{"kind":"observed","outcome":"finished","key":"polix:reviewer","branch":"main"}`)}
	s, err := Parse(lines, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Entries) != 3 || s.Entries[0].Kind != "fact" || s.Entries[0].Hash != "h" ||
		s.Entries[0].Fact != "reviewed" || s.Entries[1].Key != "x" ||
		s.Entries[1].Kind != "" || s.Entries[2].Key != "polix:reviewer" || s.Skipped != 1 {
		t.Fatalf("state: %+v", s)
	}
	if got := s.Observed(Query{Outcome: OutcomeFinished, Branch: "main"}); len(got) != 1 || got[0].Kind != KindObserved {
		t.Fatalf("the old fact line is ignored: %+v", got)
	}
	if s, err := Parse(nil, 0); err != nil || len(s.Entries) != 0 {
		t.Fatalf("an empty log: %v %+v", err, s)
	}
}

// EncodeLine caps the reason, so no line of the log comes near MaxLine, and the line decodes back.
func TestEncodeLineCapsReason(t *testing.T) {
	line, err := EncodeLine(Entry{At: "2026-01-01T00:00:00Z", Kind: KindDecision, Reason: strings.Repeat("é", 512*1024)})
	if err != nil {
		t.Fatal(err)
	}
	if len(line) > MaxReason+512 || strings.Contains(string(line), "\n") {
		t.Fatalf("the line has %d bytes", len(line))
	}
	st, err := Parse([][]byte{line}, 0)
	if err != nil || len(st.Entries) != 1 {
		t.Fatal(st, err)
	}
	if e := st.Entries[0]; len(e.Reason) > MaxReason || !strings.HasSuffix(e.Reason, "[recortado]") {
		t.Fatalf("reason of %d bytes", len(e.Reason))
	}
}

// The fields keep the names of the log, and empty ones are left out.
func TestEncodeLineFields(t *testing.T) {
	line, err := EncodeLine(Entry{At: "t", Kind: KindDecision, Session: "s", Line: 3})
	if err != nil {
		t.Fatal(err)
	}
	if got := string(line); got != `{"at":"t","kind":"decision","session":"s","line":3}` {
		t.Fatal(got)
	}
	line, err = EncodeLine(Entry{At: "t", Kind: KindObserved, Actor: "polix:reviewer", Hash: "h",
		Key: "polix:reviewer", Outcome: OutcomeFinished})
	if err != nil {
		t.Fatal(err)
	}
	want := `{"at":"t","kind":"observed","actor":"polix:reviewer","hash":"h","key":"polix:reviewer","outcome":"finished"}`
	if got := string(line); got != want {
		t.Fatal(got)
	}
	if _, err := EncodeLine(Entry{}); err == nil {
		t.Fatal("an entry without kind")
	}
}
