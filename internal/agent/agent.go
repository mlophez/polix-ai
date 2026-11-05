// Package agent is the domain of the agents polix answers: the formats of the hooks of Claude Code and Codex,
// the neutral event a hook call turns into and the answer polix gives back. It is the only code that knows which
// agent is behind a call; every format is pure, bytes in and bytes out.
package agent

import "fmt"

// Agent is the agent behind a hook call, by the name used in `polix hook <name>`.
type Agent struct {
	Name string
}

// Names of the agents.
const (
	Claude = "claude"
	Codex  = "codex"
)

// Exit codes of a hook: 0 lets the action go on, 2 blocks it and gives stderr to the model (Claude Code and
// Codex agree on both).
const (
	ExitOK    = 0
	ExitBlock = 2
)

// Valid tells why the agent is not one polix knows.
func (a Agent) Valid() error {
	switch a.Name {
	case Claude, Codex:
		return nil
	}
	return fmt.Errorf("unknown agent %q (claude, codex)", a.Name)
}

// Parse turns the input of a hook call into the events polix evaluates (none when the call is not relevant, such
// as a Read) and returns the native name of the event, needed to answer. The paths of the edits are not located in
// the repository yet: Event.Locate does it.
func (a Agent) Parse(input []byte) (native string, events []Event, err error) {
	if a.Name == Codex {
		return parseCodex(input)
	}
	return parseClaude(input)
}

// Render turns an answer into what the agent expects for its native event: the text of stdout and stderr and the
// exit code.
func (a Agent) Render(native string, ans Answer) (stdout, stderr string, code int) {
	events := claudeContextEvents
	if a.Name == Codex {
		events = codexContextEvents
	}
	return ans.render(native, events)
}
