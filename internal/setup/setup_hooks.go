package setup

import (
	"bytes"
	"encoding/json"
	"slices"
	"strings"
	"time"

	"github.com/mlophez/polix-ai/internal/hook"
)

// hookGroup and hookHandler are the format of the hooks of Claude Code and Codex: by event, groups with a
// matcher, each with its handlers.
type hookGroup struct {
	Matcher string        `json:"matcher,omitempty"`
	Hooks   []hookHandler `json:"hooks"`
}

type hookHandler struct {
	Type    string  `json:"type"`
	Command string  `json:"command"`
	Timeout float64 `json:"timeout,omitempty"`
}

// hooksFileJSON is the part of a settings file of Claude Code, or a hooks file of Codex, that holds the hooks.
type hooksFileJSON struct {
	Hooks map[string][]hookGroup `json:"hooks"`
}

const (
	// HookTimeout is the timeout, in seconds, of every `polix hook` call: five minutes above the run budget of a
	// call (hook.RunBudget), so the agent never kills a gate in the middle and lets the action through.
	HookTimeout = float64(hook.RunBudget/time.Second) + 300
	// updateTimeout bounds the session start check, in seconds: an update downloads the binary.
	updateTimeout = 180
	// claudeTools are the tools whose use polix governs in Claude Code.
	claudeTools = "Edit|Write|MultiEdit|NotebookEdit|Bash"
)

// UpdateCommand is the session start command that keeps polix up to date and reports the problems of the machine:
// polix update --hook ends with the doctor of the binary it leaves. It is one plain command, with no shell syntax,
// because it is committed in the hooks files of the repositories, which every release, and every shell an agent
// runs hooks with, must understand; what it does lives in the binary.
const UpdateCommand = "polix update --hook"

// legacyCommands are the commands earlier builds registered, recognized so install replaces them.
var legacyCommands = []string{"polix update --hook; polix doctor --hook"}

// hookEvents are the events polix registers, in lifecycle order: the order new events are added to a file.
var hookEvents = []string{"SessionStart", "UserPromptSubmit", "PreToolUse", "PostToolUse", "PostToolUseFailure",
	"SubagentStop", "Stop"}

// claudeHookEvents are the events polix needs in Claude Code, in lifecycle order, with the tools the matcher of
// the tool events must cover.
var claudeHookEvents = []struct {
	name  string
	tools []string
}{
	{"SessionStart", nil},
	{"UserPromptSubmit", nil},
	{"PreToolUse", []string{"Edit", "Write", "MultiEdit", "NotebookEdit", "Bash"}},
	{"PostToolUse", []string{"Edit", "Write", "MultiEdit", "NotebookEdit", "Bash"}},
	{"PostToolUseFailure", []string{"Edit", "Write", "MultiEdit", "NotebookEdit", "Bash"}},
	{"SubagentStop", nil},
	{"Stop", nil},
}

// hooks returns the hooks polix registers for an agent, by event.
func hooks(agent string) map[string][]hookGroup {
	call := hookHandler{Type: "command", Command: "polix hook " + agent, Timeout: HookTimeout}
	one := func(matcher string) []hookGroup { return []hookGroup{{Matcher: matcher, Hooks: []hookHandler{call}}} }
	tools := claudeTools
	if agent == Codex {
		tools = "" // Codex names its tools differently (shell, apply_patch...): polix reads every call
	}
	out := map[string][]hookGroup{
		"SessionStart": append(one(""), hookGroup{Matcher: "startup",
			Hooks: []hookHandler{{Type: "command", Command: UpdateCommand, Timeout: updateTimeout}}}),
		"UserPromptSubmit": one(""),
		"PreToolUse":       one(tools),
		"PostToolUse":      one(tools),
		"SubagentStop":     one(""),
		"Stop":             one(""),
	}
	if agent == Claude {
		// A failed command reaches post_command with its exit code; the adapter ignores failed edits.
		out["PostToolUseFailure"] = one(tools)
	}
	return out
}

// IsPolixCommand reports whether a hook command is one polix registers.
func IsPolixCommand(command string) bool {
	c := strings.TrimSpace(command)
	return strings.HasPrefix(c, "polix hook ") || IsUpdateCommand(c)
}

// IsUpdateCommand reports whether a hook command is the session start command of polix, of this release, of an
// earlier one or of a later one that adds flags: install replaces it instead of adding a second update next to it.
func IsUpdateCommand(command string) bool {
	c := strings.TrimSpace(command)
	return c == UpdateCommand || strings.HasPrefix(c, UpdateCommand+" ") || strings.HasPrefix(c, UpdateCommand+";")
}

// IsLegacyCommand reports whether a hook command is one an earlier build registered and install now replaces.
func IsLegacyCommand(command string) bool {
	return slices.Contains(legacyCommands, strings.TrimSpace(command))
}

// readHooks returns the hooks of a settings file of Claude Code or a hooks file of Codex, by event; an empty
// file has none.
func readHooks(data []byte) (map[string][]hookGroup, error) {
	var doc hooksFileJSON
	if len(bytes.TrimSpace(data)) > 0 {
		if err := json.Unmarshal(data, &doc); err != nil {
			return nil, err
		}
	}
	return doc.Hooks, nil
}

// HooksError returns why the content of a hooks file cannot be read as one, or nil.
func HooksError(data []byte) error {
	_, err := readHooks(data)
	return err
}

// Registered reports whether the content of a hooks file registers any hook of polix.
func Registered(data []byte) bool {
	h, _ := readHooks(data)
	for _, groups := range h {
		for _, g := range groups {
			for _, c := range g.Hooks {
				if IsPolixCommand(c.Command) {
					return true
				}
			}
		}
	}
	return false
}
