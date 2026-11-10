package check

import (
	"context"
	"path/filepath"

	"github.com/mlophez/polix-ai/internal/app/rule"
	"github.com/mlophez/polix-ai/internal/app/workflow"
	"github.com/mlophez/polix-ai/internal/platform/file"
	"github.com/mlophez/polix-ai/internal/platform/git"
)

// Adapter implements Port on the disk and git of a repository. The entry fills it and passes
// it as the port; there is no constructor.
type Adapter struct {
	Repo     *git.Repo     // the repository whose workflow and rules are checked
	Path     string        // the workflow file: polix.yaml at the root of the repository, or the file of the argument
	Rules    rule.Load     // the loader of the rules (internal/app/rule/load), which cmd/polix builds once
	Workflow workflow.Load // the loader of polix.yaml (internal/app/workflow/load), which cmd/polix builds once too
}

// LoadWorkflow loads the workflow file of the adapter (Path) through the loader of polix.yaml: the workflow, whether the file
// exists, and the error of loading it, a workflow.InvalidError with the workflow when it parses; without the file, the
// default workflow and false.
func (a Adapter) LoadWorkflow(ctx context.Context, running string) (workflow.Workflow, bool, error) {
	loaded := a.Workflow(ctx, a.Path, running)
	return loaded.Workflow, loaded.Exists, loaded.Err
}

// LoadRules loads the rules of the folder dir through the loader of the rules: every rule, the problems of the
// invalid ones, and the error of a file that cannot be read or is not yaml; without the folder there are none.
func (a Adapter) LoadRules(ctx context.Context, dir string) ([]rule.Rule, []string, error) {
	loaded := a.Rules(ctx, a.Repo.Root, dir)
	return loaded.Rules, loaded.Problems, loaded.Err
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
