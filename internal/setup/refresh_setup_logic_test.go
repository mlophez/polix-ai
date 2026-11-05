package setup

import (
	"slices"
	"testing"
)

func TestRefreshSetup(t *testing.T) {
	s := Setup{Home: "/h", Root: "/r", Agents: []string{Claude, Codex}, Version: "1.2.3"}
	claude, _, _ := RegisterHooks(nil, false, Claude)
	out := RefreshSetup(s, map[string][]byte{s.HooksFile(Claude): claude}, logicPlugin)
	if out.Plugin || !slices.Equal(out.Hooks, []string{Claude}) {
		t.Fatalf("no plugin installed, only the hooks registered: %+v", out)
	}
	out = RefreshSetup(s, map[string][]byte{s.PluginManifest(): []byte(`{"name": "polix", "version": "1"}`)}, logicPlugin)
	if !out.Plugin || out.Files == nil || out.Hooks != nil {
		t.Fatalf("the plugin installed: %+v", out)
	}
	noRepo := Setup{Home: "/h", Agents: []string{Claude}, Version: "1"}
	if out := RefreshSetup(noRepo, map[string][]byte{noRepo.HooksFile(Claude): claude}, logicPlugin); out.Hooks != nil {
		t.Fatal("no hooks without a repository")
	}
}
