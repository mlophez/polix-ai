package install

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/mlophez/polix-ai/internal/app/setup"

	"charm.land/huh/v2"

	"github.com/mlophez/polix-ai/internal/platform/agentconf"
	"github.com/mlophez/polix-ai/internal/platform/file"
	"github.com/mlophez/polix-ai/internal/platform/git"
	"github.com/mlophez/polix-ai/internal/platform/proc"
	"github.com/mlophez/polix-ai/plugins"
)

// Adapter implements Port on the real disk, git, terminal and CLIs of the agents. The
// entry fills it and passes it as the port; there is no constructor.
type Adapter struct {
	Ask bool // a person is at the terminal and did not pass --yes: Confirm asks them
}

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

// Confirm asks the person, with Ask, to confirm the summary; setup.ErrCancelled when they do not.
func (a Adapter) Confirm(_ context.Context, summary string) error {
	if !a.Ask {
		return nil
	}
	ok := true
	err := huh.NewConfirm().Title("Install polix?").Description(summary).
		Affirmative("Install").Negative("Cancel").Value(&ok).Run()
	if errors.Is(err, huh.ErrUserAborted) || err == nil && !ok {
		return setup.ErrCancelled
	}
	return err
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

// RegisterPlugin registers the plugin with an agent. Codex: `codex plugin marketplace add` and the plugin enabled
// in its configuration; without its CLI, or when it fails, the command is left to run by hand. Claude Code:
// `claude plugin marketplace add` and `claude plugin install`, and `claude plugin update` when the version
// installed is not the one of the folder; without its CLI, the commands are left to run by hand.
func (a Adapter) RegisterPlugin(ctx context.Context, s setup.Setup, agent string) ([]setup.Change, error) {
	if agent == setup.Codex {
		return a.registerCodex(ctx, s)
	}
	dir := s.MarketplaceDir()
	if _, err := exec.LookPath("claude"); err != nil {
		return setup.Changes{}.Add("todo", a.claudeTodo(dir))
	}
	if _, _, _, err := a.claude(ctx, "plugin", "marketplace", "add", dir, "--scope", "user", "--json"); err != nil {
		return nil, err
	}
	_, installed, available, err := a.claude(ctx, "plugin", "install", setup.PluginID, "--scope", "user", "--json")
	if err != nil {
		return nil, err
	}
	if installed == "" {
		return setup.Changes{}.Add("registered", setup.PluginID+" in Claude Code")
	}
	if installed != available {
		_, _, _, err = a.claude(ctx, "plugin", "update", setup.PluginID, "--scope", "user", "--json")
	}
	return nil, err
}

// RegisterHooks writes hooks in place of the hooks of polix of file (agentconf.MergeHooks), adding the events the
// file lacks in the order of setup.HookEvents.
func (a Adapter) RegisterHooks(_ context.Context, path string, hooks map[string][]setup.HookGroup) ([]setup.Change, error) {
	want := a.groups(hooks)
	return a.editFile(path, func(data []byte, existed bool) ([]byte, string, error) {
		return agentconf.MergeHooks(data, existed, want, setup.HookEvents, a.polix)
	})
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

// OnPath looks name up in the PATH.
func (a Adapter) OnPath(_ context.Context, name string) error {
	_, err := exec.LookPath(name)
	return err
}

// registerCodex adds the marketplace to Codex, or leaves the command to run by hand without its CLI or when it
// fails, and enables the plugin in the configuration of Codex.
func (a Adapter) registerCodex(ctx context.Context, s setup.Setup) ([]setup.Change, error) {
	dir := s.MarketplaceDir()
	var todo []setup.Change
	var err error
	if _, lerr := exec.LookPath("codex"); lerr != nil {
		todo, err = setup.Changes{}.Add("todo", a.codexTodo(dir, nil))
	} else if out, rerr := a.run(ctx, "codex", "plugin", "marketplace", "add", dir); !a.codexAdded(out, rerr) {
		todo, err = setup.Changes{}.Add("todo", a.codexTodo(dir, rerr))
	}
	if err != nil {
		return nil, err
	}
	ch, err := a.editFile(s.CodexConfigFile(), func(data []byte, existed bool) ([]byte, string, error) {
		data, action := agentconf.CodexPlugin(data, existed, setup.PluginID, true)
		return data, action, nil
	})
	return append(todo, ch...), err
}

// editFile reads path, asks edit for its new content and what that does to it, and carries it out: nothing,
// removed, or written. The errors of edit name the file.
func (a Adapter) editFile(path string, edit func([]byte, bool) ([]byte, string, error)) ([]setup.Change, error) {
	old, existed, err := file.Read(path)
	if err != nil {
		return nil, err
	}
	data, action, err := edit(old, existed)
	switch {
	case err != nil:
		return nil, fmt.Errorf("%s: %w", path, err)
	case action == "":
		return nil, nil
	case action == "removed":
		err = os.Remove(path)
	default:
		err = file.Write(path, data)
	}
	if err != nil {
		return nil, err
	}
	return setup.Changes{}.Add(action, path)
}

// groups returns the hooks of polix as the groups of a hooks file, by event, each handler with its status.
func (a Adapter) groups(hooks map[string][]setup.HookGroup) map[string][]agentconf.Group {
	out := map[string][]agentconf.Group{}
	for ev, groups := range hooks {
		for _, g := range groups {
			group := agentconf.Group{Matcher: g.Matcher}
			for _, h := range g.Handlers {
				group.Handlers = append(group.Handlers, agentconf.Handler{Command: h.Command, Timeout: h.Timeout,
					Status: h.Status})
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

// claudeTodo is the command that registers the plugin with Claude Code, for the user to run by hand when its CLI
// is missing.
func (a Adapter) claudeTodo(marketplace string) string {
	return "claude plugin marketplace add " + a.quote(marketplace) + " --scope user && claude plugin install " +
		setup.PluginID + " --scope user"
}

// codexTodo is the command that adds the marketplace to Codex, for the user to run by hand when its CLI is
// missing, or failed with failed.
func (a Adapter) codexTodo(marketplace string, failed error) string {
	add := "codex plugin marketplace add " + a.quote(marketplace)
	if failed != nil {
		first, _, _ := strings.Cut(strings.TrimSpace(failed.Error()), "\n")
		return add + " (it failed: " + first + ")"
	}
	return add
}

// codexAdded reports whether adding the marketplace to Codex succeeded, or failed because it was already there:
// out is what the CLI printed and failed the error of the run.
func (a Adapter) codexAdded(out string, failed error) bool {
	return failed == nil || strings.Contains(strings.ToLower(out+failed.Error()), "already")
}

// quote quotes a path for a command shown to the user when it holds a space.
func (a Adapter) quote(p string) string {
	if strings.ContainsAny(p, " \t") {
		return `"` + p + `"`
	}
	return p
}

// claude runs a command of the CLI of Claude Code with --json and reads its answer (agentconf.ClaudeAnswer).
func (a Adapter) claude(ctx context.Context, args ...string) (string, string, string, error) {
	out, err := a.run(ctx, "claude", args...)
	return agentconf.ClaudeAnswer(args, out, err, nil)
}

// run runs the CLI of an agent within a minute (proc.Stdout).
func (a Adapter) run(ctx context.Context, name string, args ...string) (string, error) {
	return proc.Stdout(ctx, "", append([]string{name}, args...), cliTimeout)
}
