package setup

import (
	"context"
	"os"
	"slices"
	"testing"
)

// uninstall unregisters the plugin from Claude Code once, tolerating what is gone, and from Codex, removing its
// table and the marketplace.
func TestUninstallSetupAdapter(t *testing.T) {
	st := machine(t, true, true)
	ctx := context.Background()
	s := Setup{Home: os.Getenv("HOME"), Agents: []string{Claude, Codex}, Version: "1.2.3"}
	in := InstallSetupAdapter{}
	if _, err := in.RegisterPlugin(ctx, s, Claude); err != nil {
		t.Fatal(err)
	}
	if _, err := in.RegisterPlugin(ctx, s, Codex); err != nil {
		t.Fatal(err)
	}
	write(t, s.PluginManifest(), "{}")
	a := UninstallSetupAdapter{}
	ch, err := a.UnregisterPlugin(ctx, s, Claude)
	if err != nil || len(ch) != 1 || ch[0].Action != "unregistered" {
		t.Fatal(ch, err)
	}
	if ch, err := a.UnregisterPlugin(ctx, s, Claude); err != nil || ch != nil {
		t.Fatalf("what is gone is tolerated: %v %v", ch, err)
	}
	if ch, err := a.UnregisterPlugin(ctx, s, Codex); err != nil || len(ch) != 1 || ch[0].Action != "removed" ||
		!slices.Contains(calls(st), "codex plugin marketplace remove polix") {
		t.Fatalf("%v %v %v", ch, err, calls(st))
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
