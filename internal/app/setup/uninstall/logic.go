package uninstall

import "github.com/mlophez/polix-ai/internal/app/setup"

// Output is the decision of the logic of UninstallSetup, which the shell carries out through the
// port.
type Output struct {
	Setup        setup.Setup // where to remove from, for which agents
	Summary      string      // what the uninstall is about to do, for the person to confirm
	RemovePlugin bool        // remove the local marketplace: no agent is left to use it
}

// Decide decides what polix uninstall removes for the setup asked: the plugin of the user below its home from its
// agents, or else from both, and the hooks of each of them in the repository of its root, when there is one; the
// local marketplace only when both agents leave it.
func Decide(asked setup.Setup) Output {
	s := asked.Defaulted(setup.Claude, setup.Codex)
	return Output{Setup: s, Summary: s.Summary("remove"), RemovePlugin: s.For(setup.Claude) && s.For(setup.Codex)}
}
