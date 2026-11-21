package answer

import (
	"encoding/json"
	"strings"

	"github.com/mlophez/polix-ai/internal/app/agent"
)

// contextEvents are the native events of an agent whose JSON output can add context to the session.
type contextEvents map[string]bool

// contextOutput is the JSON output of a hook on exit 0, as both agents read it: the messages for the user, which
// every event takes, and the context to add, only on the events that take it. Codex fails a hook on a field it does
// not accept, so each one is left out when empty.
type contextOutput struct {
	SystemMessage      string       `json:"systemMessage,omitempty"`
	HookSpecificOutput *contextBody `json:"hookSpecificOutput,omitempty"`
}

// contextBody is the body of a contextOutput: the context to add and the native event it answers.
type contextBody struct {
	AdditionalContext string `json:"additionalContext"`
	HookEventName     string `json:"hookEventName"`
}

// messagePrefix starts each message for the user, so the user knows who speaks.
const messagePrefix = "polix: "

// render writes a block as exit 2 with the reasons on stderr, its contexts after them, and stdout empty: neither
// agent reads stdout on exit 2, so the messages for the user are not written (the logic logs them as not shown).
// Otherwise it writes, on exit 0 and with stderr empty, one JSON object with the messages for the user, each with
// messagePrefix, joined in systemMessage, and the contexts in hookSpecificOutput only on the native events of ce;
// with neither, stdout stays empty.
func (ce contextEvents) render(native string, a agent.Answer) (stdout, stderr string, code int) {
	if a.Block {
		msg := strings.Join(a.Reasons, "\n\n")
		if len(a.Contexts) > 0 {
			msg += "\n\n" + strings.Join(a.Contexts, "\n\n")
		}
		return "", msg + "\n", agent.ExitBlock
	}
	var out contextOutput
	if len(a.Warnings) > 0 {
		messages := make([]string, 0, len(a.Warnings))
		for _, w := range a.Warnings {
			messages = append(messages, messagePrefix+w)
		}
		out.SystemMessage = strings.Join(messages, "\n")
	}
	if len(a.Contexts) > 0 && ce[native] {
		out.HookSpecificOutput = &contextBody{AdditionalContext: strings.Join(a.Contexts, "\n\n"), HookEventName: native}
	}
	if out.SystemMessage == "" && out.HookSpecificOutput == nil {
		return "", "", agent.ExitOK
	}
	if data, err := json.Marshal(out); err == nil {
		stdout = string(data) + "\n"
	}
	return stdout, "", agent.ExitOK
}
