package check

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/mlophez/polix-ai/internal/app/rule"
	ruleload "github.com/mlophez/polix-ai/internal/app/rule/load"
	workflowload "github.com/mlophez/polix-ai/internal/app/workflow/load"
	"github.com/mlophez/polix-ai/internal/platform/git"
	"github.com/mlophez/polix-ai/internal/platform/proc"
)

// The adapter is tested against a temporary repository: real disk, git and processes, as the port promises.

// write writes the files of a test under root, with folders and the permission to run them.
func write(t *testing.T, root string, files map[string]string) {
	t.Helper()
	for f, data := range files {
		p := filepath.Join(root, filepath.FromSlash(f))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(data), 0o755); err != nil {
			t.Fatal(err)
		}
	}
}

// runGit runs git in dir and fails the test on an error.
func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-c", "user.name=t", "-c", "user.email=t@t", "-c",
		"commit.gpgsign=false"}, args...)...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v %s", args, err, out)
	}
}

// adapterOf opens a new git repository with the files and returns the adapter on it.
func adapterOf(t *testing.T, files map[string]string) Adapter {
	t.Helper()
	dir := t.TempDir()
	runGit(t, dir, "init", "--quiet", "--initial-branch=main")
	write(t, dir, files)
	repo, err := git.Open(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	return Adapter{Repo: repo}
}

// loaderOf is a loader of the rules that answers loaded, as the one of internal/app/rule/load that cmd/polix builds
// would for a folder, and records the root and the folder of each call.
func loaderOf(loaded rule.Loaded, asked *[]string) rule.Load {
	return func(_ context.Context, root, dir string) rule.Loaded {
		*asked = append(*asked, root+" "+dir)
		return loaded
	}
}

// Rules load through the loader, from the root of the repository and the folder asked, ordered by id; the reading
// of the files is tested in internal/app/rule/load.
func TestAdapterLoadRules(t *testing.T) {
	var asked []string
	rules := []rule.Rule{{ID: "A-1", Description: "a", File: "ai/rules/a.yaml"}, {ID: "B-1", Description: "b"}}
	a := Adapter{Repo: &git.Repo{Root: "/repo"}, Rules: loaderOf(rule.Loaded{Rules: rules, Exists: true}, &asked)}
	all, err := a.LoadRules(context.Background(), "ai/rules")
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 2 || all[0].ID != "A-1" || all[1].ID != "B-1" || !slices.Equal(asked, []string{"/repo ai/rules"}) {
		t.Fatalf("LoadRules = %+v, asked %v", all, asked)
	}
}

func TestAdapterLoadRulesWithoutFolder(t *testing.T) {
	var asked []string
	a := Adapter{Repo: &git.Repo{Root: t.TempDir()}, Rules: loaderOf(rule.Loaded{}, &asked)}
	if all, err := a.LoadRules(context.Background(), "ai/rules"); err != nil || len(all) != 0 {
		t.Fatalf("no folder, no rules: %v %v", all, err)
	}
}

// An invalid rule, or a file that cannot be read, stops the strict reading of CheckRule, with its file.
func TestAdapterLoadRulesInvalid(t *testing.T) {
	var asked []string
	strict := errors.New("governance/rules/bad.yaml: rule X-1 has no description")
	loaded := rule.Loaded{Rules: []rule.Rule{{ID: "X-1", File: "governance/rules/bad.yaml"}}, Exists: true,
		Problems: []string{"governance/rules/bad.yaml X-1: no description"}, Strict: strict}
	a := Adapter{Repo: &git.Repo{Root: t.TempDir()}, Rules: loaderOf(loaded, &asked)}
	all, err := a.LoadRules(context.Background(), "governance/rules")
	if err == nil || !strings.Contains(err.Error(), "governance/rules/bad.yaml") || all != nil {
		t.Fatalf("got %v %v, want the error of the file", all, err)
	}
}

// An id that an earlier rule or a retired rule already has stops the strict reading of CheckRule, through the
// loader of the rules that cmd/polix builds, naming both files.
func TestAdapterLoadRulesRepeatedID(t *testing.T) {
	a := adapterOf(t, map[string]string{
		"ai/rules/a.yaml": "rules:\n  - id: A-1\n    description: a\n",
		"ai/rules/b.yaml": "rules:\n  - id: A-1\n    description: b\n",
	})
	a.Rules = ruleload.Command()
	all, err := a.LoadRules(context.Background(), "ai/rules")
	if err == nil || err.Error() != "ai/rules/b.yaml A-1: id repeated, also in ai/rules/a.yaml" || all != nil {
		t.Fatalf("got %v %v", all, err)
	}
	a = adapterOf(t, map[string]string{
		"ai/rules/a.yaml":       "rules:\n  - id: GOARCH-31\n    description: a\n",
		"ai/rules/retired.yaml": "rules:\n  - id: GOARCH-31\n    reason: retired\n",
	})
	a.Rules = ruleload.Command()
	all, err = a.LoadRules(context.Background(), "ai/rules")
	if err == nil || err.Error() != "ai/rules/a.yaml GOARCH-31: id repeated, also in ai/rules/retired.yaml" ||
		all != nil {
		t.Fatalf("a retired id: %v %v", all, err)
	}
}

// workflowOn is the adapter on the folder root with a new loader of polix.yaml, as cmd/polix builds it: the loader
// reads polix.yaml once, so a test that rewrites it takes a new one.
func workflowOn(root string) Adapter {
	return Adapter{Repo: &git.Repo{Root: root}, Workflow: workflowload.Command()}
}

func TestAdapterLoadWorkflow(t *testing.T) {
	root := t.TempDir()
	wf, err := workflowOn(root).LoadWorkflow(context.Background(), "dev")
	if err != nil || !slices.Equal(wf.BaseBranches(), []string{"main"}) {
		t.Fatalf("without polix.yaml, the default workflow: %+v %v", wf, err)
	}
	write(t, root, map[string]string{"polix.yaml": "version: 1\nbase_branch: develop\nrules:\n  run:\n" +
		"    \"*_test.go\": [go, test, \"{dir}\"]\n"})
	wf, err = workflowOn(root).LoadWorkflow(context.Background(), "dev")
	if err != nil || !slices.Equal(wf.BaseBranches(), []string{"develop"}) || len(wf.Rules.Run) != 1 {
		t.Fatalf("got %+v %v", wf, err)
	}
	write(t, root, map[string]string{"polix.yaml": "version: 1\nbase_branch: develop\non:\n  nope: []\n"})
	if wf, err = workflowOn(root).LoadWorkflow(context.Background(), "dev"); err != nil ||
		wf.BaseBranches()[0] != "develop" {
		t.Fatalf("the problems of a workflow that parses are left to polix config check: %+v %v", wf, err)
	}
	write(t, root, map[string]string{"polix.yaml": "version: [\n"})
	if _, err := workflowOn(root).LoadWorkflow(context.Background(), "dev"); err == nil ||
		!strings.Contains(err.Error(), "polix.yaml") {
		t.Fatalf("a broken polix.yaml stops: %v", err)
	}
	write(t, root, map[string]string{"polix.yaml": "version: 1\nrequires: 9.0.0\n"})
	if _, err := workflowOn(root).LoadWorkflow(context.Background(), "1.0.0"); err == nil {
		t.Fatal("a polix.yaml that requires a newer polix is read")
	}
}

// Without any base branch the error asks to set one; with one, the files changed against it.
func TestAdapterChanged(t *testing.T) {
	a := adapterOf(t, map[string]string{"a.txt": "a\n"})
	runGit(t, a.Repo.Root, "add", "--all")
	runGit(t, a.Repo.Root, "commit", "--quiet", "-m", "a")
	_, err := a.Changed(context.Background(), []string{"trunk"})
	if err == nil || !strings.Contains(err.Error(), "trunk") || !strings.Contains(err.Error(), "set base_branch in polix.yaml") {
		t.Fatalf("got %v, want the hint to set base_branch", err)
	}
	runGit(t, a.Repo.Root, "checkout", "--quiet", "-b", "feature")
	write(t, a.Repo.Root, map[string]string{"b.txt": "b\n"})
	got, err := a.Changed(context.Background(), []string{"main"})
	if err != nil || !slices.Equal(got, []string{"b.txt"}) {
		t.Fatalf("Changed = %v %v", got, err)
	}
}

// A check with an argv runs it from the root, with the output and the error of the command.
func TestAdapterRunCheckThroughARunner(t *testing.T) {
	a := adapterOf(t, map[string]string{"x/a_test.go": "package x\n"})
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	run := rule.Run{Rules: []rule.Rule{{ID: "X-1", Description: "x", Deterministic: true, Check: "x/a_test.go"}},
		Argv: []string{"git", "ls-files", "--others", "x"}}
	out, err := a.RunCheck(context.Background(), run)
	if err != nil || strings.TrimSpace(out) != "x/a_test.go" {
		t.Fatalf("the runner ran from the root: %q %v", out, err)
	}
	run.Argv = []string{self, "-test.run", "^$", "-not-a-flag"}
	if _, err := a.RunCheck(context.Background(), run); err == nil {
		t.Fatal("a runner that fails passes")
	}
}

// A run of several rules runs its argv once from the root.
func TestAdapterRunCheckOfSeveralRules(t *testing.T) {
	a := adapterOf(t, map[string]string{"x/a_test.go": "package x\n", "x/b_test.go": "package x\n"})
	run := rule.Run{Rules: []rule.Rule{
		{ID: "X-1", Description: "x", Deterministic: true, Check: "x/a_test.go"},
		{ID: "X-2", Description: "x", Deterministic: true, Check: "x/b_test.go"},
	}, Argv: []string{"git", "ls-files", "--others", "x"}}
	out, err := a.RunCheck(context.Background(), run)
	if err != nil || strings.Fields(out)[0] != "x/a_test.go" || len(strings.Fields(out)) != 2 {
		t.Fatalf("the shared argv ran once: %q %v", out, err)
	}
}

// A check without argv runs as a program; on Windows through its interpreter.
func TestAdapterRunCheckAsAProgram(t *testing.T) {
	if runtime.GOOS == "windows" {
		if _, err := proc.Program(runtime.GOOS, "a.sh", exec.LookPath, proc.FileExists); err != nil {
			t.Skip("no bash to run a .sh check")
		}
	}
	a := adapterOf(t, map[string]string{
		"checks/ok.sh":   "#!/bin/sh\necho fine\n",
		"checks/fail.sh": "#!/bin/sh\necho broken\nexit 1\n",
	})
	r := rule.Rule{ID: "S-1", Description: "s", Deterministic: true, Check: "checks/ok.sh"}
	if out, err := a.RunCheck(context.Background(), rule.Run{Rules: []rule.Rule{r}}); err != nil ||
		!strings.Contains(out, "fine") {
		t.Fatalf("got %q %v", out, err)
	}
	r.Check = "checks/fail.sh"
	if out, err := a.RunCheck(context.Background(), rule.Run{Rules: []rule.Rule{r}}); err == nil || !strings.Contains(out, "broken") {
		t.Fatalf("got %q %v, want the error with the output", out, err)
	}
}

// With Log, RunCheck reports the command and the rules before running, and the verdict after; a long group is
// counted and cut to its first ids.
func TestAdapterRunCheckLog(t *testing.T) {
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	var rules []rule.Rule
	for _, id := range []string{"A-1", "A-2", "A-3", "A-4", "A-5", "A-6"} {
		rules = append(rules, rule.Rule{ID: id, Description: "d", Deterministic: true, Check: "c_test.go"})
	}
	var log strings.Builder
	a := Adapter{Repo: &git.Repo{Root: t.TempDir()}, Log: &log}
	if _, err := a.RunCheck(context.Background(), rule.Run{Rules: rules, Argv: []string{exe, "-test.run=^$"}}); err != nil {
		t.Fatal(err)
	}
	got := log.String()
	for _, want := range []string{"run " + exe + " -test.run=^$ (6 rules: A-1, A-2, A-3, A-4, A-5, ...)\n", "    ok in "} {
		if !strings.Contains(got, want) {
			t.Fatalf("log %q lacks %q", got, want)
		}
	}
}

// A check killed by its deadline returns even when a grandchild still holds its output open.
func TestAdapterRunCheckTimeoutWithGrandchild(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the check is a sh script")
	}
	root := t.TempDir()
	write(t, root, map[string]string{"check.sh": "#!/bin/sh\nsleep 60 &\nsleep 60\n"})
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	start := time.Now()
	a := Adapter{Repo: &git.Repo{Root: root}}
	run := rule.Run{Rules: []rule.Rule{{ID: "T-1", Description: "t", Deterministic: true, Check: "check.sh"}}}
	if _, err := a.RunCheck(ctx, run); err == nil {
		t.Fatal("the check should fail on its deadline")
	}
	if d := time.Since(start); d > time.Second+proc.WaitDelay+5*time.Second {
		t.Fatalf("the check returned after %v", d)
	}
}
