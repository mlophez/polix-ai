package uninstall

import (
	"slices"

	"github.com/mlophez/polix-ai/internal/app/setup"
)

// Output is the decision of the logic of UninstallSetup, which the shell carries out through the
// port.
type Output struct {
	Setup        setup.Setup // where to remove from, for which agents
	Summary      string      // what the uninstall is about to do, for the person to confirm
	RemovePlugin bool        // remove the local marketplace: no agent is left to use it
	// GitHooks are the hooks of git to remove from the local configuration of the repository: those polix
	// registers, when both agents leave a repository; none without one.
	GitHooks []setup.GitHook
}

// Decide decides what polix uninstall removes for the setup asked: the plugin of the user below its home from its
// agents, or else from both, and the hooks of each of them in the repository of its root, when there is one; the
// local marketplace only when both agents leave it, and the hooks of git of the repository only when both leave
// it, since install writes them for any agent. The summary names those hooks of git.
func Decide(asked setup.Setup) Output {
	s := asked.Defaulted(setup.Claude, setup.Codex)
	all := s.For(setup.Claude) && s.For(setup.Codex)
	var gitHooks []setup.GitHook
	if all && s.Root != "" {
		gitHooks = slices.Clone(setup.GitHooks)
	}
	return Output{Setup: s, Summary: s.Summary("remove", gitHooks), RemovePlugin: all, GitHooks: gitHooks}
}
