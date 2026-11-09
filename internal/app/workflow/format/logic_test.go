package format

import (
	"strings"
	"testing"
)

func TestDecideWithoutFile(t *testing.T) {
	if out := Decide(nil, false, false); !out.Missing || out.Write != nil || out.Changed {
		t.Fatalf("got %+v", out)
	}
}

func TestDecideWritesOnlyAChange(t *testing.T) {
	src := []byte("on:\n    stop: []\nversion: 1\n")
	out := Decide(src, true, false)
	if !out.Changed || string(out.Write) != "version: 1\non:\n  stop: []\n" || out.Problem != "" {
		t.Fatalf("got %+v %q", out, out.Write)
	}
	if out := Decide(out.Write, true, false); out.Changed || out.Write != nil {
		t.Fatalf("formatted: %+v", out)
	}
}

// With check the change is told but nothing is written.
func TestDecideCheck(t *testing.T) {
	if out := Decide([]byte("version:   1\n"), true, true); !out.Changed || out.Write != nil {
		t.Fatalf("got %+v", out)
	}
}

// A polix.yaml that cannot be formatted is a problem, never written.
func TestDecideProblem(t *testing.T) {
	out := Decide([]byte("version: [\n"), true, false)
	if out.Problem == "" || out.Write != nil || out.Changed {
		t.Fatalf("got %+v", out)
	}
	out = Decide([]byte("a: &x 1\nb: *x\n"), true, false)
	if !strings.Contains(out.Problem, "anchors") || out.Write != nil {
		t.Fatalf("got %+v", out)
	}
}
