package check

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/mlophez/polix-ai/internal/app/workflow"
	workflowadapters "github.com/mlophez/polix-ai/internal/app/workflow/adapters"
	"github.com/mlophez/polix-ai/internal/platform/cli"
	"github.com/mlophez/polix-ai/internal/platform/testenv"
)

// fakePort answers the port of CheckSetup from memory.
type fakePort struct {
	found bool
	wfErr error
}

// TestMain runs the tests without the variables git exports to its hooks (see testenv.ClearGitHookEnv): the githooks
// of polix.yaml run go test, and these tests run git in repositories of their own.
func TestMain(m *testing.M) {
	testenv.ClearGitHookEnv()
	os.Exit(m.Run())
}

func (f *fakePort) LoadWorkflow(_ context.Context, _ string) (workflow.Workflow, bool, error) {
	return workflow.Workflow{Base: []string{"trunk"}, Path: "docs"}, f.found, f.wfErr
}

// The shell reads only the workflow: its problems and its hint come back, and the version is required.
func TestShell(t *testing.T) {
	port := &fakePort{found: true, wfErr: workflow.InvalidError{`polix.yaml:2: unknown key "an_option"`}}
	res, err := run(context.Background(), port, Request{Version: "0.1.0"})
	if err != nil || len(res.WorkflowProblems) != 1 || res.Hint == "" || !res.Found {
		t.Fatalf("%+v %v", res, err)
	}
}

// Without the running version there is no setup to check: the shell fails instead of reporting.
func TestShellWithoutVersion(t *testing.T) {
	if _, err := run(context.Background(), &fakePort{found: true}, Request{}); err == nil {
		t.Fatal("the shell runs without the running version")
	}
}

// runCommand runs the command of polix config check on dir through a root of internal/platform/cli, so the exit
// codes are those of internal/platform/cli.Run; the command sits at the root here, not below config as in
// cmd/polix. It builds the loader of polix.yaml as cmd/polix does, once per run.
func runCommand(dir string, args ...string) (int, string, string) {
	root := cli.Root("polix", "short", "long", "")
	root.AddCommand(Command((&workflowadapters.File{}).Load))
	var out, errb bytes.Buffer
	code := cli.Run(root, append([]string{"check", "--dir", dir}, args...), nil, &out, &errb)
	return code, out.String(), errb.String()
}

// The cases of polix config check, through the command and its exit codes.
func TestCommand(t *testing.T) {
	r := testenv.GitRepo(t)
	code, stdout, _ := runCommand(r)
	if code != 0 || stdout != "ok: no polix.yaml (only the built-in protections apply)\n" {
		t.Fatalf("without workflow: %d %q", code, stdout)
	}
	example, err := os.ReadFile(filepath.Join("..", "..", "..", "..", "..", "examples", "polix.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	testenv.Write(t, filepath.Join(r, "polix.yaml"), string(example))
	if code, stdout, stderr := runCommand(r); code != 0 || stdout != "ok: polix.yaml\n" {
		t.Fatalf("the example is valid: %d %q %q", code, stdout, stderr)
	}
	testenv.Write(t, filepath.Join(r, "polix.yaml"), "version: 1\non:\n  pre_edit:\n    - action: { block: x }\n      an_option_of_0_2: 1\n")
	if code, _, stderr := runCommand(r); code != 1 || !strings.Contains(stderr, "newer polix") ||
		strings.Contains(stderr, "polix:") {
		t.Fatalf("unknown options hint an update, exit 1 without the error handler: %d %q", code, stderr)
	}
	if code, _, stderr := runCommand(t.TempDir()); code != 1 || !strings.Contains(stderr, "not inside a git repository") {
		t.Fatalf("outside a repository: %d %q", code, stderr)
	}
}

// polix config check validates polix.yaml whatever its path: a path outside the root (CUJ-05) and a removed folder key
// (CUJ-06, CUJ-08) fail with their message, and never with the hint of a newer polix.
func TestCommandPath(t *testing.T) {
	r := testenv.GitRepo(t)
	testenv.Write(t, filepath.Join(r, "polix.yaml"), "version: 1\npath: docs/ai\n")
	if code, stdout, stderr := runCommand(r); code != 0 || stdout != "ok: polix.yaml\n" {
		t.Fatalf("a path: %d %q %q", code, stdout, stderr)
	}
	for src, want := range map[string]string{
		"version: 1\npath: ../x\n":                `  fail  polix.yaml: path "../x" must be below the root`,
		"version: 1\nplans: { path: ai/plans }\n": `  fail  polix.yaml:2: unknown key "plans": remove it: the plans go to plans/ below path (ai/plans by default)`,
		"version: 1\nrules: { path: docs/rules }\n": `  fail  polix.yaml: unknown key "path" in rules (run): move it to the top-level key path, ` +
			`the folder that holds rules/ (ai by default)`,
	} {
		testenv.Write(t, filepath.Join(r, "polix.yaml"), src)
		code, _, stderr := runCommand(r)
		if code != 1 || !slices.Contains(strings.Split(stderr, "\n"), want) || strings.Contains(stderr, "→ these problems may be") {
			t.Errorf("%q: %d %q, want %s", src, code, stderr, want)
		}
	}
	testenv.Write(t, filepath.Join(r, "polix.yaml"), "version: 1\nplans: { path: ai/plans }\n")
	const plans = "Workflow  polix.yaml\n  fail  polix.yaml:2: unknown key \"plans\": remove it: the plans go to plans/ below path " +
		"(ai/plans by default)\n\n0 ok · 0 warn · 1 fail\n"
	if _, _, stderr := runCommand(r); stderr != plans {
		t.Errorf("CUJ-06: stderr is exactly the report of the problem: %q", stderr)
	}
}

// CUJ-07: polix config check on a polix.yaml with run_on_stop exits 1 with the unknown key of guardrails, its
// shortcuts and the hint to use check_on_stop, and never with the hint of a newer polix.
func TestCheckRenamed(t *testing.T) {
	r := testenv.GitRepo(t)
	testenv.Write(t, filepath.Join(r, "polix.yaml"), "version: 1\nguardrails:\n  run_on_stop:\n    - run: go test ./...\n")
	want := `line 3: unknown key "run_on_stop" in guardrails (builtin, protected_files, protected_branches, forbidden_commands, ` +
		`exec_on_start, format_on_edit, format_on_save, check_on_stop, check_on_save): use check_on_stop`
	code, _, stderr := runCommand(r)
	if code != 1 || !strings.Contains(stderr, want) || strings.Contains(stderr, "newer polix") {
		t.Fatalf("%d %q, want %s", code, stderr, want)
	}
}

// CUJ-04, AC-03: polix config check on a check_on_save item with a placeholder of an agent's event or with
// wait_agents exits 1 with the problem at the item's line and never with the hint of a newer polix, since git can
// never give those names to its hooks. A measure git can compute, there or in an entry written in githooks, and any
// unknown placeholder of such an entry keep the hint, as before check_on_save.
func TestCheckCheckOnSave(t *testing.T) {
	r := testenv.GitRepo(t)
	for src, want := range map[string]struct {
		problem string
		newer   bool
	}{
		"guardrails:\n  check_on_save:\n    - run: gofmt -l {path}\n": {"polix.yaml:4: guardrails.check_on_save: " +
			"unknown placeholder {path} (githooks allow {branch}, {root}, or a variable of vars)", false},
		"guardrails:\n  check_on_save:\n    - { run: go test, wait_agents: false }\n": {`polix.yaml:4: unknown key ` +
			`"wait_agents" in guardrails.check_on_save (run, changed, message)`, false},
		"guardrails:\n  check_on_save:\n    - { run: go test, message: \"{changed_lines} lines\" }\n": {"polix.yaml:4: " +
			"guardrails.check_on_save: unknown placeholder {changed_lines} (githooks allow {branch}, {root}, or a variable of vars)",
			true},
		"githooks:\n  pre_commit:\n    - action: { run: \"echo {changed_files}\" }\n": {"polix.yaml:4: " +
			"githooks.pre_commit: unknown placeholder {changed_files} (githooks allow {branch}, {root}, or a variable of vars)", true},
		"githooks:\n  pre_commit:\n    - action: { run: \"gofmt -l {path}\" }\n": {"polix.yaml:4: githooks.pre_commit: " +
			"unknown placeholder {path} (githooks allow {branch}, {root}, or a variable of vars)", true},
	} {
		testenv.Write(t, filepath.Join(r, "polix.yaml"), "version: 1\n"+src)
		code, _, stderr := runCommand(r)
		if code != 1 || !strings.Contains(stderr, want.problem) || strings.Contains(stderr, "newer polix") != want.newer {
			t.Errorf("%s: %d %q, want %s, newer polix %v", src, code, stderr, want.problem, want.newer)
		}
	}
}

// With a file, polix config check validates that workflow file in place of polix.yaml, and names it in its problems
// and its summary; a missing file is a problem.
func TestCommandFile(t *testing.T) {
	r := testenv.GitRepo(t)
	testenv.Write(t, filepath.Join(r, "polix.yaml"), "version: 1\n")
	other := filepath.Join(r, "examples", "reference.yaml")
	testenv.Write(t, other, "version: 1\non:\n  pre_edit:\n    - action: { run: { rule: x } }\n")
	if code, _, stderr := runCommand(r, other); code != 1 ||
		!strings.Contains(stderr, other+`:4: unknown key "rule" in run`) {
		t.Fatalf("the problems of the file: %d %q", code, stderr)
	}
	testenv.Write(t, other, "version: 1\n")
	if code, stdout, stderr := runCommand(r, other); code != 0 || stdout != "ok: "+other+"\n" {
		t.Fatalf("a valid file: %d %q %q", code, stdout, stderr)
	}
	missing := filepath.Join(r, "missing.yaml")
	if code, _, stderr := runCommand(r, missing); code != 1 || !strings.Contains(stderr, missing+" not found") {
		t.Fatalf("a missing file: %d %q", code, stderr)
	}
	if code, _, stderr := runCommand(r, ""); code != 1 || !strings.Contains(stderr, "cannot be empty") {
		t.Fatalf("an empty file: %d %q", code, stderr)
	}
}

// The problems of polix.yaml go to stderr in the section Workflow, the hint of a newer polix is the fix of the last
// problem, and stdout stays empty.
func TestPrintProblems(t *testing.T) {
	res := Response{File: "polix.yaml", WorkflowProblems: []string{"polix.yaml:3: unknown key foo"},
		Hint: "these problems may be options of a newer polix than this one (dev): run polix update before removing them"}
	var stdout, stderr bytes.Buffer
	code := res.print(&stdout, &stderr)
	want := `Workflow  polix.yaml
  fail  polix.yaml:3: unknown key foo
        → these problems may be options of a newer polix than this one (dev): run polix update before removing them

0 ok · 0 warn · 1 fail
`
	if code != 1 || stdout.Len() != 0 || stderr.String() != want {
		t.Fatalf("%d %q\n%q", code, stdout.String(), stderr.String())
	}
}

// CUJ-07, AC-04: polix config check never reads the rules: with an invalid rule and a retired.yaml it prints the
// summary of the workflow and exits with 0, with nothing about the rules nor about rules check --changed.
func TestRunIgnoresRules(t *testing.T) {
	r := testenv.GitRepo(t)
	testenv.Write(t, filepath.Join(r, "polix.yaml"), "version: 1\nbase_branch: trunk\n")
	testenv.Write(t, filepath.Join(r, "ai", "rules", "a.yaml"),
		"rules:\n  - id: A-1\n    subject: s\n    description: d\n    scope: [\"**\"]\n    deterministic: true\n    check: checks/x.sh\n")
	testenv.Write(t, filepath.Join(r, "ai", "rules", "retired.yaml"), "rules:\n  - id: GOARCH-31\n    reason: r\n")
	code, stdout, stderr := runCommand(r)
	if code != 0 || stdout != "ok: polix.yaml\n" || stderr != "" {
		t.Fatalf("%d %q %q", code, stdout, stderr)
	}
}

// CUJ-08, AC-04: without polix.yaml the summary says so plainly.
func TestRunNoWorkflow(t *testing.T) {
	code, stdout, stderr := runCommand(testenv.GitRepo(t))
	if code != 0 || stdout != "ok: no polix.yaml (only the built-in protections apply)\n" || stderr != "" {
		t.Fatalf("%d %q %q", code, stdout, stderr)
	}
}
