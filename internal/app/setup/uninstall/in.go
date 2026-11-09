// Package uninstall is the use case UninstallSetup, the command `polix uninstall`: it removes the plugin from the
// agents and the local marketplace, and the hooks of polix from the repository.
package uninstall

import (
	"context"
	"errors"
	"io"
	"os"
	"slices"
	"strings"

	"github.com/mlophez/polix-ai/internal/app/setup"

	"github.com/charmbracelet/x/term"
	"github.com/spf13/cobra"

	"github.com/mlophez/polix-ai/internal/platform/build"
	"github.com/mlophez/polix-ai/internal/platform/cli"
)

// Request is what the caller asks of the use case UninstallSetup, whatever the transport: the
// command fills it from its flags.
type Request struct {
	User    bool     // only the plugin of the user, not the hooks of the repository
	Dir     string   // any folder of the repository whose hooks to remove
	Agents  []string // the agents asked, parsed (parseAgents); empty for both
	Quiet   bool     // only print what changed
	Version string   // version of the running polix
}

// Response is what the use case UninstallSetup gives back to the caller, in the shape of the
// transport: the command serializes it to the console.
type Response struct {
	Changes []setup.Change // what changed, also when the uninstall failed halfway
	Quiet   bool           // only print what changed
}

// Port is what the shell of UninstallSetup needs from the outside: the home, the repository, the
// files of the agents and their CLIs. It speaks in domain values, and every method takes the context of the
// caller.
type Port interface {
	// Home returns the home folder of the user.
	Home(ctx context.Context) (string, error)
	// Root returns the root of the repository that holds dir; empty outside one.
	Root(ctx context.Context, dir string) string
	// Confirm asks for confirmation of the summary of what is about to be removed; setup.ErrCancelled when refused.
	Confirm(ctx context.Context, summary string) error
	// UnregisterPlugin unregisters the plugin from an agent, for the user.
	UnregisterPlugin(ctx context.Context, s setup.Setup, agent string) ([]setup.Change, error)
	// UnregisterHooks removes the hooks of polix from a hooks file, keeping every other setting and hook.
	UnregisterHooks(ctx context.Context, file string) ([]setup.Change, error)
	// RemovePlugin removes the local marketplace dir, and its parent when nothing else is left in it.
	RemovePlugin(ctx context.Context, dir string) ([]setup.Change, error)
}

// Command is the command `polix uninstall`, the entry of the use case UninstallSetup: it turns the
// flags into a Request and validates them, tells the adapter whether to ask for confirmation,
// injects the adapter as the port, runs the shell and serializes the Response.
func Command() *cobra.Command {
	req := Request{Version: build.Current()}
	var agents []string
	var yes bool
	c := &cobra.Command{
		Use:   "uninstall",
		Short: "Remove the plugin of polix and the hooks of polix of the repository",
		Long: "Unregister the plugin polix from Claude Code, and from Codex when it was set up with --agent codex " +
			"(Codex support is not implemented), and remove ~/.polix/marketplace/ and, unless --user, the hooks of " +
			"polix from the .claude/settings.json and .codex/hooks.json of the repository of --dir, keeping every " +
			"other setting and hook. The binary stays: delete it by hand.",
		Args: cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			if err := req.parseAgents(agents); err != nil {
				return err
			}
			in, okIn := c.InOrStdin().(*os.File)
			out, okOut := c.OutOrStdout().(*os.File)
			ask := !yes && okIn && okOut && term.IsTerminal(in.Fd()) && term.IsTerminal(out.Fd())
			res, err := run(c.Context(), Adapter{Ask: ask}, req)
			if errors.Is(err, setup.ErrCancelled) {
				return cli.Exit(1)
			}
			res.print(c.OutOrStdout(), err)
			return err
		},
	}
	c.Flags().BoolVarP(&req.User, "user", "u", false, "only the plugin of the user, not the hooks of the repository")
	c.Flags().StringVarP(&req.Dir, "dir", "C", ".", "repository whose hooks to remove")
	c.Flags().StringSliceVarP(&agents, "agent", "a", nil, "agents: claude, codex (default: both; codex is not implemented)")
	c.Flags().BoolVarP(&yes, "yes", "y", false, "do not ask for confirmation")
	c.Flags().BoolVarP(&req.Quiet, "quiet", "q", false, "only print what changed")
	_ = c.RegisterFlagCompletionFunc("agent", cobra.FixedCompletions([]string{setup.Claude, setup.Codex}, cobra.ShellCompDirectiveNoFileComp))
	_ = c.MarkFlagDirname("dir")
	return c
}

// run is the shell, from request to response: it reads through the port the home and the repository
// unless --user, calls the logic once, asks for confirmation, and carries out its output: for each agent the
// plugin and the hooks of the repository, then the local marketplace. The first error stops it.
func run(ctx context.Context, port Port, req Request) (Response, error) {
	res := Response{Quiet: req.Quiet}
	home, err := port.Home(ctx)
	if err != nil {
		return res, err
	}
	var root string
	if !req.User {
		root = port.Root(ctx, req.Dir)
	}
	asked := setup.Setup{Home: home, Root: root, Agents: req.Agents, Version: req.Version}
	if err := asked.Valid(); err != nil {
		return res, err
	}
	out := Decide(asked)
	if err := port.Confirm(ctx, out.Summary); err != nil {
		return res, err
	}
	for _, a := range out.Setup.Agents {
		ch, err := port.UnregisterPlugin(ctx, out.Setup, a)
		res.Changes = append(res.Changes, ch...)
		if err != nil {
			return res, err
		}
		if out.Setup.Root != "" {
			ch, err := port.UnregisterHooks(ctx, out.Setup.HooksFile(a))
			res.Changes = append(res.Changes, ch...)
			if err != nil {
				return res, err
			}
		}
	}
	if out.RemovePlugin {
		ch, err := port.RemovePlugin(ctx, out.Setup.MarketplaceDir())
		res.Changes = append(res.Changes, ch...)
		if err != nil {
			return res, err
		}
	}
	return res, nil
}

// parseAgents reads the agents asked with --agent into the request: trimmed, in lower case and once each; an agent
// the domain does not know (setup.Setup.Valid: neither Claude nor Codex) is an error.
func (req *Request) parseAgents(asked []string) error {
	var agents []string
	for _, a := range asked {
		a = strings.ToLower(strings.TrimSpace(a))
		if !slices.Contains(agents, a) {
			agents = append(agents, a)
		}
	}
	s := setup.Setup{Agents: agents, Version: req.Version}
	if err := s.Valid(); err != nil {
		return err
	}
	req.Agents = agents
	return nil
}

// print writes what changed to stdout, or that there was nothing to remove when the uninstall did not fail (err).
func (res Response) print(stdout io.Writer, err error) {
	u := cli.NewUI(stdout)
	for _, ch := range res.Changes {
		if ch.Todo() {
			u.Warn("run by hand: %s", cli.Code(ch.Path))
			continue
		}
		u.Change(ch.Removal(), ch.Action, ch.Path)
	}
	if err == nil && len(res.Changes) == 0 && !res.Quiet {
		u.OK("nothing to remove")
	}
}
