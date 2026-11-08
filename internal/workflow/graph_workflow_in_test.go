package workflow

import (
	"bytes"
	"context"
	"errors"
	"testing"
)

// fakeGraphWorkflowPort answers the port of GraphWorkflow from memory.
type fakeGraphWorkflowPort struct {
	wf      Workflow
	found   bool
	err     error
	running string // the version LoadWorkflow was asked for
}

func (f *fakeGraphWorkflowPort) LoadWorkflow(_ context.Context, running string) (Workflow, bool, error) {
	f.running = running
	return f.wf, f.found, f.err
}

func TestGraphWorkflowShell(t *testing.T) {
	port := &fakeGraphWorkflowPort{found: true, wf: Workflow{Version: 1, On: map[string][]Entry{
		PreEdit: {{Action: Action{Block: &Block{Message: "x"}}}}}}}
	res, err := graphWorkflow(context.Background(), port, GraphWorkflowRequest{Version: "1.2.3"})
	if err != nil {
		t.Fatal(err)
	}
	if port.running != "1.2.3" {
		t.Fatalf("running %q", port.running)
	}
	var out bytes.Buffer
	res.print(&out)
	if want := "flowchart TD\n  subgraph pre_edit\n    n0[\"block: x\"]\n  end\n"; out.String() != want || res.Missing {
		t.Fatalf("got %+v", res)
	}
}

func TestGraphWorkflowShellMissingAndErrors(t *testing.T) {
	res, err := graphWorkflow(context.Background(), &fakeGraphWorkflowPort{}, GraphWorkflowRequest{})
	if err != nil || !res.Missing {
		t.Fatalf("missing: %+v %v", res, err)
	}
	boom := errors.New("boom")
	if _, err := graphWorkflow(context.Background(), &fakeGraphWorkflowPort{err: boom}, GraphWorkflowRequest{}); !errors.Is(err, boom) {
		t.Fatalf("error: %v", err)
	}
}
