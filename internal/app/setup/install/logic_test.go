package install

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

func TestDecideAgents(t *testing.T) {
	out := Decide(setup.Setup{Home: "/h", Version: "1.2.3"}, logicPlugin)
	if out.Failure != "" || !slices.Equal(out.Setup.Agents, []string{setup.Claude}) || out.HooksFiles != nil {
		t.Fatalf("Claude Code by default: %+v", out)
	}
	out = Decide(setup.Setup{Home: "/h", Root: "/r", Version: "1.2.3"}, logicPlugin)
	if !slices.Equal(out.Setup.Agents, []string{setup.Claude}) || !slices.Equal(out.HooksFiles, []string{out.Setup.HooksFile(setup.Claude)}) ||
		!slices.Equal(out.Skills, []string{"plan"}) || !slices.Equal(out.PluginAgents, []string{"reviewer"}) ||
		out.Plugin.Embedded["polix/.claude-plugin/plugin.json"] == nil || out.Plugin.Version != "1.2.3" || out.Summary == "" {
		t.Fatalf("Claude Code alone in a repository, never Codex by itself: %+v", out)
	}
	out = Decide(setup.Setup{Home: "/h", Root: "/r", Agents: []string{setup.Codex}, Version: "1.2.3"}, logicPlugin)
	if !slices.Equal(out.Setup.Agents, []string{setup.Codex}) || !slices.Equal(out.HooksFiles, []string{out.Setup.HooksFile(setup.Codex)}) {
		t.Fatalf("Codex when asked: %+v", out)
	}
	both := setup.Setup{Home: "/h", Agents: []string{setup.Claude, setup.Codex}, Version: "1.2.3"}
	if out := Decide(both, logicPlugin); !slices.Equal(out.Setup.Agents, []string{setup.Claude, setup.Codex}) {
		t.Fatalf("the agents asked: %+v", out.Setup)
	}
	if out := Decide(setup.Setup{Home: "/h"}, logicPlugin); out.Failure == "" {
		t.Fatal("a binary without version installs nothing")
	}
}
