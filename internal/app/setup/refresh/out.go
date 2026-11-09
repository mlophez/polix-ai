package refresh

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"time"

	"github.com/mlophez/polix-ai/internal/app/setup"
	"github.com/mlophez/polix-ai/internal/platform/agentconf"
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

// ReadManifest reads the manifest of the plugin at path (agentconf.ReadManifest): its version, and whether it is
// the plugin polix.
func (a Adapter) ReadManifest(_ context.Context, path string) (string, bool) {
	data, _, err := file.Read(path)
	if err != nil {
		return "", false
	}
	name, v, err := agentconf.ReadManifest(data)
	if err != nil || name != setup.Name {
		return "", false
	}
	return v, true
}

// Registered reads the files of paths that exist and can be read, and tells which register a hook of polix
// (agentconf.Registers).
func (a Adapter) Registered(_ context.Context, paths []string) map[string]bool {
	out := map[string]bool{}
	for _, p := range paths {
		if data, ok, err := file.Read(p); ok && err == nil {
			out[p] = agentconf.Registers(data, a.polix)
		}
	}
	return out
}

// MarketplaceFiles encodes the local marketplace of p (agentconf.Marketplace): the plugin of the binary with its
// ManifestVersion in its manifests, and a marketplace manifest for each agent.
func (a Adapter) MarketplaceFiles(_ context.Context, p setup.Plugin) (map[string][]byte, error) {
	return agentconf.Marketplace(setup.Name, p.ManifestVersion(), p.Description(), p.Embedded)
}

// WritePlugin syncs dir to files (file.Sync).
func (a Adapter) WritePlugin(_ context.Context, dir string, files map[string][]byte) ([]setup.Change, error) {
	created, updated, removed, err := file.Sync(dir, files)
	changes, cerr := setup.Changes{}.Synced(created, updated, removed)
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
	if _, err := exec.LookPath("codex"); err == nil && agentconf.CodexEnabled(config, setup.PluginID) {
		_, _ = proc.Stdout(ctx, "", []string{"codex", "plugin", "marketplace", "upgrade", setup.Name}, cliTimeout)
	}
}

// RegisterHooks rewrites hooks in place of the hooks of polix of path (agentconf.MergeHooks), adding the events the
// file lacks in the order of setup.HookEvents.
func (a Adapter) RegisterHooks(_ context.Context, path string, hooks map[string][]setup.HookGroup) ([]setup.Change, error) {
	old, existed, err := file.Read(path)
	if err != nil {
		return nil, err
	}
	data, action, err := agentconf.MergeHooks(old, existed, a.groups(hooks), setup.HookEvents, a.polix)
	switch {
	case err != nil:
		return nil, fmt.Errorf("%s: %w", path, err)
	case action == "":
		return nil, nil
	}
	if err := file.Write(path, data); err != nil {
		return nil, err
	}
	return setup.Changes{}.Add(action, path)
}

// groups returns the hooks of polix as the groups of a hooks file, by event.
func (a Adapter) groups(hooks map[string][]setup.HookGroup) map[string][]agentconf.Group {
	out := map[string][]agentconf.Group{}
	for ev, groups := range hooks {
		for _, g := range groups {
			group := agentconf.Group{Matcher: g.Matcher}
			for _, h := range g.Handlers {
				group.Handlers = append(group.Handlers, agentconf.Handler{Command: h.Command, Timeout: h.Timeout})
			}
			out[ev] = append(out[ev], group)
		}
	}
	return out
}

// polix reports whether a hook command is one polix registers (setup.HookHandler.Polix).
func (a Adapter) polix(command string) bool {
	h := setup.HookHandler{Command: command}
	return h.Valid() == nil && h.Polix()
}
