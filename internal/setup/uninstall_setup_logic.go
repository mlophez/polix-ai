package setup

// UninstallSetupOutput is the decision of the logic of UninstallSetup, which the shell carries out through the
// port.
type UninstallSetupOutput struct {
	Failure      string // why nothing can be removed; empty when it can
	Setup        Setup  // where to remove from, for which agents
	Summary      string // what the uninstall is about to do, for the person to confirm
	RemovePlugin bool   // remove the local marketplace: no agent is left to use it
}

// UninstallSetup decides what polix uninstall removes for the polix of version running: the plugin of the user
// below home from the agents asked, or else from both, and the hooks of each of them in the repository of root,
// when there is one; the local marketplace only when both agents leave it.
func UninstallSetup(home, root string, asked []string, running string) UninstallSetupOutput {
	agents := asked
	if len(agents) == 0 {
		agents = []string{Claude, Codex}
	}
	s := Setup{Home: home, Root: root, Agents: agents, Version: running}
	if err := s.Valid(); err != nil {
		return UninstallSetupOutput{Failure: err.Error()}
	}
	return UninstallSetupOutput{Setup: s, Summary: s.Summary("remove"), RemovePlugin: s.For(Claude) && s.For(Codex)}
}
