package install

import (
	"slices"
	"strings"
	"testing"

	"github.com/mlophez/polix-ai/internal/app/setup"
)

var logicPlugin = map[string][]byte{
	"polix/.claude-plugin/plugin.json": []byte(`{"name": "polix", "version": "0", "description": "d"}`),
	"polix/skills/plan/SKILL.md":       []byte("x"),
	"polix/agents/reviewer.md":         []byte("x"),
}

func TestDecideAgents(t *testing.T) {
	out := Decide(setup.Setup{Home: "/h", Version: "1.2.3"}, logicPlugin, false)
	if out.Failure != "" || !slices.Equal(out.Setup.Agents, []string{setup.Claude}) || out.HooksFiles != nil {
		t.Fatalf("Claude Code by default: %+v", out)
	}
	out = Decide(setup.Setup{Home: "/h", Root: "/r", Version: "1.2.3"}, logicPlugin, false)
	if !slices.Equal(out.Setup.Agents, []string{setup.Claude}) || !slices.Equal(out.HooksFiles, []string{out.Setup.HooksFile(setup.Claude)}) ||
		!slices.Equal(out.Skills, []string{"plan"}) || !slices.Equal(out.PluginAgents, []string{"reviewer"}) ||
		out.Plugin.Embedded["polix/.claude-plugin/plugin.json"] == nil || out.Plugin.Version != "1.2.3" || out.Summary == "" {
		t.Fatalf("Claude Code alone in a repository, never Codex by itself: %+v", out)
	}
	out = Decide(setup.Setup{Home: "/h", Root: "/r", Agents: []string{setup.Codex}, Version: "1.2.3"}, logicPlugin, false)
	if !slices.Equal(out.Setup.Agents, []string{setup.Codex}) || !slices.Equal(out.HooksFiles, []string{out.Setup.HooksFile(setup.Codex)}) {
		t.Fatalf("Codex when asked: %+v", out)
	}
	both := setup.Setup{Home: "/h", Agents: []string{setup.Claude, setup.Codex}, Version: "1.2.3"}
	if out := Decide(both, logicPlugin, false); !slices.Equal(out.Setup.Agents, []string{setup.Claude, setup.Codex}) {
		t.Fatalf("the agents asked: %+v", out.Setup)
	}
	if out := Decide(setup.Setup{Home: "/h"}, logicPlugin, false); out.Failure == "" {
		t.Fatal("a binary without version installs nothing")
	}
}

// AC11, AC13: the hooks of git go into every repository, for any agent, and never without one.
func TestDecideGitHooks(t *testing.T) {
	for _, agents := range [][]string{nil, {setup.Codex}} {
		out := Decide(setup.Setup{Home: "/h", Root: "/r", Agents: agents, Version: "1.2.3"}, logicPlugin, false)
		if !slices.Equal(out.GitHooks, setup.GitHooks) {
			t.Fatalf("%v: %+v", agents, out.GitHooks)
		}
	}
	if out := Decide(setup.Setup{Home: "/h", Version: "1.2.3"}, logicPlugin, false); out.GitHooks != nil {
		t.Fatalf("without a repository: %+v", out.GitHooks)
	}
}

// AC21: the summary to confirm names the hooks of git inside a repository, and not outside one.
func TestDecideSummaryGitHooks(t *testing.T) {
	out := Decide(setup.Setup{Home: "/h", Root: "/r", Version: "1.2.3"}, logicPlugin, false)
	if !strings.Contains(out.Summary, "hook.polix-pre-commit") || !strings.Contains(out.Summary, "hook.polix-commit-msg") {
		t.Fatalf("inside a repository:\n%s", out.Summary)
	}
	out = Decide(setup.Setup{Home: "/h", Version: "1.2.3"}, logicPlugin, false)
	if strings.Contains(out.Summary, "hook.polix-") {
		t.Fatalf("outside a repository:\n%s", out.Summary)
	}
}

// AC18, AC20: only the hooks of git inside a repository: the two of setup.GitHooks, without plugin, hooks files
// nor summary to confirm, and without the plugin of the binary; outside a repository a failure.
func TestDecideGitHooksOnly(t *testing.T) {
	out := Decide(setup.Setup{Root: "/r", Version: "1.2.3"}, nil, true)
	if out.Failure != "" || !out.GitHooksOnly || !slices.Equal(out.GitHooks, setup.GitHooks) || out.Setup.Root != "/r" ||
		out.Plugin.Embedded != nil || out.HooksFiles != nil || out.Summary != "" || out.Skills != nil || out.PluginAgents != nil {
		t.Fatalf("inside a repository: %+v", out)
	}
	out = Decide(setup.Setup{Version: "1.2.3"}, nil, true)
	if !strings.Contains(out.Failure, "not inside a git repository") || out.GitHooks != nil {
		t.Fatalf("outside a repository: %+v", out)
	}
}
