package refresh

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"time"

	"github.com/mlophez/polix-ai/internal/app/setup"
	"github.com/mlophez/polix-ai/internal/platform/file"
	"github.com/mlophez/polix-ai/internal/platform/git"
	"github.com/mlophez/polix-ai/internal/platform/proc"
	"github.com/mlophez/polix-ai/plugins"
)

// Adapter implements Port on the real disk, git and CLIs of the agents. The entry passes
// it as the port; there is no constructor.
type Adapter struct{}

// cliTimeout bounds every call to the CLI of an agent.
const cliTimeout = time.Minute

// Home returns the home folder of the user.
func (a Adapter) Home(_ context.Context) (string, error) { return os.UserHomeDir() }

// Root returns the root of the repository of dir, or "" outside one.
func (a Adapter) Root(ctx context.Context, dir string) string {
	repo, err := git.Open(ctx, dir)
	if err != nil {
		return ""
	}
	return repo.Root
}

// Plugin reads the plugin embedded in the binary.
func (a Adapter) Plugin(_ context.Context) (map[string][]byte, error) {
	return plugins.Files()
}

// ReadFiles reads the files of paths that exist and can be read.
func (a Adapter) ReadFiles(_ context.Context, paths []string) map[string][]byte {
	files := map[string][]byte{}
	for _, p := range paths {
		if data, ok, err := file.Read(p); ok && err == nil {
			files[p] = data
		}
	}
	return files
}

// WritePlugin syncs dir to files (file.Sync).
func (a Adapter) WritePlugin(_ context.Context, dir string, files map[string][]byte) ([]setup.Change, error) {
	created, updated, removed, err := file.Sync(dir, files)
	changes, cerr := setup.Synced(created, updated, removed)
	return changes, errors.Join(err, cerr)
}

// UpdatePlugin runs `claude plugin update` when the CLI of Claude Code is here, and `codex plugin marketplace
// upgrade` when the one of Codex is and the plugin is enabled in it; their failures leave the copy of each agent
// to its next update.
func (a Adapter) UpdatePlugin(ctx context.Context, s setup.Setup) {
	if _, err := exec.LookPath("claude"); err == nil {
		_, _ = proc.Stdout(ctx, "", []string{"claude", "plugin", "update", setup.PluginID, "--scope", "user", "--json"},
			cliTimeout)
	}
	config, _, _ := file.Read(s.CodexConfigFile())
	if _, err := exec.LookPath("codex"); err == nil && setup.CodexEnabled(config) {
		_, _ = proc.Stdout(ctx, "", []string{"codex", "plugin", "marketplace", "upgrade", setup.Name}, cliTimeout)
	}
}

// RegisterHooks rewrites the hooks of polix for agent in path.
func (a Adapter) RegisterHooks(_ context.Context, path, agent string) ([]setup.Change, error) {
	old, existed, err := file.Read(path)
	if err != nil {
		return nil, err
	}
	data, action, err := setup.RegisterHooks(old, existed, agent)
	switch {
	case err != nil:
		return nil, fmt.Errorf("%s: %w", path, err)
	case action == "":
		return nil, nil
	}
	if err := file.Write(path, data); err != nil {
		return nil, err
	}
	return setup.Changes(action, path)
}
