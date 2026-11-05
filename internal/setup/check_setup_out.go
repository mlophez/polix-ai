package setup

import (
	"context"
	"path/filepath"

	"github.com/mlophez/polix-ai/internal/rule"
	"github.com/mlophez/polix-ai/internal/workflow"
	"github.com/mlophez/polix-ai/pkg/file"
	"github.com/mlophez/polix-ai/pkg/git"
)

// CheckSetupAdapter implements CheckSetupPort on the disk and git of a repository. The entry fills it and passes
// it as the port; there is no constructor.
type CheckSetupAdapter struct {
	Repo *git.Repo // the repository whose workflow and rules are checked
}

// LoadWorkflow reads polix.yaml and parses it (ParseWorkflow); without the file, the default workflow and false.
func (a CheckSetupAdapter) LoadWorkflow(_ context.Context, running string) (workflow.Workflow, bool, error) {
	data, existed, err := file.Read(filepath.Join(a.Repo.Root, workflow.File))
	if err != nil {
		return workflow.Workflow{}, true, err
	}
	return ParseWorkflow(data, existed, running)
}

// LoadRules reads the yaml files of ai/rules/ and parses them (rule.ParseFilesAll); without the folder there are none.
func (a CheckSetupAdapter) LoadRules(_ context.Context) ([]rule.Rule, []string, error) {
	files, err := file.ReadDir(filepath.Join(a.Repo.Root, rule.Dir), ".yaml")
	if err != nil {
		return nil, nil, err
	}
	return rule.ParseFilesAll(files)
}

// Missing reports whether the check does not exist below the root.
func (a CheckSetupAdapter) Missing(_ context.Context, check string) bool {
	return !file.Exists(filepath.Join(a.Repo.Root, filepath.FromSlash(check)))
}

// ResolveBase asks git for the first of candidates that exists, locally or on origin.
func (a CheckSetupAdapter) ResolveBase(ctx context.Context, candidates []string) error {
	_, err := a.Repo.ResolveBase(ctx, candidates)
	return err
}
