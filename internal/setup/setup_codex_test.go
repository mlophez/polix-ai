package setup

import (
	"strings"
	"testing"
)

// Enabling the plugin keeps the rest of the configuration, enables it again when disabled by hand, and removing
// it gives the configuration back as it was, or no file when polix was all it held.
func TestCodexPlugin(t *testing.T) {
	mine := "model = \"gpt-5\"\n\n[plugins.\"other@x\"]\nenabled = true\n"
	on, action := CodexPlugin([]byte(mine), true, true)
	if action != "updated" || !strings.HasPrefix(string(on), mine) ||
		!strings.Contains(string(on), "[plugins.\"polix@polix\"]\nenabled = true\n") || !CodexEnabled(on) {
		t.Fatalf("%q\n%s", action, on)
	}
	if _, action := CodexPlugin(on, true, true); action != "" {
		t.Fatalf("enabled twice: %q", action)
	}
	off := strings.Replace(string(on), "polix@polix\"]\nenabled = true", "polix@polix\"]\nenabled = false", 1)
	if again, _ := CodexPlugin([]byte(off), true, true); !CodexEnabled(again) || CodexEnabled([]byte(off)) {
		t.Fatal("enabled again")
	}
	back, action := CodexPlugin(on, true, false)
	if action != "updated" || string(back) != mine {
		t.Fatalf("removed: %q %q", action, back)
	}
	alone, action := CodexPlugin(nil, false, true)
	if action != "created" {
		t.Fatalf("created: %q", action)
	}
	if _, action := CodexPlugin(alone, true, false); action != "removed" {
		t.Fatalf("a configuration of polix alone is removed: %q", action)
	}
	if _, action := CodexPlugin(nil, false, false); action != "" {
		t.Fatalf("nothing to remove: %q", action)
	}
}
