package uninstall

import (
	"slices"

	"github.com/mlophez/polix-ai/internal/app/setup"
)

// Output is the decision of the logic of UninstallSetup, which the shell carries out through the
// port.
type Output struct {
	Failure      string      // why nothing can be removed; empty when it can
	Setup        setup.Setup // where to remove from, for which agents
	Parts        setup.Parts // the parts to remove: those asked, else every one
	Summary      string      // what the uninstall is about to do, for the person to confirm
	RemovePlugin bool        // remove the local marketplace: the plugin is removed and no agent is left to use it
	// GitHooks are the hooks of git to remove from the local configuration of the repository: those polix
	// registers; none without a repository.
	GitHooks []setup.GitHook
}

// Decide decides what polix uninstall removes for the setup asked and its parts (none asked is every one): with
// Skills the plugin of the user below its home from its agents, or else from both, and the local marketplace only
// when both leave it; with Hooks the hooks of each of them in the repository of its root; with GitHooks the hooks
// of git of that repository. Without parts asked, the hooks of git leave only when both agents do, since install
// writes them for any agent, and outside a repository only the plugin is removed; a part of the repository asked
// outside one is a failure. The summary names the hooks of git.
func Decide(asked setup.Setup, parts setup.Parts) Output {
	p := parts.Defaulted()
	if asked.Root == "" && !parts.None() && p.Repository() {
		return Output{Failure: "not inside a git repository: polix uninstall --hooks and --githooks remove from a repository"}
	}
	s := asked.Defaulted(setup.Claude, setup.Codex)
	all := s.For(setup.Claude) && s.For(setup.Codex)
	if s.Root == "" || !all && parts.None() {
		p = p.WithoutGitHooks()
	}
	var gitHooks []setup.GitHook
	if p.GitHooks {
		gitHooks = slices.Clone(setup.GitHooks)
	}
	return Output{Setup: s, Parts: p, Summary: s.Summary("remove", p), RemovePlugin: p.Skills && all, GitHooks: gitHooks}
}
