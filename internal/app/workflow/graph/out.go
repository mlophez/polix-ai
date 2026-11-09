package graph

import (
	"context"
	"errors"

	"github.com/mlophez/polix-ai/internal/app/workflow"
	"github.com/mlophez/polix-ai/internal/platform/git"
)

// Adapter implements Port on the disk of a repository. The entry fills it; there is no constructor.
type Adapter struct {
	Repo     *git.Repo     // the repository whose workflow is drawn
	Workflow workflow.Load // the loader of polix.yaml (internal/app/workflow/load), which cmd/polix builds once
}

// LoadWorkflow loads polix.yaml of the repository through the loader of polix.yaml: without the file, the default
// workflow and false; a file that cannot be read, cannot be parsed or requires a newer polix is an error. The
// problems of a workflow that parses (workflow.InvalidError) are left to polix config check: it is drawn as it is.
func (a Adapter) LoadWorkflow(ctx context.Context, running string) (workflow.Workflow, bool, error) {
	loaded := a.Workflow(ctx, a.Repo.Root, running)
	if _, invalid := errors.AsType[workflow.InvalidError](loaded.Err); loaded.Err != nil && !invalid {
		return workflow.Workflow{}, loaded.Exists, loaded.Err
	}
	return loaded.Workflow, loaded.Exists, nil
}
