package load

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/mlophez/polix-ai/internal/app/workflow"
)

// fakePort answers the reading of polix.yaml with what it is given, and records each root and version it was asked
// for.
type fakePort struct {
	wf     workflow.Workflow
	exists bool
	err    error
	asked  []string
}

func (f *fakePort) ReadWorkflow(_ context.Context, root, running string) (workflow.Workflow, workflow.Guardrails,
	bool, error) {
	f.asked = append(f.asked, root+" "+running)
	return f.wf, workflow.Guardrails{}, f.exists, f.err
}

// The shell reads polix.yaml of the request through the port and builds the response from the logic.
func TestRun(t *testing.T) {
	port := &fakePort{wf: workflow.Workflow{Version: 1, MaxStopRetries: 5}, exists: true}
	res, err := run(context.Background(), port, Request{Root: "/repo", Running: "0.2.0"})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(port.asked, []string{"/repo 0.2.0"}) {
		t.Fatalf("asked %v", port.asked)
	}
	if l := res.Loaded; !l.Exists || l.Err != nil || l.Workflow.MaxStopRetries != 5 {
		t.Fatalf("loaded %+v", l)
	}
}

// A file that cannot be read is data of the response, never an error of the shell.
func TestRunNeverFails(t *testing.T) {
	boom := errors.New("boom")
	res, err := run(context.Background(), &fakePort{exists: true, err: boom}, Request{Root: "/repo", Running: "dev"})
	if err != nil || !errors.Is(res.Loaded.Err, boom) || !res.Loaded.Exists {
		t.Fatalf("got %+v %v", res, err)
	}
}

// The loader reads polix.yaml once for the life of the process: a second call for the same root and version returns
// the same result without reading again; another root, or another version, is read.
func TestLoaderReadsOnce(t *testing.T) {
	port := &fakePort{wf: workflow.Workflow{Version: 1, MaxStopRetries: 5}, exists: true}
	l := &loader{port: port, seen: map[Request]workflow.Loaded{}}
	ctx := context.Background()
	first := l.load(ctx, "/repo", "dev")
	port.wf = workflow.Workflow{Version: 1, MaxStopRetries: 7}
	second := l.load(ctx, "/repo", "dev")
	if first.Workflow.MaxStopRetries != 5 || second.Workflow.MaxStopRetries != 5 || len(port.asked) != 1 {
		t.Fatalf("first %+v, second %+v, asked %v", first, second, port.asked)
	}
	l.load(ctx, "/repo", "0.2.0")
	l.load(ctx, "/other", "dev")
	if want := []string{"/repo dev", "/repo 0.2.0", "/other dev"}; !slices.Equal(port.asked, want) {
		t.Fatalf("asked %v, want %v", port.asked, want)
	}
}

// Command returns a loader, ready to be called.
func TestCommand(t *testing.T) {
	load := Command()
	if got := load(context.Background(), t.TempDir(), "dev"); got.Exists || got.Err != nil || got.Workflow.Version != 1 {
		t.Fatalf("a root without polix.yaml: %+v", got)
	}
}
