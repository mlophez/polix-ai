package format

import (
	"context"
	"path/filepath"

	"github.com/mlophez/polix-ai/internal/app/workflow"
	"github.com/mlophez/polix-ai/internal/platform/file"
	"github.com/mlophez/polix-ai/internal/platform/git"
)

// Adapter implements Port on the disk of a repository. The entry fills it; there is no constructor.
type Adapter struct {
	Repo *git.Repo // the repository whose polix.yaml is formatted
}

// ReadWorkflow reads polix.yaml at the root of the repository; without the file, no data and false.
func (a Adapter) ReadWorkflow(_ context.Context) ([]byte, bool, error) {
	return file.Read(filepath.Join(a.Repo.Root, workflow.File))
}

// WriteWorkflow writes polix.yaml at the root of the repository, keeping its mode and a symlink to it.
func (a Adapter) WriteWorkflow(_ context.Context, data []byte) error {
	return file.Write(filepath.Join(a.Repo.Root, workflow.File), data)
}
