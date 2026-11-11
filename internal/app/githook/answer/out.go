package answer

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/mlophez/polix-ai/internal/app/workflow"
	"github.com/mlophez/polix-ai/internal/platform/git"
	"github.com/mlophez/polix-ai/internal/platform/proc"
)

// Adapter implements Port on the real git and processes of a repository. The entry passes one with the loader of
// polix.yaml and the writers of the terminal of the commit as the port; Open fills it with the repository. It
// writes nothing to the repository, the log of polix (<git-dir>/polix) included.
type Adapter struct {
	Repo     *git.Repo     // the repository of the call, once open
	Workflow workflow.Load // the loader of polix.yaml (internal/app/workflow/load), which cmd/polix builds once
	Stdout   io.Writer     // where the stdout of a run goes, live
	Stderr   io.Writer     // where the stderr of a run goes, live
}

// Open opens the repository that contains dir, the working directory when empty.
func (a *Adapter) Open(ctx context.Context, dir string) error {
	repo, err := git.Open(ctx, dir)
	if err != nil {
		return err
	}
	a.Repo = repo
	return nil
}

// LoadWorkflow loads polix.yaml of the repository through the loader of polix.yaml. Without the file there is no
// workflow and no error; a file that cannot be read, cannot be parsed, has problems or requires a newer polix
// exists, with its error.
func (a *Adapter) LoadWorkflow(ctx context.Context, running string) (workflow.Workflow, bool, error) {
	loaded := a.Workflow(ctx, filepath.Join(a.Repo.Root, workflow.File), running)
	if !loaded.Exists {
		return workflow.Workflow{}, false, nil
	}
	return loaded.Workflow, true, loaded.Err
}

// Branch returns the current branch through git, also on a branch without commits.
func (a *Adapter) Branch(ctx context.Context) string { return a.Repo.Branch(ctx) }

// Staged returns the files staged for the commit through git: the index against HEAD, the index git gives the hook
// included.
func (a *Adapter) Staged(ctx context.Context) ([]string, error) { return a.Repo.Staged(ctx) }

// Message reads the message file git passed to commit-msg, relative to the working directory, the root where git
// runs its hooks.
func (a *Adapter) Message(_ context.Context, file string) (string, error) {
	data, err := os.ReadFile(file)
	if err != nil {
		return "", fmt.Errorf("the message of the commit: %w", err)
	}
	return string(data), nil
}

// CommentChar returns the comment string of the repository through git.
func (a *Adapter) CommentChar(ctx context.Context) string { return a.Repo.CommentChar(ctx) }

// Run runs argv from the root without a time limit, never through a shell, writing its output live to the writers
// of the adapter.
func (a *Adapter) Run(ctx context.Context, argv []string) error {
	return proc.Live(ctx, a.Repo.Root, argv, a.Stdout, a.Stderr)
}
