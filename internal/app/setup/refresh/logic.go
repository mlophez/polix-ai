package refresh

import "github.com/mlophez/polix-ai/internal/app/setup"

// Output is the decision of the logic of RefreshSetup, which the shell carries out through the port.
type Output struct {
	Failure string            // why nothing can be refreshed; empty when it can
	Plugin  bool              // rewrite the plugin: polix installed it
	Files   map[string][]byte // the files of the local marketplace, relative to it
	Hooks   []string          // the agents whose hooks of the repository polix registered, to rewrite
}

// Decide decides what the hidden polix __refresh brings in line with the binary of s.Version, installing
// nothing new: the plugin when its manifest is among files (path to content of the files that exist), and the
// hooks of each agent whose hooks file of the repository of s registers polix. embedded is the plugin of the
// binary.
func Decide(s setup.Setup, files map[string][]byte, embedded map[string][]byte) Output {
	var out Output
	if _, ok := setup.PluginVersion(files[s.PluginManifest()]); ok {
		marketplace, err := setup.MarketplaceFiles(embedded, s.Version)
		if err != nil {
			return Output{Failure: err.Error()}
		}
		out.Plugin, out.Files = true, marketplace
	}
	for _, a := range []string{setup.Claude, setup.Codex} {
		if s.Root != "" && setup.Registered(files[s.HooksFile(a)]) {
			out.Hooks = append(out.Hooks, a)
		}
	}
	return out
}
