package setup

// InstallSetupOutput is the decision of the logic of InstallSetup, which the shell carries out through the port.
type InstallSetupOutput struct {
	Failure      string            // why nothing can be installed; empty when it can
	Setup        Setup             // where to install, for which agents
	Files        map[string][]byte // the files of the local marketplace, relative to it
	Summary      string            // what the install is about to do, for the person to confirm
	Skills       []string          // the skills of the plugin
	PluginAgents []string          // the agents of the plugin
	HooksFiles   []string          // the hooks files of the repository, one per agent; none without a repository
}

// InstallSetup decides what polix install writes for the polix of version running: the plugin of the user below
// home, for the agents asked or else Claude Code and, when it is used here (codexUsed), Codex; and the hooks of each
// agent in the repository of root, when there is one. embedded is the plugin of the binary.
func InstallSetup(home, root string, asked []string, codexUsed bool, running string,
	embedded map[string][]byte) InstallSetupOutput {
	agents := asked
	if len(agents) == 0 {
		agents = []string{Claude}
		if codexUsed {
			agents = append(agents, Codex)
		}
	}
	s := Setup{Home: home, Root: root, Agents: agents, Version: running}
	if err := s.Valid(); err != nil {
		return InstallSetupOutput{Failure: err.Error()}
	}
	files, err := MarketplaceFiles(embedded, running)
	if err != nil {
		return InstallSetupOutput{Failure: err.Error()}
	}
	var hooksFiles []string
	for _, a := range agents {
		if root != "" {
			hooksFiles = append(hooksFiles, s.HooksFile(a))
		}
	}
	return InstallSetupOutput{Setup: s, Files: files, Summary: s.Summary("write"), Skills: PluginSkills(embedded),
		PluginAgents: PluginAgents(embedded), HooksFiles: hooksFiles}
}
