package workflow

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"github.com/mlophez/polix-ai/internal/state"
)

// fakeLogWorkflowPort answers the port of LogWorkflow from memory.
type fakeLogWorkflowPort struct {
	st  state.State
	err error
}

func (f fakeLogWorkflowPort) ReadLog(context.Context) (state.State, error) { return f.st, f.err }

var logWorkflowEntries = []state.Entry{
	{At: "t1", Kind: state.KindFact, Event: "stop", Actor: "main", Fact: "reviewed", Hash: "0123456789abcdef"},
	{At: "t2", Kind: state.KindDecision, Event: "pre_edit", Actor: "builder", Action: "deny", Outcome: "deny",
		Line: 4, Reason: "first\nsecond"},
}

func TestLogWorkflowShellPrintsText(t *testing.T) {
	port := fakeLogWorkflowPort{st: state.State{Entries: logWorkflowEntries, Skipped: 1}}
	res, err := logWorkflow(context.Background(), port, LogWorkflowRequest{Tail: 20})
	if err != nil {
		t.Fatal(err)
	}
	var out, errb bytes.Buffer
	res.print(&out, &errb, false)
	want := "t1  fact       stop          main      reviewed         01234567  \n" +
		"t2  decision   pre_edit      builder   deny deny          first\n"
	if out.String() != want {
		t.Fatalf("got:\n%q\nwant:\n%q", out.String(), want)
	}
	if want := "polix: 1 lines of the log are longer than 4194304 bytes and were skipped\n"; errb.String() != want {
		t.Fatalf("stderr %q", errb.String())
	}
}

func TestLogWorkflowShellPrintsJSON(t *testing.T) {
	port := fakeLogWorkflowPort{st: state.State{Entries: logWorkflowEntries}}
	res, err := logWorkflow(context.Background(), port, LogWorkflowRequest{Tail: 1, JSON: true})
	if err != nil {
		t.Fatal(err)
	}
	var out, errb bytes.Buffer
	res.print(&out, &errb, true)
	want := `{"at":"t2","kind":"decision","event":"pre_edit","actor":"builder","action":"deny","line":4,` +
		`"outcome":"deny","reason":"first\nsecond"}` + "\n"
	if out.String() != want || errb.Len() != 0 {
		t.Fatalf("got %q %q", out.String(), errb.String())
	}
}

func TestLogWorkflowShellError(t *testing.T) {
	boom := errors.New("boom")
	if _, err := logWorkflow(context.Background(), fakeLogWorkflowPort{err: boom}, LogWorkflowRequest{}); !errors.Is(err, boom) {
		t.Fatalf("got %v", err)
	}
}
