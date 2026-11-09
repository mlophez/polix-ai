package uninstall

import (
	"slices"
	"testing"

	"github.com/mlophez/polix-ai/internal/app/setup"
)

func TestDecide(t *testing.T) {
	out := Decide(setup.Setup{Home: "/h", Root: "/r", Version: "1.2.3"})
	if !slices.Equal(out.Setup.Agents, []string{setup.Claude, setup.Codex}) || !out.RemovePlugin || out.Summary == "" {
		t.Fatalf("both agents by default, and the marketplace: %+v", out)
	}
	if out := Decide(setup.Setup{Home: "/h", Agents: []string{setup.Claude}, Version: "1.2.3"}); out.RemovePlugin {
		t.Fatal("Codex may still use the marketplace")
	}
}
