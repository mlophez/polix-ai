package setup

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"charm.land/huh/v2"

	"github.com/mlophez/polix-ai/pkg/file"
	"github.com/mlophez/polix-ai/pkg/git"
	"github.com/mlophez/polix-ai/pkg/proc"
	"github.com/mlophez/polix-ai/plugins"
)

// InstallSetupAdapter implements InstallSetupPort on the real disk, git, terminal and CLIs of the agents. The
// entry fills it and passes it as the port; there is no constructor.
type InstallSetupAdapter struct {
	Ask bool // a person is at the terminal and did not pass --yes: Confirm asks them
}

// installSetupCLITimeout bounds every call to the CLI of an agent.
const installSetupCLITimeout = time.Minute

// Home returns the home folder of the user.
func (a InstallSetupAdapter) Home(_ context.Context) (string, error) { return os.UserHomeDir() }

// Root returns the root of the repository of dir, or "" outside one.
func (a InstallSetupAdapter) Root(ctx context.Context, dir string) string {
	repo, err := git.Open(ctx, dir)
	if err != nil {
		return ""
	}
	return repo.Root
}

// CodexUsed reports whether codex is on the PATH, or ~/.codex or <root>/.codex is a folder.
func (a InstallSetupAdapter) CodexUsed(_ context.Context, home, root string) bool {
	if _, err := exec.LookPath("codex"); err == nil {
		return true
	}
	return file.IsDir(filepath.Join(home, ".codex")) || root != "" && file.IsDir(filepath.Join(root, ".codex"))
}

// Plugin reads the plugin embedded in the binary.
func (a InstallSetupAdapter) Plugin(_ context.Context) (map[string][]byte, error) {
	return plugins.Files()
}

// Confirm asks the person, with Ask, to confirm the summary; ErrCancelled when they do not.
func (a InstallSetupAdapter) Confirm(_ context.Context, summary string) error {
	if !a.Ask {
		return nil
	}
	ok := true
	err := huh.NewConfirm().Title("Install polix?").Description(summary).
		Affirmative("Install").Negative("Cancel").Value(&ok).Run()
	if errors.Is(err, huh.ErrUserAborted) || err == nil && !ok {
		return ErrCancelled
	}
	return err
}

// WritePlugin syncs dir to files (file.Sync).
func (a InstallSetupAdapter) WritePlugin(_ context.Context, dir string, files map[string][]byte) ([]Change, error) {
	created, updated, removed, err := file.Sync(dir, files)
	changes, cerr := Synced(created, updated, removed)
	return changes, errors.Join(err, cerr)
}

// RegisterPlugin registers the plugin with an agent. Codex: `codex plugin marketplace add` and the plugin enabled
// in its configuration; without its CLI, or when it fails, the command is left to run by hand. Claude Code:
// `claude plugin marketplace add` and `claude plugin install`, and `claude plugin update` when the version
// installed is not the one of the folder; without its CLI, the commands are left to run by hand.
func (a InstallSetupAdapter) RegisterPlugin(ctx context.Context, s Setup, agent string) ([]Change, error) {
	dir := s.MarketplaceDir()
	if agent == Codex {
		var todo []Change
		var err error
		if _, lerr := exec.LookPath("codex"); lerr != nil {
			todo, err = Changes("todo", CodexTodo(dir, nil))
		} else if out, rerr := a.run(ctx, "codex", "plugin", "marketplace", "add", dir); !CodexAdded(out, rerr) {
			todo, err = Changes("todo", CodexTodo(dir, rerr))
		}
		if err != nil {
			return nil, err
		}
		ch, err := a.editFile(s.CodexConfigFile(), func(data []byte, existed bool) ([]byte, string, error) {
			data, action := CodexPlugin(data, existed, true)
			return data, action, nil
		})
		return append(todo, ch...), err
	}
	if _, err := exec.LookPath("claude"); err != nil {
		return Changes("todo", ClaudeTodo(dir))
	}
	if _, _, _, err := a.claude(ctx, "plugin", "marketplace", "add", dir, "--scope", "user", "--json"); err != nil {
		return nil, err
	}
	_, installed, available, err := a.claude(ctx, "plugin", "install", PluginID, "--scope", "user", "--json")
	if err != nil {
		return nil, err
	}
	if installed == "" {
		return Changes("registered", PluginID+" in Claude Code")
	}
	if installed != available {
		_, _, _, err = a.claude(ctx, "plugin", "update", PluginID, "--scope", "user", "--json")
	}
	return nil, err
}

// RegisterHooks merges the hooks of polix for agent into file.
func (a InstallSetupAdapter) RegisterHooks(_ context.Context, path, agent string) ([]Change, error) {
	return a.editFile(path, func(data []byte, existed bool) ([]byte, string, error) {
		return RegisterHooks(data, existed, agent)
	})
}

// ReadFiles reads the files of paths that exist and can be read.
func (a InstallSetupAdapter) ReadFiles(_ context.Context, paths []string) map[string][]byte {
	files := map[string][]byte{}
	for _, p := range paths {
		if data, ok, err := file.Read(p); ok && err == nil {
			files[p] = data
		}
	}
	return files
}

// OnPath looks name up in the PATH.
func (a InstallSetupAdapter) OnPath(_ context.Context, name string) error {
	_, err := exec.LookPath(name)
	return err
}

// editFile reads path, asks edit for its new content and what that does to it, and carries it out: nothing,
// removed, or written. The errors of edit name the file.
func (a InstallSetupAdapter) editFile(path string, edit func([]byte, bool) ([]byte, string, error)) ([]Change, error) {
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
	return Changes(action, path)
}

// claude runs a command of the CLI of Claude Code with --json and reads its answer (ClaudeAnswer).
func (a InstallSetupAdapter) claude(ctx context.Context, args ...string) (string, string, string, error) {
	out, err := a.run(ctx, "claude", args...)
	return ClaudeAnswer(args, out, err, nil)
}

// run runs the CLI of an agent within a minute (proc.Stdout).
func (a InstallSetupAdapter) run(ctx context.Context, name string, args ...string) (string, error) {
	return proc.Stdout(ctx, "", append([]string{name}, args...), installSetupCLITimeout)
}
