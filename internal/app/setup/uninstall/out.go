package uninstall

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/mlophez/polix-ai/internal/app/setup"

	"charm.land/huh/v2"

	"github.com/mlophez/polix-ai/internal/platform/file"
	"github.com/mlophez/polix-ai/internal/platform/git"
	"github.com/mlophez/polix-ai/internal/platform/proc"
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

// Confirm asks the person, with Ask, to confirm the summary; setup.ErrCancelled when they do not.
func (a Adapter) Confirm(_ context.Context, summary string) error {
	if !a.Ask {
		return nil
	}
	ok := true
	err := huh.NewConfirm().Title("Uninstall polix?").Description(summary).
		Affirmative("Uninstall").Negative("Cancel").Value(&ok).Run()
	if errors.Is(err, huh.ErrUserAborted) || err == nil && !ok {
		return setup.ErrCancelled
	}
	return err
}

// UnregisterPlugin unregisters the plugin. Codex: `codex plugin marketplace remove` when its CLI is here and the
// plugin enabled, and the table of the plugin removed from its configuration. Claude Code, when its CLI is here:
// `claude plugin uninstall` and `claude plugin marketplace remove`, tolerating what is already gone.
func (a Adapter) UnregisterPlugin(ctx context.Context, s setup.Setup, agent string) ([]setup.Change, error) {
	if agent == setup.Codex {
		config, _, _ := file.Read(s.CodexConfigFile())
		if _, err := exec.LookPath("codex"); err == nil && setup.CodexEnabled(config) {
			_, _ = a.run(ctx, "codex", "plugin", "marketplace", "remove", setup.Name)
		}
		return a.editFile(s.CodexConfigFile(), func(data []byte, existed bool) ([]byte, string, error) {
			data, action := setup.CodexPlugin(data, existed, false)
			return data, action, nil
		})
	}
	if _, err := exec.LookPath("claude"); err != nil {
		return nil, nil //nolint:nilerr // without the CLI of Claude Code there is no plugin to unregister
	}
	outcome, _, _, err := a.claude(ctx, []string{"not_installed"}, "plugin", "uninstall", setup.PluginID, "--scope", "user", "--json")
	if err != nil {
		return nil, err
	}
	if _, _, _, err := a.claude(ctx, []string{"not_configured"}, "plugin", "marketplace", "remove", setup.Name, "--json"); err != nil {
		return nil, err
	}
	if outcome == "ok" {
		return setup.Changes("unregistered", setup.PluginID+" in Claude Code")
	}
	return nil, nil
}

// UnregisterHooks removes the hooks of polix from path, and the file when polix was all it held.
func (a Adapter) UnregisterHooks(_ context.Context, path string) ([]setup.Change, error) {
	return a.editFile(path, setup.UnregisterHooks)
}

// RemovePlugin removes the local marketplace, and ~/.polix when nothing else is left in it.
func (a Adapter) RemovePlugin(_ context.Context, dir string) ([]setup.Change, error) {
	if !file.Exists(dir) {
		return nil, nil
	}
	if err := os.RemoveAll(dir); err != nil {
		return nil, err
	}
	_ = os.Remove(filepath.Dir(dir))
	return setup.Changes("removed", dir)
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
	return setup.Changes(action, path)
}

// claude runs a command of the CLI of Claude Code with --json and reads its answer (setup.ClaudeAnswer).
func (a Adapter) claude(ctx context.Context, tolerated []string, args ...string) (string, string, string, error) {
	out, err := a.run(ctx, "claude", args...)
	return setup.ClaudeAnswer(args, out, err, tolerated)
}

// run runs the CLI of an agent within a minute (proc.Stdout).
func (a Adapter) run(ctx context.Context, name string, args ...string) (string, error) {
	return proc.Stdout(ctx, "", append([]string{name}, args...), cliTimeout)
}
