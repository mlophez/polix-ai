package workflow

import (
	"context"
	"path/filepath"

	"github.com/mlophez/polix-ai/pkg/file"
	"github.com/mlophez/polix-ai/pkg/git"
)

// FormatWorkflowAdapter implements FormatWorkflowPort on the disk of a repository. The entry fills it; there is no
// constructor.
type FormatWorkflowAdapter struct {
	Repo *git.Repo // the repository whose polix.yaml is formatted
}

// ReadWorkflow reads polix.yaml at the root of the repository; without the file, no data and false.
func (a FormatWorkflowAdapter) ReadWorkflow(_ context.Context) ([]byte, bool, error) {
	return file.Read(filepath.Join(a.Repo.Root, File))
}

// WriteWorkflow writes polix.yaml at the root of the repository, keeping its mode and a symlink to it.
func (a FormatWorkflowAdapter) WriteWorkflow(_ context.Context, data []byte) error {
	return file.Write(filepath.Join(a.Repo.Root, File), data)
}
