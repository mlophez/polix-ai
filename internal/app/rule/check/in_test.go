package check

import (
	"bytes"
	"context"
	"errors"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/mlophez/polix-ai/internal/app/rule"
	"github.com/mlophez/polix-ai/internal/app/workflow"
	"github.com/mlophez/polix-ai/internal/platform/cli"
)

// The shell is tested with a fake of the port: no disk, git nor processes.

// fakePort answers the port of CheckRule from memory and records the methods called.
type fakePort struct {
	rules      []rule.Rule
	wf         workflow.Workflow
	changed    []string
	rulesErr   error            // error of reading the rules
	wfErr      error            // error of reading polix.yaml
	changedErr error            // error of listing the changed files, as when no base branch exists
	fails      map[string]error // error of the run of each check that fails
	calls      []string         // the methods called, in order
	candidates []string         // the bases Changed was asked for
	argv       map[string][]string
}

func (f *fakePort) LoadRules(_ context.Context, dir string) ([]rule.Rule, error) {
	f.calls = append(f.calls, "LoadRules "+dir)
	return f.rules, f.rulesErr
}

func (f *fakePort) LoadWorkflow(_ context.Context, _ string) (workflow.Workflow, error) {
	f.calls = append(f.calls, "LoadWorkflow")
	return f.wf, f.wfErr
}

func (f *fakePort) Changed(_ context.Context, candidates []string) ([]string, error) {
	f.calls = append(f.calls, "Changed")
	f.candidates = candidates
	return f.changed, f.changedErr
}

func (f *fakePort) RunCheck(_ context.Context, run rule.Run) (string, error) {
	var checks []string
	var err error
	for _, r := range run.Rules {
		checks = append(checks, r.Check)
		err = errors.Join(err, f.fails[r.Check])
	}
	check := strings.Join(checks, " ")
	f.calls = append(f.calls, "RunCheck "+check)
	if f.argv == nil {
		f.argv = map[string][]string{}
	}
	f.argv[check] = run.Argv
	return "ran " + check, err
}

// batchShellFlow runs the Go tests of a folder together, selected by their names.
var batchShellFlow = workflow.Workflow{Rules: workflow.Rules{Run: []workflow.Runner{
	{Glob: "*_test.go", Argv: []string{"go", "test", "{dir}", "-run", "^Test({names})$"}},
}}}

// batchShellRules are three rules whose checks are tests of one folder.
var batchShellRules = []rule.Rule{
	{ID: "A-1", Description: "a1", Deterministic: true, Check: "t/a1_test.go"},
	{ID: "A-2", Description: "a2", Deterministic: true, Check: "t/a2_test.go"},
	{ID: "A-3", Description: "a3", Deterministic: true, Check: "t/a3_test.go"},
}

func TestShellRunsAPassingGroupOnce(t *testing.T) {
	port := &fakePort{rules: batchShellRules, wf: batchShellFlow}
	res, err := run(context.Background(), port, Request{})
	if err != nil || res.Ran != 3 || len(res.Failed) != 0 {
		t.Fatalf("got %+v, %v", res, err)
	}
	if want := []string{"LoadWorkflow", "LoadRules ai/rules", "RunCheck t/a1_test.go t/a2_test.go t/a3_test.go"}; !slices.Equal(port.calls, want) {
		t.Fatalf("called %v, want %v", port.calls, want)
	}
}

func TestShellRerunsAFailedGroupPerRule(t *testing.T) {
	port := &fakePort{rules: batchShellRules, wf: batchShellFlow,
		fails: map[string]error{"t/a2_test.go": errors.New("exit 1")}}
	res, err := run(context.Background(), port, Request{})
	if err != nil || res.Ran != 3 {
		t.Fatalf("got %+v, %v", res, err)
	}
	if len(port.calls) != 2+1+3 {
		t.Fatalf("one shared run and one per rule: %v", port.calls)
	}
	if len(res.Failed) != 1 || res.Failed[0].ID != "A-2" || res.Failed[0].Output != "ran t/a2_test.go\nexit 1" {
		t.Fatalf("only the failing rule, with its own output: %+v", res.Failed)
	}
}

// shellRules are two rules with a check and one without.
var shellRules = []rule.Rule{
	{ID: "go", Description: "gofmt", Scope: []string{"**/*.go"}, Deterministic: true, Check: "ai/checks/gofmt_test.go"},
	{ID: "sh", Description: "shfmt", Scope: []string{"**/*.sh"}, Deterministic: true, Check: "ai/checks/shfmt.sh"},
	{ID: "prose", Description: "no check"},
}

// developBase is a workflow whose base is develop and whose runner runs the Go tests.
var developBase = workflow.Workflow{Base: []string{"develop"}, Rules: workflow.Rules{Run: []workflow.Runner{
	{Glob: "*_test.go", Argv: []string{"go", "test", "{dir}"}},
}}}

func TestShellRunsTheChecksAndReportsTheFailures(t *testing.T) {
	port := &fakePort{rules: shellRules, wf: developBase,
		fails: map[string]error{"ai/checks/shfmt.sh": errors.New("exit 1")}}
	res, err := run(context.Background(), port, Request{})
	if err != nil {
		t.Fatal(err)
	}
	want := Response{Ran: 2, Failed: []Failed{
		{ID: "sh", Check: "ai/checks/shfmt.sh", Description: "shfmt", Output: "ran ai/checks/shfmt.sh\nexit 1"},
	}}
	if res.Ran != want.Ran || !slices.Equal(res.Failed, want.Failed) {
		t.Fatalf("got %+v, want %+v", res, want)
	}
	calls := []string{"LoadWorkflow", "LoadRules ai/rules", "RunCheck ai/checks/gofmt_test.go", "RunCheck ai/checks/shfmt.sh"}
	if !slices.Equal(port.calls, calls) {
		t.Fatalf("called %v, want %v: without --changed the shell asks nothing about the branch", port.calls, calls)
	}
	if got := port.argv["ai/checks/gofmt_test.go"]; !slices.Equal(got, []string{"go", "test", "./ai/checks"}) {
		t.Fatalf("the Go check runs with %q, want the argv of its runner", got)
	}
	if got := port.argv["ai/checks/shfmt.sh"]; got != nil {
		t.Fatalf("the script runs with %q, want as a program", got)
	}
}

func TestShellOnlyChanged(t *testing.T) {
	port := &fakePort{rules: shellRules, wf: developBase, changed: []string{"install.sh"}}
	res, err := run(context.Background(), port, Request{OnlyChanged: true})
	if err != nil || res.Ran != 1 {
		t.Fatalf("got %+v, %v", res, err)
	}
	if !slices.Equal(port.candidates, []string{"develop"}) {
		t.Fatalf("asked git for %v, want the bases of polix.yaml", port.candidates)
	}
}

// The rules are read from the folder rules.path of polix.yaml, after the workflow.
func TestShellReadsTheRulesOfRulesPath(t *testing.T) {
	port := &fakePort{wf: workflow.Workflow{Rules: workflow.Rules{Path: "governance/rules/"}}}
	if _, err := run(context.Background(), port, Request{}); err != nil {
		t.Fatal(err)
	}
	if want := []string{"LoadWorkflow", "LoadRules governance/rules"}; !slices.Equal(port.calls, want) {
		t.Fatalf("called %v, want %v", port.calls, want)
	}
}

// --base replaces the bases of polix.yaml, but the workflow is still read for its runners.
func TestShellPrefersTheBaseAsked(t *testing.T) {
	port := &fakePort{rules: shellRules, wf: developBase, changed: []string{"a_test.go"}}
	res, err := run(context.Background(), port, Request{OnlyChanged: true, Base: "main"})
	if err != nil || res.Ran != 1 {
		t.Fatalf("got %+v, %v", res, err)
	}
	if !slices.Equal(port.candidates, []string{"main"}) {
		t.Fatalf("asked git for %v, want the base asked", port.candidates)
	}
	if !slices.Contains(port.calls, "LoadWorkflow") || port.argv["ai/checks/gofmt_test.go"] == nil {
		t.Fatalf("with --base the runners still apply: %v %v", port.calls, port.argv)
	}
}

// An error of each method of the port stops the shell before any check runs, and is returned as it is.
func TestShellReturnsTheErrorsOfThePort(t *testing.T) {
	boom := errors.New("boom")
	for name, port := range map[string]*fakePort{
		"LoadRules":    {rules: shellRules, rulesErr: boom},
		"LoadWorkflow": {rules: shellRules, wfErr: boom},
		"Changed":      {rules: shellRules, changedErr: boom},
	} {
		_, err := run(context.Background(), port, Request{OnlyChanged: true})
		if !errors.Is(err, boom) {
			t.Errorf("%s: got %v, want its error", name, err)
		}
		if slices.ContainsFunc(port.calls, func(c string) bool { return strings.HasPrefix(c, "RunCheck") }) {
			t.Errorf("%s: a check ran after the error: %v", name, port.calls)
		}
	}
}

// Invalid runners stop the shell before any check runs: a check would otherwise run as a program instead.
func TestShellStopsOnInvalidRunners(t *testing.T) {
	wf := workflow.Workflow{Rules: workflow.Rules{Run: []workflow.Runner{{Glob: "*_test.go"}}}}
	port := &fakePort{rules: shellRules, wf: wf}
	if _, err := run(context.Background(), port, Request{}); err == nil || len(port.argv) != 0 {
		t.Fatalf("got %v after running %v", err, port.argv)
	}
}

// A rule that is not Valid stops the command: its result is not the result of a check.
func TestShellStopsOnAnInvalidRule(t *testing.T) {
	port := &fakePort{rules: []rule.Rule{{ID: "x", Deterministic: true, Check: "c.sh"}}}
	if _, err := run(context.Background(), port, Request{}); err == nil {
		t.Fatal("an invalid rule was run")
	}
}

func TestFlags(t *testing.T) {
	if err := flags(Request{Base: "develop"}); err == nil {
		t.Fatal("--base without --changed is accepted")
	}
	if err := flags(Request{Base: "develop", OnlyChanged: true}); err != nil {
		t.Fatal(err)
	}
}

// runCommand runs the command `check` over args with its ui on buffers and returns the exit code and output.
func runCommand(t *testing.T, args ...string) (int, string, string) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	code := cli.Run(Command(), args, nil, &stdout, &stderr)
	return code, stdout.String(), stderr.String()
}

// repo makes a git repository with a rule whose check passes or fails.
func repo(t *testing.T, pass bool) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("the check is a sh script")
	}
	dir := t.TempDir()
	runGit(t, dir, "init", "--quiet", "--initial-branch=main")
	code := "exit 1"
	if pass {
		code = "exit 0"
	}
	write(t, dir, map[string]string{
		"ai/rules/a.yaml": "rules:\n  - id: A-1\n    description: a\n    deterministic: true\n    check: checks/a.sh\n",
		"checks/a.sh":     "#!/bin/sh\necho checked\n" + code + "\n",
	})
	return dir
}

func TestCommandExitCodes(t *testing.T) {
	code, stdout, _ := runCommand(t, "--dir", repo(t, true))
	if code != 0 || !strings.Contains(stdout, "1 checks, 0 failed") {
		t.Fatalf("passing check: %d %q", code, stdout)
	}
	code, stdout, stderr := runCommand(t, "--dir", repo(t, false))
	if code != 1 || !strings.Contains(stdout, "1 checks, 1 failed") ||
		!strings.Contains(stderr, "FAIL A-1 (checks/a.sh): a") || !strings.Contains(stderr, "checked") {
		t.Fatalf("failing check: %d %q %q", code, stdout, stderr)
	}
}

func TestCommandBaseNeedsChanged(t *testing.T) {
	code, _, stderr := runCommand(t, "--dir", t.TempDir(), "--base", "main")
	if code != 1 || !strings.Contains(stderr, "--base needs --changed") {
		t.Fatalf("got %d %q", code, stderr)
	}
}

func TestCommandOutsideARepository(t *testing.T) {
	code, _, stderr := runCommand(t, "--dir", t.TempDir())
	// The error goes to the error handler of internal/platform/cli.Run, which names the program: polix: <error>
	// under polix.
	if code != 1 || !strings.HasPrefix(stderr, "check: not inside a git repository") {
		t.Fatalf("got %d %q", code, stderr)
	}
}

// --changed compares with the base of polix.yaml unless --base says otherwise, the closest of a list wins, and a
// base that does not exist fails naming it.
func TestCommandBase(t *testing.T) {
	dir := repo(t, false)
	writePolix := func(s string) { write(t, dir, map[string]string{"polix.yaml": s}) }
	write(t, dir, map[string]string{"ai/rules/a.yaml": "rules:\n  - id: A-1\n    description: a\n" +
		"    scope: [\"src/**\"]\n    deterministic: true\n    check: checks/a.sh\n"})
	runGit(t, dir, "add", "--all")
	runGit(t, dir, "commit", "--quiet", "-m", "rules")
	runGit(t, dir, "checkout", "--quiet", "-b", "develop")
	write(t, dir, map[string]string{"src/a.txt": "x\n"})
	runGit(t, dir, "add", "--all")
	runGit(t, dir, "commit", "--quiet", "-m", "develop")
	runGit(t, dir, "checkout", "--quiet", "-b", "feature")

	code, stdout, _ := runCommand(t, "--dir", dir, "--changed")
	if code != 1 || !strings.Contains(stdout, "1 checks, 1 failed") {
		t.Fatalf("without a workflow the base is main: %d %q", code, stdout)
	}
	writePolix("version: 1\nbase_branch: develop\n")
	code, stdout, _ = runCommand(t, "--dir", dir, "--changed")
	if code != 0 || !strings.Contains(stdout, "0 checks") {
		t.Fatalf("the base of polix.yaml is develop: %d %q", code, stdout)
	}
	if code, stdout, _ = runCommand(t, "--dir", dir, "--changed", "--base", "main"); code != 1 {
		t.Fatalf("--base overrides polix.yaml: %d %q", code, stdout)
	}
	writePolix("version: 1\nbase_branch: [main, develop]\n")
	code, stdout, _ = runCommand(t, "--dir", dir, "--changed")
	if code != 0 || !strings.Contains(stdout, "0 checks") {
		t.Fatalf("the closest base of the list is develop: %d %q", code, stdout)
	}
	writePolix("version: 1\nbase_branch: trunk\n")
	code, stdout, stderr := runCommand(t, "--dir", dir, "--changed")
	if code != 1 || !strings.Contains(stderr, "trunk") || !strings.Contains(stderr, "set base_branch in polix.yaml") {
		t.Fatalf("a base that does not exist fails naming it: %d %q %q", code, stdout, stderr)
	}
	writePolix("version: [\n")
	if code, _, stderr = runCommand(t, "--dir", dir); code != 1 || !strings.Contains(stderr, "polix.yaml") {
		t.Fatalf("a broken polix.yaml stops: %d %q", code, stderr)
	}
}
