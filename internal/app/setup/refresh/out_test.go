package refresh

import (
	"context"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/mlophez/polix-ai/internal/app/setup"
	"github.com/mlophez/polix-ai/internal/platform/agentconf"
	"github.com/mlophez/polix-ai/internal/platform/testenv"
)

// UpdatePlugin asks Claude Code to copy the new plugin, and Codex only when it enables it; the hooks of the
// repository are rewritten.
func TestAdapter(t *testing.T) {
	st := testenv.Machine(t, true, true)
	ctx := context.Background()
	s := setup.Setup{Home: os.Getenv("HOME"), Root: testenv.GitRepo(t), Agents: []string{setup.Claude, setup.Codex}, Version: "1"}
	a := Adapter{}
	a.UpdatePlugin(ctx, s)
	if c := testenv.Calls(st); len(c) != 1 || !strings.HasPrefix(c[0], "claude plugin update") {
		t.Fatalf("calls: %v", c)
	}
	config, _ := agentconf.CodexPlugin(nil, false, setup.PluginID, true)
	testenv.Write(t, s.CodexConfigFile(), string(config))
	a.UpdatePlugin(ctx, s)
	if !slices.Contains(testenv.Calls(st), "codex plugin marketplace upgrade polix") {
		t.Fatalf("calls: %v", testenv.Calls(st))
	}
	testenv.Write(t, s.HooksFile(setup.Claude), `{"hooks": {"Stop": [{"hooks": [{"type": "command", "command": "polix hook claude"}]}]}}`)
	if ch, err := a.RegisterHooks(ctx, s.HooksFile(setup.Claude), s.Hooks(setup.Claude)); err != nil || len(ch) != 1 || ch[0].Action != "updated" {
		t.Fatal(ch, err)
	}
	if got := a.Registered(ctx, []string{s.HooksFile(setup.Claude), s.HooksFile(setup.Codex)}); len(got) != 1 ||
		!got[s.HooksFile(setup.Claude)] {
		t.Fatalf("only the files that exist: %v", got)
	}
	if _, ok := a.ReadManifest(ctx, s.PluginManifest()); ok {
		t.Fatal("no manifest, no plugin")
	}
	testenv.Write(t, s.PluginManifest(), `{"name": "other", "version": "1"}`)
	if _, ok := a.ReadManifest(ctx, s.PluginManifest()); ok {
		t.Fatal("only the plugin polix")
	}
	files, err := a.MarketplaceFiles(ctx, setup.Plugin{Embedded: map[string][]byte{"polix/.claude-plugin/plugin.json": []byte(`{"name": "polix", "version": "0"}`)}, Version: "1.2.3"})
	if err != nil {
		t.Fatal(err)
	}
	testenv.Write(t, s.PluginManifest(), string(files["polix/.claude-plugin/plugin.json"]))
	if v, ok := a.ReadManifest(ctx, s.PluginManifest()); !ok || v != "1.2.3" {
		t.Fatal(v, ok)
	}
}
