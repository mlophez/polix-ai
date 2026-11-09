package format

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mlophez/polix-ai/internal/app/workflow"
	"github.com/mlophez/polix-ai/internal/platform/cli"
	"github.com/mlophez/polix-ai/internal/platform/git"
)

// The adapter of FormatWorkflow is tested against a temporary repository.

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

// runCommand runs the command of FormatWorkflow on the repository through internal/platform/cli, as polix does.
func runCommand(t *testing.T, repo *git.Repo, args ...string) (code int, stdout, stderr string) {
	t.Helper()
	root := cli.Root("polix", "", "", "")
	root.AddCommand(Command())
	var out, errb bytes.Buffer
	code = cli.Run(root, append(args, "--dir", repo.Root), strings.NewReader(""), &out, &errb)
	return code, out.String(), errb.String()
}

func TestAdapter(t *testing.T) {
	ctx := context.Background()
	repo := newRepo(t, nil)
	a := Adapter{Repo: repo}
	if data, found, err := a.ReadWorkflow(ctx); data != nil || found || err != nil {
		t.Fatalf("without file: %q %v %v", data, found, err)
	}
	if err := a.WriteWorkflow(ctx, []byte("version: 1\n")); err != nil {
		t.Fatal(err)
	}
	if data, found, err := a.ReadWorkflow(ctx); string(data) != "version: 1\n" || !found || err != nil {
		t.Fatalf("written: %q %v %v", data, found, err)
	}
}

// polix config fmt rewrites polix.yaml, and --check then finds it formatted.
func TestCommand(t *testing.T) {
	repo := newRepo(t, map[string]string{workflow.File: "on:\n    stop: []\nversion: 1\n"})
	code, _, stderr := runCommand(t, repo, "fmt", "--check")
	if code != 1 || !strings.Contains(stderr, "not formatted") {
		t.Fatalf("check: %d %q", code, stderr)
	}
	if code, stdout, stderr := runCommand(t, repo, "fmt"); code != 0 || !strings.Contains(stdout, "formatted") {
		t.Fatalf("fmt: %d %q %q", code, stdout, stderr)
	}
	data, err := os.ReadFile(filepath.Join(repo.Root, workflow.File))
	if err != nil || string(data) != "version: 1\non:\n  stop: []\n" {
		t.Fatalf("file: %q %v", data, err)
	}
	if code, _, stderr := runCommand(t, repo, "fmt", "--check"); code != 0 {
		t.Fatalf("formatted: %d %q", code, stderr)
	}
}
