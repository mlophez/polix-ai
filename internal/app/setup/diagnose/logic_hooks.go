package diagnose

import (
	"cmp"
	"regexp"
	"slices"
	"strings"

	"github.com/mlophez/polix-ai/internal/app/hook"
	"github.com/mlophez/polix-ai/internal/app/setup"
)

// hooks checks that Claude Code runs `polix hook claude` on every event polix needs, for every tool
// it governs, with a timeout above the run budget of a call, and only once. The hooks come from the settings of
// the project, the local ones and the ones of the user, which Claude Code merges.
func hooks(i setup.Inspection) []setup.Finding {
	f, merged, sources := claudeSettings(i)
	if len(sources) == 0 {
		return append(f, found(setup.LevelFail, "hooks", "Claude Code does not run polix in this repository: run polix install")...)
	}
	f = append(f, twice(merged, sources)...)
	f = append(f, sessionStart(merged)...)
	for _, ev := range setup.ClaudeHookEvents {
		f = append(f, event(ev.Name, ev.Tools, merged[ev.Name])...)
	}
	if slices.ContainsFunc(f, setup.Finding.Problem) {
		return f
	}
	return append(f, found(setup.LevelOK, "hooks", "registered in %s", strings.Join(sources, ", "))...)
}

// claudeSettings merges the hooks of the settings of Claude Code that exist, by event, and returns them with the
// failures of the settings that cannot be read or are not JSON, and the settings that register polix.
func claudeSettings(i setup.Inspection) ([]setup.Finding, map[string][]setup.HookGroup, []string) {
	var f []setup.Finding
	merged := map[string][]setup.HookGroup{}
	var sources []string
	for _, file := range i.Setup.ClaudeSettingsFiles() {
		hf, ok := i.ClaudeHooks[file]
		if !ok {
			continue
		}
		if err := cmp.Or(hf.ReadErr, hf.Err); err != nil {
			f = append(f, found(setup.LevelFail, "hooks", "%s: %v", file, err)...)
			continue
		}
		for ev, groups := range hf.Hooks {
			merged[ev] = append(merged[ev], groups...)
		}
		if registers(hf.Hooks) {
			sources = append(sources, file)
		}
	}
	return f, merged, sources
}

// twice checks that no event runs `polix hook claude` more than once, which settings registered in two of sources
// do.
func twice(merged map[string][]setup.HookGroup, sources []string) []setup.Finding {
	var dup []string
	for _, ev := range setup.ClaudeHookEvents {
		if calls(merged[ev.Name]) > 1 {
			dup = append(dup, ev.Name)
		}
	}
	if len(dup) == 0 {
		return nil
	}
	return found(setup.LevelFail, "hooks", "Claude Code runs polix twice on %s: keep the hooks of polix in one of %s",
		strings.Join(dup, ", "), strings.Join(sources, ", "))
}

// sessionStart checks that SessionStart runs the session start command of polix, and not one of an earlier build.
func sessionStart(merged map[string][]setup.HookGroup) []setup.Finding {
	updates, legacy := false, false
	for _, g := range merged["SessionStart"] {
		for _, h := range g.Handlers {
			legacy = legacy || h.Legacy()
			updates = updates || h.Update() && !h.Legacy()
		}
	}
	switch {
	case legacy:
		return found(setup.LevelWarn, "hooks", "SessionStart runs a command of an earlier polix: run polix install to refresh the hooks")
	case !updates:
		return found(setup.LevelWarn, "hooks", "SessionStart does not run %s, so polix does not update itself: run polix install", setup.UpdateCommand)
	}
	return nil
}

// event checks that the groups of an event run `polix hook claude`, on every one of its tools, with a timeout
// above the run budget of a call. Settings written by an older polix lack PostToolUseFailure: only the feedback of
// failed commands is lost, so it is a warning.
func event(name string, tools []string, groups []setup.HookGroup) []setup.Finding {
	polix, short := claudeCalls(name, groups)
	if len(polix) == 0 {
		level := setup.LevelFail
		if name == "PostToolUseFailure" {
			level = setup.LevelWarn
		}
		return found(level, "hooks", "%s does not run polix hook claude: run polix install", name)
	}
	var f []setup.Finding
	var missing []string
	for _, tool := range tools {
		if !matches(polix, tool) {
			missing = append(missing, tool)
		}
	}
	if len(missing) > 0 {
		f = append(f, found(setup.LevelFail, "hooks", "the matcher of %s does not cover %s: run polix install", name, strings.Join(missing, ", "))...)
	}
	if short {
		f = append(f, found(setup.LevelWarn, "hooks", "%s: the timeout of polix hook claude must be above %.0f seconds, the run budget, "+
			"or Claude Code kills a long gate and lets the action through", name, hook.RunBudget.Seconds())...)
	}
	return f
}

// claudeCalls returns the groups of an event that run `polix hook claude`, once per call, and whether a call has
// a timeout within the run budget of a call, the default of Claude Code when it sets none.
func claudeCalls(name string, groups []setup.HookGroup) ([]setup.HookGroup, bool) {
	var polix []setup.HookGroup
	short := false
	for _, g := range groups {
		for _, h := range g.Handlers {
			if !strings.Contains(h.Command, "polix hook claude") {
				continue
			}
			polix = append(polix, g)
			t := h.Timeout
			if t == 0 {
				t = defaultTimeout(name)
			}
			short = short || t <= hook.RunBudget.Seconds()
		}
	}
	return polix, short
}

// codex checks Codex only when polix is registered for it, as polix install --agent codex leaves it: a
// hooks file of Codex, of the project or the user, runs polix, or the plugin is enabled in the configuration of
// Codex. Then polix must be registered and the plugin enabled; a Codex never set up for polix gets no finding.
func codex(i setup.Inspection) []setup.Finding {
	var f []setup.Finding
	registered := false
	for _, file := range i.Setup.CodexHooksFiles() {
		hf := i.CodexHooks[file]
		switch {
		case hf.ReadErr != nil:
			f = append(f, found(setup.LevelFail, "codex", "%v", hf.ReadErr)...)
		case hf.Err != nil:
			f = append(f, found(setup.LevelFail, "codex", "%s: %v", file, hf.Err)...)
		}
		registered = registered || registers(hf.Hooks)
	}
	if !registered && !i.CodexEnabled {
		return nil
	}
	if !i.CodexEnabled {
		f = append(f, found(setup.LevelWarn, "codex", "the plugin %s is not enabled in Codex: run polix install --user --agent codex", setup.PluginID)...)
	}
	if !registered {
		return append(f, found(setup.LevelWarn, "codex", "Codex does not run polix in this repository: run polix install --agent codex")...)
	}
	return append(f, found(setup.LevelOK, "codex", "hooks registered (Codex runs them once trusted with /hooks); "+
		"Codex support is not implemented and untested")...)
}

// registers reports whether any group of hooks of a file, by event, runs a hook command of polix.
func registers(hooks map[string][]setup.HookGroup) bool {
	for _, groups := range hooks {
		for _, g := range groups {
			if slices.ContainsFunc(g.Handlers, setup.HookHandler.Polix) {
				return true
			}
		}
	}
	return false
}

// calls counts the calls to `polix hook claude` of the groups of an event.
func calls(groups []setup.HookGroup) int {
	n := 0
	for _, g := range groups {
		for _, h := range g.Handlers {
			if strings.Contains(h.Command, "polix hook claude") {
				n++
			}
		}
	}
	return n
}

// defaultTimeout is the timeout, in seconds, Claude Code gives a command hook that does not set one:
// 600, lowered to 30 on UserPromptSubmit.
func defaultTimeout(event string) float64 {
	if event == "UserPromptSubmit" {
		return 30
	}
	return 600
}

// matches reports whether the matcher of a group covers tool; Claude Code reads an empty matcher or *
// as every tool, and any other as a regular expression.
func matches(groups []setup.HookGroup, tool string) bool {
	for _, g := range groups {
		m := g.Matcher
		if m == "" || m == "*" {
			return true
		}
		if re, err := regexp.Compile("^(?:" + m + ")$"); err == nil && re.MatchString(tool) || m == tool {
			return true
		}
	}
	return false
}
