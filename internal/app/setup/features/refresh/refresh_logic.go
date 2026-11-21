package refresh

import "github.com/mlophez/polix-ai/internal/app/setup"

// Output is the decision of the logic of RefreshSetup, which the shell carries out through the port.
type Output struct {
	Plugin      bool         // rewrite the plugin: polix installed it
	Marketplace setup.Plugin // the plugin of the binary, to write to the local marketplace when Plugin
	Hooks       []string     // the agents whose hooks of the repository polix registered, to rewrite
}

// Decide decides what the hidden polix __refresh brings in line with the binary of s.Version, installing
// nothing new: the plugin p of the binary when polix wrote one in the marketplace (p.Written), and the hooks of
// each agent whose hooks file of the repository of s registers polix (registered, by path).
func Decide(s setup.Setup, p setup.Plugin, registered map[string]bool) Output {
	var out Output
	if p.Written {
		out.Plugin, out.Marketplace = true, p
	}
	for _, a := range []string{setup.Claude, setup.Codex} {
		if s.Root != "" && registered[s.HooksFile(a)] {
			out.Hooks = append(out.Hooks, a)
		}
	}
	return out
}
