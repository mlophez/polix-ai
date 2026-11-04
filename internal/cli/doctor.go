package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/mlophez/polix-ai/internal/engine"
	"github.com/mlophez/polix-ai/internal/gitx"
	"github.com/mlophez/polix-ai/internal/rules"
	"github.com/mlophez/polix-ai/internal/setup"
	"github.com/mlophez/polix-ai/internal/state"
	"github.com/mlophez/polix-ai/internal/update"
	"github.com/mlophez/polix-ai/internal/version"
	"github.com/mlophez/polix-ai/internal/workflow"
)

// Seams for the tests: where the running binary is and what the PATH resolves.
var (
	executable = update.Executable
	lookPath   = exec.LookPath
)

// Levels of a finding of doctor.
const (
	levelOK   = "ok"
	levelWarn = "warn"
	levelFail = "fail"
)

type finding struct{ level, area, msg string }

type doctorRun struct{ findings []finding }

// problemsSince reports whether a warning or a failure was added after the first n findings.
func (d *doctorRun) problemsSince(n int) bool {
	for _, f := range d.findings[n:] {
		if f.level != levelOK {
			return true
		}
	}
	return false
}

func (d *doctorRun) add(level, area, format string, args ...any) {
	d.findings = append(d.findings, finding{level, area, fmt.Sprintf(format, args...)})
}

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
	{"SubagentStop", nil},
	{"Stop", nil},
}

// defaultHookTimeout is the timeout, in seconds, Claude Code gives a command hook that does not set one: 600,
// lowered to 30 on UserPromptSubmit.
func defaultHookTimeout(event string) float64 {
	if event == "UserPromptSubmit" {
		return 30
	}
	return 600
}

func doctorCmd() *cobra.Command {
	var dir string
	var hook, offline bool
	c := &cobra.Command{
		Use:   "doctor",
		Short: "Check that this machine and repository are ready for polix",
		Long: "Check the binary and its version, the PATH the hooks use, git, the workflow, the rules and their checks, " +
			"the hooks of Claude Code and Codex, the plugin of the user and the state. Exits 1 on any failure.",
		Args: cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			return exit(doctor(dir, hook, offline, c.OutOrStdout()))
		},
	}
	dirFlag(c, &dir, ".")
	c.Flags().BoolVar(&hook, "hook", false, "for a session start hook: quiet when fine, never fails, no network")
	c.Flags().BoolVar(&offline, "offline", false, "do not ask for the latest release")
	return c
}

// doctor checks that this machine and this repository are ready for polix: the binary and its version, the
// PATH the hooks use, git, the workflow, the rules and their checks, the hooks of the agents and the state.
//
// With hook it is meant for the session start hook: it never fails, skips the network (it reads the version
// the last `polix update` saw) and prints nothing when everything is fine; otherwise it prints the problems,
// which Claude Code adds to the context of the session, so the agent can tell the user and offer to fix them.
func doctor(dir string, hook, offline bool, stdout io.Writer) int {
	d := &doctorRun{}
	d.binary(hook || offline)
	home, _ := os.UserHomeDir()
	if _, err := lookPath("git"); err != nil {
		d.add(levelFail, "git", "git is not on the PATH: polix reads every fact of the repository through it")
		d.plugin(home, hook)
	} else if repo, err := gitx.Open(dir); err != nil {
		d.add(levelOK, "repo", "not inside a git repository: only the checks of this machine apply")
		d.plugin(home, hook)
	} else {
		d.add(levelOK, "git", "repository at %s", repo.Root)
		d.workflow(repo)
		d.rules(repo)
		d.hooks(repo.Root, home)
		d.plugin(home, hook)
		d.codex(repo.Root, home)
		d.state(repo)
	}

	if hook {
		var lines []string
		for _, f := range d.findings {
			if f.level != levelOK {
				lines = append(lines, fmt.Sprintf("- [%s] %s: %s", f.level, f.area, f.msg))
			}
		}
		if len(lines) > 0 {
			fmt.Fprintf(stdout, "polix doctor ha encontrado problemas en este equipo o repositorio; cuéntaselos al usuario y "+
				"ofrécete a arreglarlos (polix doctor da el detalle, y la skill polix:doctor sabe cómo):\n%s\n", strings.Join(lines, "\n"))
		}
		return 0
	}
	code := 0
	u := newUI(stdout)
	for _, f := range d.findings {
		u.status(f.level, f.area, f.msg)
		if f.level == levelFail {
			code = 1
		}
	}
	return code
}

// binary checks the version against the latest release and that the PATH, which the hooks use, runs this
// binary.
func (d *doctorRun) binary(noNetwork bool) {
	exe, err := executable()
	if err != nil {
		d.add(levelFail, "binary", "cannot find the running binary: %v", err)
		return
	}
	switch latest := d.latest(noNetwork); {
	case !version.IsRelease(Version):
		d.add(levelWarn, "version", "polix %s is a development build: install a release with polix update --version <version>", Version)
	case latest == "":
		d.add(levelOK, "version", "polix %s (latest release unknown)", Version)
	case version.Compare(Version, latest) < 0:
		d.add(levelWarn, "version", "polix %s, the latest release is %s: run polix update", Version, latest)
	default:
		d.add(levelOK, "version", "polix %s, the latest release", Version)
	}
	found, err := lookPath("polix")
	if err != nil {
		d.add(levelFail, "path", "polix is not on the PATH, so the hooks cannot run it: add %s to the PATH", filepath.Dir(exe))
		return
	}
	if real, err := filepath.EvalSymlinks(found); err == nil {
		found = real
	}
	if !sameFile(found, exe) {
		d.add(levelWarn, "path", "the PATH runs another polix (%s), not this one (%s)", found, exe)
		return
	}
	d.add(levelOK, "path", "the hooks run %s", exe)
}

// latest returns the latest release: asked to the releases page, or the one the last check saw.
func (d *doctorRun) latest(noNetwork bool) string {
	if noNetwork || !version.IsRelease(Version) {
		return update.LoadCache().Latest
	}
	ctx, cancel := context.WithTimeout(context.Background(), hookLatestTimeout)
	defer cancel()
	latest, err := update.New().Latest(ctx)
	if err != nil {
		d.add(levelWarn, "version", "cannot reach the releases: %v", err)
		return update.LoadCache().Latest
	}
	// Like polix update, the time of the check is only saved when this binary is the latest, so a newer release
	// seen here does not delay the update of the next session start.
	cache := update.LoadCache()
	cache.Latest = latest
	if version.Compare(Version, latest) >= 0 {
		cache.CheckedAt = time.Now()
	}
	_ = update.SaveCache(cache)
	return latest
}

func sameFile(a, b string) bool {
	ia, errA := os.Stat(a)
	ib, errB := os.Stat(b)
	return errA == nil && errB == nil && os.SameFile(ia, ib)
}

func (d *doctorRun) workflow(repo *gitx.Repo) {
	wf, err := workflow.Load(repo.Root, Version)
	var old *workflow.TooOld
	switch {
	case err != nil:
		d.add(levelFail, "workflow", "%v", err)
		if !errors.As(err, &old) && mayNeedNewer(err.Error()) {
			d.add(levelWarn, "workflow", "%s", newerHint())
		}
	case wf == nil:
		d.add(levelWarn, "workflow", "no %s: only the built-in protections apply", workflow.File)
	default:
		problems := workflow.Validate(wf)
		for _, p := range problems {
			d.add(levelFail, "workflow", "%s", p)
		}
		if len(problems) == 0 {
			d.add(levelOK, "workflow", "%s is valid", workflow.File)
		} else if mayNeedNewer(strings.Join(problems, "\n")) {
			d.add(levelWarn, "workflow", "%s", newerHint())
		}
	}
}

func (d *doctorRun) rules(repo *gitx.Repo) {
	all, err := rules.Load(repo.Root)
	if err != nil {
		d.add(levelFail, "rules", "%v", err)
		return
	}
	problems := rules.Validate(repo.Root, all)
	for _, r := range all {
		if !r.Deterministic || r.Check == "" || runtime.GOOS == "windows" {
			continue
		}
		info, err := os.Stat(filepath.Join(repo.Root, filepath.FromSlash(r.Check)))
		if err == nil && info.Mode()&0o111 == 0 {
			problems = append(problems, fmt.Sprintf("%s %s: check %s is not executable (chmod +x)", r.File, r.ID, r.Check))
		}
	}
	for _, p := range problems {
		d.add(levelFail, "rules", "%s", p)
	}
	if len(problems) == 0 {
		d.add(levelOK, "rules", "%d rules of %s/", len(all), rules.Dir)
	}
}

type claudeHook struct {
	Command string  `json:"command"`
	Timeout float64 `json:"timeout"`
}

type claudeGroup struct {
	Matcher string       `json:"matcher"`
	Hooks   []claudeHook `json:"hooks"`
}

// claudeSettingsFiles are the settings of Claude Code that may register hooks: of the project and local when
// root is a repository, and of the user. Claude Code merges them.
func claudeSettingsFiles(root, home string) []string {
	var files []string
	if root != "" {
		files = append(files, filepath.Join(root, ".claude", "settings.json"), filepath.Join(root, ".claude", "settings.local.json"))
	}
	if home != "" {
		files = append(files, filepath.Join(home, ".claude", "settings.json"))
	}
	return files
}

// readSettingsHooks reads the hooks of a settings file of Claude Code; a missing file is nil and no error.
func readSettingsHooks(f string) (map[string][]claudeGroup, error) {
	data, err := os.ReadFile(f)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, err
	}
	var s struct {
		Hooks map[string][]claudeGroup `json:"hooks"`
	}
	if err == nil {
		err = json.Unmarshal(data, &s)
	}
	return s.Hooks, err
}

// polixGroups counts the groups that run a hook command of polix.
func polixGroups(groups []claudeGroup) int {
	n := 0
	for _, g := range groups {
		for _, h := range g.Hooks {
			if setup.IsPolixCommand(h.Command) {
				n++
				break
			}
		}
	}
	return n
}

// hooks checks that Claude Code runs `polix hook claude` on every event polix needs, for every tool it governs,
// with a timeout above the run budget of a call, and only once. The hooks come from the settings of the project,
// the local ones and the ones of the user, which Claude Code merges.
func (d *doctorRun) hooks(root, home string) {
	before := len(d.findings)
	merged := map[string][]claudeGroup{}
	var sources []string
	for _, f := range claudeSettingsFiles(root, home) {
		hooks, err := readSettingsHooks(f)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			d.add(levelFail, "hooks", "%s: %v", f, err)
			continue
		}
		registers := false
		for ev, groups := range hooks {
			if polixGroups(groups) > 0 {
				registers = true
			}
			merged[ev] = append(merged[ev], groups...)
		}
		if registers {
			sources = append(sources, f)
		}
	}
	if len(sources) == 0 {
		d.add(levelFail, "hooks", "Claude Code does not run polix in this repository: run polix install")
		return
	}
	var dup []string
	for _, ev := range claudeHookEvents {
		n := 0
		for _, g := range merged[ev.name] {
			for _, h := range g.Hooks {
				if strings.Contains(h.Command, "polix hook claude") {
					n++
				}
			}
		}
		if n > 1 {
			dup = append(dup, ev.name)
		}
	}
	if len(dup) > 0 {
		d.add(levelFail, "hooks", "Claude Code runs polix twice on %s: keep the hooks of polix in one of %s",
			strings.Join(dup, ", "), strings.Join(sources, ", "))
	}
	updates, legacy := false, false
	for _, g := range merged["SessionStart"] {
		for _, h := range g.Hooks {
			legacy = legacy || setup.IsLegacyCommand(h.Command)
			updates = updates || setup.IsUpdateCommand(h.Command) && !setup.IsLegacyCommand(h.Command)
		}
	}
	switch {
	case legacy:
		d.add(levelWarn, "hooks", "SessionStart runs a command of an earlier polix: run polix install to refresh the hooks")
	case !updates:
		d.add(levelWarn, "hooks", "SessionStart does not run %s, so polix does not update itself: run polix install",
			setup.UpdateCommand)
	}
	budget := engine.RunBudget.Seconds()
	for _, ev := range claudeHookEvents {
		var polix []claudeGroup
		short := false
		for _, g := range merged[ev.name] {
			for _, h := range g.Hooks {
				if strings.Contains(h.Command, "polix hook claude") {
					polix = append(polix, g)
					t := h.Timeout
					if t == 0 {
						t = defaultHookTimeout(ev.name)
					}
					if t <= budget {
						short = true
					}
				}
			}
		}
		if len(polix) == 0 {
			d.add(levelFail, "hooks", "%s does not run polix hook claude: run polix install", ev.name)
			continue
		}
		var missing []string
		for _, tool := range ev.tools {
			if !anyMatcher(polix, tool) {
				missing = append(missing, tool)
			}
		}
		if len(missing) > 0 {
			d.add(levelFail, "hooks", "the matcher of %s does not cover %s: run polix install", ev.name, strings.Join(missing, ", "))
		}
		if short {
			d.add(levelWarn, "hooks", "%s: the timeout of polix hook claude must be above %.0f seconds, the run budget, "+
				"or Claude Code kills a long gate and lets the action through", ev.name, budget)
		}
	}
	if !d.problemsSince(before) {
		d.add(levelOK, "hooks", "registered in %s", strings.Join(sources, ", "))
	}
}

// plugin checks that the plugin polix is written for the user by this binary and, unless quick (the session start
// hook, which skips the slower CLI of Claude Code), that Claude Code has it installed and enabled.
func (d *doctorRun) plugin(home string, quick bool) {
	if home == "" {
		return
	}
	o := setup.Options{Home: home, Version: Version}
	v, ok := setup.PluginVersion(o)
	switch {
	case !ok:
		d.add(levelWarn, "plugin", "the plugin polix (skills and agents) is not installed: run polix install --user")
		return
	case v != Version:
		d.add(levelWarn, "plugin", "%s is from polix %s, this binary is %s: run polix install --user", o.PluginDir(), v, Version)
		return
	}
	if !quick {
		st, err := setup.ClaudeStatus()
		switch {
		case errors.Is(err, setup.ErrNoCLI):
		case err != nil:
			d.add(levelWarn, "plugin", "cannot ask Claude Code for its plugins: %v", err)
			return
		case !st.Installed:
			d.add(levelWarn, "plugin", "Claude Code does not have %s installed: run polix install --user", setup.PluginID)
			return
		case !st.Enabled:
			d.add(levelWarn, "plugin", "%s is disabled in Claude Code: run claude plugin enable %s", setup.PluginID, setup.PluginID)
			return
		case st.Folder != "" && !sameFile(st.Folder, o.PluginDir()):
			d.add(levelWarn, "plugin", "Claude Code loads %s from %s, not from %s: run polix install --user",
				setup.PluginID, st.Folder, o.PluginDir())
			return
		}
	}
	d.add(levelOK, "plugin", "%s %s: skills %s, agents %s", setup.PluginID, v,
		strings.Join(prefixed("/polix:", setup.Skills()), " "), strings.Join(prefixed("polix:", setup.Agents()), " "))
}

// codex checks Codex when it is used on this machine: polix must be registered for the project or the user, and
// the plugin enabled.
func (d *doctorRun) codex(root, home string) {
	o := setup.Options{Home: home, Project: root, Version: Version}
	if !codexPresent(o) {
		return
	}
	files := []string{o.HooksFile(setup.Codex), filepath.Join(home, ".codex", "hooks.json")}
	registered := false
	for _, f := range files {
		if _, err := setup.ReadHooks(f); err != nil {
			d.add(levelFail, "codex", "%v", err)
		}
		registered = registered || setup.Registered(f)
	}
	if !setup.CodexEnabled(o) {
		d.add(levelWarn, "codex", "the plugin %s is not enabled in Codex: run polix install --user --agent codex", setup.PluginID)
	}
	if !registered {
		d.add(levelWarn, "codex", "Codex is used here but does not run polix in this repository: run polix install --agent codex")
		return
	}
	d.add(levelOK, "codex", "hooks registered (Codex runs them once trusted with /hooks)")
}

// anyMatcher reports whether the matcher of a group covers tool; Claude Code reads an empty matcher or * as
// every tool, and any other as a regular expression.
func anyMatcher(groups []claudeGroup, tool string) bool {
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

// state checks that the log can be written: without it there are no facts, no loop guard and no evidence.
func (d *doctorRun) state(repo *gitx.Repo) {
	dir := state.Open(repo.GitDir).Dir
	err := os.MkdirAll(dir, 0o755)
	if err == nil {
		var f *os.File
		if f, err = os.CreateTemp(dir, "doctor-*"); err == nil {
			f.Close()
			os.Remove(f.Name())
		}
	}
	if err != nil {
		d.add(levelFail, "state", "cannot write the state in %s: %v", dir, err)
		return
	}
	d.add(levelOK, "state", "%s is writable", dir)
}
