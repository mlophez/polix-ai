package check

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/mlophez/polix-ai/internal/app/workflow"
	"github.com/mlophez/polix-ai/internal/platform/git"
	"github.com/mlophez/polix-ai/internal/platform/testenv"
)

func TestAdapter(t *testing.T) {
	dir := testenv.GitRepo(t)
	ctx := context.Background()
	repo, err := git.Open(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	a := Adapter{Repo: repo}
	if _, found, err := a.LoadWorkflow(ctx, "dev"); found || err != nil {
		t.Fatal(found, err)
	}
	testenv.Write(t, filepath.Join(dir, "polix.yaml"), "version: 2\n")
	if _, found, err := a.LoadWorkflow(ctx, "dev"); !found || !errors.As(err, new(workflow.Invalid)) {
		t.Fatal(found, err)
	}
	testenv.Write(t, filepath.Join(dir, "ai", "rules", "b.yaml"), "rules:\n  - id: B-1\n    description: d\n  - id: A-2\n")
	testenv.Write(t, filepath.Join(dir, "ai", "rules", "retired.yaml"), "rules:\n  - id: R-1\n")
	rules, invalid, err := a.LoadRules(ctx, "ai/rules")
	if err != nil || len(rules) != 2 || rules[0].ID != "A-2" || len(invalid) != 1 {
		t.Fatal(rules, invalid, err)
	}
	testenv.Write(t, filepath.Join(dir, "ai", "rules", "c.yaml"), "rules: [\n")
	if _, _, err := a.LoadRules(ctx, "ai/rules"); err == nil {
		t.Fatal("broken yaml")
	}
	if !a.Missing(ctx, "checks/x.sh") || a.Missing(ctx, "polix.yaml") {
		t.Fatal("missing")
	}
	if a.ResolveBase(ctx, []string{"main"}) != nil || a.ResolveBase(ctx, []string{"trunk"}) == nil {
		t.Fatal("base")
	}
}
