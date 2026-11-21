package agent

import (
	"errors"
	"fmt"
	"slices"

	"github.com/mlophez/polix-ai/internal/app/workflow"
)

// Event is what a hook call turns into: the same shape for every agent.
type Event struct {
	Name    string // a neutral event of the workflow package
	Agent   string // claude, codex
	Session string
	Actor   string // ActorSession or the subagent type
	// AgentID is the id the agent gives the subagent that acts, so two subagents of one type are told apart; empty
	// for the main session and when the agent gives none.
	AgentID string
	// Source is why a start happened as the agent gives it (startup, resume, clear, compact): only on start, empty
	// when the agent gives none.
	Source   string
	Path     string // pre_edit and post_edit: repository relative, slash separated, once located
	AbsPath  string // pre_edit and post_edit: the path as the agent gave it, absolute from the cwd when relative
	Outside  bool   // the edited path is outside the repository
	Command  string // pre_command, post_command, pre_commit, post_commit
	ExitCode int    // post_command, post_commit
	Prompt   string // prompt
	Cwd      string
	ToolUse  string // the id the agent gives the call of a tool, the same before and after it; empty when it gives none
	// ByCommand is set on a pre_edit or post_edit that polix derived from the effect of a command, not on one of the
	// edit tool: the edit already happened.
	ByCommand bool
	// InFlight are the subagents the agent lists as still in flight on stop and subagent_stop (background_tasks of
	// Claude Code), whatever their status, the one that acts included when the agent lists it; InFlightKnown tells
	// that the agent gave the list, so an empty one means that none is.
	InFlight      []Subagent
	InFlightKnown bool
}

// Subagent is a subagent in flight the agent lists on a stop: its id, the AgentID of its events, and its type.
type Subagent struct {
	ID   string
	Type string
}

// ActorSession is the actor of the main session; subagents use their own type (builder, reviewer...).
const ActorSession = "session"

// Valid tells why the event is not one polix evaluates: its name is not a neutral event, it has no agent, it is
// derived from a command without being an edit with its path, it carries the id of a subagent while its actor is the
// main session, a source without being a start, or subagents in flight without being a stop, without the list
// known or with one that is not valid.
func (e Event) Valid() error {
	if !slices.Contains(workflow.Events, e.Name) {
		return fmt.Errorf("unknown event %q", e.Name)
	}
	if e.Agent == "" {
		return errors.New("an event needs its agent")
	}
	if e.ByCommand && (!e.Edits() || e.Path == "") {
		return errors.New("only an edit with its path is derived from a command")
	}
	if e.AgentID != "" && e.Actor == ActorSession {
		return errors.New("the main session has no agent id")
	}
	if e.Source != "" && e.Name != workflow.Start {
		return errors.New("only a start has a source")
	}
	return e.validInFlight()
}

// validInFlight tells why the subagents in flight of the event are not valid: they come on an event other than stop
// and subagent_stop, or without the list known, or one of them is not valid.
func (e Event) validInFlight() error {
	if e.InFlightKnown && e.Name != workflow.Stop && e.Name != workflow.SubagentStop {
		return errors.New("only a stop lists the subagents in flight")
	}
	if len(e.InFlight) > 0 && !e.InFlightKnown {
		return errors.New("subagents in flight need the list known")
	}
	for _, s := range e.InFlight {
		if err := s.Valid(); err != nil {
			return err
		}
	}
	return nil
}

// Valid tells why the subagent in flight is not valid: it has neither id nor type, so nothing can tell it.
func (s Subagent) Valid() error {
	if s.ID == "" && s.Type == "" {
		return errors.New("a subagent in flight needs its id or its type")
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
