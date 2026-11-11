package git

import (
	"context"
	"slices"
	"strings"
	"testing"
)

// SetHook writes one event and the command of a hook into the local configuration, replaces them when they differ
// and changes nothing when they are the same; RemoveHook removes the whole section and leaves the other keys, and
// tells whether there was one; Hooks names the hook for its event when this git lists hooks.
func TestHooksOfTheConfiguration(t *testing.T) {
	ctx := context.Background()
	dir, git, _ := gitRepo(t)
	r, err := Open(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	git("config", "--local", "hook.other.event", "pre-commit")
	git("config", "--local", "hook.other.command", "true")
	git("config", "--local", "hook.polix-pre-commit.event", "pre-push")
	git("config", "--local", "--add", "hook.polix-pre-commit.event", "pre-commit")
	if action, err := r.SetHook(ctx, "polix-pre-commit", "pre-commit", "polix githook pre-commit"); err != nil ||
		action != "updated" {
		t.Fatalf("SetHook over other values = %q, %v", action, err)
	}
	if action, err := r.SetHook(ctx, "polix-commit-msg", "commit-msg", "polix githook commit-msg"); err != nil ||
		action != "created" {
		t.Fatalf("SetHook = %q, %v", action, err)
	}
	if action, err := r.SetHook(ctx, "polix-commit-msg", "commit-msg", "polix githook commit-msg"); err != nil ||
		action != "" {
		t.Fatalf("SetHook again = %q, %v", action, err)
	}
	if got := git("config", "--local", "--get-all", "hook.polix-pre-commit.event"); got != "pre-commit" {
		t.Fatalf("events: %q", got)
	}
	if got := git("config", "--local", "--get-all", "hook.polix-commit-msg.command"); got != "polix githook commit-msg" {
		t.Fatalf("command: %q", got)
	}
	names, disabled, ok, err := r.Hooks(ctx, "pre-commit")
	if err != nil || (ok && (!slices.Contains(names, "polix-pre-commit") || !slices.Contains(names, "other"))) ||
		(!ok && names != nil) || disabled != nil {
		t.Fatalf("Hooks = %q, %q, %v, %v", names, disabled, ok, err)
	}
	if removed, err := r.RemoveHook(ctx, "polix-pre-commit"); err != nil || !removed {
		t.Fatalf("RemoveHook = %v, %v", removed, err)
	}
	if removed, err := r.RemoveHook(ctx, "polix-pre-commit"); err != nil || removed {
		t.Fatalf("RemoveHook again = %v, %v", removed, err)
	}
	list := git("config", "--local", "--list")
	if strings.Contains(list, "polix-pre-commit") || !strings.Contains(list, "hook.other.command=true") ||
		!strings.Contains(list, "hook.polix-commit-msg.event=commit-msg") {
		t.Fatalf("configuration: %s", list)
	}
}

// Hooks of an event without hooks of the configuration names none of them and is no error, whether this git lists
// hooks or not.
func TestHooksOfAnEventWithout(t *testing.T) {
	ctx := context.Background()
	dir, _, _ := gitRepo(t)
	r, err := Open(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	names, disabled, ok, err := r.Hooks(ctx, "commit-msg")
	if err != nil || slices.Contains(names, "polix-commit-msg") || (!ok && names != nil) || disabled != nil {
		t.Fatalf("Hooks = %q, %q, %v, %v", names, disabled, ok, err)
	}
}

// Hooks names apart a hook of the configuration with hook.<name>.enabled=false, which git lists as
// "disabled<TAB><name>" and does not run, from the ones git runs: one disabled hook alone is no event without hooks.
func TestHooksDisabled(t *testing.T) {
	ctx := context.Background()
	dir, git, _ := gitRepo(t)
	r, err := Open(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.SetHook(ctx, "polix-pre-commit", "pre-commit", "polix githook pre-commit"); err != nil {
		t.Fatal(err)
	}
	if _, err := r.SetHook(ctx, "polix-commit-msg", "commit-msg", "polix githook commit-msg"); err != nil {
		t.Fatal(err)
	}
	git("config", "--local", "hook.other.event", "pre-commit")
	git("config", "--local", "hook.other.command", "true")
	git("config", "--local", "hook.polix-pre-commit.enabled", "false")
	git("config", "--local", "hook.polix-commit-msg.enabled", "false")
	names, disabled, ok, err := r.Hooks(ctx, "pre-commit")
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Skip("this git does not list the hooks of its configuration")
	}
	if slices.Contains(names, "polix-pre-commit") || !slices.Contains(names, "other") ||
		!slices.Equal(disabled, []string{"polix-pre-commit"}) {
		t.Fatalf("Hooks(pre-commit) = %q, %q", names, disabled)
	}
	names, disabled, ok, err = r.Hooks(ctx, "commit-msg")
	if err != nil || !ok || slices.Contains(names, "polix-commit-msg") ||
		!slices.Equal(disabled, []string{"polix-commit-msg"}) {
		t.Fatalf("Hooks(commit-msg) = %q, %q, %v, %v", names, disabled, ok, err)
	}
}
