package check

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/mlophez/polix-ai/internal/app/workflow"
	workflowadapters "github.com/mlophez/polix-ai/internal/app/workflow/adapters"
	"github.com/mlophez/polix-ai/internal/platform/testenv"
)

func TestAdapter(t *testing.T) {
	dir := testenv.GitRepo(t)
	ctx := context.Background()
	a := Adapter{Path: filepath.Join(dir, workflow.File), Workflow: (&workflowadapters.File{}).Load}
	if _, found, err := a.LoadWorkflow(ctx, "dev"); found || err != nil {
		t.Fatal(found, err)
	}
	testenv.Write(t, filepath.Join(dir, "polix.yaml"), "version: 2\n")
	a.Workflow = (&workflowadapters.File{}).Load // the loader reads polix.yaml once: a new one reads it again
	if _, found, err := a.LoadWorkflow(ctx, "dev"); !found || !errors.As(err, new(workflow.InvalidError)) {
		t.Fatal(found, err)
	}
}
