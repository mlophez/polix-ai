package agent

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/mlophez/polix-ai/internal/app/workflow"
)

// codexShellTools are the tools of Codex that run commands; their command is a string or a list of arguments.
var codexShellTools = map[string]bool{"Bash": true, "shell": true, "local_shell": true, "exec_command": true}

// codexEditTools are the tools of Codex that change files; apply_patch carries the paths inside the patch text.
var codexEditTools = map[string]bool{"apply_patch": true, "Edit": true, "Write": true}

// codexContextEvents are the events of Codex whose JSON output can add context to the session.
var codexContextEvents = map[string]bool{"SessionStart": true, "UserPromptSubmit": true, "PreToolUse": true,
	"PostToolUse": true}

// codexPatchFile finds the files an apply_patch touches.
var codexPatchFile = regexp.MustCompile(`(?m)^\*\*\* (?:Add|Update|Delete) File: (.+)$|^\*\*\* Move to: (.+)$`)

// parseCodex reads the input of a hook of Codex (registered in .codex/hooks.json). EXPERIMENTAL: Codex uses the
// same shape as Claude Code (hook_event_name, tool_name, tool_input, exit 2 to block), but its tool names and
// whether it has a subagent stop event are still to be confirmed against a running codex; unknown tools produce
// no event.
func parseCodex(input []byte) (string, []Event, error) {
	var in inputJSON
	if err := json.Unmarshal(input, &in); err != nil {
		return "", nil, fmt.Errorf("codex hook input: %w", err)
	}
	native := in.HookEventName
	switch native {
	case "SessionStart":
		return native, []Event{Event{}.begin(in, Codex, workflow.Start)}, nil
	case "UserPromptSubmit":
		return native, []Event{Event{}.begin(in, Codex, workflow.Prompt)}, nil
	case "Stop":
		return native, []Event{Event{}.begin(in, Codex, workflow.Stop)}, nil
	case "SubagentStop":
		return native, []Event{Event{}.begin(in, Codex, workflow.SubagentStop)}, nil
	case "PreToolUse", "PostToolUse":
		pre := native == "PreToolUse"
		switch {
		case codexEditTools[in.ToolName]:
			name := workflow.PostEdit
			if pre {
				name = workflow.PreEdit
			}
			return native, editEvents(Event{}.begin(in, Codex, name), codexPaths(in.ToolInput)), nil
		case codexShellTools[in.ToolName]:
			name := workflow.PostCommand
			if pre {
				name = workflow.PreCommand
			}
			ev := Event{}.begin(in, Codex, name).ran(codexCommand(in.ToolInput["command"]), in.ToolResponse, false)
			return native, commandEvents(ev), nil
		}
	}
	return native, nil, nil
}

// codexPaths returns the files of an edit: file_path, or the files named by the patch.
func codexPaths(input map[string]any) []string {
	if p := str(input, "file_path"); p != "" {
		return []string{p}
	}
	var paths []string
	for _, key := range []string{"patch", "input", "command"} {
		text := codexCommand(input[key])
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
