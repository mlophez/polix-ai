// Package setup is the domain of what an agent needs to run polix, from the binary.
//
// For the user, the plugin polix (plugins/polix of the repository, embedded in the binary): its skills and, for
// Claude Code, its agents, which help in any repository. The plugin is written to a local marketplace,
// ~/.polix/marketplace/, and registered with each agent:
//   - Claude Code: `claude plugin marketplace add` and `claude plugin install polix@polix`, for the user. Claude Code
//     copies the plugin to its cache, so a rewrite of the folder needs `claude plugin update` to reach it.
//   - Codex: `codex plugin marketplace add` and `[plugins."polix@polix"] enabled = true` in ~/.codex/config.toml.
//
// For a repository, the hooks that call `polix hook`, which belong to the project like its polix.yaml and are
// committed with it: in <repo>/.claude/settings.json and <repo>/.codex/hooks.json, merged with every other
// setting and hook of the file, whose order is kept. Codex skips new or changed hooks until the user trusts them
// with /hooks.
//
// Everything here is pure, bytes in and bytes out: reading and writing the files, and running the CLIs of the
// agents, is the job of the adapters.
package setup

import (
	"fmt"
	"path/filepath"
	"slices"
	"strings"
)

// Setup is where polix is set up: the home of the user, the repository whose hooks to register, the agents and
// the version of the binary. It is built with a literal followed by Valid.
type Setup struct {
	Home    string   // home folder of the user: the plugin goes below it
	Root    string   // root of the repository whose hooks to register; empty registers none
	Agents  []string // agents to set up: Claude, Codex
	Version string   // version of polix, written in the manifests of the plugin
}

// Agents polix sets up.
const (
	Claude = "claude"
	Codex  = "codex"
)

// Name is the name of the plugin and of the local marketplace that holds it, so its id is polix@polix.
const Name = "polix"

// PluginID is the id of the plugin in Claude Code and Codex.
const PluginID = Name + "@" + Name

// Valid reports why s is not a setup: every agent must be Claude or Codex, and the version is known.
func (s Setup) Valid() error {
	for _, a := range s.Agents {
		if a != Claude && a != Codex {
			return fmt.Errorf("unknown agent %q (claude, codex)", a)
		}
	}
	if s.Version == "" {
		return fmt.Errorf("a setup needs the version of polix")
	}
	return nil
}

// ParseAgents reads the agents asked with --agent: trimmed, in lower case and once each; an agent other than
// Claude or Codex is an error.
func ParseAgents(asked []string) ([]string, error) {
	var out []string
	for _, a := range asked {
		a = strings.ToLower(strings.TrimSpace(a))
		if a != Claude && a != Codex {
			return nil, fmt.Errorf("unknown agent %q (claude, codex)", a)
		}
		if !slices.Contains(out, a) {
			out = append(out, a)
		}
	}
	return out, nil
}

// For reports whether the setup includes agent.
func (s Setup) For(agent string) bool { return slices.Contains(s.Agents, agent) }

// HooksFile is the file of the repository where the hooks of an agent are registered.
func (s Setup) HooksFile(agent string) string {
	if agent == Codex {
		return filepath.Join(s.Root, ".codex", "hooks.json")
	}
	return filepath.Join(s.Root, ".claude", "settings.json")
}

// MarketplaceDir is the local marketplace polix writes its plugin to.
func (s Setup) MarketplaceDir() string {
	return filepath.Join(s.Home, ".polix", "marketplace")
}

// PluginDir is the folder of the plugin inside the marketplace, which Claude Code copies to its cache on install
// and update.
func (s Setup) PluginDir() string {
	return filepath.Join(s.MarketplaceDir(), Name)
}

// PluginManifest is the manifest of the plugin written in the marketplace, which holds its version.
func (s Setup) PluginManifest() string {
	return filepath.Join(s.PluginDir(), ".claude-plugin", "plugin.json")
}

// CodexConfigFile is the configuration of Codex of the user, where the plugin is enabled.
func (s Setup) CodexConfigFile() string {
	return filepath.Join(s.Home, ".codex", "config.toml")
}

// ClaudeSettingsFiles are the settings of Claude Code that may register hooks: of the project and local when
// there is a repository, and of the user. Claude Code merges them.
func (s Setup) ClaudeSettingsFiles() []string {
	var files []string
	if s.Root != "" {
		files = append(files, s.HooksFile(Claude), filepath.Join(s.Root, ".claude", "settings.local.json"))
	}
	if s.Home != "" {
		files = append(files, filepath.Join(s.Home, ".claude", "settings.json"))
	}
	return files
}

// CodexHooksFiles are the hooks files of Codex that may register polix: of the project and of the user.
func (s Setup) CodexHooksFiles() []string {
	return []string{s.HooksFile(Codex), filepath.Join(s.Home, ".codex", "hooks.json")}
}

// Duplicates returns the settings of Claude Code, among files (path to content), other than the one of the
// repository that also register the hooks of polix: Claude Code merges the settings and would run each hook
// twice. Only a setup of Claude Code in a repository has them.
func (s Setup) Duplicates(files map[string][]byte) []string {
	if !s.For(Claude) || s.Root == "" {
		return nil
	}
	var out []string
	for _, f := range s.ClaudeSettingsFiles() {
		if data, ok := files[f]; ok && f != s.HooksFile(Claude) && Registered(data) {
			out = append(out, f)
		}
	}
	return out
}

// quote quotes a path for a command shown to the user when it holds a space.
func quote(p string) string {
	if strings.ContainsAny(p, " \t") {
		return `"` + p + `"`
	}
	return p
}

// oneLine is the first line of a message.
func oneLine(s string) string {
	s, _, _ = strings.Cut(strings.TrimSpace(s), "\n")
	return s
}

// Summary tells what install (verb write) or uninstall (verb remove) is about to do, for the person to confirm.
func (s Setup) Summary(verb string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "polix %s will %s:\n", s.Version, verb)
	for _, a := range s.Agents {
		switch a {
		case Claude:
			fmt.Fprintf(&b, "\n  Claude Code: the plugin %s, from %s\n", PluginID, s.MarketplaceDir())
		case Codex:
			fmt.Fprintf(&b, "\n  Codex: the plugin %s, enabled in %s\n", PluginID, s.CodexConfigFile())
		}
		if s.Root != "" {
			fmt.Fprintf(&b, "         the hooks of %s\n", s.HooksFile(a))
		}
	}
	return b.String()
}
