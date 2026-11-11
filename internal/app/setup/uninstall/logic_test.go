package uninstall

import (
	"slices"
	"strings"
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

// AC14: the hooks of git leave a repository with both agents, and stay while one of them is still installed.
func TestDecideGitHooks(t *testing.T) {
	if out := Decide(setup.Setup{Home: "/h", Root: "/r", Version: "1.2.3"}); !slices.Equal(out.GitHooks, setup.GitHooks) {
		t.Fatalf("both agents: %+v", out.GitHooks)
	}
	for _, s := range []setup.Setup{{Home: "/h", Version: "1.2.3"},
		{Home: "/h", Root: "/r", Agents: []string{setup.Codex}, Version: "1.2.3"}} {
		if out := Decide(s); out.GitHooks != nil {
			t.Fatalf("%+v: %+v", s, out.GitHooks)
		}
	}
}

// AC21: the summary to confirm names the hooks of git it removes inside a repository, and not outside one.
func TestDecideSummaryGitHooks(t *testing.T) {
	out := Decide(setup.Setup{Home: "/h", Root: "/r", Version: "1.2.3"})
	if !strings.Contains(out.Summary, "hook.polix-pre-commit") || !strings.Contains(out.Summary, "hook.polix-commit-msg") {
		t.Fatalf("inside a repository:\n%s", out.Summary)
	}
	if out := Decide(setup.Setup{Home: "/h", Version: "1.2.3"}); strings.Contains(out.Summary, "hook.polix-") {
		t.Fatalf("outside a repository:\n%s", out.Summary)
	}
}
