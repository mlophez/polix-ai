package text

import (
	"strings"
	"testing"
)

func TestCut(t *testing.T) {
	if got := Cut("short", 100); got != "short" {
		t.Fatalf("Cut of a short text = %q", got)
	}
	got := Cut(strings.Repeat("é", 512*1024), 8*1024)
	if len(got) > 8*1024 || !strings.HasSuffix(got, "[truncated]") {
		t.Fatalf("Cut of %d bytes", len(got))
	}
	if got := strings.TrimSuffix(got, Mark); !strings.HasSuffix(got, "é") {
		t.Fatalf("Cut splits a rune: %q", got[len(got)-4:])
	}
	if got := Cut("abcdef", 2); got != Mark {
		t.Fatalf("Cut below the mark = %q", got)
	}
}

// Tail cuts long lines, so a huge output never reaches the log whole.
func TestTailCutsLines(t *testing.T) {
	out := Tail("a\n"+strings.Repeat("x", 10000)+"\n", 15, 500)
	if len(out) > 500+2 || !strings.HasPrefix(out, "a\nx") {
		t.Fatalf("tail of %d bytes", len(out))
	}
}

func TestTailSkipsEmptyLines(t *testing.T) {
	if got := Tail("a\n\n  \nb\nc\n", 2, 500); got != "b\nc" {
		t.Fatalf("Tail = %q", got)
	}
}

func TestLastLines(t *testing.T) {
	if got := LastLines("a\n\nb\nc\n\n", 3, 500); got != "\nb\nc" {
		t.Fatalf("LastLines = %q", got)
	}
	if got := LastLines("abcdef", 3, 2); got != Mark {
		t.Fatalf("LastLines cut = %q", got)
	}
}
