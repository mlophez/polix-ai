package check

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mlophez/polix-ai/internal/app/rule"
	ruleload "github.com/mlophez/polix-ai/internal/app/rule/load"
	"github.com/mlophez/polix-ai/internal/app/workflow"
	workflowload "github.com/mlophez/polix-ai/internal/app/workflow/load"
	"github.com/mlophez/polix-ai/internal/platform/cli"
	"github.com/mlophez/polix-ai/internal/platform/testenv"
)

// fakePort answers the port of CheckSetup from memory.
type fakePort struct {
	found   bool
	wfErr   error
	rules   []rule.Rule
	missing map[string]bool
	baseErr error
	asked   []string
	dir     string // the folder LoadRules was asked for
}

func (f *fakePort) LoadWorkflow(_ context.Context, _ string) (workflow.Workflow, bool, error) {
	return workflow.Workflow{Base: []string{"trunk"}, Rules: workflow.Rules{Path: "docs/rules"}}, f.found, f.wfErr
}

func (f *fakePort) LoadRules(_ context.Context, dir string) ([]rule.Rule, []string, error) {
	f.dir = dir
	return f.rules, nil, nil
}

func (f *fakePort) Missing(_ context.Context, check string) bool { return f.missing[check] }

func (f *fakePort) ResolveBase(_ context.Context, candidates []string) error {
	f.asked = candidates
	return f.baseErr
}

func TestShell(t *testing.T) {
	port := &fakePort{found: true, baseErr: errors.New("no trunk"),
		rules:   []rule.Rule{{ID: "S-1", Description: "d", Deterministic: true, Check: "c.sh", File: "f"}},
		missing: map[string]bool{"c.sh": true}}
	res, err := run(context.Background(), port, Request{Version: "0.1.0"})
	if err != nil || len(res.Problems) != 1 || !strings.Contains(res.Problems[0], "does not exist") ||
		port.asked[0] != "trunk" {
		t.Fatalf("%+v %v", res, err)
	}
	if port.dir != "docs/rules" || res.RulesPath != "docs/rules" {
		t.Fatalf("rules read from %q and reported as %q, want rules.path", port.dir, res.RulesPath)
	}
}

// runCommand runs the command of polix config check on dir through a root of internal/platform/cli, so the exit
// codes are those of internal/platform/cli.Run; the command sits at the root here, not below config as in
// cmd/polix. It builds the loaders of the rules and of polix.yaml as cmd/polix does, once per run.
func runCommand(dir string) (int, string, string) {
	root := cli.Root("polix", "short", "long", "")
	root.AddCommand(Command(ruleload.Command(), workflowload.Command()))
	var out, errb bytes.Buffer
	code := cli.Run(root, []string{"check", "--dir", dir}, nil, &out, &errb)
	return code, out.String(), errb.String()
}

// The cases of polix config check, through the command and its exit codes.
func TestCommand(t *testing.T) {
	r := testenv.GitRepo(t)
	code, stdout, _ := runCommand(r)
	if code != 0 || stdout != "ok: no polix.yaml (only the built-in protections apply) and 0 rules of ai/rules/\n" {
		t.Fatalf("without workflow: %d %q", code, stdout)
	}
	example, err := os.ReadFile(filepath.Join("..", "..", "..", "..", "examples", "polix.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	testenv.Write(t, filepath.Join(r, "polix.yaml"), string(example))
	if code, stdout, stderr := runCommand(r); code != 0 || !strings.HasPrefix(stdout, "ok: polix.yaml and 0 rules") {
		t.Fatalf("the example is valid: %d %q %q", code, stdout, stderr)
	}
	testenv.Write(t, filepath.Join(r, "polix.yaml"), "version: 1\non:\n  pre_edit:\n    - action: { block: x }\n      an_option_of_0_2: 1\n")
	if code, _, stderr := runCommand(r); code != 1 || !strings.Contains(stderr, "newer polix") ||
		strings.Contains(stderr, "polix:") {
		t.Fatalf("unknown options hint an update, exit 1 without the error handler: %d %q", code, stderr)
	}
	testenv.Write(t, filepath.Join(r, "polix.yaml"), "version: 1\nbase_branch: trunk\n")
	testenv.Write(t, filepath.Join(r, "ai", "rules", "s.yaml"), "rules:\n  - id: S-1\n    description: d\n    deterministic: true\n    check: checks/s.sh\n  - id: S-2\n")
	code, _, stderr := runCommand(r)
	if code != 1 || !strings.Contains(stderr, "S-1: check checks/s.sh does not exist") || !strings.Contains(stderr, "S-2: no description") {
		t.Fatalf("every problem of the rules: %d %q", code, stderr)
	}
	testenv.Write(t, filepath.Join(r, "ai", "rules", "s.yaml"), "rules:\n  - id: S-1\n    description: d\n    deterministic: true\n    check: checks/s.sh\n")
	testenv.Write(t, filepath.Join(r, "checks", "s.sh"), "#!/bin/sh\n")
	if code, _, stderr := runCommand(r); code != 0 || !strings.Contains(stderr, "rules check --changed would fail") {
		t.Fatalf("a missing base is a warning: %d %q", code, stderr)
	}
	testenv.Write(t, filepath.Join(r, "polix.yaml"), "version: 1\nbase_branch: trunk\nrules: { path: docs/rules/ }\n")
	testenv.Write(t, filepath.Join(r, "docs", "rules", "d.yaml"), "rules:\n  - id: D-1\n    description: d\n")
	if code, stdout, _ := runCommand(r); code != 0 || !strings.HasSuffix(stdout, "1 rules of docs/rules/\n") {
		t.Fatalf("the rules of rules.path, not of ai/rules/: %d %q", code, stdout)
	}
	testenv.Write(t, filepath.Join(r, "polix.yaml"), "version: 1\nrules: { path: ../rules }\n")
	if code, _, stderr := runCommand(r); code != 1 || !strings.Contains(stderr, `rules.path "../rules" must be below the root`) {
		t.Fatalf("a folder outside the root: %d %q", code, stderr)
	}
	if code, _, stderr := runCommand(t.TempDir()); code != 1 || !strings.Contains(stderr, "not inside a git repository") {
		t.Fatalf("outside a repository: %d %q", code, stderr)
	}
}
