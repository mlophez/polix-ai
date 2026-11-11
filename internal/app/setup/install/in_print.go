package install

import (
	"io"
	"slices"
	"strings"

	"github.com/mlophez/polix-ai/internal/app/setup"
	"github.com/mlophez/polix-ai/internal/platform/cli"
)

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

// print writes, unless quiet or only the hooks of git were written, what the install leaves and what to do next to
// stdout, and to stderr the settings that also register the hooks and, when git does not list the hooks of git
// written or lists them as disabled, that their gates do not apply.
func (res Response) print(stdout, stderr io.Writer) {
	if !res.Quiet && !res.Parts.OnlyGitHooks() {
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
	if res.Parts.OnlyGitHooks() && len(res.Unlisted) > 0 {
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
		if !res.Parts.Skills {
			break
		}
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
	case res.Parts.Hooks:
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
