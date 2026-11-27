package check

import (
	"context"

	"github.com/mlophez/polix-ai/internal/app/workflow"
)

// Adapter implements Port on the disk of a repository. The entry fills it and passes it as the port; there is no
// constructor.
type Adapter struct {
	Path     string        // the workflow file: polix.yaml at the root of the repository, or the file of the argument
	Workflow workflow.Load // the loader of polix.yaml (internal/app/workflow/adapters), which cmd/polix builds once
}

// LoadWorkflow loads the workflow file of the adapter (Path) through the loader of polix.yaml: the workflow, whether
// the file exists, and the error of loading it, a workflow.InvalidError with the workflow when it parses; without the
// file, the default workflow and false.
func (a Adapter) LoadWorkflow(ctx context.Context, running string) (workflow.Workflow, bool, error) {
	loaded := a.Workflow(ctx, a.Path, running)
	return loaded.Workflow, loaded.Exists, loaded.Err
}
