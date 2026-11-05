package state

import (
	"strings"
	"testing"
)

// Every line that decodes is kept, even without kind; the lines that do not decode are left out, and the count
// of skipped lines is kept.
func TestParse(t *testing.T) {
	lines := [][]byte{[]byte(`{"kind":"fact","fact":"a"}`), []byte(`not json`), []byte(`{"fact":"x"}`),
		[]byte(`{"kind":"fact","fact":"b"}`)}
	s, err := Parse(lines, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Entries) != 3 || s.Entries[0].Fact != "a" || s.Entries[1].Fact != "x" || s.Entries[1].Kind != "" ||
		s.Entries[2].Fact != "b" || s.Skipped != 1 {
		t.Fatalf("state: %+v", s)
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
	line, err := EncodeLine(Entry{At: "t", Kind: KindStopBlock, Session: "s", Line: 3})
	if err != nil {
		t.Fatal(err)
	}
	if got := string(line); got != `{"at":"t","kind":"stop_block","session":"s","line":3}` {
		t.Fatal(got)
	}
	if _, err := EncodeLine(Entry{}); err == nil {
		t.Fatal("an entry without kind")
	}
}
