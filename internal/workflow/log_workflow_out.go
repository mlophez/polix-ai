package workflow

import (
	"context"
	"path/filepath"

	"github.com/mlophez/polix-ai/internal/state"
	"github.com/mlophez/polix-ai/pkg/git"
	"github.com/mlophez/polix-ai/pkg/jsonl"
)

// LogWorkflowAdapter implements LogWorkflowPort on the log of a repository, in its git directory. The entry fills
// it; there is no constructor.
type LogWorkflowAdapter struct {
	Repo *git.Repo // the repository whose log is read
}

// ReadLog reads <git-dir>/polix/log.jsonl through pkg/jsonl, skipping the lines over state.MaxLine, and keeps
// every line that decodes. Without the file the log is empty.
func (a LogWorkflowAdapter) ReadLog(_ context.Context) (state.State, error) {
	lines, skipped, err := jsonl.Read(filepath.Join(a.Repo.GitDir, state.DirName, state.LogName), state.MaxLine)
	if err != nil {
		return state.State{}, err
	}
	return state.Parse(lines, skipped)
}
