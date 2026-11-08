// Package hook is the domain of a hook call of an agent: the events it carries, the evaluation of the workflow
// for each of them, step by step, the protections that come before the workflow, the conditions of its entries
// and the heuristic of the commands that write files.
//
// It measures and never judges: conditions are questions about the repository, the event and the log of what polix
// observed; judging the code is the job of the reviewer subagent.
package hook

import (
	"errors"
	"time"

	"github.com/mlophez/polix-ai/internal/agent"
)

// Hook is one hook call: the agent behind it, the native name of its event and the neutral events it carries.
type Hook struct {
	Agent  agent.Agent
	Native string
	Events []agent.Event
}

// RunBudget bounds every run of one hook call together (commands and rule checks). The timeout of the hook in the
// settings of the agent must be longer (setup.HookTimeout), or the agent kills the hook first and the gate is
// skipped.
const RunBudget = 10 * time.Minute

// Valid tells why the call cannot be answered: an unknown agent or an event polix does not evaluate.
func (h Hook) Valid() error {
	errs := []error{h.Agent.Valid()}
	for _, e := range h.Events {
		errs = append(errs, e.Valid())
	}
	return errors.Join(errs...)
}

// Folder returns the folder the repository of the call is looked for from: dir when given, else the cwd of the
// call, else "" (the working directory).
func (h Hook) Folder(dir string) string {
	if dir != "" || len(h.Events) == 0 {
		return dir
	}
	return h.Events[0].Cwd
}
