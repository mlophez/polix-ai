package log

import (
	"context"
	"path/filepath"

	"github.com/mlophez/polix-ai/internal/app/state"
	"github.com/mlophez/polix-ai/internal/platform/git"
)

// Adapter implements Port on the log of a repository, in its git directory. The entry fills it; there is no
// constructor.
type Adapter struct {
	Repo  *git.Repo  // the repository whose log is read
	State state.Load // the loader of the log (internal/app/state/load), which cmd/polix builds once
}

// ReadLog loads <git-dir>/polix/log.jsonl through the loader of the log, which skips the lines over state.MaxLine
// and keeps every line that decodes. Without the file the log is empty.
func (a Adapter) ReadLog(ctx context.Context) (state.State, error) {
	loaded := a.State(ctx, filepath.Join(a.Repo.GitDir, state.DirName, state.LogName))
	return loaded.State, loaded.Err
}
