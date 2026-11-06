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
)

// UninstallSetupAdapter implements UninstallSetupPort on the real disk, git, terminal and CLIs of the agents. The
// entry fills it and passes it as the port; there is no constructor.
type UninstallSetupAdapter struct {
	Ask bool // a person is at the terminal and did not pass --yes: Confirm asks them
}

// uninstallSetupCLITimeout bounds every call to the CLI of an agent.
const uninstallSetupCLITimeout = time.Minute

// Home returns the home folder of the user.
func (a UninstallSetupAdapter) Home(_ context.Context) (string, error) { return os.UserHomeDir() }

// Root returns the root of the repository of dir, or "" outside one.
func (a UninstallSetupAdapter) Root(ctx context.Context, dir string) string {
	repo, err := git.Open(ctx, dir)
	if err != nil {
		return ""
	}
	return repo.Root
}

// Confirm asks the person, with Ask, to confirm the summary; ErrCancelled when they do not.
func (a UninstallSetupAdapter) Confirm(_ context.Context, summary string) error {
	if !a.Ask {
		return nil
	}
	ok := true
	err := huh.NewConfirm().Title("Uninstall polix?").Description(summary).
		Affirmative("Uninstall").Negative("Cancel").Value(&ok).Run()
	if errors.Is(err, huh.ErrUserAborted) || err == nil && !ok {
		return ErrCancelled
	}
	return err
}

// UnregisterPlugin unregisters the plugin. Codex: `codex plugin marketplace remove` when its CLI is here and the
// plugin enabled, and the table of the plugin removed from its configuration. Claude Code, when its CLI is here:
// `claude plugin uninstall` and `claude plugin marketplace remove`, tolerating what is already gone.
func (a UninstallSetupAdapter) UnregisterPlugin(ctx context.Context, s Setup, agent string) ([]Change, error) {
	if agent == Codex {
		config, _, _ := file.Read(s.CodexConfigFile())
		if _, err := exec.LookPath("codex"); err == nil && CodexEnabled(config) {
			_, _ = a.run(ctx, "codex", "plugin", "marketplace", "remove", Name)
		}
		return a.editFile(s.CodexConfigFile(), func(data []byte, existed bool) ([]byte, string, error) {
			data, action := CodexPlugin(data, existed, false)
			return data, action, nil
		})
	}
	if _, err := exec.LookPath("claude"); err != nil {
		return nil, nil //nolint:nilerr // without the CLI of Claude Code there is no plugin to unregister
	}
	outcome, _, _, err := a.claude(ctx, []string{"not_installed"}, "plugin", "uninstall", PluginID, "--scope", "user", "--json")
	if err != nil {
		return nil, err
	}
	if _, _, _, err := a.claude(ctx, []string{"not_configured"}, "plugin", "marketplace", "remove", Name, "--json"); err != nil {
		return nil, err
	}
	if outcome == "ok" {
		return Changes("unregistered", PluginID+" in Claude Code")
	}
	return nil, nil
}

// UnregisterHooks removes the hooks of polix from path, and the file when polix was all it held.
func (a UninstallSetupAdapter) UnregisterHooks(_ context.Context, path string) ([]Change, error) {
	return a.editFile(path, UnregisterHooks)
}

// RemovePlugin removes the local marketplace, and ~/.polix when nothing else is left in it.
func (a UninstallSetupAdapter) RemovePlugin(_ context.Context, dir string) ([]Change, error) {
	if !file.Exists(dir) {
		return nil, nil
	}
	if err := os.RemoveAll(dir); err != nil {
		return nil, err
	}
	_ = os.Remove(filepath.Dir(dir))
	return Changes("removed", dir)
}

// editFile reads path, asks edit for its new content and what that does to it, and carries it out: nothing,
// removed, or written. The errors of edit name the file.
func (a UninstallSetupAdapter) editFile(path string, edit func([]byte, bool) ([]byte, string, error)) ([]Change, error) {
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
func (a UninstallSetupAdapter) claude(ctx context.Context, tolerated []string, args ...string) (string, string, string, error) {
	out, err := a.run(ctx, "claude", args...)
	return ClaudeAnswer(args, out, err, tolerated)
}

// run runs the CLI of an agent within a minute (proc.Stdout).
func (a UninstallSetupAdapter) run(ctx context.Context, name string, args ...string) (string, error) {
	return proc.Stdout(ctx, "", append([]string{name}, args...), uninstallSetupCLITimeout)
}
