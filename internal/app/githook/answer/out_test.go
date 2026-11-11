package answer

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/mlophez/polix-ai/internal/app/workflow"
	workflowload "github.com/mlophez/polix-ai/internal/app/workflow/load"
	"github.com/mlophez/polix-ai/internal/platform/testenv"
)

// gitIn runs git in dir without the configuration of the machine, failing the test on an error.
func gitIn(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-c", "user.name=t", "-c", "user.email=t@x"}, args...)...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL="+os.DevNull, "GIT_CONFIG_SYSTEM="+os.DevNull)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

// AC5: the staged files are the index against HEAD: a file modified and not staged is left out, a new staged file
// is in; on a repository without commits, every staged file.
func TestAdapterStaged(t *testing.T) {
	ctx := context.Background()
	root := testenv.GitRepo(t)
	testenv.Write(t, filepath.Join(root, "a.txt"), "a")
	gitIn(t, root, "add", "a.txt")
	gitIn(t, root, "commit", "--quiet", "-m", "a")
	testenv.Write(t, filepath.Join(root, "a.txt"), "changed, not staged")
	testenv.Write(t, filepath.Join(root, "src", "new.go"), "package src")
	gitIn(t, root, "add", "src/new.go")
	a := &Adapter{}
	if err := a.Open(ctx, root); err != nil {
		t.Fatal(err)
	}
	if staged, err := a.Staged(ctx); err != nil || !slices.Equal(staged, []string{"src/new.go"}) {
		t.Fatalf("staged: %q %v", staged, err)
	}
	if b := a.Branch(ctx); b != "main" {
		t.Fatalf("branch: %q", b)
	}

	fresh := t.TempDir()
	gitIn(t, fresh, "init", "--quiet", "--initial-branch=PROJ-1-x")
	testenv.Write(t, filepath.Join(fresh, "first.md"), "x")
	gitIn(t, fresh, "add", "first.md")
	b := &Adapter{}
	if err := b.Open(ctx, fresh); err != nil {
		t.Fatal(err)
	}
	if staged, err := b.Staged(ctx); err != nil || !slices.Equal(staged, []string{"first.md"}) {
		t.Fatalf("staged without commits: %q %v", staged, err)
	}
	if br := b.Branch(ctx); br != "PROJ-1-x" {
		t.Fatalf("branch without commits: %q", br)
	}
}

// AC6: a run runs from the root with its output live to the writers of the adapter, and returns its failure.
func TestAdapterRun(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the commands run sh, which Windows may not have on the PATH")
	}
	ctx := context.Background()
	root := testenv.GitRepo(t)
	var stdout, stderr strings.Builder
	a := &Adapter{Stdout: &stdout, Stderr: &stderr}
	if err := a.Open(ctx, root); err != nil {
		t.Fatal(err)
	}
	if err := a.Run(ctx, []string{"sh", "-c", "pwd; echo boom >&2"}); err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(stdout.String()); got != root || stderr.String() != "boom\n" {
		t.Fatalf("stdout %q, stderr %q", got, stderr.String())
	}
	if err := a.Run(ctx, []string{"sh", "-c", "exit 3"}); err == nil || err.Error() != "exit status 3" {
		t.Fatalf("failure: %v", err)
	}
}

// AC9, AC10: the shell with the adapter on a real repository blocks with the githooks of polix.yaml, and writes
// nothing under the git directory, the log of polix included; without polix.yaml the commit goes on.
func TestAdapterAnswersWithoutLog(t *testing.T) {
	ctx := context.Background()
	root := testenv.GitRepo(t)
	gitDir := filepath.Join(root, ".git")
	if res, err := run(ctx, &Adapter{Workflow: workflowload.Command()}, Request{Event: workflow.PreCommit,
		Dir: root, Version: "dev"}); err != nil || res.Code != 0 {
		t.Fatalf("no polix.yaml: %+v %v", res, err)
	}
	testenv.Write(t, filepath.Join(root, workflow.File), "version: 1\ngithooks:\n  pre_commit:\n"+
		"    - when: { branch: \"^main$\", changed: \"*.go\" }\n      action: { block: \"No se commitea en {branch}\" }\n")
	testenv.Write(t, filepath.Join(root, "main.go"), "package main")
	gitIn(t, root, "add", "main.go")
	before := tree(t, gitDir)
	res, err := run(ctx, &Adapter{Workflow: workflowload.Command()}, Request{Event: workflow.PreCommit, Dir: root,
		Version: "dev"})
	if err != nil || res.Code != 1 || !slices.Equal(res.Reasons, []string{"No se commitea en main"}) {
		t.Fatalf("got %+v %v", res, err)
	}
	if after := tree(t, gitDir); !slices.Equal(before, after) {
		t.Fatalf("the git directory changed:\n%q\n%q", before, after)
	}
	if _, err := os.Stat(filepath.Join(gitDir, "polix")); !os.IsNotExist(err) {
		t.Fatalf("the log of polix exists: %v", err)
	}
}

// tree returns every path under dir with its size and time of change, in order.
func tree(t *testing.T, dir string) []string {
	t.Helper()
	var out []string
	err := filepath.Walk(dir, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		out = append(out, p+" "+info.ModTime().String()+" "+strconv.FormatInt(info.Size(), 10))
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}
