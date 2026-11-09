package uninstall

import "github.com/mlophez/polix-ai/internal/app/setup"

// Output is the decision of the logic of UninstallSetup, which the shell carries out through the
// port.
type Output struct {
	Failure      string      // why nothing can be removed; empty when it can
	Setup        setup.Setup // where to remove from, for which agents
	Summary      string      // what the uninstall is about to do, for the person to confirm
	RemovePlugin bool        // remove the local marketplace: no agent is left to use it
}

// Decide decides what polix uninstall removes for the polix of version running: the plugin of the user
// below home from the agents asked, or else from both, and the hooks of each of them in the repository of root,
// when there is one; the local marketplace only when both agents leave it.
func Decide(home, root string, asked []string, running string) Output {
	agents := asked
	if len(agents) == 0 {
		agents = []string{setup.Claude, setup.Codex}
	}
	s := setup.Setup{Home: home, Root: root, Agents: agents, Version: running}
	if err := s.Valid(); err != nil {
		return Output{Failure: err.Error()}
	}
	return Output{Setup: s, Summary: s.Summary("remove"), RemovePlugin: s.For(setup.Claude) && s.For(setup.Codex)}
}
