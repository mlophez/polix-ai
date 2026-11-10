package diagnose

import (
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"strings"

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
// the PATH the hooks use, git, and inside a repository the workflow, the programs it requires on the PATH, the rules
// and their checks, the hooks of Claude Code, the plugin, Codex when polix is registered for it, and the state;
// outside one, or without git, only the binary and the plugin. With hook (the session start hook) Claude Code is
// not asked for its plugins.
func Decide(i setup.Inspection, hook bool) Output {
	f := binary(i)
	switch {
	case i.GitErr != nil:
		f = append(f, found(setup.LevelFail, "git", "git is not on the PATH: polix reads every fact of the repository through it")...)
		f = append(f, plugin(i, hook)...)
	case i.RepoErr != nil:
		f = append(f, found(setup.LevelOK, "repo", "not inside a git repository: only the checks of this machine apply")...)
		f = append(f, plugin(i, hook)...)
	default:
		f = append(f, found(setup.LevelOK, "git", "repository at %s", i.Setup.Root)...)
		f = append(f, workflowFile(i)...)
		f = append(f, requirements(i)...)
		f = append(f, rules(i)...)
		f = append(f, hooks(i)...)
		f = append(f, plugin(i, hook)...)
		f = append(f, codex(i)...)
		f = append(f, stateFolder(i)...)
	}
	return Output{Findings: f, Failed: slices.ContainsFunc(f, setup.Finding.Failure)}
}

// found returns the finding of level about area, with the message of format and args; none when it is not valid.
func found(level, area, format string, args ...any) []setup.Finding {
	f := setup.Finding{Level: level, Area: area, Message: fmt.Sprintf(format, args...)}
	if err := f.Valid(); err != nil {
		return nil
	}
	return []setup.Finding{f}
}

// binary checks the version against the latest release and that the PATH, which the hooks use, runs
// this binary.
func binary(i setup.Inspection) []setup.Finding {
	if i.ExeErr != nil {
		return found(setup.LevelFail, "binary", "cannot find the running binary: %v", i.ExeErr)
	}
	var f []setup.Finding
	if i.LatestErr != nil {
		f = append(f, found(setup.LevelWarn, "version", "cannot reach the releases: %v", i.LatestErr)...)
	}
	running := i.Setup.Version
	switch {
	case !semver.IsRelease(running):
		f = append(f, found(setup.LevelWarn, "version", "polix %s is a development build: install a release with polix update --version <version>", running)...)
	case i.Latest == "":
		f = append(f, found(setup.LevelOK, "version", "polix %s (latest release unknown)", running)...)
	case semver.Compare(running, i.Latest) < 0:
		f = append(f, found(setup.LevelWarn, "version", "polix %s, the latest release is %s: run polix update", running, i.Latest)...)
	default:
		f = append(f, found(setup.LevelOK, "version", "polix %s, the latest release", running)...)
	}
	switch {
	case i.PathErr != nil:
		f = append(f, found(setup.LevelFail, "path", "polix is not on the PATH, so the hooks cannot run it: add %s to the PATH", filepath.Dir(i.Exe))...)
	case !i.SamePath:
		f = append(f, found(setup.LevelWarn, "path", "the PATH runs another polix (%s), not this one (%s)", i.Path, i.Exe)...)
	default:
		f = append(f, found(setup.LevelOK, "path", "the hooks run %s", i.Exe)...)
	}
	return f
}

// workflowFile checks polix.yaml, and that rules check --changed finds a base branch.
func workflowFile(i setup.Inspection) []setup.Finding {
	var f []setup.Finding
	base := func() {
		if msg := i.BaseWarning(); i.RulesErr == nil && msg != "" {
			f = append(f, found(setup.LevelWarn, "workflow", "%s", msg)...)
		}
	}
	problems, newer := i.WorkflowProblems()
	if !i.Found && i.WorkflowErr == nil {
		f = append(f, found(setup.LevelWarn, "workflow", "no %s: only the built-in protections apply", workflow.File)...)
		base()
		return f
	}
	for _, p := range problems {
		f = append(f, found(setup.LevelFail, "workflow", "%s", p)...)
	}
	switch {
	case len(problems) == 0:
		f = append(f, found(setup.LevelOK, "workflow", "%s is valid", workflow.File)...)
		base()
	case newer:
		f = append(f, found(setup.LevelWarn, "workflow", "%s", i.NewerHint())...)
	}
	return f
}

// requirements checks that the PATH runs every program the workflow requires, git included (workflow.Programs): a
// warning per program it does not run, each once and in the order of Programs, or one OK line when it runs them all;
// nothing without requirements or with a workflow that cannot be used. A missing program never fails the report.
func requirements(i setup.Inspection) []setup.Finding {
	if i.WorkflowErr != nil || len(i.Workflow.Requirements) == 0 {
		return nil
	}
	var f []setup.Finding
	seen := map[string]bool{}
	for _, p := range i.Workflow.Programs() {
		if seen[p] {
			continue
		}
		seen[p] = true
		if i.Requirements[p] != nil {
			f = append(f, found(setup.LevelWarn, "requirements", "%s is not on the PATH: %s requires it", p, workflow.File)...)
		}
	}
	if len(f) == 0 {
		return found(setup.LevelOK, "requirements", "%d programs of requirements on the PATH", len(seen))
	}
	return f
}

// rules checks the rules and their checks: every problem of polix config check, and the checks that cannot
// run as programs, unless a runner of rules.run runs them.
func rules(i setup.Inspection) []setup.Finding {
	if i.RulesErr != nil {
		return found(setup.LevelFail, "rules", "%v", i.RulesErr)
	}
	problems := append(i.RuleProblems(), runProblems(i)...)
	if len(problems) == 0 {
		return found(setup.LevelOK, "rules", "%d rules of %s/", len(i.Rules), i.Workflow.RulesPath())
	}
	var f []setup.Finding
	for _, p := range problems {
		f = append(f, found(setup.LevelFail, "rules", "%s", p)...)
	}
	return f
}

// runProblems returns the checks that cannot run as programs (Runnable, by check: not executable, or on Windows
// without an interpreter), each problem once. A check that a runner of rules.run of the workflow runs is never one
// of them: its runner runs it.
func runProblems(i setup.Inspection) []string {
	var problems []string
	seen := map[string]bool{}
	for _, r := range i.Rules {
		err := i.Runnable[r.Check]
		if !r.HasCheck() || err == nil || i.Workflow.CheckCommand(r.Check, r.ID) != nil || seen[err.Error()] {
			continue
		}
		seen[err.Error()] = true
		problems = append(problems, fmt.Sprintf("%s %s: %v", r.File, r.ID, err))
	}
	return problems
}

// plugin checks that the plugin polix is written for the user by this binary and, unless hook, that
// Claude Code has it installed and enabled.
func plugin(i setup.Inspection, hook bool) []setup.Finding {
	s, p := i.Setup, i.Plugin
	if s.Home == "" {
		return nil
	}
	switch {
	case !p.Written:
		return found(setup.LevelWarn, "plugin", "the plugin polix (skills and agents) is not installed: run polix install --user")
	case p.Manifest != p.ManifestVersion():
		return found(setup.LevelWarn, "plugin", "%s is from polix %s, this binary is %s: run polix install --user", s.PluginDir(), p.Manifest, p.Version)
	}
	if !hook {
		if f := claude(i); f != nil {
			return f
		}
	}
	return found(setup.LevelOK, "plugin", "%s %s: skills %s, agents %s", setup.PluginID, p.Manifest,
		strings.Join(prefixed("/polix:", p.Skills()), " "),
		strings.Join(prefixed("polix:", p.Agents()), " "))
}

// claude checks that Claude Code has the plugin polix installed and enabled, read from the local marketplace;
// nothing when it is, or when its CLI is not on the PATH.
func claude(i setup.Inspection) []setup.Finding {
	switch {
	case errors.Is(i.ClaudeErr, setup.ErrNoCLI):
	case i.ClaudeErr != nil:
		return found(setup.LevelWarn, "plugin", "cannot ask Claude Code for its plugins: %v", i.ClaudeErr)
	case !i.ClaudeInstalled:
		return found(setup.LevelWarn, "plugin", "Claude Code does not have %s installed: run polix install --user", setup.PluginID)
	case !i.ClaudeEnabled:
		return found(setup.LevelWarn, "plugin", "%s is disabled in Claude Code: run claude plugin enable %s", setup.PluginID, setup.PluginID)
	case i.ClaudeFolder != "" && !i.SameFolder:
		return found(setup.LevelWarn, "plugin", "Claude Code loads %s from %s, not from %s: run polix install --user",
			setup.PluginID, i.ClaudeFolder, i.Setup.PluginDir())
	}
	return nil
}

// stateFolder checks that the state of polix can be written.
func stateFolder(i setup.Inspection) []setup.Finding {
	if i.StateErr != nil {
		return found(setup.LevelFail, "state", "cannot write the state in %s: %v", i.StateDir, i.StateErr)
	}
	return found(setup.LevelOK, "state", "%s is writable", i.StateDir)
}

// prefixed returns names with prefix before each one.
func prefixed(prefix string, names []string) []string {
	out := make([]string, len(names))
	for i, n := range names {
		out[i] = prefix + n
	}
	return out
}
