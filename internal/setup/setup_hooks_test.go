package setup

import (
	"strings"
	"testing"

	"github.com/mlophez/polix-ai/internal/hook"
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

func TestRegisteredAndHooksError(t *testing.T) {
	data, _, _ := RegisterHooks(nil, false, Codex)
	if !Registered(data) || Registered([]byte(`{"hooks": {"Stop": [{"hooks": [{"command": "x"}]}]}}`)) || Registered(nil) {
		t.Fatal("registered")
	}
	if HooksError(nil) != nil || HooksError([]byte("{")) == nil {
		t.Fatal("hooks error")
	}
}
