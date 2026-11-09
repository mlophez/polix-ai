package answer

import (
	"encoding/json"
	"strings"

	"github.com/mlophez/polix-ai/internal/app/agent"
)

// contextEvents are the native events of an agent whose JSON output can add context to the session.
type contextEvents map[string]bool

// contextOutput is the JSON output of a hook that adds context to the session, as both agents read it.
type contextOutput struct {
	HookSpecificOutput contextBody `json:"hookSpecificOutput"`
}

// contextBody is the body of a contextOutput: the context to add and the native event it answers.
type contextBody struct {
	AdditionalContext string `json:"additionalContext"`
	HookEventName     string `json:"hookEventName"`
}

// render writes a block as exit 2 with the reasons on stderr, its contexts after them; otherwise the warnings go
// to stderr and the contexts to the JSON both agents read, on the native events of ce.
func (ce contextEvents) render(native string, a agent.Answer) (stdout, stderr string, code int) {
	if a.Block {
		msg := strings.Join(a.Reasons, "\n\n")
		if len(a.Contexts) > 0 {
			msg += "\n\n" + strings.Join(a.Contexts, "\n\n")
		}
		return "", msg + "\n", agent.ExitBlock
	}
	var errb strings.Builder
	for _, w := range a.Warnings {
		errb.WriteString("polix: " + w + "\n")
	}
	if len(a.Contexts) > 0 && ce[native] {
		out := contextOutput{HookSpecificOutput: contextBody{
			AdditionalContext: strings.Join(a.Contexts, "\n\n"),
			HookEventName:     native,
		}}
		if data, err := json.Marshal(out); err == nil {
			stdout = string(data) + "\n"
		}
	}
	return stdout, errb.String(), agent.ExitOK
}
