package check

import (
	"context"
	"path/filepath"

	"github.com/mlophez/polix-ai/internal/app/rule"
	"github.com/mlophez/polix-ai/internal/app/setup"
	"github.com/mlophez/polix-ai/internal/app/workflow"
	"github.com/mlophez/polix-ai/internal/platform/file"
	"github.com/mlophez/polix-ai/internal/platform/git"
)

// Adapter implements Port on the disk and git of a repository. The entry fills it and passes
// it as the port; there is no constructor.
type Adapter struct {
	Repo *git.Repo // the repository whose workflow and rules are checked
}

// LoadWorkflow reads polix.yaml and parses it (setup.ParseWorkflow); without the file, the default workflow and false.
func (a Adapter) LoadWorkflow(_ context.Context, running string) (workflow.Workflow, bool, error) {
	data, existed, err := file.Read(filepath.Join(a.Repo.Root, workflow.File))
	if err != nil {
		return workflow.Workflow{}, true, err
	}
	return setup.ParseWorkflow(data, existed, running)
}

// LoadRules reads the yaml files of the folder dir and parses them (rule.ParseFilesAll); without the folder there
// are none.
func (a Adapter) LoadRules(_ context.Context, dir string) ([]rule.Rule, []string, error) {
	files, err := file.ReadDir(filepath.Join(a.Repo.Root, filepath.FromSlash(dir)), ".yaml")
	if err != nil {
		return nil, nil, err
	}
	return rule.ParseFilesAll(dir, files)
}

// Missing reports whether the check does not exist below the root.
func (a Adapter) Missing(_ context.Context, check string) bool {
	return !file.Exists(filepath.Join(a.Repo.Root, filepath.FromSlash(check)))
}

// ResolveBase asks git for the first of candidates that exists, locally or on origin.
func (a Adapter) ResolveBase(ctx context.Context, candidates []string) error {
	_, err := a.Repo.ResolveBase(ctx, candidates)
	return err
}
