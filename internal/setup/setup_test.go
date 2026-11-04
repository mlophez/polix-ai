package setup

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/mlophez/polix-ai/internal/engine"
)

func opts(t *testing.T) Options {
	t.Helper()
	return Options{Home: t.TempDir(), Version: "1.2.3"}
}

func read(t *testing.T, p string) string {
	t.Helper()
	data, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// fakeCLI stands for the CLIs of Claude Code and Codex: it records every call and answers like `claude plugin`
// does with --json.
type fakeCLI struct {
	calls     []string
	installed string // version recorded by install, empty when not installed
	folder    string // version of the plugin folder, read on install
	added     bool
}

func fakeAgents(t *testing.T, claude, codex bool) *fakeCLI {
	t.Helper()
	f := &fakeCLI{folder: "1.2.3"}
	oldLook, oldRun := LookPath, Run
	t.Cleanup(func() { LookPath, Run = oldLook, oldRun })
	LookPath = func(name string) (string, error) {
		if name == "claude" && claude || name == "codex" && codex {
			return "/bin/" + name, nil
		}
		return "", errors.New("not found")
	}
	Run = func(name string, args ...string) (string, error) {
		call := name + " " + strings.Join(args, " ")
		f.calls = append(f.calls, call)
		ok := `{"outcome":"ok"}`
		switch {
		case name == "codex":
			return "", nil
		case strings.HasPrefix(call, "claude plugin marketplace add"):
			f.added = true
			return ok, nil
		case strings.HasPrefix(call, "claude plugin install"):
			if f.installed != "" {
				return `{"outcome":"ok","installedVersion":"` + f.installed + `","availableVersion":"` + f.folder + `"}`, nil
			}
			f.installed = f.folder
			return ok, nil
		case strings.HasPrefix(call, "claude plugin update"):
			f.installed = f.folder
			return ok, nil
		case strings.HasPrefix(call, "claude plugin uninstall"):
			if f.installed == "" {
				return `{"outcome":"failed","failureCode":"not_installed","message":"not installed"}`, errors.New("exit status 1")
			}
			f.installed = ""
			return ok, nil
		case strings.HasPrefix(call, "claude plugin marketplace remove"):
			if !f.added {
				return `{"outcome":"failed","failureCode":"not_configured","message":"not found"}`, errors.New("exit status 1")
			}
			f.added = false
			return ok, nil
		case strings.HasPrefix(call, "claude plugin list"):
			if f.installed == "" {
				return "[]", nil
			}
			return `[{"id":"polix@polix","scope":"user","enabled":true,"version":"` + f.installed + `"}]`, nil
		}
		return "", errors.New("unexpected call: " + call)
	}
	return f
}

// install writes the plugin embedded in the binary to the local marketplace, with the version of the binary, and
// installs it in Claude Code from there.
func TestInstallClaude(t *testing.T) {
	o := opts(t)
	cli := fakeAgents(t, true, false)
	cli.folder = o.Version
	changes, err := Install(o, []string{Claude})
	if err != nil {
		t.Fatal(err)
	}
	if v, ok := PluginVersion(o); !ok || v != "1.2.3" {
		t.Fatalf("plugin version: %q %v", v, ok)
	}
	if !slices.Contains(changes, Change{PluginID + " in Claude Code", "registered"}) {
		t.Fatalf("a first install registers the plugin: %+v", changes)
	}
	want := []string{
		"claude plugin marketplace add " + o.MarketplaceDir() + " --scope user --json",
		"claude plugin install polix@polix --scope user --json",
	}
	if !slices.Equal(cli.calls, want) {
		t.Fatalf("calls:\n%s", strings.Join(cli.calls, "\n"))
	}
	var codex map[string]any
	if err := json.Unmarshal([]byte(read(t, filepath.Join(o.PluginDir(), ".codex-plugin", "plugin.json"))), &codex); err != nil || codex["version"] != "1.2.3" {
		t.Fatalf("manifest of Codex: %v %v", codex, err)
	}
	// The manifests are read with the exact names of their fields: owner.name is required by Claude Code.
	var cm struct {
		Name  string `json:"name"`
		Owner struct {
			Name string `json:"name"`
		} `json:"owner"`
		Plugins []struct {
			Name   string `json:"name"`
			Source string `json:"source"`
		} `json:"plugins"`
	}
	if err := json.Unmarshal([]byte(read(t, filepath.Join(o.MarketplaceDir(), ".claude-plugin", "marketplace.json"))), &cm); err != nil ||
		cm.Name != Name || cm.Owner.Name == "" || len(cm.Plugins) != 1 || cm.Plugins[0].Source != "./polix" {
		t.Fatalf("marketplace of Claude Code: %+v %v", cm, err)
	}
	var xm struct {
		Name    string `json:"name"`
		Plugins []struct {
			Source struct {
				Source string `json:"source"`
				Path   string `json:"path"`
			} `json:"source"`
		} `json:"plugins"`
	}
	if err := json.Unmarshal([]byte(read(t, filepath.Join(o.MarketplaceDir(), ".agents", "plugins", "marketplace.json"))), &xm); err != nil ||
		xm.Name != Name || len(xm.Plugins) != 1 || xm.Plugins[0].Source.Path != "./polix" {
		t.Fatalf("marketplace of Codex: %+v %v", xm, err)
	}
	skill := read(t, filepath.Join(o.PluginDir(), "skills", "add-rule", "SKILL.md"))
	if !strings.Contains(skill, "name: add-rule\n") {
		t.Fatalf("skill:\n%s", skill)
	}
	if _, err := os.Stat(filepath.Join(o.Home, ".claude")); err == nil {
		t.Fatal("install only touches ~/.claude through the CLI of Claude Code")
	}

	// A second install changes nothing; a file of an earlier release that this one dropped is removed.
	stale := filepath.Join(o.PluginDir(), "skills", "gone", "SKILL.md")
	os.MkdirAll(filepath.Dir(stale), 0o755)
	os.WriteFile(stale, []byte("old"), 0o644)
	changes, err = Install(o, []string{Claude})
	if err != nil || len(changes) != 1 || changes[0].Action != "removed" || changes[0].Path != stale {
		t.Fatalf("second install: %v %+v", err, changes)
	}
	if _, err := os.Stat(filepath.Dir(stale)); err == nil {
		t.Fatal("the emptied folder of a dropped skill is removed")
	}

	// A new binary rewrites the plugin in place and has Claude Code record its version.
	o.Version, cli.folder = "1.2.4", "1.2.4"
	cli.calls = nil
	if changes, err := Refresh(o); err != nil || len(changes) == 0 {
		t.Fatalf("refresh: %v %+v", err, changes)
	}
	if v, _ := PluginVersion(o); v != "1.2.4" || cli.installed != "1.2.4" {
		t.Fatalf("refresh: folder %s, Claude Code %s, calls %v", v, cli.installed, cli.calls)
	}
	if changes, _ := Refresh(o); len(changes) != 0 {
		t.Fatalf("nothing to refresh: %+v", changes)
	}

	if _, err := Uninstall(o, []string{Claude}); err != nil || cli.installed != "" || cli.added {
		t.Fatalf("uninstall: %v, calls %v", err, cli.calls)
	}
	if _, err := Uninstall(o, []string{Claude, Codex}); err != nil {
		t.Fatalf("uninstall what is not installed: %v", err)
	}
	if _, err := os.Stat(o.MarketplaceDir()); err == nil {
		t.Fatal("uninstall for every agent removes the plugin")
	}
	if changes, _ := Refresh(o); len(changes) != 0 {
		t.Fatal("refresh installs nothing")
	}
}

// Without the CLI of Claude Code, install writes the plugin and says what to run.
func TestInstallWithoutClaude(t *testing.T) {
	o := opts(t)
	fakeAgents(t, false, false)
	changes, err := Install(o, []string{Claude})
	if err != nil || len(changes) == 0 {
		t.Fatal(err)
	}
	last := changes[len(changes)-1]
	if last.Action != "todo" || !strings.Contains(last.Path, "claude plugin install polix@polix --scope user") {
		t.Fatalf("the command to run by hand: %+v", last)
	}
}

// For Codex, install adds the marketplace and enables the plugin in its configuration, keeping the rest of it.
func TestInstallCodex(t *testing.T) {
	o := opts(t)
	cli := fakeAgents(t, false, true)
	config := o.CodexConfigFile()
	os.MkdirAll(filepath.Dir(config), 0o755)
	mine := "model = \"gpt-5\"\n\n[plugins.\"other@x\"]\nenabled = true\n"
	os.WriteFile(config, []byte(mine), 0o644)

	if _, err := Install(o, []string{Codex}); err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(cli.calls, "codex plugin marketplace add "+o.MarketplaceDir()) {
		t.Fatalf("calls: %v", cli.calls)
	}
	got := read(t, config)
	if !strings.HasPrefix(got, mine) || !strings.Contains(got, "[plugins.\"polix@polix\"]\nenabled = true\n") || !CodexEnabled(o) {
		t.Fatalf("config of Codex:\n%s", got)
	}
	if changes, err := Install(o, []string{Codex}); err != nil || len(changes) != 0 {
		t.Fatalf("second install: %v %+v", err, changes)
	}

	// A plugin disabled by hand is enabled again.
	os.WriteFile(config, []byte(strings.Replace(got, "polix@polix\"]\nenabled = true", "polix@polix\"]\nenabled = false", 1)), 0o644)
	if _, err := Install(o, []string{Codex}); err != nil || !CodexEnabled(o) {
		t.Fatalf("enable again: %v\n%s", err, read(t, config))
	}

	if _, err := Uninstall(o, []string{Codex}); err != nil {
		t.Fatal(err)
	}
	if got := read(t, config); got != mine || CodexEnabled(o) {
		t.Fatalf("uninstall leaves the rest of the config:\n%q", got)
	}
	if !slices.Contains(cli.calls, "codex plugin marketplace remove polix") {
		t.Fatalf("calls: %v", cli.calls)
	}
}

// A config of Codex that only enabled polix goes with it.
func TestUninstallCodexRemovesItsConfig(t *testing.T) {
	o := opts(t)
	fakeAgents(t, false, false)
	if _, err := Install(o, []string{Codex}); err != nil {
		t.Fatal(err)
	}
	if _, err := Uninstall(o, []string{Codex}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(o.CodexConfigFile()); err == nil {
		t.Fatal("the config of polix alone is removed")
	}
}

// In a project, install registers the hooks in its settings, keeping the rest of the file in its order, and puts
// the plugin in the home of the user, not in the repository.
func TestRegisterHooksInTheProject(t *testing.T) {
	o := opts(t)
	fakeAgents(t, true, false)
	o.Project = t.TempDir()
	settings := o.HooksFile(Claude)
	os.MkdirAll(filepath.Dir(settings), 0o755)
	mine := `{
  "permissions": {"allow": ["Bash(go test:*)"]},
  "hooks": {
    "Stop": [
      {"hooks": [{"type": "command", "command": "notify-send done"}]},
      {"hooks": [{"type": "command", "command": "polix hook claude"}]}
    ]
  },
  "model": "opus"
}`
	os.WriteFile(settings, []byte(mine), 0o644)

	if _, err := Install(o, []string{Claude, Codex}); err != nil {
		t.Fatal(err)
	}
	got := read(t, settings)
	if p, h, m := strings.Index(got, `"permissions"`), strings.Index(got, `"hooks"`), strings.Index(got, `"model"`); !(p < h && h < m) {
		t.Fatalf("the order of the keys is kept:\n%s", got)
	}
	hooks, err := ReadHooks(settings)
	if err != nil {
		t.Fatal(err)
	}
	if len(hooks["Stop"]) != 2 || hooks["Stop"][0].Hooks[0].Command != "notify-send done" ||
		hooks["Stop"][1].Hooks[0].Command != "polix hook claude" || hooks["Stop"][1].Hooks[0].Timeout != HookTimeout {
		t.Fatalf("the hooks of the user are kept and the one of polix replaced in place: %+v", hooks["Stop"])
	}
	if len(hooks) != len(Hooks(Claude)) || hooks["PreToolUse"][0].Matcher != claudeTools {
		t.Fatalf("every event of polix: %+v", hooks)
	}
	if !Registered(o.HooksFile(Codex)) {
		t.Fatal("the hooks of Codex go to the .codex/hooks.json of the project")
	}
	if _, ok := PluginVersion(o); !ok {
		t.Fatal("the plugin goes to the home of the user")
	}
	for _, p := range []string{filepath.Join(o.Project, ".claude", "skills"), filepath.Join(o.Project, ".agents")} {
		if _, err := os.Stat(p); err == nil {
			t.Fatalf("no plugin in the repository: %s", p)
		}
	}

	// Installing again changes nothing but the command of Codex, which is not on the PATH, to run by hand.
	if changes, err := Install(o, []string{Claude, Codex}); err != nil || len(changes) != 1 || changes[0].Action != "todo" {
		t.Fatalf("second install: %v %+v", err, changes)
	}

	// Uninstall leaves the rest of the settings; a hooks file that only held polix goes with it.
	if _, err := Uninstall(o, []string{Claude, Codex}); err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := json.Unmarshal([]byte(read(t, settings)), &doc); err != nil {
		t.Fatal(err)
	}
	hooks, _ = ReadHooks(settings)
	if doc["model"] != "opus" || doc["permissions"] == nil || len(hooks) != 1 || len(hooks["Stop"]) != 1 || Registered(settings) {
		t.Fatalf("uninstall leaves the rest of the file: %v", doc)
	}
	if _, err := os.Stat(o.HooksFile(Codex)); err == nil {
		t.Fatal("the hooks file of polix alone is removed")
	}
}

// The session start command of an earlier build is replaced in place, not kept next to the new one.
func TestRegisterHooksReplacesALegacyCommand(t *testing.T) {
	o := opts(t)
	o.Project = t.TempDir()
	settings := o.HooksFile(Claude)
	os.MkdirAll(filepath.Dir(settings), 0o755)
	os.WriteFile(settings, []byte(`{"hooks": {"SessionStart": [
  {"hooks": [{"type": "command", "command": "polix hook claude", "timeout": 900}]},
  {"matcher": "startup", "hooks": [{"type": "command", "command": "polix update --hook; polix doctor --hook", "timeout": 180}]}
]}}`), 0o644)
	if _, err := RegisterHooks(o, Claude); err != nil {
		t.Fatal(err)
	}
	got := read(t, settings)
	if strings.Contains(got, "polix doctor --hook") || strings.Count(got, `"`+UpdateCommand+`"`) != 1 {
		t.Fatalf("the legacy command is replaced:\n%s", got)
	}
	for _, c := range []string{"polix update --hook", "polix update --hook --flag-of-a-later-release", "polix update --hook; polix doctor --hook"} {
		if !IsPolixCommand(c) {
			t.Errorf("%q is the update of polix", c)
		}
	}
	if IsPolixCommand("polix update --check") || IsPolixCommand("polix update --hookx") {
		t.Error("only the session start command is the update of polix")
	}
}

// The example settings of the repository are what install registers.
func TestClaudeSettingsExample(t *testing.T) {
	var example, generated any
	if err := json.Unmarshal([]byte(read(t, filepath.Join("..", "..", "examples", "claude-settings.json"))), &example); err != nil {
		t.Fatal(err)
	}
	data, _ := json.Marshal(map[string]any{"hooks": Hooks(Claude)})
	_ = json.Unmarshal(data, &generated)
	a, _ := json.Marshal(example)
	b, _ := json.Marshal(generated)
	if string(a) != string(b) {
		t.Fatalf("examples/claude-settings.json differs from the hooks install registers:\n%s\n%s", a, b)
	}
}

// The hooks give polix more time than the run budget of a call, so the agent never kills a gate in the middle.
func TestHookTimeouts(t *testing.T) {
	for _, agent := range []string{Claude, Codex} {
		for ev, groups := range Hooks(agent) {
			for _, g := range groups {
				for _, h := range g.Hooks {
					if strings.HasPrefix(h.Command, "polix hook") && float64(h.Timeout) <= engine.RunBudget.Seconds() {
						t.Errorf("%s %s: timeout %d", agent, ev, h.Timeout)
					}
				}
			}
		}
	}
}

func TestPluginNames(t *testing.T) {
	if got := strings.Join(Skills(), " "); got != "add-rule doctor workflow" {
		t.Fatalf("skills: %s", got)
	}
	if got := strings.Join(Agents(), " "); got != "auditor rules-author rules-reviewer workflow-author" {
		t.Fatalf("agents: %s", got)
	}
}

// A development build is always "dev", so the plugin carries a hash of its contents as version: Claude Code only
// copies a plugin to its cache when its version changes.
func TestManifestVersionOfDevelopmentBuild(t *testing.T) {
	if v := ManifestVersion("1.2.3"); v != "1.2.3" {
		t.Fatalf("a release writes its own version: %s", v)
	}
	dev := ManifestVersion("dev")
	if !strings.HasPrefix(dev, "0.0.0-dev+") || len(dev) != len("0.0.0-dev+")+12 || ManifestVersion("") != dev {
		t.Fatalf("a development build writes 0.0.0-dev+<hash>: %s", dev)
	}
	o := Options{Home: t.TempDir(), Version: "dev"}
	if _, err := WritePlugin(o); err != nil {
		t.Fatal(err)
	}
	if v, _ := PluginVersion(o); v != dev {
		t.Fatalf("plugin version: %s, want %s", v, dev)
	}
	m := read(t, filepath.Join(o.MarketplaceDir(), Name, ".codex-plugin", "plugin.json"))
	if !strings.Contains(m, `"version": "`+dev+`"`) {
		t.Fatalf("both manifests carry it: %s", m)
	}
}
