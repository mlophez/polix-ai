package install

import (
	"slices"

	"github.com/mlophez/polix-ai/internal/app/setup"
)

// Output is the decision of the logic of InstallSetup, which the shell carries out through the port.
type Output struct {
	Failure      string       // why nothing can be installed; empty when it can
	Setup        setup.Setup  // where to install, for which agents
	Parts        setup.Parts  // the parts to write: those asked, else every one
	Plugin       setup.Plugin // the plugin of the binary, to write to the local marketplace; only with Skills
	Summary      string       // what the install is about to do, for the person to confirm; none with only GitHooks
	Skills       []string     // the skills of the plugin
	PluginAgents []string     // the agents of the plugin
	HooksFiles   []string     // the hooks files of the repository, one per agent; none without Hooks or a repository
	// GitHooks are the hooks of git to write into the local configuration of the repository, whatever the agents
	// and polix.yaml; none without the part GitHooks or a repository.
	GitHooks []setup.GitHook
}

// Decide decides what polix install writes for the setup asked and its parts (none asked is every one): with Skills
// the plugin of the user below its home, embedded the plugin of the binary of the version of the setup; with Hooks
// the hooks of each agent in the repository of its root; with GitHooks the hooks of git of that repository
// (setup.GitHooks), even when polix.yaml has no githooks, so its gates apply once they are written. The agents are
// those asked or else Claude Code alone, since Codex is set up only when asked. Without parts asked, outside a
// repository it installs only the plugin; a part of the repository asked outside one is a failure. With only
// GitHooks there is no plugin nor summary to confirm.
func Decide(asked setup.Setup, embedded map[string][]byte, parts setup.Parts) Output {
	p := parts.Defaulted()
	if asked.Root == "" && !parts.None() && p.Repository() {
		return Output{Failure: "not inside a git repository: polix install --hooks and --githooks write into a repository"}
	}
	if p.OnlyGitHooks() {
		return Output{Setup: asked, Parts: p, GitHooks: slices.Clone(setup.GitHooks)}
	}
	s := asked.Defaulted(setup.Claude)
	out := Output{Setup: s, Parts: p, Summary: s.Summary("write", p)}
	if p.Skills {
		plugin := setup.Plugin{Embedded: embedded, Version: s.Version}
		if err := plugin.Valid(); err != nil {
			return Output{Failure: err.Error()}
		}
		out.Plugin, out.Skills, out.PluginAgents = plugin, plugin.Skills(), plugin.Agents()
	}
	if s.Root != "" && p.Hooks {
		for _, a := range s.Agents {
			out.HooksFiles = append(out.HooksFiles, s.HooksFile(a))
		}
	}
	if s.Root != "" && p.GitHooks {
		out.GitHooks = slices.Clone(setup.GitHooks)
	}
	return out
}
