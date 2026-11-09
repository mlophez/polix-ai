package load

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/mlophez/polix-ai/internal/app/rule"
)

// fakePort answers the reading of the rules with what it is given, and records each folder it was asked for.
type fakePort struct {
	rules  []rule.Rule
	exists bool
	err    error
	asked  []string
}

func (f *fakePort) ReadRules(_ context.Context, root, dir string) ([]rule.Rule, bool, error) {
	f.asked = append(f.asked, root+" "+dir)
	return f.rules, f.exists, f.err
}

// The shell reads the folder of the request through the port and builds the response from the logic.
func TestRun(t *testing.T) {
	port := &fakePort{rules: []rule.Rule{{ID: "B-1", Description: "d"}, {ID: "A-1"}}, exists: true}
	res, err := run(context.Background(), port, Request{Root: "/repo", Dir: "ai/rules"})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(port.asked, []string{"/repo ai/rules"}) {
		t.Fatalf("asked %v", port.asked)
	}
	if l := res.Loaded; len(l.Rules) != 2 || l.Rules[0].ID != "A-1" || len(l.Problems) != 1 || !l.Exists ||
		l.Strict == nil {
		t.Fatalf("loaded %+v", l)
	}
}

// A folder that cannot be read is data of the response, never an error of the shell.
func TestRunNeverFails(t *testing.T) {
	boom := errors.New("boom")
	res, err := run(context.Background(), &fakePort{exists: true, err: boom}, Request{Root: "/repo", Dir: "ai/rules"})
	if err != nil || !errors.Is(res.Loaded.Err, boom) || !errors.Is(res.Loaded.Strict, boom) {
		t.Fatalf("got %+v %v", res, err)
	}
}

// The loader reads a folder once for the life of the process: a second call for the same root and folder returns
// the same result without reading again; another folder, or another root, is read.
func TestLoaderReadsOnce(t *testing.T) {
	port := &fakePort{rules: []rule.Rule{{ID: "A-1", Description: "d"}}, exists: true}
	l := &loader{port: port, seen: map[Request]rule.Loaded{}}
	ctx := context.Background()
	first := l.load(ctx, "/repo", "ai/rules")
	port.rules = nil
	second := l.load(ctx, "/repo", "ai/rules")
	if len(first.Rules) != 1 || len(second.Rules) != 1 || len(port.asked) != 1 {
		t.Fatalf("first %+v, second %+v, asked %v", first, second, port.asked)
	}
	l.load(ctx, "/repo", "governance/rules")
	l.load(ctx, "/other", "ai/rules")
	if want := []string{"/repo ai/rules", "/repo governance/rules", "/other ai/rules"}; !slices.Equal(port.asked, want) {
		t.Fatalf("asked %v, want %v", port.asked, want)
	}
}

// Command returns a loader, ready to be called.
func TestCommand(t *testing.T) {
	load := Command()
	if got := load(context.Background(), t.TempDir(), "ai/rules"); got.Exists || got.Rules != nil || got.Err != nil {
		t.Fatalf("a root without the folder: %+v", got)
	}
}
