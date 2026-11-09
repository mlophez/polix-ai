package setup

import (
	"strings"
	"testing"

	"github.com/mlophez/polix-ai/internal/app/hook"
)

func TestIsPolixCommand(t *testing.T) {
	for _, c := range []string{"polix hook claude", "polix update --hook", "polix update --hook --flag-of-a-later-release",
		"polix update --hook; polix doctor --hook"} {
		if !IsPolixCommand(c) {
			t.Errorf("%q is a command of polix", c)
		}
	}
	if IsPolixCommand("polix update --check") || IsPolixCommand("polix update --hookx") {
		t.Error("only the session start command is the update of polix")
	}
	if !IsLegacyCommand(" polix update --hook; polix doctor --hook ") || IsLegacyCommand(UpdateCommand) {
		t.Error("the legacy command")
	}
}

// The hooks give polix more time than the run budget of a call, so the agent never kills a gate in the middle.
func TestHookTimeouts(t *testing.T) {
	for _, agent := range []string{Claude, Codex} {
		for ev, groups := range hooks(agent) {
			for _, g := range groups {
				for _, h := range g.Hooks {
					if strings.HasPrefix(h.Command, "polix hook") && h.Timeout <= hook.RunBudget.Seconds() {
						t.Errorf("%s %s: timeout %v", agent, ev, h.Timeout)
					}
				}
			}
		}
	}
}

func TestHookEvents(t *testing.T) {
	if _, ok := hooks(Claude)["PostToolUseFailure"]; !ok {
		t.Fatal("Claude Code gets PostToolUseFailure")
	}
	if _, ok := hooks(Codex)["PostToolUseFailure"]; ok {
		t.Fatal("only Claude Code gets PostToolUseFailure")
	}
}

func TestParseHooks(t *testing.T) {
	data := []byte(`{"permissions": {}, "hooks": {
		"PreToolUse": [{"matcher": "Edit|Bash", "hooks": [{"type": "command", "command": "polix hook claude", "timeout": 900}]}],
		"Stop": [{"hooks": [{"type": "command", "command": "echo hi"}, {"type": "prompt", "prompt": "p"}]},
			{"hooks": [{"type": "prompt", "prompt": "p"}]}]}}`)
	got, err := ParseHooks(data)
	if err != nil {
		t.Fatal(err)
	}
	pre, stop := got["PreToolUse"], got["Stop"]
	if len(got) != 2 || len(pre) != 1 || pre[0].Matcher != "Edit|Bash" ||
		len(pre[0].Handlers) != 1 || pre[0].Handlers[0] != (HookHandler{Command: "polix hook claude", Timeout: 900}) {
		t.Fatalf("%+v", got)
	}
	if len(stop) != 1 || stop[0].Matcher != "" || len(stop[0].Handlers) != 1 || stop[0].Handlers[0].Command != "echo hi" {
		t.Fatalf("a handler without a command and a group without handlers are left out: %+v", stop)
	}
	if got, err := ParseHooks([]byte(`{"permissions": {"allow": []}}`)); err != nil || len(got) != 0 {
		t.Fatalf("no hooks: %+v, %v", got, err)
	}
	for _, bad := range []string{"{", "", `{"hooks": []}`} {
		if _, err := ParseHooks([]byte(bad)); err == nil {
			t.Errorf("%q is not a settings file", bad)
		}
	}
}

func TestHookGroupValid(t *testing.T) {
	h := HookHandler{Command: "polix hook claude", Timeout: 900}
	if h.Valid() != nil || (HookHandler{Command: " ", Timeout: 900}).Valid() == nil {
		t.Fatal("a handler needs a command")
	}
	if (HookGroup{Matcher: "Bash", Handlers: []HookHandler{h}}).Valid() != nil ||
		(HookGroup{Matcher: "Bash"}).Valid() == nil ||
		(HookGroup{Handlers: []HookHandler{h, {}}}).Valid() == nil {
		t.Fatal("a group needs valid handlers")
	}
}

func TestRegisteredAndHooksError(t *testing.T) {
	data, _, _ := RegisterHooks(nil, false, Codex)
	if !Registered(data) || Registered([]byte(`{"hooks": {"Stop": [{"hooks": [{"command": "x"}]}]}}`)) || Registered(nil) {
		t.Fatal("registered")
	}
	if HooksError(nil) != nil || HooksError([]byte("{")) == nil {
		t.Fatal("hooks error")
	}
}
