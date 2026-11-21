package format

import (
	"context"

	"github.com/mlophez/polix-ai/internal/platform/file"
)

// Adapter implements Port on the disk. The entry fills it; there is no constructor.
type Adapter struct {
	Path string // the workflow file to format: polix.yaml at the root of the repository, or the file of the argument
}

// ReadWorkflow reads the workflow file; without it, no data and false.
func (a Adapter) ReadWorkflow(_ context.Context) ([]byte, bool, error) {
	return file.Read(a.Path)
}

// WriteWorkflow writes the workflow file, keeping its mode and a symlink to it.
func (a Adapter) WriteWorkflow(_ context.Context, data []byte) error {
	return file.Write(a.Path, data)
}
