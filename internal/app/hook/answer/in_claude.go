package answer

import (
	"encoding/json"
	"fmt"

	"github.com/mlophez/polix-ai/internal/app/agent"
	"github.com/mlophez/polix-ai/internal/app/workflow"
)

// claude is the format of the hooks of Claude Code, registered in .claude/settings.json.
type claude struct{}

// claudeInput is the input of a hook of Claude Code: the fields both agents share, and the id Claude Code gives the
// call of a tool, the same on PreToolUse and PostToolUse, which pairs the snapshot of a command with its end.
type claudeInput struct {
	hookInput
	ToolUseID string `json:"tool_use_id"`
}

// claudeEditTools are the tools of Claude Code that change files.
var claudeEditTools = map[string]bool{"Edit": true, "Write": true, "MultiEdit": true, "NotebookEdit": true}

// claudeContextEvents are the events of Claude Code whose JSON output can add context to the session.
var claudeContextEvents = contextEvents{"SessionStart": true, "UserPromptSubmit": true, "SubagentStart": true,
	"PreToolUse": true, "PostToolUse": true, "PostToolUseFailure": true}

// parse reads the input of a hook of Claude Code, the id of the call of a tool included.
func (c claude) parse(input []byte) (string, []agent.Event, error) {
	var raw claudeInput
	if err := json.Unmarshal(input, &raw); err != nil {
		return "", nil, fmt.Errorf("claude hook input: %w", err)
	}
	in := raw.hookInput
	in.ToolUse = raw.ToolUseID
	native := in.HookEventName
	var events []agent.Event
	var err error
	switch native {
	case "SessionStart":
		events, err = in.single(agent.Claude, workflow.Start)
	case "UserPromptSubmit":
		events, err = in.single(agent.Claude, workflow.Prompt)
	case "SubagentStart":
		events, err = in.single(agent.Claude, workflow.SubagentStart)
	case "Stop":
		events, err = in.single(agent.Claude, workflow.Stop)
	case "SubagentStop":
		events, err = in.single(agent.Claude, workflow.SubagentStop)
	case "PreToolUse", "PostToolUse", "PostToolUseFailure":
		events, err = c.tool(in)
	}
	return native, events, err
}

// tool returns the events of a call of a tool of Claude Code: an edit, a command, or none for any other tool.
func (claude) tool(in hookInput) ([]agent.Event, error) {
	native := in.HookEventName
	pre := native == "PreToolUse"
	switch {
	case claudeEditTools[in.ToolName] && native == "PostToolUseFailure":
		return nil, nil // a failed edit changed nothing: no post_edit
	case claudeEditTools[in.ToolName]:
		name := workflow.PostEdit
		if pre {
			name = workflow.PreEdit
		}
		path := in.str("file_path")
		if path == "" {
			path = in.str("notebook_path")
		}
		return in.edits(agent.Claude, name, []string{path})
	case in.ToolName == "Bash":
		name := workflow.PostCommand
		if pre {
			name = workflow.PreCommand
		}
		return in.commands(agent.Claude, name, in.str("command"), native == "PostToolUseFailure")
	}
	return nil, nil
}

// render writes the answer for the native event of Claude Code.
func (claude) render(native string, ans agent.Answer) (stdout, stderr string, code int) {
	return claudeContextEvents.render(native, ans)
}
