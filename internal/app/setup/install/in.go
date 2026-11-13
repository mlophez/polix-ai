// Package install is the use case InstallSetup, the command `polix install`: it writes the plugin of the user,
// registers it with the agents, and registers the hooks of polix of the agents and of git in the repository; each
// part alone with its flag (--skills, --hooks, --githooks).
package install

import (
	"context"
	"errors"
	"os"
	"slices"
	"strings"

	"github.com/mlophez/polix-ai/internal/app/setup"

	"github.com/charmbracelet/x/term"
	"github.com/spf13/cobra"

	"github.com/mlophez/polix-ai/internal/platform/build"
	"github.com/mlophez/polix-ai/internal/platform/cli"
)

// Request is what the caller asks of the use case InstallSetup, whatever the transport: the command
// fills it from its flags.
type Request struct {
	// Parts are the parts asked with --skills, --hooks and --githooks; none asks for every one. Only the hooks of
	// git need no confirmation.
	Parts   setup.Parts
	Dir     string   // any folder of the repository whose hooks to register
	Agents  []string // the agents asked, parsed (parseAgents); empty for the default ones
	Quiet   bool     // only print what changed
	Version string   // version of the running polix
}

// Response is what the use case InstallSetup gives back to the caller, in the shape of the transport:
// the command serializes it to the console.
type Response struct {
	Changes      []setup.Change // what changed, also when the install failed halfway
	Version      string         // version of the running polix
	Agents       []string       // the agents installed for
	Skills       []string       // the skills of the plugin
	PluginAgents []string       // the agents of the plugin
	HooksFiles   []string       // the hooks files registered, one per agent; none outside a repository
	Parts        setup.Parts    // the parts written: those asked, else every one
	Quiet        bool           // only print what changed
	NotOnPath    bool           // polix is not on the PATH, which the hooks run
	Duplicates   []string       // other settings of Claude Code that also register the hooks of polix
	// Unlisted are the hooks of git written that git does not list for their events: a git older than 2.54 does
	// not run the hooks of its configuration.
	Unlisted []setup.GitHook
	// Disabled are the hooks of git written that git lists as disabled (hook.<name>.enabled=false): it does not
	// run them, and writing them again does not enable them.
	Disabled []setup.GitHook
}

// Port is what the shell of InstallSetup needs from the outside: the home, the repository, the plugin
// of the binary, the files of the agents and their CLIs. It speaks in domain values, and every method takes the
// context of the caller.
type Port interface {
	// Home returns the home folder of the user.
	Home(ctx context.Context) (string, error)
	// Root returns the root of the repository that holds dir; empty outside one.
	Root(ctx context.Context, dir string) string
	// Confirm asks for confirmation of the summary of what is about to be written; setup.ErrCancelled when refused.
	Confirm(ctx context.Context, summary string) error
	// Plugin returns the files of the plugin embedded in the binary, by path below polix/.
	Plugin(ctx context.Context) (map[string][]byte, error)
	// MarketplaceFiles returns the files of the local marketplace that holds the plugin p, relative to it, with
	// slashes; the error tells why a manifest of the plugin cannot take its version.
	MarketplaceFiles(ctx context.Context, p setup.Plugin) (map[string][]byte, error)
	// WritePlugin makes dir hold exactly files (relative to it, with slashes): it writes the ones that differ and
	// removes the rest.
	WritePlugin(ctx context.Context, dir string, files map[string][]byte) ([]setup.Change, error)
	// RegisterPlugin registers the local marketplace and its plugin with an agent, for the user.
	RegisterPlugin(ctx context.Context, s setup.Setup, agent string) ([]setup.Change, error)
	// RegisterHooks writes hooks, the hooks of polix of an agent by event, in a hooks file in place of the ones of
	// polix it had, keeping every other setting and hook.
	RegisterHooks(ctx context.Context, file string, hooks map[string][]setup.HookGroup) ([]setup.Change, error)
	// Registered reports, by path, whether each of the files of paths exists, can be read and registers a hook of
	// polix.
	Registered(ctx context.Context, paths []string) map[string]bool
	// OnPath returns why the program name is not on the PATH; nil when it is.
	OnPath(ctx context.Context, name string) error
	// RegisterGitHook writes the hook of git h into the local configuration of the repository at root, in place of
	// the values it had, keeping every other key.
	RegisterGitHook(ctx context.Context, root string, h setup.GitHook) ([]setup.Change, error)
	// Unlisted returns the hooks, of hooks, that git does not list for their events in the repository at root: all
	// of them when git cannot list the hooks of its configuration; and apart, those it lists as disabled.
	Unlisted(ctx context.Context, root string, hooks []setup.GitHook) (unlisted, disabled []setup.GitHook)
}

// Command is the command `polix install`, the entry of the use case InstallSetup: it turns the flags
// into a Request and validates them, asks for confirmation when a person is at the terminal, injects
// the adapter as the port, runs the shell and serializes the Response to the console.
func Command() *cobra.Command {
	req := Request{Version: build.Current()}
	var agents []string
	var yes, skills, hooks, gitHooks bool
	c := &cobra.Command{
		Use:   "install",
		Short: "Install the plugin of polix for the user and register its hooks in the repository",
		Long: "Install what the agents need to run polix, from this binary: by default every part below, or only those\n" +
			"asked with --skills, --hooks and --githooks, which combine.\n\n" +
			"Claude Code is the supported agent and the default. Codex is opt-in, set up only with --agent codex: its\n" +
			"support is not implemented in this release and the setup is untested.\n\n" +
			"--skills: for the user, the plugin polix, with skills (/polix:workflow, /polix:add-rule, /polix:risk,\n" +
			"/polix:doctor, /polix:init, /polix:plan, /polix:build, /polix:review, /polix:audit) and agents\n" +
			"(polix:workflow-author, polix:rules-author, polix:builder, polix:verifier, polix:reviewer,\n" +
			"polix:rules-reviewer, polix:auditor) that help in any repository. It is written to the local marketplace\n" +
			"~/.polix/marketplace/ and registered with each agent:\n" +
			"  Claude Code  claude plugin marketplace add, claude plugin install polix@polix --scope user\n" +
			"  Codex        codex plugin marketplace add, and enabled in ~/.codex/config.toml (not implemented)\n\n" +
			"--hooks: for the repository of --dir, the hooks of the agents that call polix hook, to commit with its\n" +
			"polix.yaml:\n" +
			"  Claude Code  .claude/settings.json\n" +
			"  Codex        .codex/hooks.json (not implemented)\n\n" +
			"--githooks: for the repository of --dir, the hooks of git that call polix githook on pre-commit and\n" +
			"commit-msg, in the local git configuration (hook.polix-pre-commit, hook.polix-commit-msg; git 2.54 or\n" +
			"later runs them), also without githooks in polix.yaml. Alone it takes no --agent and asks for no\n" +
			"confirmation; it prints only what changed, and exits 1 when git does not run them. A polix.yaml with\n" +
			"githooks runs it on every session start.\n\n" +
			"Outside a git repository, polix install writes only the plugin; --hooks and --githooks fail there.\n" +
			"Every other setting and hook of those files is kept. Running it again updates what changed; polix update " +
			"refreshes the plugin after every update.",
		Example: "  # The plugin for this machine and the hooks for this repository\n  polix install\n\n" +
			"  # Only the plugin, from anywhere\n  polix install --skills\n\n" +
			"  # Only the hooks of the agents and of git of another repository\n  polix install --hooks --githooks --dir ~/Code/service\n\n" +
			"  # Only the hooks of git of this repository\n  polix install --githooks\n\n" +
			"  # Codex too (not implemented: untested)\n  polix install --agent claude,codex",
		Args: cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			parts := setup.Parts{Skills: skills, Hooks: hooks, GitHooks: gitHooks}
			if err := parts.Valid(); err != nil {
				return err
			}
			req.Parts = parts
			if req.Parts.OnlyGitHooks() && c.Flags().Changed("agent") {
				return errors.New("--githooks alone installs only the hooks of git of the repository: it takes no --agent")
			}
			if err := req.parseAgents(agents); err != nil {
				return err
			}
			in, okIn := c.InOrStdin().(*os.File)
			out, okOut := c.OutOrStdout().(*os.File)
			ask := !yes && okIn && okOut && term.IsTerminal(in.Fd()) && term.IsTerminal(out.Fd())
			res, err := run(c.Context(), Adapter{Ask: ask}, req)
			res.printChanges(c.OutOrStdout())
			if errors.Is(err, setup.ErrCancelled) {
				return cli.Exit(1)
			}
			if err != nil {
				return err
			}
			res.print(c.OutOrStdout(), c.ErrOrStderr())
			return res.exit()
		},
	}
	c.Flags().BoolVar(&skills, "skills", false, "only the plugin of the user, with its skills and agents")
	c.Flags().BoolVar(&hooks, "hooks", false, "only the hooks of the agents of the repository")
	c.Flags().BoolVar(&gitHooks, "githooks", false, "only the hooks of git of the repository")
	// --user is the --skills of polix 0.6 and earlier, which polix update of those releases runs on the new binary.
	c.Flags().BoolVarP(&skills, "user", "u", false, "only the plugin of the user (use --skills)")
	_ = c.Flags().MarkHidden("user")
	c.Flags().StringVarP(&req.Dir, "dir", "C", ".", "repository whose hooks to register")
	c.Flags().StringSliceVarP(&agents, "agent", "a", nil, "agents: claude, codex (default: claude; codex is not implemented)")
	c.Flags().BoolVarP(&yes, "yes", "y", false, "do not ask for confirmation")
	c.Flags().BoolVarP(&req.Quiet, "quiet", "q", false, "only print what changed")
	_ = c.RegisterFlagCompletionFunc("agent", cobra.FixedCompletions([]string{setup.Claude, setup.Codex}, cobra.ShellCompDirectiveNoFileComp))
	_ = c.MarkFlagDirname("dir")
	return c
}

// run is the shell, from request to response: it reads through the port the home, with the plugin or the hooks of
// the agents, the plugin of the binary, with the plugin, and the repository, with one of its parts; calls the logic
// once; and carries out its output: only the hooks of git (installGitHooks), or the rest (installParts).
func run(ctx context.Context, port Port, req Request) (Response, error) {
	res := Response{Version: req.Version, Parts: req.Parts.Defaulted(), Quiet: req.Quiet}
	asked, embedded, err := read(ctx, port, req)
	if err != nil {
		return res, err
	}
	out := Decide(asked, embedded, req.Parts)
	if out.Failure != "" {
		return res, errors.New(out.Failure)
	}
	if out.Parts.OnlyGitHooks() {
		return installGitHooks(ctx, port, out, res)
	}
	return installParts(ctx, port, out, res)
}

// read reads through the port what the logic needs for the parts asked: the home with the plugin or the hooks of
// the agents, the plugin of the binary with the plugin, and the repository with the hooks of the agents or of git;
// and builds the setup asked.
func read(ctx context.Context, port Port, req Request) (setup.Setup, map[string][]byte, error) {
	p := req.Parts.Defaulted()
	var home, root string
	var embedded map[string][]byte
	var err error
	if p.Skills || p.Hooks {
		if home, err = port.Home(ctx); err != nil {
			return setup.Setup{}, nil, err
		}
	}
	if p.Skills {
		if embedded, err = port.Plugin(ctx); err != nil {
			return setup.Setup{}, nil, err
		}
	}
	if p.Repository() {
		root = port.Root(ctx, req.Dir)
	}
	asked := setup.Setup{Home: home, Root: root, Agents: req.Agents, Version: req.Version}
	if err := asked.Valid(); err != nil {
		return setup.Setup{}, nil, err
	}
	return asked, embedded, nil
}

// installGitHooks carries out an output of only the hooks of git: it writes each of them, without confirmation,
// and asks which of them git does not list or lists as disabled.
func installGitHooks(ctx context.Context, port Port, out Output, res Response) (Response, error) {
	ch, err := registerGitHooks(ctx, port, out)
	res.Changes = ch
	if err != nil {
		return res, err
	}
	res.Unlisted, res.Disabled = port.Unlisted(ctx, out.Setup.Root, out.GitHooks)
	return res, nil
}

// registerGitHooks writes each hook of git of the output into the repository, and returns their changes and their
// errors joined, so one that fails does not keep the others from being written.
func registerGitHooks(ctx context.Context, port Port, out Output) ([]setup.Change, error) {
	var changes []setup.Change
	var errs []error
	for _, h := range out.GitHooks {
		ch, err := port.RegisterGitHook(ctx, out.Setup.Root, h)
		changes, errs = append(changes, ch...), append(errs, err)
	}
	return changes, errors.Join(errs...)
}

// installParts carries out the output of the parts other than only the hooks of git: with the plugin it asks the
// port for the files of the marketplace; it asks for confirmation; it writes the plugin, then for each agent its
// registration and its hooks in the repository, as the parts ask, then the hooks of git of the output and which of
// them git does not list or lists as disabled; then it asks whether polix is on the PATH and, with the hooks of the
// agents, which other settings register them too.
func installParts(ctx context.Context, port Port, out Output, res Response) (Response, error) {
	var files map[string][]byte
	if out.Parts.Skills {
		var err error
		if files, err = port.MarketplaceFiles(ctx, out.Plugin); err != nil {
			return res, err
		}
	}
	res.Agents, res.Skills, res.PluginAgents, res.HooksFiles = out.Setup.Agents, out.Skills, out.PluginAgents, out.HooksFiles
	if err := port.Confirm(ctx, out.Summary); err != nil {
		return res, err
	}
	if out.Parts.Skills {
		ch, err := port.WritePlugin(ctx, out.Setup.MarketplaceDir(), files)
		res.Changes = ch
		if err != nil {
			return res, err
		}
	}
	// An agent that refuses the plugin does not keep the hooks of the repository from being registered.
	var errs []error
	for _, a := range out.Setup.Agents {
		if out.Parts.Skills {
			ch, err := port.RegisterPlugin(ctx, out.Setup, a)
			res.Changes, errs = append(res.Changes, ch...), append(errs, err)
		}
		if len(out.HooksFiles) != 0 {
			ch, err := port.RegisterHooks(ctx, out.Setup.HooksFile(a), out.Setup.Hooks(a))
			res.Changes, errs = append(res.Changes, ch...), append(errs, err)
		}
	}
	ch, err := registerGitHooks(ctx, port, out)
	res.Changes, errs = append(res.Changes, ch...), append(errs, err)
	if err := errors.Join(errs...); err != nil {
		return res, err
	}
	if len(out.GitHooks) != 0 {
		res.Unlisted, res.Disabled = port.Unlisted(ctx, out.Setup.Root, out.GitHooks)
	}
	res.NotOnPath = port.OnPath(ctx, "polix") != nil
	if len(out.HooksFiles) != 0 {
		res.Duplicates = out.Setup.Duplicates(port.Registered(ctx, out.Setup.ClaudeSettingsFiles()))
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
