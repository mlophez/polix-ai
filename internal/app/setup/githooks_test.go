package setup

import "testing"

// AC11: the hooks of git polix registers are pre-commit and commit-msg, each named polix-<event>, since git forbids a
// name equal to an event, with the command polix githook <event>; a change of one names its key.
func TestGitHooks(t *testing.T) {
	want := []struct{ event, name, command, key string }{
		{"pre-commit", "polix-pre-commit", "polix githook pre-commit", "git config hook.polix-pre-commit"},
		{"commit-msg", "polix-commit-msg", "polix githook commit-msg", "git config hook.polix-commit-msg"},
	}
	if len(GitHooks) != len(want) {
		t.Fatalf("GitHooks = %q", GitHooks)
	}
	for i, h := range GitHooks {
		w := want[i]
		if string(h) != w.event || h.Name() != w.name || h.Command() != w.command || h.Key() != w.key {
			t.Errorf("%d: %q %q %q %q", i, h, h.Name(), h.Command(), h.Key())
		}
	}
}
