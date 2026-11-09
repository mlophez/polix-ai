package measure

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/mlophez/polix-ai/internal/app/diff"
	workflowload "github.com/mlophez/polix-ai/internal/app/workflow/load"
	"github.com/mlophez/polix-ai/internal/platform/git"
)

// adapterOf creates a repository with polix.yaml and one more commit, and returns the adapter on it.
func adapterOf(t *testing.T) (Adapter, func(rel, content string)) {
	t.Helper()
	dir := t.TempDir()
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-c", "user.name=t", "-c", "user.email=t@example.com",
			"-c", "commit.gpgsign=false"}, args...)...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL="+os.DevNull, "GIT_CONFIG_SYSTEM="+os.DevNull)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	write := func(rel, content string) {
		t.Helper()
		p := filepath.Join(dir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	run("init", "--quiet", "--initial-branch=main")
	write("polix.yaml", "version: 1\nrisk:\n  weights:\n    \"src/**\": 2\n")
	run("add", "--all")
	run("commit", "--quiet", "-m", "init")
	write("src/a.go", "package a\n\nfunc A() {}\n")
	run("add", "--all")
	run("commit", "--quiet", "-m", "add a")
	repo, err := git.Open(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	return Adapter{Repo: repo, Workflow: workflowload.Command()}, write
}

// The adapter loads the weights and measures the uncommitted diff and a commit.
func TestAdapter(t *testing.T) {
	ctx := context.Background()
	a, write := adapterOf(t)
	wf, err := a.LoadWorkflow(ctx, "dev")
	if err != nil || len(wf.Weights) != 1 || wf.Weights[0].Factor != 2 {
		t.Fatalf("LoadWorkflow = %+v, %v", wf.Weights, err)
	}
	write("src/a.go", "package a\n")
	write("new.md", "x\n")
	d, err := a.Diff(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if want := []diff.Change{{Path: "new.md", Added: 1}, {Path: "src/a.go", Deleted: 2}}; !slices.Equal(d.Changes, want) {
		t.Fatalf("Diff = %+v", d.Changes)
	}
	d, summary, err := a.CommitDiff(ctx, "HEAD")
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(d.Changes, []diff.Change{{Path: "src/a.go", Added: 3}}) || !strings.HasSuffix(summary, " add a") {
		t.Fatalf("CommitDiff = %+v %q", d.Changes, summary)
	}
	if _, _, err := a.CommitDiff(ctx, "nope"); err == nil || !strings.Contains(err.Error(), "not a commit") {
		t.Fatalf("CommitDiff of a missing revision = %v", err)
	}
}

// A polix.yaml that is not valid is an error.
func TestAdapterInvalidWorkflow(t *testing.T) {
	a, write := adapterOf(t)
	write("polix.yaml", "version: 1\nrisk:\n  weights:\n    a: -1\n")
	if _, err := a.LoadWorkflow(context.Background(), "dev"); err == nil {
		t.Fatal("an invalid workflow is an error")
	}
}
