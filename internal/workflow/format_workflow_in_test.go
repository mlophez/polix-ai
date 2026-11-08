package workflow

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
)

// fakeFormatWorkflowPort answers the port of FormatWorkflow from memory and records what was written.
type fakeFormatWorkflowPort struct {
	data     []byte
	found    bool
	readErr  error
	writeErr error
	written  []byte
}

func (f *fakeFormatWorkflowPort) ReadWorkflow(_ context.Context) ([]byte, bool, error) {
	return f.data, f.found, f.readErr
}

func (f *fakeFormatWorkflowPort) WriteWorkflow(_ context.Context, data []byte) error {
	f.written = data
	return f.writeErr
}

func TestFormatWorkflowShellWrites(t *testing.T) {
	port := &fakeFormatWorkflowPort{data: []byte("version:   1\n"), found: true}
	res, err := formatWorkflow(context.Background(), port, FormatWorkflowRequest{})
	if err != nil || !res.Written || !res.Changed || string(port.written) != "version: 1\n" {
		t.Fatalf("got %+v %q %v", res, port.written, err)
	}
	var out, errb bytes.Buffer
	if code := res.print(&out, &errb); code != 0 || !strings.Contains(out.String(), "formatted polix.yaml") {
		t.Fatalf("print: %d %q %q", code, out.String(), errb.String())
	}
}

func TestFormatWorkflowShellCheck(t *testing.T) {
	port := &fakeFormatWorkflowPort{data: []byte("version:   1\n"), found: true}
	res, err := formatWorkflow(context.Background(), port, FormatWorkflowRequest{Check: true})
	if err != nil || res.Written || !res.Changed || port.written != nil {
		t.Fatalf("got %+v %v", res, err)
	}
	var out, errb bytes.Buffer
	if code := res.print(&out, &errb); code != 1 || !strings.Contains(errb.String(), "not formatted") {
		t.Fatalf("print: %d %q", code, errb.String())
	}
	port = &fakeFormatWorkflowPort{data: []byte("version: 1\n"), found: true}
	res, _ = formatWorkflow(context.Background(), port, FormatWorkflowRequest{Check: true})
	out.Reset()
	if code := res.print(&out, &errb); code != 0 || !strings.Contains(out.String(), "is formatted") {
		t.Fatalf("formatted: %d %q", code, out.String())
	}
}

func TestFormatWorkflowShellMissingAndProblem(t *testing.T) {
	res, err := formatWorkflow(context.Background(), &fakeFormatWorkflowPort{}, FormatWorkflowRequest{})
	var out, errb bytes.Buffer
	if err != nil || !res.Missing || res.print(&out, &errb) != 1 || !strings.Contains(errb.String(), "not found") {
		t.Fatalf("missing: %+v %v %q", res, err, errb.String())
	}
	port := &fakeFormatWorkflowPort{data: []byte("version: [\n"), found: true}
	res, err = formatWorkflow(context.Background(), port, FormatWorkflowRequest{})
	errb.Reset()
	if err != nil || res.Problem == "" || port.written != nil || res.print(&out, &errb) != 1 {
		t.Fatalf("problem: %+v %v", res, err)
	}
}

func TestFormatWorkflowShellErrors(t *testing.T) {
	boom := errors.New("boom")
	if _, err := formatWorkflow(context.Background(), &fakeFormatWorkflowPort{readErr: boom},
		FormatWorkflowRequest{}); !errors.Is(err, boom) {
		t.Fatalf("read: %v", err)
	}
	port := &fakeFormatWorkflowPort{data: []byte("version:   1\n"), found: true, writeErr: boom}
	if _, err := formatWorkflow(context.Background(), port, FormatWorkflowRequest{}); !errors.Is(err, boom) {
		t.Fatalf("write: %v", err)
	}
}
