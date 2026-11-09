package format

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
)

// fakePort answers the port of FormatWorkflow from memory and records what was written.
type fakePort struct {
	data     []byte
	found    bool
	readErr  error
	writeErr error
	written  []byte
}

func (f *fakePort) ReadWorkflow(_ context.Context) ([]byte, bool, error) {
	return f.data, f.found, f.readErr
}

func (f *fakePort) WriteWorkflow(_ context.Context, data []byte) error {
	f.written = data
	return f.writeErr
}

func TestShellWrites(t *testing.T) {
	port := &fakePort{data: []byte("version:   1\n"), found: true}
	res, err := run(context.Background(), port, Request{})
	if err != nil || !res.Written || !res.Changed || string(port.written) != "version: 1\n" {
		t.Fatalf("got %+v %q %v", res, port.written, err)
	}
	var out, errb bytes.Buffer
	if code := res.print(&out, &errb); code != 0 || !strings.Contains(out.String(), "formatted polix.yaml") {
		t.Fatalf("print: %d %q %q", code, out.String(), errb.String())
	}
}

func TestShellCheck(t *testing.T) {
	port := &fakePort{data: []byte("version:   1\n"), found: true}
	res, err := run(context.Background(), port, Request{Check: true})
	if err != nil || res.Written || !res.Changed || port.written != nil {
		t.Fatalf("got %+v %v", res, err)
	}
	var out, errb bytes.Buffer
	if code := res.print(&out, &errb); code != 1 || !strings.Contains(errb.String(), "not formatted") {
		t.Fatalf("print: %d %q", code, errb.String())
	}
	port = &fakePort{data: []byte("version: 1\n"), found: true}
	res, _ = run(context.Background(), port, Request{Check: true})
	out.Reset()
	if code := res.print(&out, &errb); code != 0 || !strings.Contains(out.String(), "is formatted") {
		t.Fatalf("formatted: %d %q", code, out.String())
	}
}

func TestShellMissingAndProblem(t *testing.T) {
	res, err := run(context.Background(), &fakePort{}, Request{})
	var out, errb bytes.Buffer
	if err != nil || !res.Missing || res.print(&out, &errb) != 1 || !strings.Contains(errb.String(), "not found") {
		t.Fatalf("missing: %+v %v %q", res, err, errb.String())
	}
	port := &fakePort{data: []byte("version: [\n"), found: true}
	res, err = run(context.Background(), port, Request{})
	errb.Reset()
	if err != nil || res.Problem == "" || port.written != nil || res.print(&out, &errb) != 1 {
		t.Fatalf("problem: %+v %v", res, err)
	}
}

func TestShellErrors(t *testing.T) {
	boom := errors.New("boom")
	if _, err := run(context.Background(), &fakePort{readErr: boom},
		Request{}); !errors.Is(err, boom) {
		t.Fatalf("read: %v", err)
	}
	port := &fakePort{data: []byte("version:   1\n"), found: true, writeErr: boom}
	if _, err := run(context.Background(), port, Request{}); !errors.Is(err, boom) {
		t.Fatalf("write: %v", err)
	}
}
