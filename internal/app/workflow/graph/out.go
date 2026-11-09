package graph

import (
	"context"
	"errors"
	"path/filepath"

	"github.com/mlophez/polix-ai/internal/app/workflow"
	"github.com/mlophez/polix-ai/internal/platform/file"
	"github.com/mlophez/polix-ai/internal/platform/git"
)

// Adapter implements Port on the disk of a repository. The entry fills it; there is no constructor.
type Adapter struct {
	Repo *git.Repo // the repository whose workflow is drawn
}

// LoadWorkflow reads polix.yaml: without the file, the default workflow and false; a file that cannot be read,
// cannot be parsed or requires a newer polix is an error. The problems of a workflow that parses (workflow.Invalid)
// are left to polix config check: it is drawn as it is.
func (a Adapter) LoadWorkflow(_ context.Context, running string) (workflow.Workflow, bool, error) {
	data, existed, err := file.Read(filepath.Join(a.Repo.Root, workflow.File))
	if err != nil {
		return workflow.Workflow{}, true, err
	}
	if !existed {
		wf, _ := workflow.Parse(nil, running)
		return wf, false, nil
	}
	wf, err := workflow.Parse(data, running)
	if _, invalid := errors.AsType[workflow.Invalid](err); err != nil && !invalid {
		return workflow.Workflow{}, true, err
	}
	return wf, true, nil
}
