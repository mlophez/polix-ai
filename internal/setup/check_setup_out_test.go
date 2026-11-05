package setup

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/mlophez/polix-ai/internal/workflow"
	"github.com/mlophez/polix-ai/pkg/git"
)

func TestCheckSetupAdapter(t *testing.T) {
	dir := gitRepo(t)
	ctx := context.Background()
	repo, err := git.Open(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	a := CheckSetupAdapter{Repo: repo}
	if _, found, err := a.LoadWorkflow(ctx, "dev"); found || err != nil {
		t.Fatal(found, err)
	}
	write(t, filepath.Join(dir, "polix.yaml"), "version: 2\n")
	if _, found, err := a.LoadWorkflow(ctx, "dev"); !found || !errors.As(err, new(workflow.Invalid)) {
		t.Fatal(found, err)
	}
	write(t, filepath.Join(dir, "ai", "rules", "b.yaml"), "rules:\n  - id: B-1\n    description: d\n  - id: A-2\n")
	write(t, filepath.Join(dir, "ai", "rules", "retired.yaml"), "rules:\n  - id: R-1\n")
	rules, invalid, err := a.LoadRules(ctx)
	if err != nil || len(rules) != 2 || rules[0].ID != "A-2" || len(invalid) != 1 {
		t.Fatal(rules, invalid, err)
	}
	write(t, filepath.Join(dir, "ai", "rules", "c.yaml"), "rules: [\n")
	if _, _, err := a.LoadRules(ctx); err == nil {
		t.Fatal("broken yaml")
	}
	if !a.Missing(ctx, "checks/x.sh") || a.Missing(ctx, "polix.yaml") {
		t.Fatal("missing")
	}
	if a.ResolveBase(ctx, []string{"main"}) != nil || a.ResolveBase(ctx, []string{"trunk"}) == nil {
		t.Fatal("base")
	}
}
