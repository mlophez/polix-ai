package agent

import (
	"encoding/json"
	"fmt"

	"github.com/mlophez/polix-ai/internal/app/workflow"
)

// claudeEditTools are the tools of Claude Code that change files.
var claudeEditTools = map[string]bool{"Edit": true, "Write": true, "MultiEdit": true, "NotebookEdit": true}

// claudeContextEvents are the events of Claude Code whose JSON output can add context to the session.
var claudeContextEvents = map[string]bool{"SessionStart": true, "UserPromptSubmit": true, "PreToolUse": true,
	"PostToolUse": true, "PostToolUseFailure": true}

// parseClaude reads the input of a hook of Claude Code (registered in .claude/settings.json).
func parseClaude(input []byte) (string, []Event, error) {
	var in inputJSON
	if err := json.Unmarshal(input, &in); err != nil {
		return "", nil, fmt.Errorf("claude hook input: %w", err)
	}
	native := in.HookEventName
	switch native {
	case "SessionStart":
		return native, []Event{Event{}.begin(in, Claude, workflow.Start)}, nil
	case "UserPromptSubmit":
		return native, []Event{Event{}.begin(in, Claude, workflow.Prompt)}, nil
	case "Stop":
		return native, []Event{Event{}.begin(in, Claude, workflow.Stop)}, nil
	case "SubagentStop":
		return native, []Event{Event{}.begin(in, Claude, workflow.SubagentStop)}, nil
	case "PreToolUse", "PostToolUse", "PostToolUseFailure":
		pre := native == "PreToolUse"
		switch {
		case claudeEditTools[in.ToolName] && native == "PostToolUseFailure":
			return native, nil, nil // a failed edit changed nothing: no post_edit
		case claudeEditTools[in.ToolName]:
			name := workflow.PostEdit
			if pre {
				name = workflow.PreEdit
			}
			path := str(in.ToolInput, "file_path")
			if path == "" {
				path = str(in.ToolInput, "notebook_path")
			}
			return native, editEvents(Event{}.begin(in, Claude, name), []string{path}), nil
		case in.ToolName == "Bash":
			name := workflow.PostCommand
			if pre {
				name = workflow.PreCommand
			}
			ev := Event{}.begin(in, Claude, name).ran(str(in.ToolInput, "command"), in.ToolResponse,
				native == "PostToolUseFailure")
			return native, commandEvents(ev), nil
		}
	}
	return native, nil, nil
}
