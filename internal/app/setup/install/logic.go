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

// Decide decides what polix install writes for the polix of version running: the plugin of the user below
// home, for the agents asked or else Claude Code alone, since Codex is set up only when asked; and the hooks of each
// agent in the repository of root, when there is one. embedded is the plugin of the binary.
func Decide(home, root string, asked []string, running string, embedded map[string][]byte) Output {
	agents := asked
	if len(agents) == 0 {
		agents = []string{setup.Claude}
	}
	s := setup.Setup{Home: home, Root: root, Agents: agents, Version: running}
	if err := s.Valid(); err != nil {
		return Output{Failure: err.Error()}
	}
	p := setup.Plugin{Embedded: embedded, Version: running}
	if err := p.Valid(); err != nil {
		return Output{Failure: err.Error()}
	}
	var hooksFiles []string
	for _, a := range agents {
		if root != "" {
			hooksFiles = append(hooksFiles, s.HooksFile(a))
		}
	}
	return Output{Setup: s, Plugin: p, Summary: s.Summary("write"), Skills: p.Skills(), PluginAgents: p.Agents(),
		HooksFiles: hooksFiles}
}
