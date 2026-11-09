// Package refresh is the use case RefreshSetup, the hidden command `polix __refresh`: it brings what polix
// installed in line with a new binary, installing nothing new.
package refresh

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/mlophez/polix-ai/internal/app/setup"

	"github.com/spf13/cobra"

	"github.com/mlophez/polix-ai/internal/platform/build"
	"github.com/mlophez/polix-ai/internal/platform/cli"
)

// Request is what the caller asks of the use case RefreshSetup, whatever the transport: the command
// fills it from its flags.
type Request struct {
	Dir     string // any folder of the repository whose hooks to refresh
	Quiet   bool   // one line in Spanish when something changed, for the session start hook
	Hooks   bool   // also refresh the hooks of polix of the repository
	Version string // version of the running polix
}

// Response is what the use case RefreshSetup gives back to the caller, in the shape of the transport:
// the command serializes it to the console.
type Response struct {
	Changes []setup.Change // what changed, also when the refresh failed
	Quiet   bool           // one line in Spanish instead of the changes
}

// Port is what the shell of RefreshSetup needs from the outside: the home, the repository, the plugin
// of the binary, the files of the agents and their CLIs. It speaks in domain values, and every method takes the
// context of the caller.
type Port interface {
	// Home returns the home folder of the user.
	Home(ctx context.Context) (string, error)
	// Root returns the root of the repository that holds dir; empty outside one.
	Root(ctx context.Context, dir string) string
	// Plugin returns the files of the plugin embedded in the binary, by path below polix/.
	Plugin(ctx context.Context) (map[string][]byte, error)
	// ReadFiles returns the content of the files of paths that exist and can be read.
	ReadFiles(ctx context.Context, paths []string) map[string][]byte
	// WritePlugin makes dir hold exactly files (relative to it, with slashes).
	WritePlugin(ctx context.Context, dir string, files map[string][]byte) ([]setup.Change, error)
	// UpdatePlugin tells the agents that use the plugin of s that it changed, so they copy the new version.
	UpdatePlugin(ctx context.Context, s setup.Setup)
	// RegisterHooks writes the hooks of polix for agent in its hooks file, keeping every other setting and hook.
	RegisterHooks(ctx context.Context, file, agent string) ([]setup.Change, error)
}

// Command is the hidden command `polix __refresh`, the entry of the use case RefreshSetup, which the
// new binary runs after an update: it turns the flags into a Request, injects the adapter as the
// port, runs the shell and serializes the Response to the console.
func Command() *cobra.Command {
	req := Request{Dir: ".", Version: build.Current()}
	c := &cobra.Command{
		Use:    "__refresh",
		Hidden: true,
		Args:   cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			res, err := run(c.Context(), Adapter{}, req)
			res.print(c.OutOrStdout())
			return err
		},
	}
	c.Flags().BoolVarP(&req.Quiet, "quiet", "q", false, "one line in Spanish when something changed, for the session start hook")
	c.Flags().BoolVar(&req.Hooks, "hooks", false, "also refresh the hooks of polix of the repository")
	return c
}

// run is the shell, from request to response: it reads through the port the home, with --hooks the
// repository, the plugin of the binary and the files that tell what polix installed, calls the logic once and
// carries out its output: the plugin, telling the agents when it changed, and the hooks. Every error is joined.
func run(ctx context.Context, port Port, req Request) (Response, error) {
	res := Response{Quiet: req.Quiet}
	home, err := port.Home(ctx)
	if err != nil {
		return res, err
	}
	var root string
	if req.Hooks {
		root = port.Root(ctx, req.Dir)
	}
	s := setup.Setup{Home: home, Root: root, Agents: []string{setup.Claude, setup.Codex}, Version: req.Version}
	if err := s.Valid(); err != nil {
		return res, err
	}
	embedded, err := port.Plugin(ctx)
	if err != nil {
		return res, err
	}
	files := port.ReadFiles(ctx, []string{s.PluginManifest(), s.HooksFile(setup.Claude), s.HooksFile(setup.Codex)})
	out := Decide(s, files, embedded)
	if out.Failure != "" {
		return res, errors.New(out.Failure)
	}
	var errs []error
	if out.Plugin {
		ch, err := port.WritePlugin(ctx, s.MarketplaceDir(), out.Files)
		res.Changes, errs = append(res.Changes, ch...), append(errs, err)
		if err == nil && len(ch) != 0 {
			port.UpdatePlugin(ctx, s)
		}
	}
	for _, a := range out.Hooks {
		ch, err := port.RegisterHooks(ctx, s.HooksFile(a), a)
		res.Changes, errs = append(res.Changes, ch...), append(errs, err)
	}
	return res, errors.Join(errs...)
}

// print writes what changed to stdout: with --quiet one line in Spanish, else one line per change.
func (res Response) print(stdout io.Writer) {
	if len(res.Changes) == 0 {
		return
	}
	if res.Quiet {
		fmt.Fprintln(stdout, "polix ha actualizado su plugin: se carga en la próxima sesión.")
		return
	}
	u := cli.NewUI(stdout)
	for _, ch := range res.Changes {
		if ch.Todo() {
			u.Warn("run by hand: %s", cli.Code(ch.Path))
			continue
		}
		u.Change(ch.Removal(), ch.Action, ch.Path)
	}
}
