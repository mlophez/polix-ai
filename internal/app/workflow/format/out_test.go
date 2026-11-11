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
	a := Adapter{Path: filepath.Join(repo.Root, workflow.File)}
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

// With a file, polix config fmt formats that file, named from the working folder, and leaves polix.yaml of the
// repository alone; it needs no repository, and refuses a file together with --dir.
func TestCommandFile(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "reference.yaml")
	if err := os.WriteFile(p, []byte("on:\n    stop: []\nversion: 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	fmtFile := func(args ...string) (int, string, string) {
		root := cli.Root("polix", "", "", "")
		root.AddCommand(Command())
		var out, errb bytes.Buffer
		code := cli.Run(root, append([]string{"fmt"}, args...), strings.NewReader(""), &out, &errb)
		return code, out.String(), errb.String()
	}
	if code, _, stderr := fmtFile("--check", p); code != 1 ||
		!strings.Contains(stderr, p+" is not formatted: run "+fixCommand(p)) {
		t.Fatalf("check: %d %q", code, stderr)
	}
	if code, stdout, stderr := fmtFile(p); code != 0 || !strings.Contains(stdout, "formatted "+p) {
		t.Fatalf("fmt: %d %q %q", code, stdout, stderr)
	}
	if data, err := os.ReadFile(p); err != nil || string(data) != "version: 1\non:\n  stop: []\n" {
		t.Fatalf("file: %q %v", data, err)
	}
	if code, _, stderr := fmtFile(filepath.Join(dir, "missing.yaml")); code != 1 || !strings.Contains(stderr, "not found") {
		t.Fatalf("missing: %d %q", code, stderr)
	}
	if code, _, stderr := fmtFile(p, "--dir", dir); code != 1 || !strings.Contains(stderr, "cannot go together") {
		t.Fatalf("file and --dir: %d %q", code, stderr)
	}
	if code, _, _ := fmtFile(p, p); code == 0 {
		t.Fatal("two files: want a failure")
	}
	if code, _, stderr := fmtFile(""); code != 1 || !strings.Contains(stderr, "cannot be empty") {
		t.Fatalf("empty file: %d %q", code, stderr)
	}
	spaced := filepath.Join(dir, "my workflow.yaml")
	if err := os.WriteFile(spaced, []byte("version:   1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if code, _, stderr := fmtFile("--check", spaced); code != 1 ||
		!strings.Contains(stderr, "run polix config fmt '"+spaced+"'") {
		t.Fatalf("a file with a blank: %d %q", code, stderr)
	}
}
