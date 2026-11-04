package agent

import (
	"encoding/json"
	"fmt"
	"io"
	"regexp"
	"strings"

	"github.com/mlophez/polix-ai/internal/engine"
	"github.com/mlophez/polix-ai/internal/workflow"
)

// Codex adapts Codex CLI hooks (registered in .codex/hooks.json). EXPERIMENTAL: Codex uses the same shape as
// Claude Code (hook_event_name, tool_name, tool_input, exit 2 to block), but its tool names and whether it has
// a subagent stop event are still to be confirmed against a running codex; unknown tools produce no event.
type Codex struct{}

// codexShellTools run commands; their command is a string or a list of arguments.
var codexShellTools = map[string]bool{"Bash": true, "shell": true, "local_shell": true, "exec_command": true}

// codexEditTools change files; apply_patch carries the paths inside the patch text.
var codexEditTools = map[string]bool{"apply_patch": true, "Edit": true, "Write": true}

var codexContextEvents = map[string]bool{"SessionStart": true, "UserPromptSubmit": true, "PreToolUse": true, "PostToolUse": true}

// patchFile finds the files an apply_patch touches.
var patchFile = regexp.MustCompile(`(?m)^\*\*\* (?:Add|Update|Delete) File: (.+)$|^\*\*\* Move to: (.+)$`)

func (Codex) Name() string { return "codex" }

func (Codex) Parse(input []byte, rel Resolver) (string, []engine.Event, error) {
	var in hookInput
	if err := json.Unmarshal(input, &in); err != nil {
		return "", nil, fmt.Errorf("codex hook input: %w", err)
	}
	const agent = "codex"
	switch in.HookEventName {
	case "SessionStart":
		return in.HookEventName, []engine.Event{in.base(agent, workflow.SessionStart)}, nil
	case "UserPromptSubmit":
		return in.HookEventName, []engine.Event{in.base(agent, workflow.Prompt)}, nil
	case "Stop":
		ev := in.base(agent, workflow.Stop)
		ev.Actor = engine.ActorSession
		return in.HookEventName, []engine.Event{ev}, nil
	case "SubagentStop":
		return in.HookEventName, []engine.Event{in.base(agent, workflow.SubagentStop)}, nil
	case "PreToolUse", "PostToolUse":
		pre := in.HookEventName == "PreToolUse"
		switch {
		case codexEditTools[in.ToolName]:
			name := workflow.PostEdit
			if pre {
				name = workflow.PreEdit
			}
			return in.HookEventName, editEvents(in.base(agent, name), codexPaths(in.ToolInput), rel), nil
		case codexShellTools[in.ToolName]:
			name := workflow.PostCommand
			if pre {
				name = workflow.PreCommand
			}
			ev := in.base(agent, name)
			ev.Command = codexCommand(in.ToolInput["command"])
			if !pre {
				ev.ExitCode = exitCode(in.ToolResponse)
			}
			return in.HookEventName, commandEvents(ev, pre), nil
		}
	}
	return in.HookEventName, nil, nil
}

func (Codex) Render(native string, res engine.Result, stdout, stderr io.Writer) int {
	return renderCommon(native, codexContextEvents, res, stdout, stderr)
}

// codexPaths returns the files of an edit: file_path, or the files named by the patch.
func codexPaths(input map[string]any) []string {
	if p := str(input, "file_path"); p != "" {
		return []string{p}
	}
	var paths []string
	for _, key := range []string{"patch", "input", "command"} {
		text := codexCommand(input[key])
		for _, m := range patchFile.FindAllStringSubmatch(text, -1) {
			for _, g := range m[1:] {
				if g = strings.TrimSpace(g); g != "" {
					paths = append(paths, g)
				}
			}
		}
	}
	return paths
}

// codexCommand joins a command given as a list of arguments.
func codexCommand(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case []any:
		parts := make([]string, 0, len(t))
		for _, p := range t {
			parts = append(parts, fmt.Sprint(p))
		}
		return strings.Join(parts, " ")
	}
	return ""
}
