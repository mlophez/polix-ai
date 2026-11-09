package agent

import (
	"errors"
	"fmt"
	"slices"

	"github.com/mlophez/polix-ai/internal/app/workflow"
)

// Event is what a hook call turns into: the same shape for every agent.
type Event struct {
	Name     string // a neutral event of the workflow package
	Agent    string // claude, codex
	Session  string
	Actor    string // ActorSession or the subagent type
	Path     string // pre_edit and post_edit: repository relative, slash separated, once located
	AbsPath  string // pre_edit and post_edit: the path as the agent gave it, absolute from the cwd when relative
	Outside  bool   // the edited path is outside the repository
	Command  string // pre_command, post_command, pre_commit
	ExitCode int    // post_command
	Prompt   string // prompt
	Cwd      string
}

// ActorSession is the actor of the main session; subagents use their own type (builder, reviewer...).
const ActorSession = "session"

// Valid tells why the event is not one polix evaluates: its name is not a neutral event.
func (e Event) Valid() error {
	if !slices.Contains(workflow.Events, e.Name) {
		return fmt.Errorf("unknown event %q", e.Name)
	}
	if e.Agent == "" {
		return errors.New("an event needs its agent")
	}
	return nil
}

// Edits reports whether the event is an edit, whose path must be located in the repository.
func (e Event) Edits() bool {
	return e.Name == workflow.PreEdit || e.Name == workflow.PostEdit
}

// Locate returns the event with its path located in the repository: rel, relative to the root, when inside is
// true; outside of it otherwise.
func (e Event) Locate(rel string, inside bool) Event {
	e.Outside = !inside
	if inside {
		e.Path = rel
	}
	return e
}
