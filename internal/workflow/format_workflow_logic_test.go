package workflow

import (
	"strings"
	"testing"
)

func TestFormatWorkflowWithoutFile(t *testing.T) {
	if out := FormatWorkflow(nil, false, false); !out.Missing || out.Write != nil || out.Changed {
		t.Fatalf("got %+v", out)
	}
}

func TestFormatWorkflowWritesOnlyAChange(t *testing.T) {
	src := []byte("on:\n    stop: []\nversion: 1\n")
	out := FormatWorkflow(src, true, false)
	if !out.Changed || string(out.Write) != "version: 1\non:\n  stop: []\n" || out.Problem != "" {
		t.Fatalf("got %+v %q", out, out.Write)
	}
	if out := FormatWorkflow(out.Write, true, false); out.Changed || out.Write != nil {
		t.Fatalf("formatted: %+v", out)
	}
}

// With check the change is told but nothing is written.
func TestFormatWorkflowCheck(t *testing.T) {
	if out := FormatWorkflow([]byte("version:   1\n"), true, true); !out.Changed || out.Write != nil {
		t.Fatalf("got %+v", out)
	}
}

// A polix.yaml that cannot be formatted is a problem, never written.
func TestFormatWorkflowProblem(t *testing.T) {
	out := FormatWorkflow([]byte("version: [\n"), true, false)
	if out.Problem == "" || out.Write != nil || out.Changed {
		t.Fatalf("got %+v", out)
	}
	out = FormatWorkflow([]byte("a: &x 1\nb: *x\n"), true, false)
	if !strings.Contains(out.Problem, "anchors") || out.Write != nil {
		t.Fatalf("got %+v", out)
	}
}
