// Package plugins embeds the plugin of polix for Claude Code and Codex (polix/), so the binary installs the plugin
// of its own version. The marketplaces of the repository (.claude-plugin/ and .agents/plugins/ at its root) serve
// the same folder to whoever adds the repository as a marketplace.
package plugins

import "embed"

// FS holds the folder polix/: the manifests of both agents, skills/ and agents/.
//
//go:embed all:polix
var FS embed.FS
