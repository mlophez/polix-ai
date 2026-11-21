package uninstall

import (
	"slices"
	"strings"
	"testing"

	"github.com/mlophez/polix-ai/internal/app/setup"
)

func TestDecide(t *testing.T) {
	out := Decide(setup.Setup{Home: "/h", Root: "/r", Version: "1.2.3"}, setup.Parts{})
	if !slices.Equal(out.Setup.Agents, []string{setup.Claude, setup.Codex}) || !out.RemovePlugin || out.Summary == "" {
		t.Fatalf("both agents by default, and the marketplace: %+v", out)
	}
	if out := Decide(setup.Setup{Home: "/h", Agents: []string{setup.Claude}, Version: "1.2.3"}, setup.Parts{}); out.RemovePlugin {
		t.Fatal("Codex may still use the marketplace")
	}
}

// AC14: the hooks of git leave a repository with both agents, and stay while one of them is still installed.
func TestDecideGitHooks(t *testing.T) {
	if out := Decide(setup.Setup{Home: "/h", Root: "/r", Version: "1.2.3"}, setup.Parts{}); !slices.Equal(out.GitHooks, setup.GitHooks) {
		t.Fatalf("both agents: %+v", out.GitHooks)
	}
	for _, s := range []setup.Setup{{Home: "/h", Version: "1.2.3"},
		{Home: "/h", Root: "/r", Agents: []string{setup.Codex}, Version: "1.2.3"}} {
		if out := Decide(s, setup.Parts{}); out.GitHooks != nil {
			t.Fatalf("%+v: %+v", s, out.GitHooks)
		}
	}
}

// AC21: the summary to confirm names the hooks of git it removes inside a repository, and not outside one.
func TestDecideSummaryGitHooks(t *testing.T) {
	out := Decide(setup.Setup{Home: "/h", Root: "/r", Version: "1.2.3"}, setup.Parts{})
	if !strings.Contains(out.Summary, "hook.polix-pre-commit") || !strings.Contains(out.Summary, "hook.polix-commit-msg") {
		t.Fatalf("inside a repository:\n%s", out.Summary)
	}
	if out := Decide(setup.Setup{Home: "/h", Version: "1.2.3"}, setup.Parts{}); strings.Contains(out.Summary, "hook.polix-") {
		t.Fatalf("outside a repository:\n%s", out.Summary)
	}
}

// CUJ-06: uninstall leaves the auto memory of Claude Code as it is, so its summary to confirm does not name it.
func TestDecideSummaryKeepsAutoMemory(t *testing.T) {
	s := setup.Setup{Home: "/h", Root: "/r", Agents: []string{setup.Claude}, Version: "1.2.3"}
	for _, p := range []setup.Parts{{}, {Hooks: true}} {
		out := Decide(s, p)
		if !strings.Contains(out.Summary, "the hooks of "+s.HooksFile(setup.Claude)+"\n") ||
			strings.Contains(out.Summary, "with auto memory off") {
			t.Fatalf("%+v:\n%s", p, out.Summary)
		}
	}
}

// An explicit --githooks removes the hooks of git also when an agent stays; --skills removes the marketplace only
// with both agents, and the parts of a repository outside one are a failure.
func TestDecideParts(t *testing.T) {
	codex := setup.Setup{Home: "/h", Root: "/r", Agents: []string{setup.Codex}, Version: "1.2.3"}
	if out := Decide(codex, setup.Parts{GitHooks: true}); !slices.Equal(out.GitHooks, setup.GitHooks) || out.Parts.Skills {
		t.Fatalf("--githooks: %+v", out)
	}
	if out := Decide(codex, setup.Parts{Skills: true}); out.RemovePlugin || out.GitHooks != nil {
		t.Fatalf("--skills of one agent: %+v", out)
	}
	if out := Decide(setup.Setup{Home: "/h", Version: "1.2.3"}, setup.Parts{Hooks: true}); !strings.Contains(out.Failure, "not inside a git repository") {
		t.Fatalf("outside: %+v", out)
	}
}
