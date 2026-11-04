package cli

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
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

	"github.com/mlophez/polix-ai/internal/engine"
	"github.com/mlophez/polix-ai/internal/gitx"
	"github.com/mlophez/polix-ai/internal/rules"
	"github.com/mlophez/polix-ai/internal/state"
	"github.com/mlophez/polix-ai/internal/update"
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

// doctor checks that this machine and this repository are ready for polix: the binary and its version, the
// PATH the hooks use, git, the workflow, the rules and their checks, the hooks of Claude Code and the state.
//
// With --hook it is meant for the session start hook: it never fails, skips the network (it reads the version
// the last `polix update` saw) and prints nothing when everything is fine; otherwise it prints the problems,
// which Claude Code adds to the context of the session, so the agent can tell the user and offer to fix them.
func doctor(args []string, stdout, stderr io.Writer) int {
	fset := flag.NewFlagSet("doctor", flag.ContinueOnError)
	fset.SetOutput(stderr)
	dir := fset.String("dir", ".", "repository")
	hook := fset.Bool("hook", false, "for a session start hook: quiet when fine, never fails, no network")
	offline := fset.Bool("offline", false, "do not ask for the latest release")
	if err := fset.Parse(args); err != nil {
		return 1
	}
	d := &doctorRun{}
	d.binary(*hook || *offline)
	if _, err := lookPath("git"); err != nil {
		d.add(levelFail, "git", "git is not on the PATH: polix reads every fact of the repository through it")
	} else if repo, err := gitx.Open(*dir); err != nil {
		d.add(levelOK, "repo", "not inside a git repository: only the checks of this machine apply")
	} else {
		d.add(levelOK, "git", "repository at %s", repo.Root)
		d.workflow(repo)
		d.rules(repo)
		d.hooks(repo.Root)
		d.state(repo)
	}

	if *hook {
		var lines []string
		for _, f := range d.findings {
			if f.level != levelOK {
				lines = append(lines, fmt.Sprintf("- [%s] %s: %s", f.level, f.area, f.msg))
			}
		}
		if len(lines) > 0 {
			fmt.Fprintf(stdout, "polix doctor ha encontrado problemas en este equipo o repositorio; cuéntaselos al usuario y "+
				"ofrécete a arreglarlos (polix doctor da el detalle):\n%s\n", strings.Join(lines, "\n"))
		}
		return 0
	}
	code := 0
	for _, f := range d.findings {
		fmt.Fprintf(stdout, "%-4s  %-8s  %s\n", f.level, f.area, f.msg)
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
	case !update.IsRelease(Version):
		d.add(levelWarn, "version", "polix %s is a development build: install a release with polix update --version <version>", Version)
	case latest == "":
		d.add(levelOK, "version", "polix %s (latest release unknown)", Version)
	case update.Compare(Version, latest) < 0:
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
	if noNetwork || !update.IsRelease(Version) {
		return update.LoadCache().Latest
	}
	ctx, cancel := context.WithTimeout(context.Background(), hookLatestTimeout)
	defer cancel()
	latest, err := update.New().Latest(ctx)
	if err != nil {
		d.add(levelWarn, "version", "cannot reach the releases: %v", err)
		return update.LoadCache().Latest
	}
	_ = update.SaveCache(update.Cache{CheckedAt: time.Now(), Latest: latest})
	return latest
}

func sameFile(a, b string) bool {
	ia, errA := os.Stat(a)
	ib, errB := os.Stat(b)
	return errA == nil && errB == nil && os.SameFile(ia, ib)
}

func (d *doctorRun) workflow(repo *gitx.Repo) {
	wf, err := workflow.Load(repo.Root)
	switch {
	case err != nil:
		d.add(levelFail, "workflow", "%v", err)
	case wf == nil:
		d.add(levelWarn, "workflow", "no %s: only the built-in protections apply", workflow.File)
	default:
		problems := workflow.Validate(wf)
		for _, p := range problems {
			d.add(levelFail, "workflow", "%s", p)
		}
		if len(problems) == 0 {
			d.add(levelOK, "workflow", "%s is valid", workflow.File)
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

// hooks checks that the settings of Claude Code (of the project, local and of the user, which Claude Code
// merges) run `polix hook claude` on every event polix needs, for every tool it governs, with a timeout above
// the run budget of a call.
func (d *doctorRun) hooks(root string) {
	files := []string{filepath.Join(root, ".claude", "settings.json"), filepath.Join(root, ".claude", "settings.local.json")}
	if home, err := os.UserHomeDir(); err == nil {
		files = append(files, filepath.Join(home, ".claude", "settings.json"))
	}
	merged := map[string][]claudeGroup{}
	var read []string
	for _, f := range files {
		data, err := os.ReadFile(f)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		var s struct {
			Hooks map[string][]claudeGroup `json:"hooks"`
		}
		if err == nil {
			err = json.Unmarshal(data, &s)
		}
		if err != nil {
			d.add(levelFail, "hooks", "%s: %v", f, err)
			continue
		}
		read = append(read, f)
		for ev, groups := range s.Hooks {
			merged[ev] = append(merged[ev], groups...)
		}
	}
	if len(read) == 0 {
		d.add(levelFail, "hooks", "no settings of Claude Code register polix: copy examples/claude-settings.json to .claude/settings.json")
		return
	}
	before := len(d.findings)
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
			d.add(levelFail, "hooks", "%s does not run polix hook claude", ev.name)
			continue
		}
		var missing []string
		for _, tool := range ev.tools {
			if !anyMatcher(polix, tool) {
				missing = append(missing, tool)
			}
		}
		if len(missing) > 0 {
			d.add(levelFail, "hooks", "the matcher of %s does not cover %s", ev.name, strings.Join(missing, ", "))
		}
		if short {
			d.add(levelWarn, "hooks", "%s: the timeout of polix hook claude must be above %.0f seconds, the run budget, "+
				"or Claude Code kills a long gate and lets the action through", ev.name, budget)
		}
	}
	if len(d.findings) == before {
		d.add(levelOK, "hooks", "registered in %s", strings.Join(read, ", "))
	}
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
