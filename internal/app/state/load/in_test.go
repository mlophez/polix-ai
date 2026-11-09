package load

import (
	"context"
	"errors"
	"path/filepath"
	"slices"
	"testing"

	"github.com/mlophez/polix-ai/internal/app/state"
)

// fakePort answers the reading of the log with what it is given, and records each file it was asked for.
type fakePort struct {
	entries []state.Entry
	skipped int
	err     error
	asked   []string
}

func (f *fakePort) ReadLog(_ context.Context, path string) ([]state.Entry, int, error) {
	f.asked = append(f.asked, path)
	return f.entries, f.skipped, f.err
}

// The shell reads the file of the request through the port and builds the response from the logic.
func TestRun(t *testing.T) {
	port := &fakePort{entries: []state.Entry{{Kind: state.KindDecision}}, skipped: 1}
	res, err := run(context.Background(), port, Request{Path: "/repo/.git/polix/log.jsonl"})
	if err != nil || !slices.Equal(port.asked, []string{"/repo/.git/polix/log.jsonl"}) {
		t.Fatal(port.asked, err)
	}
	if len(res.Loaded.State.Entries) != 1 || res.Loaded.State.Skipped != 1 || res.Loaded.Err != nil {
		t.Fatalf("loaded %+v", res.Loaded)
	}
}

// A log that cannot be read is data of the response, never an error of the shell.
func TestRunNeverFails(t *testing.T) {
	boom := errors.New("boom")
	res, err := run(context.Background(), &fakePort{err: boom}, Request{Path: "/repo/.git/polix/log.jsonl"})
	if err != nil || !errors.Is(res.Loaded.Err, boom) || len(res.Loaded.State.Entries) != 0 {
		t.Fatalf("got %+v %v", res, err)
	}
}

// The loader reads the log at every call: the process that reads it also appends to it.
func TestLoaderReadsEveryTime(t *testing.T) {
	port := &fakePort{entries: []state.Entry{{Kind: state.KindDecision}}}
	l := loader{port: port}
	ctx := context.Background()
	first := l.load(ctx, "/a/log.jsonl")
	port.entries = append(port.entries, state.Entry{Kind: state.KindDecision})
	second := l.load(ctx, "/a/log.jsonl")
	if len(first.State.Entries) != 1 || len(second.State.Entries) != 2 || len(port.asked) != 2 {
		t.Fatalf("first %+v, second %+v, asked %v", first, second, port.asked)
	}
}

// Command returns a loader, ready to be called.
func TestCommand(t *testing.T) {
	load := Command()
	got := load(context.Background(), filepath.Join(t.TempDir(), "log.jsonl"))
	if len(got.State.Entries) != 0 || got.State.Skipped != 0 || got.Err != nil {
		t.Fatalf("no file: %+v", got)
	}
}
