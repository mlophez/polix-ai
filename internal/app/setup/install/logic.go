package install

import "github.com/mlophez/polix-ai/internal/app/setup"

// Output is the decision of the logic of InstallSetup, which the shell carries out through the port.
type Output struct {
	Failure      string       // why nothing can be installed; empty when it can
	Setup        setup.Setup  // where to install, for which agents
	Plugin       setup.Plugin // the plugin of the binary, to write to the local marketplace
	Summary      string       // what the install is about to do, for the person to confirm
	Skills       []string     // the skills of the plugin
	PluginAgents []string     // the agents of the plugin
	HooksFiles   []string     // the hooks files of the repository, one per agent; none without a repository
}

// Decide decides what polix install writes for the setup asked: the plugin of the user below its home, for its
// agents or else Claude Code alone, since Codex is set up only when asked; and the hooks of each agent in the
// repository of its root, when there is one. embedded is the plugin of the binary, of the version of the setup.
func Decide(asked setup.Setup, embedded map[string][]byte) Output {
	s := asked.Defaulted(setup.Claude)
	p := setup.Plugin{Embedded: embedded, Version: s.Version}
	if err := p.Valid(); err != nil {
		return Output{Failure: err.Error()}
	}
	var hooksFiles []string
	for _, a := range s.Agents {
		if s.Root != "" {
			hooksFiles = append(hooksFiles, s.HooksFile(a))
		}
	}
	return Output{Setup: s, Plugin: p, Summary: s.Summary("write"), Skills: p.Skills(), PluginAgents: p.Agents(),
		HooksFiles: hooksFiles}
}
