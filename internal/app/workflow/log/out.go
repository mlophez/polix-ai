package log

import (
	"context"
	"errors"
	"io/fs"
	"os"
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

// ReadLog loads each file of <git-dir>/polix/sessions/ with the extension of a session (state.FileExt) through the
// loader of the log, which skips the lines over state.MaxLine and keeps every line that decodes. The other files and
// folders, and the old <git-dir>/polix/log.jsonl, are left out. Without the folder there are no sessions; a folder
// or a file of a session that cannot be read is the error.
func (a Adapter) ReadLog(ctx context.Context) ([]state.State, error) {
	dir := filepath.Join(a.Repo.GitDir, state.DirName, state.SessionsDir)
	files, err := os.ReadDir(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var sessions []state.State
	for _, f := range files {
		if f.IsDir() || filepath.Ext(f.Name()) != state.FileExt {
			continue
		}
		loaded := a.State(ctx, filepath.Join(dir, f.Name()))
		if loaded.Err != nil {
			return nil, loaded.Err
		}
		sessions = append(sessions, loaded.State)
	}
	return sessions, nil
}
