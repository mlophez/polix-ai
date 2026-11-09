package uninstall

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/mlophez/polix-ai/internal/app/setup"
	"github.com/mlophez/polix-ai/internal/platform/testenv"
)

// uninstall unregisters the plugin from Claude Code once, tolerating what is gone, and from Codex, removing its
// table and the marketplace.
func TestAdapter(t *testing.T) {
	st := testenv.Machine(t, true, true)
	ctx := context.Background()
	s := setup.Setup{Home: os.Getenv("HOME"), Agents: []string{setup.Claude, setup.Codex}, Version: "1.2.3"}
	// What install leaves: the marketplace and the plugin in the fake CLI of Claude Code, the plugin enabled in the
	// configuration of Codex, and the marketplace on disk.
	testenv.Write(t, filepath.Join(st, "added"), "")
	testenv.Write(t, filepath.Join(st, "installed"), "")
	config, _ := setup.CodexPlugin(nil, false, true)
	testenv.Write(t, s.CodexConfigFile(), string(config))
	testenv.Write(t, s.PluginManifest(), "{}")
	a := Adapter{}
	ch, err := a.UnregisterPlugin(ctx, s, setup.Claude)
	if err != nil || len(ch) != 1 || ch[0].Action != "unregistered" {
		t.Fatal(ch, err)
	}
	if ch, err := a.UnregisterPlugin(ctx, s, setup.Claude); err != nil || ch != nil {
		t.Fatalf("what is gone is tolerated: %v %v", ch, err)
	}
	if ch, err := a.UnregisterPlugin(ctx, s, setup.Codex); err != nil || len(ch) != 1 || ch[0].Action != "removed" ||
		!slices.Contains(testenv.Calls(st), "codex plugin marketplace remove polix") {
		t.Fatalf("%v %v %v", ch, err, testenv.Calls(st))
	}
	if ch, err := a.RemovePlugin(ctx, s.MarketplaceDir()); err != nil || len(ch) != 1 {
		t.Fatal(ch, err)
	}
	if _, err := os.Stat(s.MarketplaceDir()); err == nil {
		t.Fatal("the marketplace is removed")
	}
	if ch, err := a.RemovePlugin(ctx, s.MarketplaceDir()); err != nil || ch != nil {
		t.Fatal("nothing to remove")
	}
}
