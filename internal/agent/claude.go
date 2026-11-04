package agent

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/mlophez/polix-ai/internal/engine"
	"github.com/mlophez/polix-ai/internal/workflow"
)

// Claude adapts Claude Code hooks (registered in .claude/settings.json).
type Claude struct{}

// claudeEditTools are the tools that change files.
var claudeEditTools = map[string]bool{"Edit": true, "Write": true, "MultiEdit": true, "NotebookEdit": true}

// claudeContextEvents are the events whose JSON output can add context to the session.
var claudeContextEvents = map[string]bool{"SessionStart": true, "UserPromptSubmit": true, "PreToolUse": true, "PostToolUse": true}

func (Claude) Name() string { return "claude" }

func (Claude) Parse(input []byte, rel Resolver) (string, []engine.Event, error) {
	var in hookInput
	if err := json.Unmarshal(input, &in); err != nil {
		return "", nil, fmt.Errorf("claude hook input: %w", err)
	}
	const agent = "claude"
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
	case "PreToolUse", "PostToolUse", "PostToolUseFailure":
		pre := in.HookEventName == "PreToolUse"
		switch {
		case claudeEditTools[in.ToolName]:
			name := workflow.PostEdit
			if pre {
				name = workflow.PreEdit
			}
			path := str(in.ToolInput, "file_path")
			if path == "" {
				path = str(in.ToolInput, "notebook_path")
			}
			return in.HookEventName, editEvents(in.base(agent, name), []string{path}, rel), nil
		case in.ToolName == "Bash":
			name := workflow.PostCommand
			if pre {
				name = workflow.PreCommand
			}
			ev := in.base(agent, name)
			ev.Command = str(in.ToolInput, "command")
			if !pre {
				ev.ExitCode = exitCode(in.ToolResponse)
				if in.HookEventName == "PostToolUseFailure" && ev.ExitCode == 0 {
					ev.ExitCode = 1
				}
			}
			return in.HookEventName, commandEvents(ev, pre), nil
		}
	}
	return in.HookEventName, nil, nil
}

func (Claude) Render(native string, res engine.Result, stdout, stderr io.Writer) int {
	return renderCommon(native, claudeContextEvents, res, stdout, stderr)
}
