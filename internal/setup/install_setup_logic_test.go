package setup

import (
	"slices"
	"testing"
)

var logicPlugin = map[string][]byte{
	"polix/.claude-plugin/plugin.json": []byte(`{"name": "polix", "version": "0", "description": "d"}`),
	"polix/skills/plan/SKILL.md":       []byte("x"),
	"polix/agents/reviewer.md":         []byte("x"),
}

func TestInstallSetupAgents(t *testing.T) {
	out := InstallSetup("/h", "", nil, false, "1.2.3", logicPlugin)
	if out.Failure != "" || !slices.Equal(out.Setup.Agents, []string{Claude}) || out.HooksFiles != nil {
		t.Fatalf("Claude Code by default: %+v", out)
	}
	out = InstallSetup("/h", "/r", nil, true, "1.2.3", logicPlugin)
	if !slices.Equal(out.Setup.Agents, []string{Claude, Codex}) || len(out.HooksFiles) != 2 ||
		!slices.Equal(out.Skills, []string{"plan"}) || !slices.Equal(out.PluginAgents, []string{"reviewer"}) ||
		out.Files["polix/.claude-plugin/plugin.json"] == nil || out.Summary == "" {
		t.Fatalf("and Codex when used: %+v", out)
	}
	if out := InstallSetup("/h", "", []string{Codex}, false, "1.2.3", logicPlugin); !slices.Equal(out.Setup.Agents, []string{Codex}) {
		t.Fatalf("the agents asked: %+v", out.Setup)
	}
	broken := map[string][]byte{"polix/.claude-plugin/plugin.json": []byte("{}")}
	if out := InstallSetup("/h", "", nil, false, "1.2.3", broken); out.Failure == "" {
		t.Fatal("a plugin without version cannot be installed")
	}
}
