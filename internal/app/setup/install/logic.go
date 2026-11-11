package install

import (
	"slices"

	"github.com/mlophez/polix-ai/internal/app/setup"
)

// Output is the decision of the logic of InstallSetup, which the shell carries out through the port.
type Output struct {
	Failure      string       // why nothing can be installed; empty when it can
	Setup        setup.Setup  // where to install, for which agents
	Plugin       setup.Plugin // the plugin of the binary, to write to the local marketplace
	Summary      string       // what the install is about to do, for the person to confirm
	Skills       []string     // the skills of the plugin
	PluginAgents []string     // the agents of the plugin
	HooksFiles   []string     // the hooks files of the repository, one per agent; none without a repository
	// GitHooks are the hooks of git to write into the local configuration of the repository, whatever the agents
	// and polix.yaml; none without a repository.
	GitHooks []setup.GitHook
	// GitHooksOnly is true when only the hooks of git are written (--githooks): no plugin, no hooks of the agents
	// and no confirmation.
	GitHooksOnly bool
}

// Decide decides what polix install writes for the setup asked: the plugin of the user below its home, for its
// agents or else Claude Code alone, since Codex is set up only when asked; and the hooks of each agent in the
// repository of its root, when there is one, with the hooks of git (setup.GitHooks), even when polix.yaml has no
// githooks, so its gates apply once they are written. embedded is the plugin of the binary, of the version of the
// setup. With gitHooksOnly (--githooks) it decides only the hooks of git of the repository, without plugin nor
// summary, and fails outside a repository.
func Decide(asked setup.Setup, embedded map[string][]byte, gitHooksOnly bool) Output {
	if gitHooksOnly {
		return decideGitHooksOnly(asked)
	}
	s := asked.Defaulted(setup.Claude)
	p := setup.Plugin{Embedded: embedded, Version: s.Version}
	if err := p.Valid(); err != nil {
		return Output{Failure: err.Error()}
	}
	var hooksFiles []string
	var gitHooks []setup.GitHook
	if s.Root != "" {
		for _, a := range s.Agents {
			hooksFiles = append(hooksFiles, s.HooksFile(a))
		}
		gitHooks = slices.Clone(setup.GitHooks)
	}
	return Output{Setup: s, Plugin: p, Summary: s.Summary("write", gitHooks), Skills: p.Skills(), PluginAgents: p.Agents(),
		HooksFiles: hooksFiles, GitHooks: gitHooks}
}

// decideGitHooksOnly decides polix install --githooks: the hooks of git (setup.GitHooks) of the repository of the
// root of the setup, or a failure without one.
func decideGitHooksOnly(s setup.Setup) Output {
	if s.Root == "" {
		return Output{Failure: "not inside a git repository: polix install --githooks writes the hooks of git of a repository"}
	}
	return Output{Setup: s, GitHooks: slices.Clone(setup.GitHooks), GitHooksOnly: true}
}
