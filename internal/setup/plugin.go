package setup

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/mlophez/polix-ai/plugins"
)

// Name is the name of the plugin and of the local marketplace that holds it, so its id is polix@polix.
const Name = "polix"

// PluginID is the id of the plugin in Claude Code and Codex.
const PluginID = Name + "@" + Name

// Seams for the tests: what the PATH resolves and how the CLI of an agent runs. Run returns the standard output
// of the command, also when it fails.
var (
	LookPath = exec.LookPath
	Run      = runCommand
)

// commandTimeout bounds every call to the CLI of an agent.
const commandTimeout = time.Minute

func runCommand(name string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), commandTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err := cmd.Run()
	if msg := strings.TrimSpace(stderr.String()); err != nil && msg != "" {
		err = fmt.Errorf("%w: %s", err, msg)
	}
	return stdout.String(), err
}

// MarketplaceDir is the local marketplace polix writes its plugin to.
func (o Options) MarketplaceDir() string {
	return filepath.Join(o.Home, ".polix", "marketplace")
}

// PluginDir is the folder of the plugin inside the marketplace, which Claude Code loads in place.
func (o Options) PluginDir() string {
	return filepath.Join(o.MarketplaceDir(), Name)
}

// CodexConfigFile is the configuration of Codex of the user, where the plugin is enabled.
func (o Options) CodexConfigFile() string {
	return filepath.Join(o.Home, ".codex", "config.toml")
}

// Skills returns the names of the skills of the plugin.
func Skills() []string { return pluginNames("polix/skills", true) }

// Agents returns the names of the agents of the plugin, which only Claude Code loads.
func Agents() []string { return pluginNames("polix/agents", false) }

func pluginNames(dir string, dirs bool) []string {
	entries, _ := fs.ReadDir(plugins.FS, dir)
	var out []string
	for _, e := range entries {
		switch {
		case dirs && e.IsDir():
			out = append(out, e.Name())
		case !dirs && !e.IsDir() && strings.HasSuffix(e.Name(), ".md"):
			out = append(out, strings.TrimSuffix(e.Name(), ".md"))
		}
	}
	slices.Sort(out)
	return out
}

// marketplace manifests, with their fields in the order the agents document them.
type claudeMarketplace struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Owner       struct {
		Name string `json:"name"`
	} `json:"owner"`
	Plugins []claudeEntry `json:"plugins"`
}

type claudeEntry struct {
	Name        string `json:"name"`
	Source      string `json:"source"`
	Description string `json:"description"`
}

type codexMarketplace struct {
	Name      string `json:"name"`
	Interface struct {
		DisplayName string `json:"displayName"`
	} `json:"interface"`
	Plugins []codexEntry `json:"plugins"`
}

type codexEntry struct {
	Name   string `json:"name"`
	Source struct {
		Source string `json:"source"`
		Path   string `json:"path"`
	} `json:"source"`
	Policy struct {
		Installation   string `json:"installation"`
		Authentication string `json:"authentication"`
	} `json:"policy"`
	Category string `json:"category"`
}

// pluginFiles are the files of the local marketplace, keyed by path relative to it: the plugin embedded in the
// binary, with the version of the binary in its manifests, and a marketplace manifest for each agent.
func pluginFiles(version string) (map[string][]byte, error) {
	files := map[string][]byte{}
	description := ""
	err := fs.WalkDir(plugins.FS, Name, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		data, err := plugins.FS.ReadFile(p)
		if err != nil {
			return err
		}
		if path.Base(p) == "plugin.json" {
			if data, err = setVersion(data, version); err != nil {
				return fmt.Errorf("%s: %w", p, err)
			}
			if strings.Contains(p, ".claude-plugin") {
				var m struct{ Description string }
				_ = json.Unmarshal(data, &m)
				description = m.Description
			}
		}
		files[p] = data
		return nil
	})
	if err != nil {
		return nil, err
	}

	var cm claudeMarketplace
	cm.Name = Name
	cm.Description = "The plugin of polix " + version + ", written by polix install from its binary: do not edit."
	cm.Owner.Name = Name
	cm.Plugins = []claudeEntry{{Name: Name, Source: "./" + Name, Description: description}}
	files[".claude-plugin/marketplace.json"] = pretty(marshal(cm))

	var xm codexMarketplace
	xm.Name = Name
	xm.Interface.DisplayName = Name
	e := codexEntry{Name: Name, Category: "Coding"}
	e.Source.Source, e.Source.Path = "local", "./"+Name
	e.Policy.Installation, e.Policy.Authentication = "AVAILABLE", "ON_INSTALL"
	xm.Plugins = []codexEntry{e}
	files[".agents/plugins/marketplace.json"] = pretty(marshal(xm))
	return files, nil
}

// setVersion sets the version of a plugin manifest, keeping the order of its keys.
func setVersion(data []byte, version string) ([]byte, error) {
	ms, err := decodeObject(data)
	if err != nil {
		return nil, err
	}
	i := slices.IndexFunc(ms, func(m member) bool { return m.key == "version" })
	if i < 0 {
		return nil, errors.New("no version")
	}
	ms[i].val = marshal(version)
	return pretty(encodeObject(ms)), nil
}

// WritePlugin writes the local marketplace with the plugin of this binary, removing the files a release dropped.
func WritePlugin(o Options) ([]Change, error) {
	files, err := pluginFiles(o.Version)
	if err != nil {
		return nil, err
	}
	return syncDir(o.MarketplaceDir(), files)
}

// PluginVersion returns the version of the plugin written in the local marketplace, and whether it is there.
func PluginVersion(o Options) (string, bool) {
	data, err := os.ReadFile(filepath.Join(o.PluginDir(), ".claude-plugin", "plugin.json"))
	if err != nil {
		return "", false
	}
	var m struct {
		Name    string `json:"name"`
		Version string `json:"version"`
	}
	if json.Unmarshal(data, &m) != nil || m.Name != Name {
		return "", false
	}
	return m.Version, true
}

// Refresh rewrites the plugin where polix installed it, for a new binary, and tells the agents about the new
// version. It installs nothing new.
func Refresh(o Options) ([]Change, error) {
	if _, ok := PluginVersion(o); !ok {
		return nil, nil
	}
	changes, err := WritePlugin(o)
	if err != nil || len(changes) == 0 {
		return changes, err
	}
	// Claude Code already reads the new files in place; update records the new version. Codex may hold a copy.
	if _, err := LookPath("claude"); err == nil {
		_, _ = claude(nil, "plugin", "update", PluginID, "--scope", "user", "--json")
	}
	if _, err := LookPath("codex"); err == nil && codexEnabled(o) {
		_, _ = Run("codex", "plugin", "marketplace", "upgrade", Name)
	}
	return changes, nil
}

// claudeResult is the last line `claude plugin ... --json` prints.
type claudeResult struct {
	Outcome          string `json:"outcome"`
	Message          string `json:"message"`
	FailureCode      string `json:"failureCode"`
	InstalledVersion string `json:"installedVersion"`
	AvailableVersion string `json:"availableVersion"`
}

// claude runs a command of the CLI of Claude Code with --json. A failure whose code is in tolerated is no error.
func claude(tolerated []string, args ...string) (claudeResult, error) {
	out, err := Run("claude", args...)
	var r claudeResult
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if jerr := json.Unmarshal([]byte(lines[len(lines)-1]), &r); jerr != nil {
		if err == nil {
			err = fmt.Errorf("unexpected output: %s", strings.TrimSpace(out))
		}
		return r, fmt.Errorf("claude %s: %w", strings.Join(args, " "), err)
	}
	if r.Outcome == "ok" || slices.Contains(tolerated, r.FailureCode) {
		return r, nil
	}
	return r, fmt.Errorf("claude %s: %s", strings.Join(args, " "), r.Message)
}

func registerPlugin(o Options, agent string) ([]Change, error) {
	dir := o.MarketplaceDir()
	if agent == Codex {
		var changes []Change
		add := "codex plugin marketplace add " + quote(dir)
		if _, err := LookPath("codex"); err != nil {
			changes = append(changes, Change{add, "todo"})
		} else if out, err := Run("codex", "plugin", "marketplace", "add", dir); err != nil &&
			!strings.Contains(strings.ToLower(out+err.Error()), "already") {
			changes = append(changes, Change{add + " (it failed: " + oneLine(err.Error()) + ")", "todo"})
		}
		ch, err := setCodexPlugin(o.CodexConfigFile(), true)
		return append(changes, ch...), err
	}
	if _, err := LookPath("claude"); err != nil {
		return []Change{{"claude plugin marketplace add " + quote(dir) + " --scope user && claude plugin install " +
			PluginID + " --scope user", "todo"}}, nil
	}
	if _, err := claude(nil, "plugin", "marketplace", "add", dir, "--scope", "user", "--json"); err != nil {
		return nil, err
	}
	r, err := claude(nil, "plugin", "install", PluginID, "--scope", "user", "--json")
	if err != nil {
		return nil, err
	}
	if r.InstalledVersion == "" {
		return []Change{{PluginID + " in Claude Code", "registered"}}, nil
	}
	if r.InstalledVersion != r.AvailableVersion {
		_, err = claude(nil, "plugin", "update", PluginID, "--scope", "user", "--json")
	}
	return nil, err
}

func unregisterPlugin(o Options, agent string) ([]Change, error) {
	if agent == Codex {
		if _, err := LookPath("codex"); err == nil && codexEnabled(o) {
			_, _ = Run("codex", "plugin", "marketplace", "remove", Name)
		}
		return setCodexPlugin(o.CodexConfigFile(), false)
	}
	if _, err := LookPath("claude"); err != nil {
		return nil, nil
	}
	r, err := claude([]string{"not_installed"}, "plugin", "uninstall", PluginID, "--scope", "user", "--json")
	if err != nil {
		return nil, err
	}
	if _, err := claude([]string{"not_configured"}, "plugin", "marketplace", "remove", Name, "--json"); err != nil {
		return nil, err
	}
	if r.Outcome == "ok" {
		return []Change{{PluginID + " in Claude Code", "unregistered"}}, nil
	}
	return nil, nil
}

// removePlugin removes the local marketplace, and ~/.polix when nothing else is left in it.
func removePlugin(o Options) ([]Change, error) {
	dir := o.MarketplaceDir()
	if _, err := os.Stat(dir); errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err := os.RemoveAll(dir); err != nil {
		return nil, err
	}
	_ = os.Remove(filepath.Dir(dir))
	return []Change{{dir, "removed"}}, nil
}

// ClaudePlugin is the plugin polix as `claude plugin list` reports it.
type ClaudePlugin struct {
	Installed bool
	Enabled   bool
	Version   string
	Folder    string
}

// ErrNoCLI is returned when the CLI of an agent is not on the PATH.
var ErrNoCLI = errors.New("not on the PATH")

// ClaudeStatus asks Claude Code whether the plugin polix is installed and enabled for the user.
func ClaudeStatus() (ClaudePlugin, error) {
	if _, err := LookPath("claude"); err != nil {
		return ClaudePlugin{}, ErrNoCLI
	}
	out, err := Run("claude", "plugin", "list", "--json")
	if err != nil {
		return ClaudePlugin{}, err
	}
	var list []struct {
		ID             string `json:"id"`
		Scope          string `json:"scope"`
		Enabled        bool   `json:"enabled"`
		Version        string `json:"version"`
		ReadFromFolder string `json:"readFromFolder"`
	}
	if err := json.Unmarshal([]byte(out), &list); err != nil {
		return ClaudePlugin{}, fmt.Errorf("claude plugin list: %w", err)
	}
	for _, p := range list {
		if p.ID == PluginID && p.Scope == "user" {
			return ClaudePlugin{Installed: true, Enabled: p.Enabled, Version: p.Version, Folder: p.ReadFromFolder}, nil
		}
	}
	return ClaudePlugin{}, nil
}

// codexTable is the table of ~/.codex/config.toml that enables the plugin.
var codexTable = `[plugins."` + PluginID + `"]`

// codexEnabled reports whether the configuration of Codex enables the plugin.
func codexEnabled(o Options) bool {
	data, _ := os.ReadFile(o.CodexConfigFile())
	lines := strings.Split(string(data), "\n")
	start, end := codexBlock(lines)
	for _, l := range lines[max(start, 0):end] {
		if k, v, ok := strings.Cut(l, "="); ok && strings.TrimSpace(k) == "enabled" {
			return strings.TrimSpace(v) == "true"
		}
	}
	return false
}

// CodexEnabled reports whether the configuration of Codex of the user enables the plugin polix.
func CodexEnabled(o Options) bool { return codexEnabled(o) }

// codexBlock returns the lines of the table of the plugin, [start, end); start is -1 when it is missing.
func codexBlock(lines []string) (int, int) {
	start := slices.IndexFunc(lines, func(l string) bool { return strings.TrimSpace(l) == codexTable })
	if start < 0 {
		return -1, 0
	}
	end := start + 1
	for end < len(lines) && !strings.HasPrefix(strings.TrimSpace(lines[end]), "[") {
		end++
	}
	return start, end
}

// setCodexPlugin enables the plugin in the configuration of Codex, or removes its table, leaving the rest of the
// file as it is.
func setCodexPlugin(file string, on bool) ([]Change, error) {
	data, err := os.ReadFile(file)
	existed := err == nil
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, err
	}
	text := string(data)
	lines := strings.Split(text, "\n")
	start, end := codexBlock(lines)
	switch {
	case on && start < 0:
		if text != "" && !strings.HasSuffix(text, "\n") {
			text += "\n"
		}
		if text != "" {
			text += "\n"
		}
		text += codexTable + "\nenabled = true\n"
	case on:
		block := lines[start+1 : end]
		i := slices.IndexFunc(block, func(l string) bool {
			k, _, ok := strings.Cut(l, "=")
			return ok && strings.TrimSpace(k) == "enabled"
		})
		if i >= 0 {
			block[i] = "enabled = true"
		} else {
			lines = slices.Insert(lines, start+1, "enabled = true")
		}
		text = strings.Join(lines, "\n")
	case start >= 0:
		// Drop the table and the blank line before it.
		if start > 0 && strings.TrimSpace(lines[start-1]) == "" {
			start--
		}
		lines = slices.Delete(lines, start, end)
		text = strings.Join(lines, "\n")
		if strings.TrimSpace(text) == "" {
			text = ""
		} else if strings.HasSuffix(string(data), "\n") && !strings.HasSuffix(text, "\n") {
			text += "\n"
		}
	}
	if text == string(data) {
		return nil, nil
	}
	if text == "" && existed {
		if err := os.Remove(file); err != nil {
			return nil, err
		}
		return []Change{{file, "removed"}}, nil
	}
	if err := writeFile(file, []byte(text)); err != nil {
		return nil, err
	}
	if existed {
		return []Change{{file, "updated"}}, nil
	}
	return []Change{{file, "created"}}, nil
}

// quote quotes a path for a command shown to the user when it holds a space.
func quote(p string) string {
	if strings.ContainsAny(p, " \t") {
		return `"` + p + `"`
	}
	return p
}

func oneLine(s string) string {
	s, _, _ = strings.Cut(strings.TrimSpace(s), "\n")
	return s
}
