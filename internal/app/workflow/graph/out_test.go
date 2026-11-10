package graph

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mlophez/polix-ai/internal/app/workflow"
	workflowload "github.com/mlophez/polix-ai/internal/app/workflow/load"
	"github.com/mlophez/polix-ai/internal/platform/cli"
	"github.com/mlophez/polix-ai/internal/platform/git"
)

// The adapter of GraphWorkflow is tested against a temporary repository.

// newRepo creates a git repository with the files and opens it.
func newRepo(t *testing.T, files map[string]string) *git.Repo {
	t.Helper()
	dir := t.TempDir()
	cmd := exec.Command("git", "init", "--quiet", "--initial-branch=main")
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git init: %v %s", err, out)
	}
	for f, data := range files {
		p := filepath.Join(dir, filepath.FromSlash(f))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(data), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	repo, err := git.Open(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	return repo
}

// runCommand runs the command of GraphWorkflow on the repository through internal/platform/cli, as polix does.
func runCommand(t *testing.T, repo *git.Repo, args ...string) (code int, stdout, stderr string) {
	t.Helper()
	root := cli.Root("polix", "", "", "")
	root.AddCommand(Command(workflowload.Command()))
	var out, errb bytes.Buffer
	code = cli.Run(root, append(args, "--dir", repo.Root), strings.NewReader(""), &out, &errb)
	return code, out.String(), errb.String()
}

// adapter is the adapter of the command on repo, with the loader of polix.yaml as cmd/polix builds it.
func adapter(repo *git.Repo) Adapter {
	return Adapter{Repo: repo, Workflow: workflowload.Command()}
}

func TestAdapterLoadWorkflow(t *testing.T) {
	ctx := context.Background()
	wf, found, err := adapter(newRepo(t, nil)).LoadWorkflow(ctx, "dev")
	if err != nil || found || wf.Version != 1 {
		t.Fatalf("without file: %+v %v %v", wf, found, err)
	}
	// A workflow with problems is drawn as it is.
	repo := newRepo(t, map[string]string{workflow.File: "version: 1\non:\n  pre_edit:\n    - when: { pth: x }\n      action: { block: x }\n"})
	wf, found, err = adapter(repo).LoadWorkflow(ctx, "dev")
	if err != nil || !found || len(wf.On[workflow.PreEdit]) != 1 {
		t.Fatalf("invalid: %+v %v %v", wf, found, err)
	}
	repo = newRepo(t, map[string]string{workflow.File: "version: 1\nrequires: 0.2.0\n"})
	var old workflow.TooOldError
	if _, _, err = adapter(repo).LoadWorkflow(ctx, "0.1.0"); !errors.As(err, &old) {
		t.Fatalf("too old: %v", err)
	}
	repo = newRepo(t, map[string]string{workflow.File: "version: [\n"})
	if _, _, err = adapter(repo).LoadWorkflow(ctx, "dev"); err == nil {
		t.Fatal("broken yaml must fail")
	}
}

// Graph without polix.yaml fails and says why.
func TestCommand(t *testing.T) {
	code, _, stderr := runCommand(t, newRepo(t, nil), "graph")
	if code != 1 || !strings.Contains(stderr, "polix.yaml not found") || strings.Contains(stderr, "<nil>") {
		t.Fatalf("without workflow: %d %q", code, stderr)
	}
	repo := newRepo(t, map[string]string{workflow.File: "version: 1\non:\n  stop:\n    - action: { block: \"r\" }\n"})
	code, stdout, stderr := runCommand(t, repo, "graph")
	if want := "flowchart TD\n  subgraph stop\n    n0[\"block: r\"]\n  end\n"; code != 0 || stdout != want {
		t.Fatalf("graph: %d %q %q", code, stdout, stderr)
	}
	// The entries of a shortcut are drawn expanded, with their origin.
	repo = newRepo(t, map[string]string{workflow.File: "version: 1\nguardrails:\n  run_on_stop:\n" +
		"    - run: \"go test ./...\"\n"})
	code, stdout, stderr = runCommand(t, repo, "graph")
	if want := "flowchart TD\n  subgraph stop\n    n0[\"run go test ./...<br/>from guardrails.run_on_stop\"]\n" +
		"  end\n"; code != 0 || stdout != want {
		t.Fatalf("graph of guardrails: %d %q %q", code, stdout, stderr)
	}
}
