// Package agent holds the adapters: they turn the JSON a hook receives on stdin into neutral events and the
// result of the engine into what each agent expects (exit code, stderr and JSON on stdout). They are the only
// code that knows which agent is behind a call.
package agent

import (
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/mlophez/polix-ai/internal/engine"
)

// Exit codes of a hook: 0 lets the action go on, 2 blocks it and gives stderr to the model (Claude Code and
// Codex agree on both).
const (
	ExitOK    = 0
	ExitBlock = 2
)

// Adapter translates one agent.
type Adapter interface {
	// Name is the name used in `polix hook <name>`.
	Name() string
	// Parse turns the hook input into events (none when the call is not relevant, e.g. a Read) and returns the
	// native event name, needed to answer.
	Parse(input []byte, rel Resolver) (native string, events []engine.Event, err error)
	// Render writes the answer and returns the exit code.
	Render(native string, res engine.Result, stdout, stderr io.Writer) int
}

// Resolver turns a path given by the agent into a repository relative one (ok false when outside).
type Resolver func(path, cwd string) (rel string, ok bool)

// For returns the adapter of an agent.
func For(name string) (Adapter, error) {
	switch name {
	case "claude":
		return Claude{}, nil
	case "codex":
		return Codex{}, nil
	}
	return nil, fmt.Errorf("unknown agent %q (claude, codex)", name)
}

// hookInput is the part of the hook input both agents share.
type hookInput struct {
	HookEventName string          `json:"hook_event_name"`
	SessionID     string          `json:"session_id"`
	Cwd           string          `json:"cwd"`
	ToolName      string          `json:"tool_name"`
	ToolInput     map[string]any  `json:"tool_input"`
	ToolResponse  json.RawMessage `json:"tool_response"`
	Prompt        string          `json:"prompt"`
	AgentType     string          `json:"agent_type"`
}

// actor is the subagent type when the call comes from a subagent, the main session otherwise.
func (in hookInput) actor() string {
	if in.AgentType != "" {
		return in.AgentType
	}
	return engine.ActorSession
}

func (in hookInput) base(agent, name string) engine.Event {
	return engine.Event{Name: name, Agent: agent, Session: in.SessionID, Actor: in.actor(), Cwd: in.Cwd, Prompt: in.Prompt}
}

// gitCommit recognises a git commit anywhere in a command line (`cd x && git commit -m ...`), after global
// options of git, those with a separate value included (`git -C dir commit`, `git -c k=v commit`), and inside a
// quoted command (`bash -c "git commit ..."`). It errs on the side of a gate: the text of a commit inside quotes
// also counts.
var gitCommit = regexp.MustCompile(`(^|[;&|(\s'"])git(\s+(-[cC]|--git-dir|--work-tree|--namespace|--config-env)\s+\S+|\s+-\S+)*\s+commit(\s|$|['"])`)

// commandEvents returns pre_command, plus pre_commit when the command commits.
func commandEvents(ev engine.Event, pre bool) []engine.Event {
	if !pre {
		return []engine.Event{ev}
	}
	events := []engine.Event{ev}
	if gitCommit.MatchString(ev.Command) {
		commit := ev
		commit.Name = "pre_commit"
		events = append(events, commit)
	}
	return events
}

// editEvents returns one event per edited path. A relative path (apply_patch gives them) is taken from the cwd
// of the call, so AbsPath is absolute whenever the cwd is known.
func editEvents(base engine.Event, paths []string, rel Resolver) []engine.Event {
	var events []engine.Event
	for _, p := range paths {
		ev := base
		ev.AbsPath = p
		if !filepath.IsAbs(p) && base.Cwd != "" {
			ev.AbsPath = filepath.Join(base.Cwd, p)
		}
		r, ok := rel(p, base.Cwd)
		ev.Outside = !ok
		if ok {
			ev.Path = r
		}
		events = append(events, ev)
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

// renderCommon writes a block as exit 2 with the reasons on stderr, and context as the JSON both agents read.
func renderCommon(native string, contextEvents map[string]bool, res engine.Result, stdout, stderr io.Writer) int {
	if res.Block {
		msg := strings.Join(res.Reasons, "\n\n")
		if len(res.Contexts) > 0 {
			msg += "\n\n" + strings.Join(res.Contexts, "\n\n")
		}
		fmt.Fprintln(stderr, msg)
		return ExitBlock
	}
	for _, w := range res.Warnings {
		fmt.Fprintln(stderr, "polix: "+w)
	}
	if len(res.Contexts) > 0 && contextEvents[native] {
		out := map[string]any{"hookSpecificOutput": map[string]any{
			"hookEventName":     native,
			"additionalContext": strings.Join(res.Contexts, "\n\n"),
		}}
		_ = json.NewEncoder(stdout).Encode(out)
	}
	return ExitOK
}

func str(m map[string]any, key string) string {
	s, _ := m[key].(string)
	return s
}
