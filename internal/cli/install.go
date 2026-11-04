package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"charm.land/huh/v2"
	"github.com/charmbracelet/x/term"
	"github.com/spf13/cobra"

	"github.com/mlophez/polix-ai/internal/gitx"
	"github.com/mlophez/polix-ai/internal/setup"
)

// installFlags are the flags install and uninstall share.
type installFlags struct {
	user   bool
	dir    string
	agents []string
	yes    bool
	quiet  bool
}

func (f *installFlags) register(c *cobra.Command, verb string) {
	c.Flags().BoolVarP(&f.user, "user", "u", false, "only the plugin of the user, not the hooks of the repository")
	c.Flags().StringVarP(&f.dir, "dir", "C", ".", "repository whose hooks to "+verb)
	c.Flags().StringSliceVarP(&f.agents, "agent", "a", nil, "agents: claude, codex (default: claude, and codex when it is installed)")
	c.Flags().BoolVarP(&f.yes, "yes", "y", false, "do not ask for confirmation")
	c.Flags().BoolVarP(&f.quiet, "quiet", "q", false, "only print what changed")
	_ = c.RegisterFlagCompletionFunc("agent", cobra.FixedCompletions([]string{setup.Claude, setup.Codex}, cobra.ShellCompDirectiveNoFileComp))
	_ = c.MarkFlagDirname("dir")
}

// options resolves where to install: the plugin in the home of the user and, unless --user, the hooks in the
// repository of --dir when there is one.
func (f *installFlags) options() (setup.Options, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return setup.Options{}, err
	}
	o := setup.Options{Home: home, Version: Version}
	if !f.user {
		if repo, err := gitx.Open(f.dir); err == nil {
			o.Project = repo.Root
		}
	}
	return o, nil
}

// resolveAgents returns the agents to install for: the ones asked, else Claude Code and, when it is on this
// machine or in the repository, Codex.
func resolveAgents(asked []string, o setup.Options) ([]string, error) {
	if len(asked) > 0 {
		var out []string
		for _, a := range asked {
			a = strings.ToLower(strings.TrimSpace(a))
			if a != setup.Claude && a != setup.Codex {
				return nil, fmt.Errorf("unknown agent %q (claude, codex)", a)
			}
			if !slices.Contains(out, a) {
				out = append(out, a)
			}
		}
		return out, nil
	}
	agents := []string{setup.Claude}
	if codexPresent(o) {
		agents = append(agents, setup.Codex)
	}
	return agents, nil
}

// codexPresent reports whether Codex is used here: on the PATH, configured for the user, or for the project.
func codexPresent(o setup.Options) bool {
	if _, err := lookPath("codex"); err == nil {
		return true
	}
	return isDir(filepath.Join(o.Home, ".codex")) || o.Project != "" && isDir(filepath.Join(o.Project, ".codex"))
}

func isDir(p string) bool {
	info, err := os.Stat(p)
	return err == nil && info.IsDir()
}

func installCmd() *cobra.Command {
	var f installFlags
	c := &cobra.Command{
		Use:   "install",
		Short: "Install the plugin of polix for the user and register its hooks in the repository",
		Long: "Install what the agents need to run polix, from this binary.\n\n" +
			"For the user, the plugin polix, with skills (/polix:workflow, /polix:add-rule, /polix:doctor, /polix:init,\n" +
			"/polix:plan, /polix:review, /polix:audit) and agents (polix:workflow-author, polix:rules-author, polix:reviewer,\n" +
			"polix:rules-reviewer, polix:auditor) that help in any repository. It is written to the local marketplace\n" +
			"~/.polix/marketplace/ and registered with each agent:\n" +
			"  Claude Code  claude plugin marketplace add, claude plugin install polix@polix --scope user\n" +
			"  Codex        codex plugin marketplace add, and enabled in ~/.codex/config.toml\n\n" +
			"For the repository of --dir, unless --user, the hooks that call polix hook, to commit with its polix.yaml:\n" +
			"  Claude Code  .claude/settings.json\n" +
			"  Codex        .codex/hooks.json\n\n" +
			"Every other setting and hook of those files is kept. Running it again updates what changed; polix update " +
			"refreshes the plugin after every update.",
		Example: "  # The plugin for this machine and the hooks for this repository\n  polix install\n\n" +
			"  # Only the plugin, from anywhere\n  polix install --user\n\n" +
			"  # Only Claude Code, for another repository\n  polix install --agent claude --dir ~/Code/service",
		Args: cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			o, err := f.options()
			if err != nil {
				return err
			}
			agents, err := resolveAgents(f.agents, o)
			if err != nil {
				return err
			}
			if !f.yes && interactive(c) {
				ok, err := confirm(installSummary(o, agents, "write"), "Install")
				if err != nil {
					return err
				}
				if !ok {
					return exit(1)
				}
			}
			changes, err := setup.Install(o, agents)
			u := newUI(c.OutOrStdout())
			printChanges(u, changes)
			if err != nil {
				return err
			}
			if !f.quiet {
				installNext(u, o, agents, f.user, len(changes) > 0)
			}
			if o.Project != "" {
				warnDuplicateHooks(newUI(c.ErrOrStderr()), o, agents)
			}
			return nil
		},
	}
	f.register(c, "register")
	return c
}

func uninstallCmd() *cobra.Command {
	var f installFlags
	c := &cobra.Command{
		Use:   "uninstall",
		Short: "Remove the plugin of polix and the hooks of polix of the repository",
		Long: "Unregister the plugin polix from Claude Code and Codex and remove ~/.polix/marketplace/ and, unless --user, " +
			"the hooks of polix from the .claude/settings.json and .codex/hooks.json of the repository of --dir, " +
			"keeping every other setting and hook. The binary stays: delete it by hand.",
		Args: cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			o, err := f.options()
			if err != nil {
				return err
			}
			agents := f.agents
			if len(agents) == 0 {
				agents = []string{setup.Claude, setup.Codex}
			}
			if agents, err = resolveAgents(agents, o); err != nil {
				return err
			}
			if !f.yes && interactive(c) {
				ok, err := confirm(installSummary(o, agents, "remove"), "Uninstall")
				if err != nil {
					return err
				}
				if !ok {
					return exit(1)
				}
			}
			changes, err := setup.Uninstall(o, agents)
			u := newUI(c.OutOrStdout())
			printChanges(u, changes)
			if err == nil && len(changes) == 0 && !f.quiet {
				u.ok("nothing to remove")
			}
			return err
		},
	}
	f.register(c, "remove")
	return c
}

func installSummary(o setup.Options, agents []string, verb string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "polix %s will %s:\n", Version, verb)
	for _, a := range agents {
		switch a {
		case setup.Claude:
			fmt.Fprintf(&b, "\n  Claude Code: the plugin %s, from %s\n", setup.PluginID, o.MarketplaceDir())
		case setup.Codex:
			fmt.Fprintf(&b, "\n  Codex: the plugin %s, enabled in %s\n", setup.PluginID, o.CodexConfigFile())
		}
		if o.Project != "" {
			fmt.Fprintf(&b, "         the hooks of %s\n", o.HooksFile(a))
		}
	}
	return b.String()
}

// interactive reports whether a person is at the terminal to answer a question.
func interactive(c *cobra.Command) bool {
	in, ok1 := c.InOrStdin().(*os.File)
	out, ok2 := c.OutOrStdout().(*os.File)
	return ok1 && ok2 && term.IsTerminal(in.Fd()) && term.IsTerminal(out.Fd())
}

func confirm(description, action string) (bool, error) {
	ok := true
	err := huh.NewConfirm().
		Title(action + " polix?").
		Description(description).
		Affirmative(action).
		Negative("Cancel").
		Value(&ok).
		Run()
	if errors.Is(err, huh.ErrUserAborted) {
		return false, nil
	}
	return ok, err
}

func printChanges(u *ui, changes []setup.Change) {
	for _, ch := range changes {
		switch ch.Action {
		case "todo":
			u.warn("run by hand: %s", code(ch.Path))
		case "removed", "unregistered":
			u.line("%s %s %s", styleWarn.Render("-"), styleMuted.Render(fmt.Sprintf("%-12s", ch.Action)), ch.Path)
		default:
			u.line("%s %s %s", styleOK.Render("+"), styleMuted.Render(fmt.Sprintf("%-12s", ch.Action)), ch.Path)
		}
	}
}

// installNext tells what the install leaves and what to do next.
func installNext(u *ui, o setup.Options, agents []string, userOnly, changed bool) {
	if !changed {
		u.ok("polix %s is already installed", Version)
	} else {
		u.ok("polix %s installed", Version)
	}
	for _, a := range agents {
		switch a {
		case setup.Claude:
			u.line("  Claude Code: skills %s and agents %s; start a new session to load them.",
				code(strings.Join(prefixed("/polix:", setup.Skills()), " ")), code(strings.Join(prefixed("polix:", setup.Agents()), " ")))
		case setup.Codex:
			u.line("  Codex: the plugin %s with the skills %s; start a new session to load them.",
				code(setup.PluginID), code(strings.Join(setup.Skills(), " ")))
		}
	}
	switch {
	case o.Project != "":
		var files []string
		for _, a := range agents {
			files = append(files, o.HooksFile(a))
		}
		u.line("  Hooks registered in %s: commit them with the polix.yaml so the team gets them.", strings.Join(files, ", "))
		if slices.Contains(agents, setup.Codex) {
			u.line("  Codex runs new or changed hooks once trusted with %s.", code("/hooks"))
		}
	case !userOnly:
		u.warn("not inside a git repository: run polix install in a repository to register its hooks")
	}
	if _, err := lookPath("polix"); err != nil {
		u.warn("polix is not on the PATH: the hooks run `polix`, add its folder to the PATH")
	}
}

// warnDuplicateHooks warns when another settings file of Claude Code also registers the hooks of polix: Claude
// Code merges the settings and would run each hook twice.
func warnDuplicateHooks(u *ui, o setup.Options, agents []string) {
	if !slices.Contains(agents, setup.Claude) {
		return
	}
	for _, f := range claudeSettingsFiles(o.Project, o.Home) {
		if f != o.HooksFile(setup.Claude) && setup.Registered(f) {
			u.warn("%s also registers the hooks of polix: remove them from there, or Claude Code runs them twice", f)
		}
	}
}

func prefixed(prefix string, names []string) []string {
	out := make([]string, len(names))
	for i, n := range names {
		out[i] = prefix + n
	}
	return out
}

// refreshInstalled brings the plugin of the user in line with this binary when it is installed and, with hooks,
// the hooks of the repository of dir where polix registered them. It installs nothing new.
func refreshInstalled(dir string, stdout io.Writer, quiet, hooks bool) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	o := setup.Options{Home: home, Version: Version}
	if repo, err := gitx.Open(dir); err == nil && hooks {
		o.Project = repo.Root
	}
	changes, err := setup.Refresh(o)
	errs := []error{err}
	for _, a := range []string{setup.Claude, setup.Codex} {
		if o.Project != "" && setup.Registered(o.HooksFile(a)) {
			ch, err := setup.RegisterHooks(o, a)
			changes, errs = append(changes, ch...), append(errs, err)
		}
	}
	if len(changes) > 0 {
		if quiet {
			fmt.Fprintln(stdout, "polix ha actualizado su plugin: se carga en la próxima sesión.")
		} else {
			printChanges(newUI(stdout), changes)
		}
	}
	return errors.Join(errs...)
}
