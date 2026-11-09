package agent

import (
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
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

// inputJSON is the part of the input of a hook both agents share.
type inputJSON struct {
	HookEventName string          `json:"hook_event_name"`
	SessionID     string          `json:"session_id"`
	Cwd           string          `json:"cwd"`
	ToolName      string          `json:"tool_name"`
	ToolInput     map[string]any  `json:"tool_input"`
	ToolResponse  json.RawMessage `json:"tool_response"`
	Prompt        string          `json:"prompt"`
	AgentType     string          `json:"agent_type"`
}

// ActorSession is the actor of the main session; subagents use their own type (builder, reviewer...).
const ActorSession = "session"

// gitCommit recognises a git commit anywhere in a command line (`cd x && git commit -m ...`), after global
// options of git, those with a separate value included (`git -C dir commit`, `git -c k=v commit`), and inside a
// quoted command (`bash -c "git commit ..."`). It errs on the side of a gate: the text of a commit inside quotes
// also counts.
var gitCommit = regexp.MustCompile(`(^|[;&|(\s'"])git(\s+(-[cC]|--git-dir|--work-tree|--namespace|--config-env)\s+\S+|\s+-\S+)*\s+commit(\s|$|['"])`)

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

// actor is the subagent type when the call comes from a subagent, the main session otherwise.
func (in inputJSON) actor() string {
	if in.AgentType != "" {
		return in.AgentType
	}
	return ActorSession
}

// begin returns the event name of agent for the input in. The stop of a turn (stop) is always of the main
// session: a subagent stops with subagent_stop.
func (e Event) begin(in inputJSON, agent, name string) Event {
	e.Name, e.Agent, e.Session, e.Cwd, e.Prompt = name, agent, in.SessionID, in.Cwd, in.Prompt
	e.Actor = in.actor()
	if name == workflow.Stop {
		e.Actor = ActorSession
	}
	return e
}

// ran returns the event of the command line command: after it ran (post_command), with the exit code of its
// response, 1 when the agent says it failed without one.
func (e Event) ran(command string, response json.RawMessage, failed bool) Event {
	e.Command = command
	if e.Name == workflow.PostCommand {
		e.ExitCode = exitCode(response)
		if failed && e.ExitCode == 0 {
			e.ExitCode = 1
		}
	}
	return e
}

// at returns the event of an edit of p. A relative path (apply_patch gives them) is taken from the cwd of the
// call, so AbsPath is absolute whenever the cwd is known.
func (e Event) at(p string) Event {
	e.AbsPath = p
	if !filepath.IsAbs(p) && e.Cwd != "" {
		e.AbsPath = filepath.Join(e.Cwd, p)
	}
	return e
}

// commit returns the pre_commit of a command about to run.
func (e Event) commit() Event {
	e.Name = workflow.PreCommit
	return e
}

// commandEvents returns the event of a command, plus pre_commit when a command about to run commits.
func commandEvents(ev Event) []Event {
	events := []Event{ev}
	if ev.Name == workflow.PreCommand && gitCommit.MatchString(ev.Command) {
		events = append(events, ev.commit())
	}
	return events
}

// editEvents returns one event per edited path.
func editEvents(base Event, paths []string) []Event {
	var events []Event
	for _, p := range paths {
		events = append(events, base.at(p))
	}
	return events
}

// exitCode reads the exit code of a command from the tool response, 0 when it does not say.
func exitCode(raw json.RawMessage) int {
	var resp map[string]any
	if json.Unmarshal(raw, &resp) != nil {
		return 0
	}
	for _, k := range []string{"exit_code", "exitCode", "returncode"} {
		if n, ok := resp[k].(float64); ok {
			return int(n)
		}
	}
	return 0
}

func str(m map[string]any, key string) string {
	s, _ := m[key].(string)
	return s
}
