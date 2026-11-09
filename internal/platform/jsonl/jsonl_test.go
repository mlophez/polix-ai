package jsonl

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

const limit = 4 * 1024 * 1024

// writeFile writes lines, each ended by a newline, to a new file and returns its path.
func writeFile(t *testing.T, lines ...string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "log.jsonl")
	if err := os.WriteFile(p, []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

// A line over the limit is skipped and counted, and the lines around it still load.
func TestReadSkipsHugeLines(t *testing.T) {
	huge := `{"kind":"decision","reason":"` + strings.Repeat("x", 5*1024*1024) + `"}`
	p := writeFile(t, `{"fact":"a"}`, huge, `{"fact":"b"}`)
	lines, skipped, err := Read(p, limit)
	if err != nil {
		t.Fatal(err)
	}
	if len(lines) != 2 || string(lines[0]) != `{"fact":"a"}` || string(lines[1]) != `{"fact":"b"}` {
		t.Fatalf("lines: %q", lines)
	}
	if skipped != 1 {
		t.Fatalf("skipped %d lines, want 1", skipped)
	}
}

// A line just over a small limit is skipped and counted; one at the limit is kept.
func TestReadLimit(t *testing.T) {
	p := writeFile(t, "12345", "123456", "1234")
	lines, skipped, err := Read(p, 5)
	if err != nil || skipped != 1 || len(lines) != 2 || string(lines[0]) != "12345" || string(lines[1]) != "1234" {
		t.Fatalf("lines %q, skipped %d, err %v", lines, skipped, err)
	}
}

// A file without a final newline still gives its last line.
func TestReadLastLineWithoutNewline(t *testing.T) {
	p := filepath.Join(t.TempDir(), "log.jsonl")
	if err := os.WriteFile(p, []byte(`{"fact":"a"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	lines, _, err := Read(p, limit)
	if err != nil || len(lines) != 1 {
		t.Fatalf("lines %q, err %v", lines, err)
	}
}

// A file that does not exist has no lines and no error.
func TestReadMissing(t *testing.T) {
	lines, skipped, err := Read(filepath.Join(t.TempDir(), "none.jsonl"), limit)
	if err != nil || lines != nil || skipped != 0 {
		t.Fatalf("lines %q, skipped %d, err %v", lines, skipped, err)
	}
}

// A file that cannot be read is an error.
func TestReadUnreadable(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("file modes do not deny reading here")
	}
	p := writeFile(t, `{"fact":"a"}`)
	if err := os.Chmod(p, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(p, 0o644) })
	if _, _, err := Read(p, limit); err == nil {
		t.Fatal("want the error of the unreadable file")
	}
}

// Append creates the folder and the file and adds one line per call.
func TestAppend(t *testing.T) {
	p := filepath.Join(t.TempDir(), "polix", "log.jsonl")
	for _, l := range []string{`{"a":1}`, `{"b":2}`} {
		if err := Append(p, []byte(l)); err != nil {
			t.Fatal(err)
		}
	}
	data, err := os.ReadFile(p)
	if err != nil || string(data) != "{\"a\":1}\n{\"b\":2}\n" {
		t.Fatalf("file %q, err %v", data, err)
	}
}
