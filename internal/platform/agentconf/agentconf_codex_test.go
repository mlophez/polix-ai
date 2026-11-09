package agentconf

import (
	"strings"
	"testing"
)

// Enabling the plugin keeps the rest of the configuration, enables it again when disabled by hand, and removing
// it gives the configuration back as it was, or no file when the plugin was all it held.
func TestCodexPlugin(t *testing.T) {
	const id = "polix@polix"
	mine := "model = \"gpt-5\"\n\n[plugins.\"other@x\"]\nenabled = true\n"
	on, action := CodexPlugin([]byte(mine), true, id, true)
	if action != "updated" || !strings.HasPrefix(string(on), mine) ||
		!strings.Contains(string(on), "[plugins.\"polix@polix\"]\nenabled = true\n") || !CodexEnabled(on, id) {
		t.Fatalf("%q\n%s", action, on)
	}
	if _, action := CodexPlugin(on, true, id, true); action != "" {
		t.Fatalf("enabled twice: %q", action)
	}
	off := strings.Replace(string(on), "polix@polix\"]\nenabled = true", "polix@polix\"]\nenabled = false", 1)
	if again, _ := CodexPlugin([]byte(off), true, id, true); !CodexEnabled(again, id) || CodexEnabled([]byte(off), id) {
		t.Fatal("enabled again")
	}
	back, action := CodexPlugin(on, true, id, false)
	if action != "updated" || string(back) != mine {
		t.Fatalf("removed: %q %q", action, back)
	}
	alone, action := CodexPlugin(nil, false, id, true)
	if action != "created" {
		t.Fatalf("created: %q", action)
	}
	if _, action := CodexPlugin(alone, true, id, false); action != "removed" {
		t.Fatalf("a configuration of the plugin alone is removed: %q", action)
	}
	if _, action := CodexPlugin(nil, false, id, false); action != "" {
		t.Fatalf("nothing to remove: %q", action)
	}
	keyless := "[plugins.\"polix@polix\"]\nother = 1"
	if got, _ := CodexPlugin([]byte(keyless), true, id, true); string(got) != "[plugins.\"polix@polix\"]\nenabled = true\nother = 1" {
		t.Fatalf("the key added to its table: %q", got)
	}
	if got, _ := CodexPlugin([]byte("a = 1\n[plugins.\"polix@polix\"]\nenabled = true"), true, id, false); string(got) != "a = 1" {
		t.Fatalf("a file without a final newline keeps none: %q", got)
	}
}
