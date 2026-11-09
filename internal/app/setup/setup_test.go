package setup

import (
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestSetupValid(t *testing.T) {
	s := Setup{Home: "/h", Agents: []string{Claude, Codex}, Version: "1.2.3"}
	if err := s.Valid(); err != nil {
		t.Fatal(err)
	}
	bad := Setup{Agents: []string{"cursor"}, Version: "1"}
	if err := bad.Valid(); err == nil || !strings.Contains(err.Error(), `unknown agent "cursor" (claude, codex)`) {
		t.Fatalf("unknown agent: %v", err)
	}
	noVersion := Setup{}
	if err := noVersion.Valid(); err == nil {
		t.Fatal("a setup needs the version")
	}
}

func TestDefaulted(t *testing.T) {
	none := Setup{Version: "1"}
	if got := none.Defaulted(Claude); !slices.Equal(got.Agents, []string{Claude}) || none.Agents != nil {
		t.Fatalf("no agent asked takes the default, in a copy: %+v %+v", got, none)
	}
	asked := Setup{Agents: []string{Codex}, Version: "1"}
	if got := asked.Defaulted(Claude); !slices.Equal(got.Agents, []string{Codex}) {
		t.Fatalf("the agents asked stay: %+v", got)
	}
}

func TestSetupPaths(t *testing.T) {
	s := Setup{Home: "/h", Root: "/r", Agents: []string{Claude}, Version: "1"}
	for got, want := range map[string]string{
		s.HooksFile(Claude): "/r/.claude/settings.json",
		s.HooksFile(Codex):  "/r/.codex/hooks.json",
		s.PluginDir():       "/h/.polix/marketplace/polix",
		s.PluginManifest():  "/h/.polix/marketplace/polix/.claude-plugin/plugin.json",
		s.CodexConfigFile(): "/h/.codex/config.toml",
	} {
		if got != filepath.FromSlash(want) {
			t.Errorf("got %s, want %s", got, want)
		}
	}
	if got := s.ClaudeSettingsFiles(); len(got) != 3 {
		t.Fatalf("project, local and user: %v", got)
	}
	userOnly := Setup{Home: "/h", Version: "1"}
	if got := userOnly.ClaudeSettingsFiles(); len(got) != 1 {
		t.Fatalf("only the user outside a repository: %v", got)
	}
}

// The other settings of Claude Code that register polix too are duplicates; the one of the repository is not.
func TestDuplicates(t *testing.T) {
	s := Setup{Home: "/h", Root: "/r", Agents: []string{Claude}, Version: "1"}
	user := filepath.FromSlash("/h/.claude/settings.json")
	registered := map[string]bool{s.HooksFile(Claude): true, user: true}
	if got := s.Duplicates(registered); !slices.Equal(got, []string{user}) {
		t.Fatalf("duplicates: %v", got)
	}
	codexOnly := Setup{Home: "/h", Root: "/r", Agents: []string{Codex}, Version: "1"}
	if got := codexOnly.Duplicates(registered); got != nil {
		t.Fatalf("only for Claude Code: %v", got)
	}
}

func TestSummary(t *testing.T) {
	s := Setup{Home: "/h", Root: "/r", Agents: []string{Claude, Codex}, Version: "1.2.3"}
	got := s.Summary("write")
	for _, want := range []string{"polix 1.2.3 will write:", "Claude Code: the plugin polix@polix", "Codex: the plugin",
		"the hooks of " + s.HooksFile(Codex)} {
		if !strings.Contains(got, want) {
			t.Errorf("summary lacks %q:\n%s", want, got)
		}
	}
}
