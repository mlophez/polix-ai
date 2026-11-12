package graph

import (
	"bytes"
	"context"
	"errors"
	"os"
	"testing"

	"github.com/mlophez/polix-ai/internal/app/workflow"
	"github.com/mlophez/polix-ai/internal/platform/testenv"
)

// fakePort answers the port of GraphWorkflow from memory.
type fakePort struct {
	wf      workflow.Workflow
	found   bool
	err     error
	running string // the version LoadWorkflow was asked for
}

// TestMain runs the tests without the variables git exports to its hooks (see testenv.ClearGitHookEnv): the githooks
// of polix.yaml run go test, and these tests run git in repositories of their own.
func TestMain(m *testing.M) {
	testenv.ClearGitHookEnv()
	os.Exit(m.Run())
}

func (f *fakePort) LoadWorkflow(_ context.Context, running string) (workflow.Workflow, bool, error) {
	f.running = running
	return f.wf, f.found, f.err
}

func TestShell(t *testing.T) {
	port := &fakePort{found: true, wf: workflow.Workflow{Version: 1, On: map[string][]workflow.Entry{
		workflow.PreEdit: {{Action: workflow.Action{Block: &workflow.Block{Message: "x"}}}}}}}
	res, err := run(context.Background(), port, Request{Version: "1.2.3"})
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

func TestShellMissingAndErrors(t *testing.T) {
	res, err := run(context.Background(), &fakePort{}, Request{})
	if err != nil || !res.Missing {
		t.Fatalf("missing: %+v %v", res, err)
	}
	boom := errors.New("boom")
	if _, err := run(context.Background(), &fakePort{err: boom}, Request{}); !errors.Is(err, boom) {
		t.Fatalf("error: %v", err)
	}
}
