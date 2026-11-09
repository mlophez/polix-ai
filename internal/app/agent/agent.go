// Package agent is the domain of the agents polix answers: Claude Code and Codex, the neutral event a hook call
// turns into and the answer polix gives back. The formats of the hooks of each agent, which turn their input into
// events and an answer into their output, are translations: they live in the entry of AnswerHook.
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
