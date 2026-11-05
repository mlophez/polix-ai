package workflow

import (
	"context"
	"errors"
	"path/filepath"

	"github.com/mlophez/polix-ai/pkg/file"
	"github.com/mlophez/polix-ai/pkg/git"
)

// GraphWorkflowAdapter implements GraphWorkflowPort on the disk of a repository. The entry fills it; there is no
// constructor.
type GraphWorkflowAdapter struct {
	Repo *git.Repo // the repository whose workflow is drawn
}

// LoadWorkflow reads polix.yaml: without the file, the default workflow and false; a file that cannot be read,
// cannot be parsed or requires a newer polix is an error. The problems of a workflow that parses (Invalid) are
// left to polix check: it is drawn as it is.
func (a GraphWorkflowAdapter) LoadWorkflow(_ context.Context, running string) (Workflow, bool, error) {
	data, existed, err := file.Read(filepath.Join(a.Repo.Root, File))
	if err != nil {
		return Workflow{}, true, err
	}
	if !existed {
		wf, _ := Parse(nil, running)
		return wf, false, nil
	}
	wf, err := Parse(data, running)
	if _, invalid := errors.AsType[Invalid](err); err != nil && !invalid {
		return Workflow{}, true, err
	}
	return wf, true, nil
}
