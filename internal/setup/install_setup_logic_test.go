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
	out := InstallSetup("/h", "", nil, "1.2.3", logicPlugin)
	if out.Failure != "" || !slices.Equal(out.Setup.Agents, []string{Claude}) || out.HooksFiles != nil {
		t.Fatalf("Claude Code by default: %+v", out)
	}
	out = InstallSetup("/h", "/r", nil, "1.2.3", logicPlugin)
	if !slices.Equal(out.Setup.Agents, []string{Claude}) || !slices.Equal(out.HooksFiles, []string{out.Setup.HooksFile(Claude)}) ||
		!slices.Equal(out.Skills, []string{"plan"}) || !slices.Equal(out.PluginAgents, []string{"reviewer"}) ||
		out.Files["polix/.claude-plugin/plugin.json"] == nil || out.Summary == "" {
		t.Fatalf("Claude Code alone in a repository, never Codex by itself: %+v", out)
	}
	out = InstallSetup("/h", "/r", []string{Codex}, "1.2.3", logicPlugin)
	if !slices.Equal(out.Setup.Agents, []string{Codex}) || !slices.Equal(out.HooksFiles, []string{out.Setup.HooksFile(Codex)}) {
		t.Fatalf("Codex when asked: %+v", out)
	}
	if out := InstallSetup("/h", "", []string{Claude, Codex}, "1.2.3", logicPlugin); !slices.Equal(out.Setup.Agents, []string{Claude, Codex}) {
		t.Fatalf("the agents asked: %+v", out.Setup)
	}
	broken := map[string][]byte{"polix/.claude-plugin/plugin.json": []byte("{}")}
	if out := InstallSetup("/h", "", nil, "1.2.3", broken); out.Failure == "" {
		t.Fatal("a plugin without version cannot be installed")
	}
}
