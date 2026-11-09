package refresh

import (
	"slices"
	"testing"

	"github.com/mlophez/polix-ai/internal/app/setup"
)

var logicPlugin = map[string][]byte{
	"polix/.claude-plugin/plugin.json": []byte(`{"name": "polix", "version": "0", "description": "d"}`),
	"polix/skills/plan/SKILL.md":       []byte("x"),
	"polix/agents/reviewer.md":         []byte("x"),
}

func TestDecide(t *testing.T) {
	s := setup.Setup{Home: "/h", Root: "/r", Agents: []string{setup.Claude, setup.Codex}, Version: "1.2.3"}
	claude, _, _ := setup.RegisterHooks(nil, false, setup.Claude)
	out := Decide(s, map[string][]byte{s.HooksFile(setup.Claude): claude}, logicPlugin)
	if out.Plugin || !slices.Equal(out.Hooks, []string{setup.Claude}) {
		t.Fatalf("no plugin installed, only the hooks registered: %+v", out)
	}
	out = Decide(s, map[string][]byte{s.PluginManifest(): []byte(`{"name": "polix", "version": "1"}`)}, logicPlugin)
	if !out.Plugin || out.Files == nil || out.Hooks != nil {
		t.Fatalf("the plugin installed: %+v", out)
	}
	noRepo := setup.Setup{Home: "/h", Agents: []string{setup.Claude}, Version: "1"}
	if out := Decide(noRepo, map[string][]byte{noRepo.HooksFile(setup.Claude): claude}, logicPlugin); out.Hooks != nil {
		t.Fatal("no hooks without a repository")
	}
}
