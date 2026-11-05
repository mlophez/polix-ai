package setup

import (
	"slices"
	"testing"
)

func TestUninstallSetup(t *testing.T) {
	out := UninstallSetup("/h", "/r", nil, "1.2.3")
	if !slices.Equal(out.Setup.Agents, []string{Claude, Codex}) || !out.RemovePlugin || out.Summary == "" {
		t.Fatalf("both agents by default, and the marketplace: %+v", out)
	}
	if out := UninstallSetup("/h", "", []string{Claude}, "1.2.3"); out.RemovePlugin {
		t.Fatal("Codex may still use the marketplace")
	}
	if out := UninstallSetup("/h", "", nil, ""); out.Failure == "" {
		t.Fatal("no version")
	}
}
