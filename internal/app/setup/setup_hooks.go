package setup

import (
	"errors"
	"slices"
	"strings"
	"time"

	"github.com/mlophez/polix-ai/internal/app/hook"
)

// HookGroup is a group of the hooks of an event in a settings file of Claude Code: the matcher of the tools it
// runs on (empty or * for every tool, otherwise a regular expression) and its handlers. It is built with a literal
// followed by Valid.
type HookGroup struct {
	Matcher  string
	Handlers []HookHandler
}

// HookHandler is a command a group of hooks runs: the command, its timeout in seconds, 0 when it sets none and
// the agent applies its default, and its status, the text the agent shows while the hook runs, empty for none. It
// is built with a literal followed by Valid.
type HookHandler struct {
	Command string
	Timeout float64
	Status  string
}

// HooksFile is a hooks file of an agent that exists, as polix doctor read it: its groups of hooks by event, or
// why it cannot be read or is not a hooks file. A handler without a command (a hook of another type, such as a
// prompt) and a group left without handlers are not hooks polix reads, and are left out. It is built with a
// literal followed by Valid.
type HooksFile struct {
	Hooks   map[string][]HookGroup // the groups of hooks of each event
	ReadErr error                  // why the file cannot be read
	Err     error                  // why its content is not a hooks file: not JSON, or its hooks not an object
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

// StatusMessage is the text both agents show next to their spinner while a hook of polix runs, the same on every
// handler polix registers, so the user knows who the wait is on.
const StatusMessage = "polix"

// legacyCommands are the commands earlier builds registered, recognized so install replaces them.
var legacyCommands = []string{"polix update --hook; polix doctor --hook"}

// HookEvents are the events polix registers, in lifecycle order: the order new events are added to a file.
var HookEvents = []string{"SessionStart", "UserPromptSubmit", "SubagentStart", "PreToolUse", "PostToolUse",
	"PostToolUseFailure", "SubagentStop", "Stop"}

// ClaudeHookEvents are the events polix needs in Claude Code, in lifecycle order, with the tools the matcher of
// the tool events must cover.
var ClaudeHookEvents = []struct {
	Name  string
	Tools []string
}{
	{"SessionStart", nil},
	{"UserPromptSubmit", nil},
	{"SubagentStart", nil},
	{"PreToolUse", []string{"Edit", "Write", "MultiEdit", "NotebookEdit", "Bash"}},
	{"PostToolUse", []string{"Edit", "Write", "MultiEdit", "NotebookEdit", "Bash"}},
	{"PostToolUseFailure", []string{"Edit", "Write", "MultiEdit", "NotebookEdit", "Bash"}},
	{"SubagentStop", nil},
	{"Stop", nil},
}

// Valid reports why g is not a group of hooks: it needs a handler, and every handler must be valid.
func (g HookGroup) Valid() error {
	if len(g.Handlers) == 0 {
		return errors.New("a group of hooks needs a handler")
	}
	for _, h := range g.Handlers {
		if err := h.Valid(); err != nil {
			return err
		}
	}
	return nil
}

// Valid reports why h is not a handler of a hook: it needs a command.
func (h HookHandler) Valid() error {
	if strings.TrimSpace(h.Command) == "" {
		return errors.New("a handler of a hook needs a command")
	}
	return nil
}

// Valid reports why f is not a hooks file as read: one that cannot be read or is not a hooks file holds no hooks,
// and every group it holds is valid.
func (f HooksFile) Valid() error {
	if (f.ReadErr != nil || f.Err != nil) && len(f.Hooks) > 0 {
		return errors.New("a hooks file that cannot be read holds no hooks")
	}
	for _, groups := range f.Hooks {
		for _, g := range groups {
			if err := g.Valid(); err != nil {
				return err
			}
		}
	}
	return nil
}

// Hooks returns the hooks polix registers for agent, by event: `polix hook <agent>` on every event, on the tools
// polix governs (every tool on Codex, which names its tools differently), and the session start command of
// UpdateCommand on startup, each with the status StatusMessage.
func (s Setup) Hooks(agent string) map[string][]HookGroup {
	call := HookHandler{Command: "polix hook " + agent, Timeout: HookTimeout, Status: StatusMessage}
	update := HookHandler{Command: UpdateCommand, Timeout: updateTimeout, Status: StatusMessage}
	if call.Valid() != nil || update.Valid() != nil {
		return nil
	}
	tools := claudeTools
	if agent == Codex {
		tools = "" // Codex names its tools differently (shell, apply_patch...): polix reads every call
	}
	out := map[string][]HookGroup{
		"SessionStart":     append(call.Alone(""), update.Alone("startup")...),
		"UserPromptSubmit": call.Alone(""),
		"SubagentStart":    call.Alone(""),
		"PreToolUse":       call.Alone(tools),
		"PostToolUse":      call.Alone(tools),
		"SubagentStop":     call.Alone(""),
		"Stop":             call.Alone(""),
	}
	if agent == Claude {
		// A failed command reaches post_command with its exit code; the adapter ignores failed edits.
		out["PostToolUseFailure"] = call.Alone(tools)
	}
	return out
}

// Alone returns the group of hooks that runs h alone on the tools of matcher.
func (h HookHandler) Alone(matcher string) []HookGroup {
	g := HookGroup{Matcher: matcher, Handlers: []HookHandler{h}}
	if g.Valid() != nil {
		return nil
	}
	return []HookGroup{g}
}

// Polix reports whether the command of h is one polix registers.
func (h HookHandler) Polix() bool {
	return strings.HasPrefix(strings.TrimSpace(h.Command), "polix hook ") || h.Update()
}

// Update reports whether the command of h is the session start command of polix, of this release, of an earlier
// one or of a later one that adds flags: install replaces it instead of adding a second update next to it.
func (h HookHandler) Update() bool {
	c := strings.TrimSpace(h.Command)
	return c == UpdateCommand || strings.HasPrefix(c, UpdateCommand+" ") || strings.HasPrefix(c, UpdateCommand+";")
}

// Legacy reports whether the command of h is one an earlier build registered and install now replaces.
func (h HookHandler) Legacy() bool {
	return slices.Contains(legacyCommands, strings.TrimSpace(h.Command))
}
