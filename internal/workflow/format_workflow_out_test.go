package workflow

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFormatWorkflowAdapter(t *testing.T) {
	ctx := context.Background()
	repo := workflowTestRepo(t, nil)
	a := FormatWorkflowAdapter{Repo: repo}
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
func TestFormatWorkflowCommand(t *testing.T) {
	repo := workflowTestRepo(t, map[string]string{File: "on:\n    stop: []\nversion: 1\n"})
	code, _, stderr := workflowTestRun(t, repo, "fmt", "--check")
	if code != 1 || !strings.Contains(stderr, "not formatted") {
		t.Fatalf("check: %d %q", code, stderr)
	}
	if code, stdout, stderr := workflowTestRun(t, repo, "fmt"); code != 0 || !strings.Contains(stdout, "formatted") {
		t.Fatalf("fmt: %d %q %q", code, stdout, stderr)
	}
	data, err := os.ReadFile(filepath.Join(repo.Root, File))
	if err != nil || string(data) != "version: 1\non:\n  stop: []\n" {
		t.Fatalf("file: %q %v", data, err)
	}
	if code, _, stderr := workflowTestRun(t, repo, "fmt", "--check"); code != 0 {
		t.Fatalf("formatted: %d %q", code, stderr)
	}
}
