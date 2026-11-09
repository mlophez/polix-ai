package setup

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"path"
	"slices"
	"strconv"
	"strings"

	"github.com/mlophez/polix-ai/internal/platform/semver"
)

// Plugin is the plugin polix wrote in the local marketplace, against the one embedded in the binary of
// Version. It is built with a literal followed by Valid.
type Plugin struct {
	Manifest []byte            // the manifest written in the marketplace; nil when there is none
	Embedded map[string][]byte // the plugin embedded in the binary, below polix/
	Version  string            // the version of the binary
}

// claudeMarketplace, claudeEntry, codexMarketplace and codexEntry are the manifests of the marketplace, with
// their fields in the order the agents document them.
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

// manifestJSON is the part of the manifest of a plugin polix reads.
type manifestJSON struct {
	Name        string `json:"name"`
	Version     string `json:"version"`
	Description string `json:"description"`
}

// claudeResultJSON is the last line `claude plugin ... --json` prints.
type claudeResultJSON struct {
	Outcome          string `json:"outcome"`
	Message          string `json:"message"`
	FailureCode      string `json:"failureCode"`
	InstalledVersion string `json:"installedVersion"`
	AvailableVersion string `json:"availableVersion"`
}

// claudePluginJSON is one plugin of `claude plugin list --json`.
type claudePluginJSON struct {
	ID             string `json:"id"`
	Scope          string `json:"scope"`
	Enabled        bool   `json:"enabled"`
	Version        string `json:"version"`
	ReadFromFolder string `json:"readFromFolder"`
}

// ErrNoCLI is the error of asking the CLI of an agent that is not on the PATH.
var ErrNoCLI = errors.New("not on the PATH")

// Valid reports why p is not a plugin to compare: it needs the version of the binary.
func (p Plugin) Valid() error {
	if p.Version == "" {
		return errors.New("a plugin needs the version of the binary")
	}
	return nil
}

// Current reports whether the plugin written is the one of the binary: installed, with its ManifestVersion.
func (p Plugin) Current() bool {
	v, ok := PluginVersion(p.Manifest)
	return ok && v == ManifestVersion(p.Embedded, p.Version)
}

// PluginSkills returns the names of the skills of the plugin, from its files embedded in the binary (path to
// content, below polix/).
func PluginSkills(embedded map[string][]byte) []string {
	var out []string
	for p := range embedded {
		rest, ok := strings.CutPrefix(p, Name+"/skills/")
		if name, _, dir := strings.Cut(rest, "/"); ok && dir && !slices.Contains(out, name) {
			out = append(out, name)
		}
	}
	slices.Sort(out)
	return out
}

// PluginAgents returns the names of the agents of the plugin, which only Claude Code loads.
func PluginAgents(embedded map[string][]byte) []string {
	var out []string
	for p := range embedded {
		rest, ok := strings.CutPrefix(p, Name+"/agents/")
		if ok && !strings.Contains(rest, "/") && strings.HasSuffix(rest, ".md") {
			out = append(out, strings.TrimSuffix(rest, ".md"))
		}
	}
	slices.Sort(out)
	return out
}

// ManifestVersion is the version polix writes in the manifests of the plugin for a binary of version binary. A
// release writes its own version. A development build is always "dev", and Claude Code only copies a plugin to its
// cache when its version changes, so it writes 0.0.0-dev+<hash of the plugin> instead.
func ManifestVersion(embedded map[string][]byte, binary string) string {
	if semver.IsRelease(binary) {
		return binary
	}
	return "0.0.0-dev+" + pluginHash(embedded)
}

// pluginHash is a short hash of the paths and contents of the plugin embedded in the binary, walked as a tree
// of folders sorted by name.
func pluginHash(embedded map[string][]byte) string {
	h := sha256.New()
	for _, p := range walkOrder(embedded) {
		data := embedded[p]
		h.Write([]byte(p + "\x00" + strconv.Itoa(len(data)) + "\x00"))
		h.Write(data)
	}
	return hex.EncodeToString(h.Sum(nil))[:12]
}

// walkOrder returns the paths of files in the order a walk of their tree visits them: the entries of each folder
// sorted by name, a folder before the next entry.
func walkOrder(files map[string][]byte) []string {
	paths := make([]string, 0, len(files))
	for p := range files {
		paths = append(paths, p)
	}
	slices.SortFunc(paths, func(a, b string) int {
		return slices.Compare(strings.Split(a, "/"), strings.Split(b, "/"))
	})
	return paths
}

// MarketplaceFiles returns the files of the local marketplace, keyed by path relative to it, with slashes: the
// plugin embedded in the binary, with the ManifestVersion of binary in its manifests, and a marketplace manifest
// for each agent.
func MarketplaceFiles(embedded map[string][]byte, binary string) (map[string][]byte, error) {
	files := map[string][]byte{}
	description := ""
	manifest := ManifestVersion(embedded, binary)
	for _, p := range walkOrder(embedded) {
		data := embedded[p]
		if path.Base(p) == "plugin.json" {
			var err error
			if data, err = setVersion(data, manifest); err != nil {
				return nil, fmt.Errorf("%s: %w", p, err)
			}
			if strings.Contains(p, ".claude-plugin") {
				var m manifestJSON
				_ = json.Unmarshal(data, &m)
				description = m.Description
			}
		}
		files[p] = data
	}

	var cm claudeMarketplace
	cm.Name = Name
	cm.Description = "The plugin of polix " + binary + ", written by polix install from its binary: do not edit."
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
func setVersion(data []byte, v string) ([]byte, error) {
	ms, err := decodeObject(data)
	if err != nil {
		return nil, err
	}
	i := slices.IndexFunc(ms, func(m member) bool { return m.key == "version" })
	if i < 0 {
		return nil, errors.New("no version")
	}
	ms[i].val = marshal(v)
	return pretty(encodeObject(ms)), nil
}

// PluginVersion returns the version of the plugin from the manifest written in the local marketplace, and whether
// it is the plugin polix. Compare it with the ManifestVersion of the binary, not with the version of the binary.
func PluginVersion(manifest []byte) (string, bool) {
	var m manifestJSON
	if json.Unmarshal(manifest, &m) != nil || m.Name != Name {
		return "", false
	}
	return m.Version, true
}

// ClaudeAnswer reads what `claude <args>` printed with --json, out, and the error of the run, failed: the outcome
// and the versions installed and available. A failure whose code is in tolerated is no error.
func ClaudeAnswer(args []string, out string, failed error, tolerated []string) (string, string, string, error) {
	var r claudeResultJSON
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if err := json.Unmarshal([]byte(lines[len(lines)-1]), &r); err != nil {
		if failed == nil {
			failed = fmt.Errorf("unexpected output: %s", strings.TrimSpace(out))
		}
		return "", "", "", fmt.Errorf("claude %s: %w", strings.Join(args, " "), failed)
	}
	if r.Outcome == "ok" || slices.Contains(tolerated, r.FailureCode) {
		return r.Outcome, r.InstalledVersion, r.AvailableVersion, nil
	}
	return r.Outcome, r.InstalledVersion, r.AvailableVersion,
		fmt.Errorf("claude %s: %s", strings.Join(args, " "), r.Message)
}

// ClaudePlugin reads what `claude plugin list --json` printed: whether the plugin polix is installed for the user,
// whether it is enabled, and the folder Claude Code reads it from.
func ClaudePlugin(list []byte) (bool, bool, string, error) {
	var all []claudePluginJSON
	if err := json.Unmarshal(list, &all); err != nil {
		return false, false, "", fmt.Errorf("claude plugin list: %w", err)
	}
	for _, p := range all {
		if p.ID == PluginID && p.Scope == "user" {
			return true, p.Enabled, p.ReadFromFolder, nil
		}
	}
	return false, false, "", nil
}

// ClaudeFolder is the folder Claude Code reads the plugin polix from, as `claude plugin list --json` printed it;
// "" when it does not say.
func ClaudeFolder(list []byte) string {
	_, _, folder, _ := ClaudePlugin(list)
	return folder
}

// ClaudeTodo is the command that registers the plugin with Claude Code, for the user to run by hand when its CLI
// is missing.
func ClaudeTodo(marketplace string) string {
	return "claude plugin marketplace add " + quote(marketplace) + " --scope user && claude plugin install " +
		PluginID + " --scope user"
}

// CodexTodo is the command that adds the marketplace to Codex, for the user to run by hand when its CLI is missing,
// or failed with failed.
func CodexTodo(marketplace string, failed error) string {
	add := "codex plugin marketplace add " + quote(marketplace)
	if failed != nil {
		return add + " (it failed: " + oneLine(failed.Error()) + ")"
	}
	return add
}

// CodexAdded reports whether adding the marketplace to Codex succeeded, or failed because it was already there:
// out is what the CLI printed and failed the error of the run.
func CodexAdded(out string, failed error) bool {
	return failed == nil || strings.Contains(strings.ToLower(out+failed.Error()), "already")
}
