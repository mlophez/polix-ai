package answer

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/mlophez/polix-ai/internal/app/agent"
	"github.com/mlophez/polix-ai/internal/app/workflow"
)

// codex is the format of the hooks of Codex, registered in .codex/hooks.json. EXPERIMENTAL: Codex uses the same
// shape as Claude Code (hook_event_name, tool_name, tool_input, exit 2 to block), but its tool names and whether it
// has a subagent stop event are still to be confirmed against a running codex; unknown tools produce no event.
type codex struct{}

// codexShellTools are the tools of Codex that run commands; their command is a string or a list of arguments.
var codexShellTools = map[string]bool{"Bash": true, "shell": true, "local_shell": true, "exec_command": true}

// codexEditTools are the tools of Codex that change files; apply_patch carries the paths inside the patch text.
var codexEditTools = map[string]bool{"apply_patch": true, "Edit": true, "Write": true}

// codexContextEvents are the events of Codex whose JSON output can add context to the session.
var codexContextEvents = contextEvents{"SessionStart": true, "UserPromptSubmit": true, "SubagentStart": true,
	"PreToolUse": true, "PostToolUse": true}

// codexPatchFile finds the files an apply_patch touches.
var codexPatchFile = regexp.MustCompile(`(?m)^\*\*\* (?:Add|Update|Delete) File: (.+)$|^\*\*\* Move to: (.+)$`)

// parse reads the input of a hook of Codex.
func (c codex) parse(input []byte) (string, []agent.Event, error) {
	var in hookInput
	if err := json.Unmarshal(input, &in); err != nil {
		return "", nil, fmt.Errorf("codex hook input: %w", err)
	}
	native := in.HookEventName
	var events []agent.Event
	var err error
	switch native {
	case "SessionStart":
		events, err = in.single(agent.Codex, workflow.Start)
	case "UserPromptSubmit":
		events, err = in.single(agent.Codex, workflow.Prompt)
	case "SubagentStart":
		events, err = in.single(agent.Codex, workflow.SubagentStart)
	case "Stop":
		events, err = in.single(agent.Codex, workflow.Stop)
	case "SubagentStop":
		events, err = in.single(agent.Codex, workflow.SubagentStop)
	case "PreToolUse", "PostToolUse":
		events, err = c.tool(in)
	}
	return native, events, err
}

// tool returns the events of a call of a tool of Codex: an edit, a command, or none for any other tool.
func (c codex) tool(in hookInput) ([]agent.Event, error) {
	pre := in.HookEventName == "PreToolUse"
	switch {
	case codexEditTools[in.ToolName]:
		name := workflow.PostEdit
		if pre {
			name = workflow.PreEdit
		}
		return in.edits(agent.Codex, name, c.paths(in.ToolInput))
	case codexShellTools[in.ToolName]:
		name := workflow.PostCommand
		if pre {
			name = workflow.PreCommand
		}
		return in.commands(agent.Codex, name, c.command(in.ToolInput["command"]), false)
	}
	return nil, nil
}

// paths returns the files of an edit: file_path, or the files named by the patch.
func (c codex) paths(input map[string]any) []string {
	if p, _ := input["file_path"].(string); p != "" {
		return []string{p}
	}
	var paths []string
	for _, key := range []string{"patch", "input", "command"} {
		text := c.command(input[key])
		for _, m := range codexPatchFile.FindAllStringSubmatch(text, -1) {
			for _, g := range m[1:] {
				if g = strings.TrimSpace(g); g != "" {
					paths = append(paths, g)
				}
			}
		}
	}
	return paths
}

// command joins a command given as a list of arguments.
func (codex) command(v any) string {
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

// render writes the answer for the native event of Codex.
func (codex) render(native string, ans agent.Answer) (stdout, stderr string, code int) {
	return codexContextEvents.render(native, ans)
}
