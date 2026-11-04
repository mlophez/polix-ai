// Package plugins embeds the plugin of polix for Claude Code and Codex (polix/), so the binary installs the plugin
// of its own version: the plugin only arrives with polix install, which writes it to a local marketplace.
package plugins

import "embed"

// FS holds the folder polix/: the manifests of both agents, skills/ and agents/.
//
//go:embed all:polix
var FS embed.FS
