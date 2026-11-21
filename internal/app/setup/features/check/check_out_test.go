package check

import (
	"context"
	"errors"
	"path/filepath"
	"slices"
	"testing"

	"github.com/mlophez/polix-ai/internal/app/rule"
	"github.com/mlophez/polix-ai/internal/app/workflow"
	workflowadapters "github.com/mlophez/polix-ai/internal/app/workflow/adapters"
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
	// The rules come from the loader, which reads them in internal/app/rule/adapters; it answers what it is given.
	var asked []string
	loaded := rule.Loaded{Rules: []rule.Rule{{ID: "A-2"}, {ID: "B-1", Description: "d"}}, Exists: true,
		Problems: []string{"ai/rules/b.yaml A-2: no description"}}
	a := Adapter{Repo: repo, Path: filepath.Join(repo.Root, workflow.File), Rules: func(_ context.Context, root, dir string) rule.Loaded {
		asked = append(asked, root+" "+dir)
		return loaded
	}, Workflow: (&workflowadapters.File{}).Load}
	if _, found, err := a.LoadWorkflow(ctx, "dev"); found || err != nil {
		t.Fatal(found, err)
	}
	testenv.Write(t, filepath.Join(dir, "polix.yaml"), "version: 2\n")
	a.Workflow = (&workflowadapters.File{}).Load // the loader reads polix.yaml once: a new one reads it again
	if _, found, err := a.LoadWorkflow(ctx, "dev"); !found || !errors.As(err, new(workflow.InvalidError)) {
		t.Fatal(found, err)
	}
	rules, invalid, err := a.LoadRules(ctx, "ai/rules")
	if err != nil || len(rules) != 2 || rules[0].ID != "A-2" || len(invalid) != 1 ||
		!slices.Equal(asked, []string{repo.Root + " ai/rules"}) {
		t.Fatal(rules, invalid, err, asked)
	}
	broken := errors.New("ai/rules/c.yaml: yaml: did not find expected node content")
	loaded = rule.Loaded{Exists: true, Err: broken, Strict: broken}
	if _, _, err := a.LoadRules(ctx, "ai/rules"); !errors.Is(err, broken) {
		t.Fatal("broken yaml", err)
	}
	if !a.Missing(ctx, "checks/x.sh") || a.Missing(ctx, "polix.yaml") {
		t.Fatal("missing")
	}
	if a.ResolveBase(ctx, []string{"main"}) != nil || a.ResolveBase(ctx, []string{"trunk"}) == nil {
		t.Fatal("base")
	}
}
