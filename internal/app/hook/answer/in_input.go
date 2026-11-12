package answer

import (
	"encoding/json"
	"path/filepath"
	"regexp"
	"runtime"

	"github.com/mlophez/polix-ai/internal/app/agent"
	"github.com/mlophez/polix-ai/internal/app/workflow"
	"github.com/mlophez/polix-ai/internal/platform/shell"
)

// hookInput is the part of the input of a hook both agents share.
type hookInput struct {
	HookEventName string          `json:"hook_event_name"`
	SessionID     string          `json:"session_id"`
	Cwd           string          `json:"cwd"`
	ToolName      string          `json:"tool_name"`
	ToolInput     map[string]any  `json:"tool_input"`
	ToolResponse  json.RawMessage `json:"tool_response"`
	Prompt        string          `json:"prompt"`
	AgentType     string          `json:"agent_type"`
	AgentID       looseText       `json:"agent_id"` // the id of the subagent that acts, on its events
	Source        looseText       `json:"source"`   // why a session started, on SessionStart
	ToolUse       string          `json:"-"`        // the id of the call of a tool, set by the format whose agent gives it
	// BackgroundTasks is the snapshot of the tasks in flight of the session that Claude Code gives on Stop and
	// SubagentStop, decoded leniently by inFlight.
	BackgroundTasks json.RawMessage `json:"background_tasks"`
}

// backgroundTask is one task of background_tasks: its id (the agent_id of a subagent), its type (subagent, shell...)
// and, for a subagent, its type of agent. Its status is not read: every subagent listed counts as in flight.
type backgroundTask struct {
	ID        looseText `json:"id"`
	Type      looseText `json:"type"`
	AgentType looseText `json:"agent_type"`
}

// looseText decodes a text of the input that polix only reads when it is there: a value that is not a text is
// left empty instead of failing the whole input.
type looseText string

// gitCommit is the fallback of commits for a line the bash parser rejects: it recognises a git commit anywhere in the
// line (`cd x && git commit -m ...`), after global options of git, those with a separate value included (`git -C
// dir commit`, `git -c k=v commit`), and inside a quoted command (`bash -c "git commit ..."`). It errs on the side
// of a gate: the text of a commit inside quotes also counts.
var gitCommit = regexp.MustCompile(`(^|[;&|(\s'"])git(\s+(-[cC]|--git-dir|--work-tree|--namespace|--config-env)\s+\S+|\s+-\S+)*\s+commit(\s|$|['"])`)

// UnmarshalJSON keeps the text of a JSON string, and leaves t empty for any other value, without error.
func (t *looseText) UnmarshalJSON(data []byte) error {
	var s string
	if json.Unmarshal(data, &s) != nil {
		s = ""
	}
	*t = looseText(s)
	return nil
}

// actor is the subagent type when the call comes from a subagent, the main session otherwise.
func (in hookInput) actor() string {
	if in.AgentType != "" {
		return in.AgentType
	}
	return agent.ActorSession
}

// str returns the text of the input of the tool under key, "" when it has none.
func (in hookInput) str(key string) string {
	s, _ := in.ToolInput[key].(string)
	return s
}

// exitCode reads the exit code of a command from the tool response, 0 when it does not say.
func (in hookInput) exitCode() int {
	var resp map[string]any
	if json.Unmarshal(in.ToolResponse, &resp) != nil {
		return 0
	}
	for _, k := range []string{"exit_code", "exitCode", "returncode"} {
		if n, ok := resp[k].(float64); ok {
			return int(n)
		}
	}
	return 0
}

// event returns the event name of the agent ag for the input, with abs, the path of an edit, and with command and
// code, a command line and its exit code, and with the id of the call of the tool when the format set it; the error
// tells why it is not valid. The stop of a turn (stop) is always of the main session: a subagent stops with
// subagent_stop. The id of the subagent goes only with a subagent as the actor, the source only with a start, and
// the subagents in flight only with stop and subagent_stop.
func (in hookInput) event(ag, name, abs, command string, code int) (agent.Event, error) {
	actor := in.actor()
	if name == workflow.Stop {
		actor = agent.ActorSession
	}
	id, source := string(in.AgentID), ""
	if actor == agent.ActorSession {
		id = ""
	}
	if name == workflow.Start {
		source = string(in.Source)
	}
	var inFlight []agent.Subagent
	known := false
	if name == workflow.Stop || name == workflow.SubagentStop {
		inFlight, known = in.inFlight()
	}
	ev := agent.Event{Name: name, Agent: ag, Session: in.SessionID, Actor: actor, AgentID: id, Source: source,
		AbsPath: abs, Command: command, ExitCode: code, Prompt: in.Prompt, Cwd: in.Cwd, ToolUse: in.ToolUse,
		InFlight: inFlight, InFlightKnown: known}
	return ev, ev.Valid()
}

// inFlight returns the subagents of background_tasks, the tasks of other types left out, and whether the input gave
// them: an array whose tasks are all objects, each subagent with its id or its type. Anything else, null and a
// missing array included, counts as not given, without error, so polix decides by its log alone.
func (in hookInput) inFlight() ([]agent.Subagent, bool) {
	var tasks []backgroundTask
	if json.Unmarshal(in.BackgroundTasks, &tasks) != nil || tasks == nil {
		return nil, false
	}
	var out []agent.Subagent
	for _, t := range tasks {
		if t.Type != "subagent" {
			continue
		}
		s := agent.Subagent{ID: string(t.ID), Type: string(t.AgentType)}
		if s.Valid() != nil {
			return nil, false
		}
		out = append(out, s)
	}
	return out, true
}

// single returns the one event name of the agent ag for an input that is neither an edit nor a command.
func (in hookInput) single(ag, name string) ([]agent.Event, error) {
	ev, err := in.event(ag, name, "", "", 0)
	if err != nil {
		return nil, err
	}
	return []agent.Event{ev}, nil
}

// edits returns one event name of the agent ag per edited path. A relative path (apply_patch gives them) is taken
// from the cwd of the call, so AbsPath is absolute whenever the cwd is known.
func (in hookInput) edits(ag, name string, paths []string) ([]agent.Event, error) {
	var events []agent.Event
	for _, p := range paths {
		if !filepath.IsAbs(p) && in.Cwd != "" {
			p = filepath.Join(in.Cwd, p)
		}
		ev, err := in.event(ag, name, p, "", 0)
		if err != nil {
			return nil, err
		}
		events = append(events, ev)
	}
	return events, nil
}

// commands returns the event name of the agent ag of the command line command: after it ran (post_command), with
// the exit code of its response, 1 when the agent says it failed without one; about to run (pre_command), plus
// pre_commit when it commits.
func (in hookInput) commands(ag, name, command string, failed bool) ([]agent.Event, error) {
	code := 0
	if name == workflow.PostCommand {
		code = in.exitCode()
		if failed && code == 0 {
			code = 1
		}
	}
	names := []string{name}
	if name == workflow.PreCommand && in.commits(command) {
		names = append(names, workflow.PreCommit)
	}
	var events []agent.Event
	for _, n := range names {
		ev, err := in.event(ag, n, "", command, code)
		if err != nil {
			return nil, err
		}
		events = append(events, ev)
	}
	return events, nil
}

// commits reports whether a command line commits: a command of it, nested ones included (`bash -c "git commit"`),
// whose normalized words start with git and commit (`git -C x commit`, `sudo git commit`), so the text of a commit
// inside quotes (`echo "git commit"`) does not count. A line the bash parser rejects is decided by gitCommit.
func (in hookInput) commits(command string) bool {
	cmds, ast := shell.Parse(command, runtime.GOOS == "windows")
	if !ast {
		return gitCommit.MatchString(command)
	}
	for _, c := range cmds {
		if ws := c.Normal(); len(ws) > 1 && shell.Program(ws[0]) == "git" && ws[1] == "commit" {
			return true
		}
	}
	return false
}
