package agentconf

import (
	"encoding/json"
	"errors"
	"fmt"
	"path"
	"slices"
	"strings"
)

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

// manifestJSON is the part of the manifest of a plugin that is read.
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

// Marketplace returns the files of a local marketplace that holds one plugin, name, keyed by path relative to the
// marketplace, with slashes: the files of the plugin (path below name/ to content) with version in each of its
// manifests (plugin.json), and a marketplace manifest for each agent, the one of Claude Code with description.
// A manifest of the plugin that is not a JSON object or has no version is an error.
func Marketplace(name, version, description string, plugin map[string][]byte) (map[string][]byte, error) {
	files := map[string][]byte{}
	about := ""
	for _, p := range walkOrder(plugin) {
		data := plugin[p]
		if path.Base(p) == "plugin.json" {
			var err error
			if data, err = setVersion(data, version); err != nil {
				return nil, fmt.Errorf("%s: %w", p, err)
			}
			if strings.Contains(p, ".claude-plugin") {
				var m manifestJSON
				_ = json.Unmarshal(data, &m)
				about = m.Description
			}
		}
		files[p] = data
	}

	var cm claudeMarketplace
	cm.Name = name
	cm.Description = description
	cm.Owner.Name = name
	cm.Plugins = []claudeEntry{{Name: name, Source: "./" + name, Description: about}}
	files[".claude-plugin/marketplace.json"] = pretty(marshal(cm))

	var xm codexMarketplace
	xm.Name = name
	xm.Interface.DisplayName = name
	e := codexEntry{Name: name, Category: "Coding"}
	e.Source.Source, e.Source.Path = "local", "./"+name
	e.Policy.Installation, e.Policy.Authentication = "AVAILABLE", "ON_INSTALL"
	xm.Plugins = []codexEntry{e}
	files[".agents/plugins/marketplace.json"] = pretty(marshal(xm))
	return files, nil
}

// ReadManifest returns the name and the version of the manifest of a plugin, or why it is not JSON.
func ReadManifest(manifest []byte) (string, string, error) {
	var m manifestJSON
	if err := json.Unmarshal(manifest, &m); err != nil {
		return "", "", err
	}
	return m.Name, m.Version, nil
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
