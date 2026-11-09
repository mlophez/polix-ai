package setup

import (
	"context"
	"errors"
	"io"
	"os"
	"slices"
	"strings"

	"github.com/charmbracelet/x/term"
	"github.com/spf13/cobra"

	"github.com/mlophez/polix-ai/pkg/build"
	"github.com/mlophez/polix-ai/pkg/cli"
)

// InstallSetupRequest is what the caller asks of the use case InstallSetup, whatever the transport: the command
// fills it from its flags.
type InstallSetupRequest struct {
	User    bool     // only the plugin of the user, not the hooks of the repository
	Dir     string   // any folder of the repository whose hooks to register
	Agents  []string // the agents asked, parsed (ParseAgents); empty for the default ones
	Quiet   bool     // only print what changed
	Version string   // version of the running polix
}

// InstallSetupResponse is what the use case InstallSetup gives back to the caller, in the shape of the transport:
// the command serializes it to the console.
type InstallSetupResponse struct {
	Changes      []Change // what changed, also when the install failed halfway
	Version      string   // version of the running polix
	Agents       []string // the agents installed for
	Skills       []string // the skills of the plugin
	PluginAgents []string // the agents of the plugin
	HooksFiles   []string // the hooks files registered, one per agent; none outside a repository
	User         bool     // only the plugin of the user was asked
	Quiet        bool     // only print what changed
	NotOnPath    bool     // polix is not on the PATH, which the hooks run
	Duplicates   []string // other settings of Claude Code that also register the hooks of polix
}

// InstallSetupPort is what the shell of InstallSetup needs from the outside: the home, the repository, the plugin
// of the binary, the files of the agents and their CLIs. It speaks in domain values, and every method takes the
// context of the caller.
type InstallSetupPort interface {
	// Home returns the home folder of the user.
	Home(ctx context.Context) (string, error)
	// Root returns the root of the repository that holds dir; empty outside one.
	Root(ctx context.Context, dir string) string
	// Confirm asks for confirmation of the summary of what is about to be written; ErrCancelled when refused.
	Confirm(ctx context.Context, summary string) error
	// Plugin returns the files of the plugin embedded in the binary, by path below polix/.
	Plugin(ctx context.Context) (map[string][]byte, error)
	// WritePlugin makes dir hold exactly files (relative to it, with slashes): it writes the ones that differ and
	// removes the rest.
	WritePlugin(ctx context.Context, dir string, files map[string][]byte) ([]Change, error)
	// RegisterPlugin registers the local marketplace and its plugin with an agent, for the user.
	RegisterPlugin(ctx context.Context, s Setup, agent string) ([]Change, error)
	// RegisterHooks writes the hooks of polix for agent in its hooks file, keeping every other setting and hook.
	RegisterHooks(ctx context.Context, file, agent string) ([]Change, error)
	// ReadFiles returns the content of the files of paths that exist and can be read.
	ReadFiles(ctx context.Context, paths []string) map[string][]byte
	// OnPath returns why the program name is not on the PATH; nil when it is.
	OnPath(ctx context.Context, name string) error
}

// InstallSetupCommand is the command `polix install`, the entry of the use case InstallSetup: it turns the flags
// into an InstallSetupRequest and validates them, asks for confirmation when a person is at the terminal, injects
// the adapter as the port, runs the shell and serializes the InstallSetupResponse to the console.
func InstallSetupCommand() *cobra.Command {
	req := InstallSetupRequest{Version: build.Current()}
	var agents []string
	var yes bool
	c := &cobra.Command{
		Use:   "install",
		Short: "Install the plugin of polix for the user and register its hooks in the repository",
		Long: "Install what the agents need to run polix, from this binary.\n\n" +
			"Claude Code is the supported agent and the default. Codex is opt-in, set up only with --agent codex: its\n" +
			"support is not implemented in this release and the setup is untested.\n\n" +
			"For the user, the plugin polix, with skills (/polix:workflow, /polix:add-rule, /polix:doctor, /polix:init,\n" +
			"/polix:plan, /polix:build, /polix:review, /polix:audit) and agents (polix:workflow-author, polix:rules-author,\n" +
			"polix:builder, polix:reviewer, polix:rules-reviewer, polix:auditor) that help in any repository. It is written\n" +
			"to the local marketplace ~/.polix/marketplace/ and registered with each agent:\n" +
			"  Claude Code  claude plugin marketplace add, claude plugin install polix@polix --scope user\n" +
			"  Codex        codex plugin marketplace add, and enabled in ~/.codex/config.toml (not implemented)\n\n" +
			"For the repository of --dir, unless --user, the hooks that call polix hook, to commit with its polix.yaml:\n" +
			"  Claude Code  .claude/settings.json\n" +
			"  Codex        .codex/hooks.json (not implemented)\n\n" +
			"Every other setting and hook of those files is kept. Running it again updates what changed; polix update " +
			"refreshes the plugin after every update.",
		Example: "  # The plugin for this machine and the hooks for this repository\n  polix install\n\n" +
			"  # Only the plugin, from anywhere\n  polix install --user\n\n" +
			"  # The hooks of another repository\n  polix install --dir ~/Code/service\n\n" +
			"  # Codex too (not implemented: untested)\n  polix install --agent claude,codex",
		Args: cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			var err error
			if req.Agents, err = ParseAgents(agents); err != nil {
				return err
			}
			in, okIn := c.InOrStdin().(*os.File)
			out, okOut := c.OutOrStdout().(*os.File)
			ask := !yes && okIn && okOut && term.IsTerminal(in.Fd()) && term.IsTerminal(out.Fd())
			res, err := installSetup(c.Context(), InstallSetupAdapter{Ask: ask}, req)
			res.printChanges(c.OutOrStdout())
			if errors.Is(err, ErrCancelled) {
				return cli.Exit(1)
			}
			if err != nil {
				return err
			}
			res.print(c.OutOrStdout(), c.ErrOrStderr())
			return nil
		},
	}
	c.Flags().BoolVarP(&req.User, "user", "u", false, "only the plugin of the user, not the hooks of the repository")
	c.Flags().StringVarP(&req.Dir, "dir", "C", ".", "repository whose hooks to register")
	c.Flags().StringSliceVarP(&agents, "agent", "a", nil, "agents: claude, codex (default: claude; codex is not implemented)")
	c.Flags().BoolVarP(&yes, "yes", "y", false, "do not ask for confirmation")
	c.Flags().BoolVarP(&req.Quiet, "quiet", "q", false, "only print what changed")
	_ = c.RegisterFlagCompletionFunc("agent", cobra.FixedCompletions([]string{Claude, Codex}, cobra.ShellCompDirectiveNoFileComp))
	_ = c.MarkFlagDirname("dir")
	return c
}

// installSetup is the shell, from request to response: it reads through the port the home, the repository unless
// --user and the plugin of the binary, calls the logic once, asks for confirmation, and carries out its output: the
// plugin, then for each agent its registration and the hooks of the repository; then it asks whether polix is on
// the PATH and which other settings register the hooks too.
func installSetup(ctx context.Context, port InstallSetupPort, req InstallSetupRequest) (InstallSetupResponse, error) {
	res := InstallSetupResponse{Version: req.Version, User: req.User, Quiet: req.Quiet}
	home, err := port.Home(ctx)
	if err != nil {
		return res, err
	}
	var root string
	if !req.User {
		root = port.Root(ctx, req.Dir)
	}
	embedded, err := port.Plugin(ctx)
	if err != nil {
		return res, err
	}
	out := InstallSetup(home, root, req.Agents, req.Version, embedded)
	if out.Failure != "" {
		return res, errors.New(out.Failure)
	}
	res.Agents, res.Skills, res.PluginAgents, res.HooksFiles = out.Setup.Agents, out.Skills, out.PluginAgents, out.HooksFiles
	if err := port.Confirm(ctx, out.Summary); err != nil {
		return res, err
	}
	if res.Changes, err = port.WritePlugin(ctx, out.Setup.MarketplaceDir(), out.Files); err != nil {
		return res, err
	}
	// An agent that refuses the plugin does not keep the hooks of the repository from being registered.
	var errs []error
	for _, a := range out.Setup.Agents {
		ch, err := port.RegisterPlugin(ctx, out.Setup, a)
		res.Changes, errs = append(res.Changes, ch...), append(errs, err)
		if out.Setup.Root != "" {
			ch, err := port.RegisterHooks(ctx, out.Setup.HooksFile(a), a)
			res.Changes, errs = append(res.Changes, ch...), append(errs, err)
		}
	}
	if err := errors.Join(errs...); err != nil {
		return res, err
	}
	res.NotOnPath = port.OnPath(ctx, "polix") != nil
	if out.Setup.Root != "" {
		res.Duplicates = out.Setup.Duplicates(port.ReadFiles(ctx, out.Setup.ClaudeSettingsFiles()))
	}
	return res, nil
}

// printChanges writes what changed to stdout: a command to run by hand as a warning, the rest one per line.
func (res InstallSetupResponse) printChanges(stdout io.Writer) {
	u := cli.NewUI(stdout)
	for _, ch := range res.Changes {
		if ch.Todo() {
			u.Warn("run by hand: %s", cli.Code(ch.Path))
			continue
		}
		u.Change(ch.Removal(), ch.Action, ch.Path)
	}
}

// print writes, unless quiet, what the install leaves and what to do next to stdout, and the settings that also
// register the hooks to stderr.
func (res InstallSetupResponse) print(stdout, stderr io.Writer) {
	u := cli.NewUI(stdout)
	if !res.Quiet {
		if len(res.Changes) == 0 {
			u.OK("polix %s is already installed", res.Version)
		} else {
			u.OK("polix %s installed", res.Version)
		}
		for _, a := range res.Agents {
			switch a {
			case Claude:
				u.Line("  Claude Code: skills %s and agents %s; start a new session to load them.",
					cli.Code(strings.Join(installSetupPrefixed("/polix:", res.Skills), " ")),
					cli.Code(strings.Join(installSetupPrefixed("polix:", res.PluginAgents), " ")))
			case Codex:
				u.Line("  Codex: the plugin %s with the skills %s; start a new session to load them.",
					cli.Code(PluginID), cli.Code(strings.Join(res.Skills, " ")))
			}
		}
		switch {
		case len(res.HooksFiles) > 0:
			u.Line("  Hooks registered in %s: commit them with the polix.yaml so the team gets them.",
				strings.Join(res.HooksFiles, ", "))
			if slices.Contains(res.Agents, Codex) {
				u.Line("  Codex runs new or changed hooks once trusted with %s.", cli.Code("/hooks"))
			}
		case !res.User:
			u.Warn("not inside a git repository: run polix install in a repository to register its hooks")
		}
		if slices.Contains(res.Agents, Codex) {
			u.Line("  Codex support is not implemented in this release and the setup is untested.")
		}
		if res.NotOnPath {
			u.Warn("polix is not on the PATH: the hooks run `polix`, add its folder to the PATH")
		}
	}
	ue := cli.NewUI(stderr)
	for _, f := range res.Duplicates {
		ue.Warn("%s also registers the hooks of polix: remove them from there, or Claude Code runs them twice", f)
	}
}

// installSetupPrefixed returns names with prefix before each one.
func installSetupPrefixed(prefix string, names []string) []string {
	out := make([]string, len(names))
	for i, n := range names {
		out[i] = prefix + n
	}
	return out
}
