package diagnose

import (
	"errors"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/mlophez/polix-ai/internal/app/hook"
	"github.com/mlophez/polix-ai/internal/app/setup"
	"github.com/mlophez/polix-ai/internal/app/workflow"
	"github.com/mlophez/polix-ai/internal/platform/semver"
)

// Output is the decision of the logic of DiagnoseSetup: the findings of the report, in order.
type Output struct {
	Findings []setup.Finding
	Failed   bool // whether a finding is a failure
}

// Decide turns what polix doctor observed (i) into the findings of its report: the binary and its version,
// the PATH the hooks use, git, and inside a repository the workflow, the rules and their checks, the hooks of
// Claude Code, the plugin, Codex when polix is registered for it, and the state; outside one, or without git, only
// the binary and the plugin. With hook (the session start hook) Claude Code is not asked for its plugins.
func Decide(i setup.Inspection, hook bool) Output {
	f := binary(i)
	switch {
	case i.GitErr != nil:
		f = append(f, setup.Found(setup.LevelFail, "git", "git is not on the PATH: polix reads every fact of the repository through it")...)
		f = append(f, plugin(i, hook)...)
	case i.RepoErr != nil:
		f = append(f, setup.Found(setup.LevelOK, "repo", "not inside a git repository: only the checks of this machine apply")...)
		f = append(f, plugin(i, hook)...)
	default:
		f = append(f, setup.Found(setup.LevelOK, "git", "repository at %s", i.Setup.Root)...)
		f = append(f, workflowFile(i)...)
		f = append(f, rules(i)...)
		f = append(f, hooks(i)...)
		f = append(f, plugin(i, hook)...)
		f = append(f, codex(i)...)
		if i.StateErr != nil {
			f = append(f, setup.Found(setup.LevelFail, "state", "cannot write the state in %s: %v", i.StateDir, i.StateErr)...)
		} else {
			f = append(f, setup.Found(setup.LevelOK, "state", "%s is writable", i.StateDir)...)
		}
	}
	return Output{Findings: f, Failed: setup.Failed(f)}
}

// binary checks the version against the latest release and that the PATH, which the hooks use, runs
// this binary.
func binary(i setup.Inspection) []setup.Finding {
	if i.ExeErr != nil {
		return setup.Found(setup.LevelFail, "binary", "cannot find the running binary: %v", i.ExeErr)
	}
	var f []setup.Finding
	if i.LatestErr != nil {
		f = append(f, setup.Found(setup.LevelWarn, "version", "cannot reach the releases: %v", i.LatestErr)...)
	}
	running := i.Setup.Version
	switch {
	case !semver.IsRelease(running):
		f = append(f, setup.Found(setup.LevelWarn, "version", "polix %s is a development build: install a release with polix update --version <version>", running)...)
	case i.Latest == "":
		f = append(f, setup.Found(setup.LevelOK, "version", "polix %s (latest release unknown)", running)...)
	case semver.Compare(running, i.Latest) < 0:
		f = append(f, setup.Found(setup.LevelWarn, "version", "polix %s, the latest release is %s: run polix update", running, i.Latest)...)
	default:
		f = append(f, setup.Found(setup.LevelOK, "version", "polix %s, the latest release", running)...)
	}
	switch {
	case i.PathErr != nil:
		f = append(f, setup.Found(setup.LevelFail, "path", "polix is not on the PATH, so the hooks cannot run it: add %s to the PATH", filepath.Dir(i.Exe))...)
	case !i.SamePath:
		f = append(f, setup.Found(setup.LevelWarn, "path", "the PATH runs another polix (%s), not this one (%s)", i.Path, i.Exe)...)
	default:
		f = append(f, setup.Found(setup.LevelOK, "path", "the hooks run %s", i.Exe)...)
	}
	return f
}

// workflowFile checks polix.yaml, and that rules check --changed finds a base branch.
func workflowFile(i setup.Inspection) []setup.Finding {
	var f []setup.Finding
	base := func() {
		if msg := setup.BaseWarning(i.Rules, i.BaseErr); i.RulesErr == nil && msg != "" {
			f = append(f, setup.Found(setup.LevelWarn, "workflow", "%s", msg)...)
		}
	}
	problems, newer := setup.WorkflowProblems(i.WorkflowErr)
	if !i.Found && i.WorkflowErr == nil {
		f = append(f, setup.Found(setup.LevelWarn, "workflow", "no %s: only the built-in protections apply", workflow.File)...)
		base()
		return f
	}
	for _, p := range problems {
		f = append(f, setup.Found(setup.LevelFail, "workflow", "%s", p)...)
	}
	switch {
	case len(problems) == 0:
		f = append(f, setup.Found(setup.LevelOK, "workflow", "%s is valid", workflow.File)...)
		base()
	case newer:
		f = append(f, setup.Found(setup.LevelWarn, "workflow", "%s", setup.NewerHint(i.Setup.Version))...)
	}
	return f
}

// rules checks the rules and their checks: every problem of polix config check, and the checks that cannot
// run as programs, unless a runner of rules.run runs them.
func rules(i setup.Inspection) []setup.Finding {
	if i.RulesErr != nil {
		return setup.Found(setup.LevelFail, "rules", "%v", i.RulesErr)
	}
	problems := append(setup.RuleProblems(i.Rules, i.Invalid, i.Missing), setup.RunProblems(i.Rules, i.Workflow, i.Runnable)...)
	if len(problems) == 0 {
		return setup.Found(setup.LevelOK, "rules", "%d rules of %s/", len(i.Rules), i.Workflow.RulesPath())
	}
	var f []setup.Finding
	for _, p := range problems {
		f = append(f, setup.Found(setup.LevelFail, "rules", "%s", p)...)
	}
	return f
}

// hooks checks that Claude Code runs `polix hook claude` on every event polix needs, for every tool
// it governs, with a timeout above the run budget of a call, and only once. The hooks come from the settings of
// the project, the local ones and the ones of the user, which Claude Code merges.
func hooks(i setup.Inspection) []setup.Finding {
	var f []setup.Finding
	merged := map[string][]setup.HookGroup{}
	var sources []string
	for _, file := range i.Setup.ClaudeSettingsFiles() {
		data, ok := i.Files[file]
		if err := i.FileErrs[file]; err != nil {
			f = append(f, setup.Found(setup.LevelFail, "hooks", "%s: %v", file, err)...)
			continue
		}
		if !ok {
			continue
		}
		parsed, err := setup.ParseHooks(data)
		if err != nil {
			f = append(f, setup.Found(setup.LevelFail, "hooks", "%s: %v", file, err)...)
			continue
		}
		registers := false
		for ev, groups := range parsed {
			registers = registers || polixGroups(groups)
			merged[ev] = append(merged[ev], groups...)
		}
		if registers {
			sources = append(sources, file)
		}
	}
	if len(sources) == 0 {
		return append(f, setup.Found(setup.LevelFail, "hooks", "Claude Code does not run polix in this repository: run polix install")...)
	}
	var dup []string
	for _, ev := range setup.ClaudeHookEvents {
		if calls(merged[ev.Name]) > 1 {
			dup = append(dup, ev.Name)
		}
	}
	if len(dup) > 0 {
		f = append(f, setup.Found(setup.LevelFail, "hooks", "Claude Code runs polix twice on %s: keep the hooks of polix in one of %s",
			strings.Join(dup, ", "), strings.Join(sources, ", "))...)
	}
	updates, legacy := false, false
	for _, g := range merged["SessionStart"] {
		for _, h := range g.Handlers {
			legacy = legacy || setup.IsLegacyCommand(h.Command)
			updates = updates || setup.IsUpdateCommand(h.Command) && !setup.IsLegacyCommand(h.Command)
		}
	}
	switch {
	case legacy:
		f = append(f, setup.Found(setup.LevelWarn, "hooks", "SessionStart runs a command of an earlier polix: run polix install to refresh the hooks")...)
	case !updates:
		f = append(f, setup.Found(setup.LevelWarn, "hooks", "SessionStart does not run %s, so polix does not update itself: run polix install", setup.UpdateCommand)...)
	}
	for _, ev := range setup.ClaudeHookEvents {
		var polix []setup.HookGroup
		short := false
		for _, g := range merged[ev.Name] {
			for _, h := range g.Handlers {
				if strings.Contains(h.Command, "polix hook claude") {
					polix = append(polix, g)
					t := h.Timeout
					if t == 0 {
						t = defaultTimeout(ev.Name)
					}
					short = short || t <= hook.RunBudget.Seconds()
				}
			}
		}
		if len(polix) == 0 {
			// Settings written by an older polix lack the event: only the feedback of failed commands is lost.
			level := setup.LevelFail
			if ev.Name == "PostToolUseFailure" {
				level = setup.LevelWarn
			}
			f = append(f, setup.Found(level, "hooks", "%s does not run polix hook claude: run polix install", ev.Name)...)
			continue
		}
		var missing []string
		for _, tool := range ev.Tools {
			if !matches(polix, tool) {
				missing = append(missing, tool)
			}
		}
		if len(missing) > 0 {
			f = append(f, setup.Found(setup.LevelFail, "hooks", "the matcher of %s does not cover %s: run polix install", ev.Name, strings.Join(missing, ", "))...)
		}
		if short {
			f = append(f, setup.Found(setup.LevelWarn, "hooks", "%s: the timeout of polix hook claude must be above %.0f seconds, the run budget, "+
				"or Claude Code kills a long gate and lets the action through", ev.Name, hook.RunBudget.Seconds())...)
		}
	}
	for _, x := range f {
		if x.Problem() {
			return f
		}
	}
	return append(f, setup.Found(setup.LevelOK, "hooks", "registered in %s", strings.Join(sources, ", "))...)
}

// polixGroups reports whether a group runs a hook command of polix.
func polixGroups(groups []setup.HookGroup) bool {
	for _, g := range groups {
		for _, h := range g.Handlers {
			if setup.IsPolixCommand(h.Command) {
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

// plugin checks that the plugin polix is written for the user by this binary and, unless hook, that
// Claude Code has it installed and enabled.
func plugin(i setup.Inspection, hook bool) []setup.Finding {
	s := i.Setup
	if s.Home == "" {
		return nil
	}
	v, ok := setup.PluginVersion(i.Files[s.PluginManifest()])
	switch {
	case !ok:
		return setup.Found(setup.LevelWarn, "plugin", "the plugin polix (skills and agents) is not installed: run polix install --user")
	case v != setup.ManifestVersion(i.Embedded, s.Version):
		return setup.Found(setup.LevelWarn, "plugin", "%s is from polix %s, this binary is %s: run polix install --user", s.PluginDir(), v, s.Version)
	}
	if !hook {
		installed, enabled, folder, err := setup.ClaudePlugin(i.Claude)
		if i.ClaudeErr != nil {
			err = i.ClaudeErr
		}
		switch {
		case errors.Is(err, setup.ErrNoCLI):
		case err != nil:
			return setup.Found(setup.LevelWarn, "plugin", "cannot ask Claude Code for its plugins: %v", err)
		case !installed:
			return setup.Found(setup.LevelWarn, "plugin", "Claude Code does not have %s installed: run polix install --user", setup.PluginID)
		case !enabled:
			return setup.Found(setup.LevelWarn, "plugin", "%s is disabled in Claude Code: run claude plugin enable %s", setup.PluginID, setup.PluginID)
		case folder != "" && !i.SameFolder:
			return setup.Found(setup.LevelWarn, "plugin", "Claude Code loads %s from %s, not from %s: run polix install --user",
				setup.PluginID, folder, s.PluginDir())
		}
	}
	return setup.Found(setup.LevelOK, "plugin", "%s %s: skills %s, agents %s", setup.PluginID, v,
		strings.Join(prefixed("/polix:", setup.PluginSkills(i.Embedded)), " "),
		strings.Join(prefixed("polix:", setup.PluginAgents(i.Embedded)), " "))
}

// codex checks Codex only when polix is registered for it, as polix install --agent codex leaves it: a
// hooks file of Codex, of the project or the user, runs polix, or the plugin is enabled in the configuration of
// Codex. Then polix must be registered and the plugin enabled; a Codex never set up for polix gets no finding.
func codex(i setup.Inspection) []setup.Finding {
	enabled := setup.CodexEnabled(i.Files[i.Setup.CodexConfigFile()])
	var f []setup.Finding
	registered := false
	for _, file := range i.Setup.CodexHooksFiles() {
		err := i.FileErrs[file]
		if data, ok := i.Files[file]; ok && err == nil {
			if err = setup.HooksError(data); err != nil {
				err = errors.New(file + ": " + err.Error())
			}
			registered = registered || setup.Registered(data)
		}
		if err != nil {
			f = append(f, setup.Found(setup.LevelFail, "codex", "%v", err)...)
		}
	}
	if !registered && !enabled {
		return nil
	}
	if !enabled {
		f = append(f, setup.Found(setup.LevelWarn, "codex", "the plugin %s is not enabled in Codex: run polix install --user --agent codex", setup.PluginID)...)
	}
	if !registered {
		return append(f, setup.Found(setup.LevelWarn, "codex", "Codex does not run polix in this repository: run polix install --agent codex")...)
	}
	return append(f, setup.Found(setup.LevelOK, "codex", "hooks registered (Codex runs them once trusted with /hooks); "+
		"Codex support is not implemented and untested")...)
}

// prefixed returns names with prefix before each one.
func prefixed(prefix string, names []string) []string {
	out := make([]string, len(names))
	for i, n := range names {
		out[i] = prefix + n
	}
	return out
}
