package measure

import (
	"bytes"
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/mlophez/polix-ai/internal/app/diff"
	"github.com/mlophez/polix-ai/internal/app/workflow"
	workflowload "github.com/mlophez/polix-ai/internal/app/workflow/load"
)

// fakePort answers the port of MeasureDiff from memory and records the calls.
type fakePort struct {
	weights   []workflow.Weight
	loadErr   error
	diff      diff.Diff
	diffErr   error
	commitErr error
	calls     []string
}

func (p *fakePort) LoadWorkflow(context.Context, string) (workflow.Workflow, error) {
	return workflow.Workflow{Version: 1, Weights: p.weights}, p.loadErr
}

func (p *fakePort) Diff(context.Context) (diff.Diff, error) {
	p.calls = append(p.calls, "Diff")
	return p.diff, p.diffErr
}

func (p *fakePort) CommitDiff(_ context.Context, rev string) (diff.Diff, string, error) {
	p.calls = append(p.calls, "CommitDiff "+rev)
	return p.diff, "abc1234 the subject", p.commitErr
}

var sample = diff.Diff{Changes: []diff.Change{
	{Path: "b.go", Added: 1},
	{Path: "a.go", Added: 3, Deleted: 4},
	{Path: "logo.png", Binary: true},
	{Path: "notes.md", Added: 1},
}}

// The uncommitted diff is measured without a revision, and printed with the totals and one line per file.
func TestShellUncommitted(t *testing.T) {
	port := &fakePort{diff: sample, weights: []workflow.Weight{{Glob: "b.go", Factor: 2}}}
	res, err := run(context.Background(), port, Request{Changed: []string{"*.go", "*.png"}})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(port.calls, []string{"Diff"}) {
		t.Fatalf("calls %q", port.calls)
	}
	var out bytes.Buffer
	res.print(&out)
	want := "uncommitted changes\n\n" +
		"changed_lines  8\nchanged_files  3\nrisk_score     5\n\n" +
		"  added deleted  weight   score  path\n" +
		"      3       4       1    3.00  a.go\n" +
		"      1       0       2    2.00  b.go\n" +
		"      -       -       1    0.00  logo.png\n"
	if out.String() != want {
		t.Fatalf("got:\n%s\nwant:\n%s", out.String(), want)
	}
}

// A revision measures that commit, and names it.
func TestShellCommit(t *testing.T) {
	port := &fakePort{diff: sample}
	res, err := run(context.Background(), port, Request{Commit: "HEAD~1"})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(port.calls, []string{"CommitDiff HEAD~1"}) || res.Commit != "abc1234 the subject" || res.Files != 4 {
		t.Fatalf("calls %q, response %+v", port.calls, res)
	}
	var out bytes.Buffer
	res.print(&out)
	if !strings.HasPrefix(out.String(), "commit abc1234 the subject\n") {
		t.Fatalf("got %q", out.String())
	}
}

// Nothing to measure prints zeros and no files.
func TestShellEmpty(t *testing.T) {
	res, err := run(context.Background(), &fakePort{}, Request{})
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	res.print(&out)
	if want := "uncommitted changes\n\nchanged_lines  0\nchanged_files  0\nrisk_score     0\n"; out.String() != want {
		t.Fatalf("got %q", out.String())
	}
}

// Each failure of the port is the error of the use case.
func TestShellErrors(t *testing.T) {
	boom := errors.New("boom")
	for name, port := range map[string]*fakePort{
		"workflow": {loadErr: boom},
		"diff":     {diffErr: boom},
	} {
		if _, err := run(context.Background(), port, Request{}); !errors.Is(err, boom) {
			t.Errorf("%s: %v", name, err)
		}
	}
	if _, err := run(context.Background(), &fakePort{commitErr: boom}, Request{Commit: "x"}); !errors.Is(err, boom) {
		t.Errorf("commit: %v", err)
	}
}

// An empty --commit given explicitly is rejected before the shell runs.
func TestCommandRejectsEmptyCommit(t *testing.T) {
	c := Command(workflowload.Command())
	c.SetArgs([]string{"--commit", ""})
	c.SetOut(&bytes.Buffer{})
	c.SetErr(&bytes.Buffer{})
	if err := c.Execute(); err == nil || !strings.Contains(err.Error(), "--commit needs a revision") {
		t.Fatalf("got %v", err)
	}
}
