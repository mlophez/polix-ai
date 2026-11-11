// Package install is the use case InstallSetup, the command `polix install`: it writes the plugin of the user,
// registers it with the agents and registers the hooks of polix in the repository.
package install

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

// Request is what the caller asks of the use case InstallSetup, whatever the transport: the command
// fills it from its flags.
type Request struct {
	User    bool     // only the plugin of the user, not the hooks of the repository
	Dir     string   // any folder of the repository whose hooks to register
	Agents  []string // the agents asked, parsed (parseAgents); empty for the default ones
	Quiet   bool     // only print what changed
	Version string   // version of the running polix
	// GitHooks asks only for the hooks of git of the repository (--githooks): no plugin, no hooks of the agents
	// and no confirmation.
	GitHooks bool
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
	User         bool           // only the plugin of the user was asked
	Quiet        bool           // only print what changed
	NotOnPath    bool           // polix is not on the PATH, which the hooks run
	Duplicates   []string       // other settings of Claude Code that also register the hooks of polix
	// Unlisted are the hooks of git written that git does not list for their events: a git older than 2.54 does
	// not run the hooks of its configuration.
	Unlisted []setup.GitHook
	// Disabled are the hooks of git written that git lists as disabled (hook.<name>.enabled=false): it does not
	// run them, and writing them again does not enable them.
	Disabled []setup.GitHook
	// GitHooksOnly is true when only the hooks of git were asked (--githooks): the command prints only the changes
	// and the warning of Unlisted, and exits 1 with it.
	GitHooksOnly bool
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
	var yes bool
	c := &cobra.Command{
		Use:   "install",
		Short: "Install the plugin of polix for the user and register its hooks in the repository",
		Long: "Install what the agents need to run polix, from this binary.\n\n" +
			"Claude Code is the supported agent and the default. Codex is opt-in, set up only with --agent codex: its\n" +
			"support is not implemented in this release and the setup is untested.\n\n" +
			"For the user, the plugin polix, with skills (/polix:workflow, /polix:add-rule, /polix:risk, /polix:doctor,\n" +
			"/polix:init, /polix:plan, /polix:build, /polix:review, /polix:audit) and agents (polix:workflow-author,\n" +
			"polix:rules-author, polix:builder, polix:reviewer, polix:rules-reviewer, polix:auditor) that help in any\n" +
			"repository. It is written to the local marketplace ~/.polix/marketplace/ and registered with each agent:\n" +
			"  Claude Code  claude plugin marketplace add, claude plugin install polix@polix --scope user\n" +
			"  Codex        codex plugin marketplace add, and enabled in ~/.codex/config.toml (not implemented)\n\n" +
			"For the repository of --dir, unless --user, the hooks that call polix hook, to commit with its polix.yaml:\n" +
			"  Claude Code  .claude/settings.json\n" +
			"  Codex        .codex/hooks.json (not implemented)\n" +
			"and the hooks of git that call polix githook on pre-commit and commit-msg, in the local git configuration\n" +
			"(hook.polix-pre-commit, hook.polix-commit-msg; git 2.54 or later runs them), also without githooks in\n" +
			"polix.yaml.\n\n" +
			"With --githooks, only the hooks of git of the repository: no plugin, no hooks of the agents and no\n" +
			"confirmation; it prints only what changed, and exits 1 when git does not run them. A polix.yaml with\n" +
			"githooks runs it on every session start.\n\n" +
			"Every other setting and hook of those files is kept. Running it again updates what changed; polix update " +
			"refreshes the plugin after every update.",
		Example: "  # The plugin for this machine and the hooks for this repository\n  polix install\n\n" +
			"  # Only the plugin, from anywhere\n  polix install --user\n\n" +
			"  # The hooks of another repository\n  polix install --dir ~/Code/service\n\n" +
			"  # Only the hooks of git of this repository\n  polix install --githooks\n\n" +
			"  # Codex too (not implemented: untested)\n  polix install --agent claude,codex",
		Args: cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			if req.GitHooks && (req.User || c.Flags().Changed("agent")) {
				return errors.New("--githooks installs only the hooks of git of the repository: it takes neither --user nor --agent")
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
	c.Flags().BoolVarP(&req.User, "user", "u", false, "only the plugin of the user, not the hooks of the repository")
	c.Flags().BoolVar(&req.GitHooks, "githooks", false, "only the hooks of git of the repository")
	c.Flags().StringVarP(&req.Dir, "dir", "C", ".", "repository whose hooks to register")
	c.Flags().StringSliceVarP(&agents, "agent", "a", nil, "agents: claude, codex (default: claude; codex is not implemented)")
	c.Flags().BoolVarP(&yes, "yes", "y", false, "do not ask for confirmation")
	c.Flags().BoolVarP(&req.Quiet, "quiet", "q", false, "only print what changed")
	_ = c.RegisterFlagCompletionFunc("agent", cobra.FixedCompletions([]string{setup.Claude, setup.Codex}, cobra.ShellCompDirectiveNoFileComp))
	_ = c.MarkFlagDirname("dir")
	return c
}

// run is the shell, from request to response: it reads through the port the home and the plugin of the binary,
// unless only the hooks of git are asked, and the repository unless --user; calls the logic once; and carries out
// its output: only the hooks of git (installGitHooks), or the whole install (installAll).
func run(ctx context.Context, port Port, req Request) (Response, error) {
	res := Response{Version: req.Version, User: req.User, Quiet: req.Quiet, GitHooksOnly: req.GitHooks}
	asked, embedded, err := read(ctx, port, req)
	if err != nil {
		return res, err
	}
	out := Decide(asked, embedded, req.GitHooks)
	if out.Failure != "" {
		return res, errors.New(out.Failure)
	}
	if out.GitHooksOnly {
		return installGitHooks(ctx, port, out, res)
	}
	return installAll(ctx, port, out, res)
}

// read reads through the port what the logic needs: the home and the plugin of the binary, unless only the hooks
// of git are asked, and the repository unless --user; and builds the setup asked.
func read(ctx context.Context, port Port, req Request) (setup.Setup, map[string][]byte, error) {
	var home string
	var embedded map[string][]byte
	if !req.GitHooks {
		var err error
		if home, err = port.Home(ctx); err != nil {
			return setup.Setup{}, nil, err
		}
		if embedded, err = port.Plugin(ctx); err != nil {
			return setup.Setup{}, nil, err
		}
	}
	var root string
	if !req.User {
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

// installAll carries out the output of the whole install: it asks the port for the files of the marketplace, asks
// for confirmation, writes the plugin, then for each agent its registration and the hooks of the repository, then
// the hooks of git of the repository and which of them git does not list or lists as disabled; then it asks whether
// polix is on the PATH and which other settings register the hooks too.
func installAll(ctx context.Context, port Port, out Output, res Response) (Response, error) {
	files, err := port.MarketplaceFiles(ctx, out.Plugin)
	if err != nil {
		return res, err
	}
	res.Agents, res.Skills, res.PluginAgents, res.HooksFiles = out.Setup.Agents, out.Skills, out.PluginAgents, out.HooksFiles
	if err := port.Confirm(ctx, out.Summary); err != nil {
		return res, err
	}
	if res.Changes, err = port.WritePlugin(ctx, out.Setup.MarketplaceDir(), files); err != nil {
		return res, err
	}
	// An agent that refuses the plugin does not keep the hooks of the repository from being registered.
	var errs []error
	for _, a := range out.Setup.Agents {
		ch, err := port.RegisterPlugin(ctx, out.Setup, a)
		res.Changes, errs = append(res.Changes, ch...), append(errs, err)
		if out.Setup.Root != "" {
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
	if out.Setup.Root != "" {
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

// printChanges writes what changed to stdout: a command to run by hand as a warning, the rest one per line.
func (res Response) printChanges(stdout io.Writer) {
	u := cli.NewUI(stdout)
	for _, ch := range res.Changes {
		if ch.Todo() {
			u.Warn("run by hand: %s", cli.Code(ch.Path))
			continue
		}
		u.Change(ch.Removal(), ch.Action, ch.Path)
	}
}

// print writes, unless quiet or only the hooks of git were asked, what the install leaves and what to do next to
// stdout, and to stderr the settings that also register the hooks and, when git does not list the hooks of git
// written or lists them as disabled, that their gates do not apply.
func (res Response) print(stdout, stderr io.Writer) {
	if !res.Quiet && !res.GitHooksOnly {
		res.summary(cli.NewUI(stdout))
	}
	ue := cli.NewUI(stderr)
	for _, f := range res.Duplicates {
		ue.Warn("%s also registers the hooks of polix: remove them from there, or Claude Code runs them twice", f)
	}
	if len(res.Unlisted) > 0 {
		ue.Warn("git does not run hooks from its configuration (it needs git 2.54 or later): the hooks of git are "+
			"written, but the githooks of polix.yaml do not apply until git is updated (%s)",
			strings.Join(gitHookKeys(res.Unlisted), ", "))
	}
	if len(res.Disabled) > 0 {
		keys := make([]string, len(res.Disabled))
		for i, h := range res.Disabled {
			keys[i] = "hook." + h.Name() + ".enabled"
		}
		ue.Warn("hooks of git disabled in the git configuration (%s false): git does not run them, so the githooks "+
			"of polix.yaml do not apply to their events until they are enabled again", strings.Join(keys, ", "))
	}
}

// exit returns the exit code 1 of an install of only the hooks of git that git does not list, so a run of start
// reaches the agent; nil otherwise, also for the whole install with unlisted hooks and for hooks git lists as
// disabled, which a person turned off and installing again does not turn on.
func (res Response) exit() error {
	if res.GitHooksOnly && len(res.Unlisted) > 0 {
		return cli.Exit(1)
	}
	return nil
}

// gitHookKeys returns the key of each hook of git.
func gitHookKeys(hooks []setup.GitHook) []string {
	keys := make([]string, len(hooks))
	for i, h := range hooks {
		keys[i] = h.Key()
	}
	return keys
}

// summary writes what the install leaves and what to do next.
func (res Response) summary(u *cli.UI) {
	if len(res.Changes) == 0 {
		u.OK("polix %s is already installed", res.Version)
	} else {
		u.OK("polix %s installed", res.Version)
	}
	for _, a := range res.Agents {
		switch a {
		case setup.Claude:
			u.Line("  Claude Code: skills %s and agents %s; start a new session to load them.",
				cli.Code(strings.Join(prefixed("/polix:", res.Skills), " ")),
				cli.Code(strings.Join(prefixed("polix:", res.PluginAgents), " ")))
		case setup.Codex:
			u.Line("  Codex: the plugin %s with the skills %s; start a new session to load them.",
				cli.Code(setup.PluginID), cli.Code(strings.Join(res.Skills, " ")))
		}
	}
	switch {
	case len(res.HooksFiles) > 0:
		u.Line("  Hooks registered in %s: commit them with the polix.yaml so the team gets them.",
			strings.Join(res.HooksFiles, ", "))
		if slices.Contains(res.Agents, setup.Codex) {
			u.Line("  Codex runs new or changed hooks once trusted with %s.", cli.Code("/hooks"))
		}
	case !res.User:
		u.Warn("not inside a git repository: run polix install in a repository to register its hooks")
	}
	if slices.Contains(res.Agents, setup.Codex) {
		u.Line("  Codex support is not implemented in this release and the setup is untested.")
	}
	if res.NotOnPath {
		u.Warn("polix is not on the PATH: the hooks run `polix`, add its folder to the PATH")
	}
}

// prefixed returns names with prefix before each one.
func prefixed(prefix string, names []string) []string {
	out := make([]string, len(names))
	for i, n := range names {
		out[i] = prefix + n
	}
	return out
}
