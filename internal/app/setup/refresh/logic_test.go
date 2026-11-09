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
	none := setup.Plugin{Embedded: logicPlugin, Version: "1.2.3"}
	out := Decide(s, none, map[string]bool{s.HooksFile(setup.Claude): true, s.HooksFile(setup.Codex): false})
	if out.Plugin || !slices.Equal(out.Hooks, []string{setup.Claude}) {
		t.Fatalf("no plugin installed, only the hooks registered: %+v", out)
	}
	written := setup.Plugin{Written: true, Manifest: "1", Embedded: logicPlugin, Version: "1.2.3"}
	out = Decide(s, written, nil)
	if !out.Plugin || out.Marketplace.Embedded == nil || out.Hooks != nil {
		t.Fatalf("the plugin installed: %+v", out)
	}
	noRepo := setup.Setup{Home: "/h", Agents: []string{setup.Claude}, Version: "1"}
	if out := Decide(noRepo, none, map[string]bool{noRepo.HooksFile(setup.Claude): true}); out.Hooks != nil {
		t.Fatal("no hooks without a repository")
	}
}
