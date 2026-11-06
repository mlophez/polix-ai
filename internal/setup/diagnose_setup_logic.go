package setup

import (
	"encoding/json"
	"errors"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/mlophez/polix-ai/internal/hook"
	"github.com/mlophez/polix-ai/internal/workflow"
	"github.com/mlophez/polix-ai/pkg/semver"
)

// DiagnoseSetupOutput is the decision of the logic of DiagnoseSetup: the findings of the report, in order.
type DiagnoseSetupOutput struct {
	Findings []Finding
	Failed   bool // whether a finding is a failure
}

// DiagnoseSetup turns what polix doctor observed (i) into the findings of its report: the binary and its version,
// the PATH the hooks use, git, and inside a repository the workflow, the rules and their checks, the hooks of
// Claude Code, the plugin, Codex and the state; outside one, or without git, only the binary and the plugin. With
// hook (the session start hook) Claude Code is not asked for its plugins.
func DiagnoseSetup(i Inspection, hook bool) DiagnoseSetupOutput {
	f := diagnoseSetupBinary(i)
	switch {
	case i.GitErr != nil:
		f = append(f, found(LevelFail, "git", "git is not on the PATH: polix reads every fact of the repository through it")...)
		f = append(f, diagnoseSetupPlugin(i, hook)...)
	case i.RepoErr != nil:
		f = append(f, found(LevelOK, "repo", "not inside a git repository: only the checks of this machine apply")...)
		f = append(f, diagnoseSetupPlugin(i, hook)...)
	default:
		f = append(f, found(LevelOK, "git", "repository at %s", i.Setup.Root)...)
		f = append(f, diagnoseSetupWorkflow(i)...)
		f = append(f, diagnoseSetupRules(i)...)
		f = append(f, diagnoseSetupHooks(i)...)
		f = append(f, diagnoseSetupPlugin(i, hook)...)
		f = append(f, diagnoseSetupCodex(i)...)
		if i.StateErr != nil {
			f = append(f, found(LevelFail, "state", "cannot write the state in %s: %v", i.StateDir, i.StateErr)...)
		} else {
			f = append(f, found(LevelOK, "state", "%s is writable", i.StateDir)...)
		}
	}
	return DiagnoseSetupOutput{Findings: f, Failed: Failed(f)}
}

// diagnoseSetupBinary checks the version against the latest release and that the PATH, which the hooks use, runs
// this binary.
func diagnoseSetupBinary(i Inspection) []Finding {
	if i.ExeErr != nil {
		return found(LevelFail, "binary", "cannot find the running binary: %v", i.ExeErr)
	}
	var f []Finding
	if i.LatestErr != nil {
		f = append(f, found(LevelWarn, "version", "cannot reach the releases: %v", i.LatestErr)...)
	}
	running := i.Setup.Version
	switch {
	case !semver.IsRelease(running):
		f = append(f, found(LevelWarn, "version", "polix %s is a development build: install a release with polix update --version <version>", running)...)
	case i.Latest == "":
		f = append(f, found(LevelOK, "version", "polix %s (latest release unknown)", running)...)
	case semver.Compare(running, i.Latest) < 0:
		f = append(f, found(LevelWarn, "version", "polix %s, the latest release is %s: run polix update", running, i.Latest)...)
	default:
		f = append(f, found(LevelOK, "version", "polix %s, the latest release", running)...)
	}
	switch {
	case i.PathErr != nil:
		f = append(f, found(LevelFail, "path", "polix is not on the PATH, so the hooks cannot run it: add %s to the PATH", filepath.Dir(i.Exe))...)
	case !i.SamePath:
		f = append(f, found(LevelWarn, "path", "the PATH runs another polix (%s), not this one (%s)", i.Path, i.Exe)...)
	default:
		f = append(f, found(LevelOK, "path", "the hooks run %s", i.Exe)...)
	}
	return f
}

// diagnoseSetupWorkflow checks polix.yaml, and that rules check --changed finds a base branch.
func diagnoseSetupWorkflow(i Inspection) []Finding {
	var f []Finding
	base := func() {
		if msg := BaseWarning(i.Rules, i.BaseErr); i.RulesErr == nil && msg != "" {
			f = append(f, found(LevelWarn, "workflow", "%s", msg)...)
		}
	}
	problems, newer := WorkflowProblems(i.WorkflowErr)
	if !i.Found && i.WorkflowErr == nil {
		f = append(f, found(LevelWarn, "workflow", "no %s: only the built-in protections apply", workflow.File)...)
		base()
		return f
	}
	for _, p := range problems {
		f = append(f, found(LevelFail, "workflow", "%s", p)...)
	}
	switch {
	case len(problems) == 0:
		f = append(f, found(LevelOK, "workflow", "%s is valid", workflow.File)...)
		base()
	case newer:
		f = append(f, found(LevelWarn, "workflow", "%s", NewerHint(i.Setup.Version))...)
	}
	return f
}

// diagnoseSetupRules checks the rules and their checks: every problem of polix check, and the checks that cannot
// run as programs, unless a runner of rules.run runs them.
func diagnoseSetupRules(i Inspection) []Finding {
	if i.RulesErr != nil {
		return found(LevelFail, "rules", "%v", i.RulesErr)
	}
	problems := append(RuleProblems(i.Rules, i.Invalid, i.Missing), RunProblems(i.Rules, i.Workflow, i.Runnable)...)
	if len(problems) == 0 {
		return found(LevelOK, "rules", "%d rules of %s/", len(i.Rules), i.Workflow.RulesPath())
	}
	var f []Finding
	for _, p := range problems {
		f = append(f, found(LevelFail, "rules", "%s", p)...)
	}
	return f
}

// diagnoseSetupHooks checks that Claude Code runs `polix hook claude` on every event polix needs, for every tool
// it governs, with a timeout above the run budget of a call, and only once. The hooks come from the settings of
// the project, the local ones and the ones of the user, which Claude Code merges.
func diagnoseSetupHooks(i Inspection) []Finding {
	var f []Finding
	merged := map[string][]hookGroup{}
	var sources []string
	for _, file := range i.Setup.ClaudeSettingsFiles() {
		data, ok := i.Files[file]
		if err := i.FileErrs[file]; err != nil {
			f = append(f, found(LevelFail, "hooks", "%s: %v", file, err)...)
			continue
		}
		if !ok {
			continue
		}
		var s hooksFileJSON
		if err := json.Unmarshal(data, &s); err != nil {
			f = append(f, found(LevelFail, "hooks", "%s: %v", file, err)...)
			continue
		}
		registers := false
		for ev, groups := range s.Hooks {
			registers = registers || diagnoseSetupPolixGroups(groups)
			merged[ev] = append(merged[ev], groups...)
		}
		if registers {
			sources = append(sources, file)
		}
	}
	if len(sources) == 0 {
		return append(f, found(LevelFail, "hooks", "Claude Code does not run polix in this repository: run polix install")...)
	}
	var dup []string
	for _, ev := range claudeHookEvents {
		if diagnoseSetupCalls(merged[ev.name]) > 1 {
			dup = append(dup, ev.name)
		}
	}
	if len(dup) > 0 {
		f = append(f, found(LevelFail, "hooks", "Claude Code runs polix twice on %s: keep the hooks of polix in one of %s",
			strings.Join(dup, ", "), strings.Join(sources, ", "))...)
	}
	updates, legacy := false, false
	for _, g := range merged["SessionStart"] {
		for _, h := range g.Hooks {
			legacy = legacy || IsLegacyCommand(h.Command)
			updates = updates || IsUpdateCommand(h.Command) && !IsLegacyCommand(h.Command)
		}
	}
	switch {
	case legacy:
		f = append(f, found(LevelWarn, "hooks", "SessionStart runs a command of an earlier polix: run polix install to refresh the hooks")...)
	case !updates:
		f = append(f, found(LevelWarn, "hooks", "SessionStart does not run %s, so polix does not update itself: run polix install", UpdateCommand)...)
	}
	for _, ev := range claudeHookEvents {
		var polix []hookGroup
		short := false
		for _, g := range merged[ev.name] {
			for _, h := range g.Hooks {
				if strings.Contains(h.Command, "polix hook claude") {
					polix = append(polix, g)
					t := h.Timeout
					if t == 0 {
						t = diagnoseSetupDefaultTimeout(ev.name)
					}
					short = short || t <= hook.RunBudget.Seconds()
				}
			}
		}
		if len(polix) == 0 {
			// Settings written by an older polix lack the event: only the feedback of failed commands is lost.
			level := LevelFail
			if ev.name == "PostToolUseFailure" {
				level = LevelWarn
			}
			f = append(f, found(level, "hooks", "%s does not run polix hook claude: run polix install", ev.name)...)
			continue
		}
		var missing []string
		for _, tool := range ev.tools {
			if !diagnoseSetupMatches(polix, tool) {
				missing = append(missing, tool)
			}
		}
		if len(missing) > 0 {
			f = append(f, found(LevelFail, "hooks", "the matcher of %s does not cover %s: run polix install", ev.name, strings.Join(missing, ", "))...)
		}
		if short {
			f = append(f, found(LevelWarn, "hooks", "%s: the timeout of polix hook claude must be above %.0f seconds, the run budget, "+
				"or Claude Code kills a long gate and lets the action through", ev.name, hook.RunBudget.Seconds())...)
		}
	}
	for _, x := range f {
		if x.Problem() {
			return f
		}
	}
	return append(f, found(LevelOK, "hooks", "registered in %s", strings.Join(sources, ", "))...)
}

// diagnoseSetupPolixGroups reports whether a group runs a hook command of polix.
func diagnoseSetupPolixGroups(groups []hookGroup) bool {
	for _, g := range groups {
		for _, h := range g.Hooks {
			if IsPolixCommand(h.Command) {
				return true
			}
		}
	}
	return false
}

// diagnoseSetupCalls counts the calls to `polix hook claude` of the groups of an event.
func diagnoseSetupCalls(groups []hookGroup) int {
	n := 0
	for _, g := range groups {
		for _, h := range g.Hooks {
			if strings.Contains(h.Command, "polix hook claude") {
				n++
			}
		}
	}
	return n
}

// diagnoseSetupDefaultTimeout is the timeout, in seconds, Claude Code gives a command hook that does not set one:
// 600, lowered to 30 on UserPromptSubmit.
func diagnoseSetupDefaultTimeout(event string) float64 {
	if event == "UserPromptSubmit" {
		return 30
	}
	return 600
}

// diagnoseSetupMatches reports whether the matcher of a group covers tool; Claude Code reads an empty matcher or *
// as every tool, and any other as a regular expression.
func diagnoseSetupMatches(groups []hookGroup, tool string) bool {
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

// diagnoseSetupPlugin checks that the plugin polix is written for the user by this binary and, unless hook, that
// Claude Code has it installed and enabled.
func diagnoseSetupPlugin(i Inspection, hook bool) []Finding {
	s := i.Setup
	if s.Home == "" {
		return nil
	}
	v, ok := PluginVersion(i.Files[s.PluginManifest()])
	switch {
	case !ok:
		return found(LevelWarn, "plugin", "the plugin polix (skills and agents) is not installed: run polix install --user")
	case v != ManifestVersion(i.Embedded, s.Version):
		return found(LevelWarn, "plugin", "%s is from polix %s, this binary is %s: run polix install --user", s.PluginDir(), v, s.Version)
	}
	if !hook {
		installed, enabled, folder, err := ClaudePlugin(i.Claude)
		if i.ClaudeErr != nil {
			err = i.ClaudeErr
		}
		switch {
		case errors.Is(err, ErrNoCLI):
		case err != nil:
			return found(LevelWarn, "plugin", "cannot ask Claude Code for its plugins: %v", err)
		case !installed:
			return found(LevelWarn, "plugin", "Claude Code does not have %s installed: run polix install --user", PluginID)
		case !enabled:
			return found(LevelWarn, "plugin", "%s is disabled in Claude Code: run claude plugin enable %s", PluginID, PluginID)
		case folder != "" && !i.SameFolder:
			return found(LevelWarn, "plugin", "Claude Code loads %s from %s, not from %s: run polix install --user",
				PluginID, folder, s.PluginDir())
		}
	}
	return found(LevelOK, "plugin", "%s %s: skills %s, agents %s", PluginID, v,
		strings.Join(diagnoseSetupPrefixed("/polix:", PluginSkills(i.Embedded)), " "),
		strings.Join(diagnoseSetupPrefixed("polix:", PluginAgents(i.Embedded)), " "))
}

// diagnoseSetupCodex checks Codex when it is used on this machine: polix must be registered for the project or the
// user, and the plugin enabled.
func diagnoseSetupCodex(i Inspection) []Finding {
	if !i.CodexUsed {
		return nil
	}
	var f []Finding
	registered := false
	for _, file := range i.Setup.CodexHooksFiles() {
		err := i.FileErrs[file]
		if data, ok := i.Files[file]; ok && err == nil {
			if err = HooksError(data); err != nil {
				err = errors.New(file + ": " + err.Error())
			}
			registered = registered || Registered(data)
		}
		if err != nil {
			f = append(f, found(LevelFail, "codex", "%v", err)...)
		}
	}
	if !CodexEnabled(i.Files[i.Setup.CodexConfigFile()]) {
		f = append(f, found(LevelWarn, "codex", "the plugin %s is not enabled in Codex: run polix install --user --agent codex", PluginID)...)
	}
	if !registered {
		return append(f, found(LevelWarn, "codex", "Codex is used here but does not run polix in this repository: run polix install --agent codex")...)
	}
	return append(f, found(LevelOK, "codex", "hooks registered (Codex runs them once trusted with /hooks)")...)
}

// diagnoseSetupPrefixed returns names with prefix before each one.
func diagnoseSetupPrefixed(prefix string, names []string) []string {
	out := make([]string, len(names))
	for i, n := range names {
		out[i] = prefix + n
	}
	return out
}
