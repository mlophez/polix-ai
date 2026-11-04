package state

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// writeLog writes lines as the log of a new store.
func writeLog(t *testing.T, lines ...string) *Store {
	t.Helper()
	s := Open(t.TempDir())
	if err := os.MkdirAll(s.Dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(s.path(), []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return s
}

// A line over MaxLine is skipped and counted, and the entries around it still load.
func TestEntriesSkipsHugeLines(t *testing.T) {
	huge := `{"kind":"decision","reason":"` + strings.Repeat("x", 5*1024*1024) + `"}`
	s := writeLog(t, `{"kind":"fact","fact":"a"}`, huge, `{"kind":"fact","fact":"b"}`)
	entries, err := s.Entries()
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 || entries[0].Fact != "a" || entries[1].Fact != "b" {
		t.Fatalf("entries: %+v", entries)
	}
	if s.Skipped() != 1 {
		t.Fatalf("skipped %d lines, want 1", s.Skipped())
	}
}

// A log without a final newline still loads its last entry.
func TestEntriesLastLineWithoutNewline(t *testing.T) {
	s := Open(t.TempDir())
	_ = os.MkdirAll(s.Dir, 0o755)
	_ = os.WriteFile(s.path(), []byte(`{"kind":"fact","fact":"a"}`), 0o644)
	entries, err := s.Entries()
	if err != nil || len(entries) != 1 {
		t.Fatalf("entries %+v, err %v", entries, err)
	}
}

// Append caps the reason, so no line of the log comes near MaxLine.
func TestAppendCapsReason(t *testing.T) {
	s := Open(t.TempDir())
	if err := s.Append(Entry{Kind: KindDecision, Reason: strings.Repeat("é", 512*1024)}); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(s.path())
	if err != nil {
		t.Fatal(err)
	}
	if len(data) > MaxReason+512 {
		t.Fatalf("the line has %d bytes", len(data))
	}
	entries, _ := Open(filepath.Dir(s.Dir)).Entries()
	if len(entries) != 1 || len(entries[0].Reason) > MaxReason || !strings.HasSuffix(entries[0].Reason, "[recortado]") {
		t.Fatalf("reason of %d bytes", len(entries[0].Reason))
	}
}

// StopBlocks returns the error of a log it cannot read, so the loop guard can let the agent stop.
func TestStopBlocksUnreadable(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("file modes do not deny reading here")
	}
	s := writeLog(t, `{"kind":"stop_block"}`)
	if err := os.Chmod(s.path(), 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(s.path(), 0o644) })
	if _, err := s.StopBlocks("s", "stop", "session", 1, ""); err == nil {
		t.Fatal("want the error of the unreadable log")
	}
}

// Done never holds for an unknown hash.
func TestDoneUnknownHash(t *testing.T) {
	s := writeLog(t, `{"kind":"fact","fact":"tested","branch":"main"}`)
	if s.Done("tested", "main", "") {
		t.Fatal("done holds for an unknown hash")
	}
}
