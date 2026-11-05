package setup

// RefreshSetupOutput is the decision of the logic of RefreshSetup, which the shell carries out through the port.
type RefreshSetupOutput struct {
	Failure string            // why nothing can be refreshed; empty when it can
	Plugin  bool              // rewrite the plugin: polix installed it
	Files   map[string][]byte // the files of the local marketplace, relative to it
	Hooks   []string          // the agents whose hooks of the repository polix registered, to rewrite
}

// RefreshSetup decides what the hidden polix __refresh brings in line with the binary of s.Version, installing
// nothing new: the plugin when its manifest is among files (path to content of the files that exist), and the
// hooks of each agent whose hooks file of the repository of s registers polix. embedded is the plugin of the
// binary.
func RefreshSetup(s Setup, files map[string][]byte, embedded map[string][]byte) RefreshSetupOutput {
	var out RefreshSetupOutput
	if _, ok := PluginVersion(files[s.PluginManifest()]); ok {
		marketplace, err := MarketplaceFiles(embedded, s.Version)
		if err != nil {
			return RefreshSetupOutput{Failure: err.Error()}
		}
		out.Plugin, out.Files = true, marketplace
	}
	for _, a := range []string{Claude, Codex} {
		if s.Root != "" && Registered(files[s.HooksFile(a)]) {
			out.Hooks = append(out.Hooks, a)
		}
	}
	return out
}
