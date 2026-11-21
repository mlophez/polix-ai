package setup

import "testing"

func TestParts(t *testing.T) {
	if !(Parts{}).None() || (Parts{}).Defaulted() != (Parts{Skills: true, Hooks: true, GitHooks: true}) || (Parts{Hooks: true}).Defaulted() != (Parts{Hooks: true}) {
		t.Fatal("none asked is every part, else the parts asked")
	}
	if !(Parts{GitHooks: true}).OnlyGitHooks() || (Parts{}).Defaulted().OnlyGitHooks() ||
		(Parts{Skills: true, GitHooks: true}).WithoutGitHooks() != (Parts{Skills: true}) || (Parts{Skills: true}).Repository() ||
		!(Parts{Hooks: true}).Repository() || !(Parts{GitHooks: true}).Repository() {
		t.Fatal("only the hooks of git, and the parts of a repository")
	}
}
