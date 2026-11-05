// Package plugins embeds the plugin of polix for Claude Code and Codex (polix/), so the binary installs the plugin
// of its own version: the plugin only arrives with polix install, which writes it to a local marketplace.
package plugins

import (
	"embed"
	"io/fs"
)

// FS holds the folder polix/: the manifests of both agents, skills/ and agents/.
//
//go:embed all:polix
var FS embed.FS

// Files returns the files of the plugin, by path below polix/ (polix/skills/plan/SKILL.md).
func Files() (map[string][]byte, error) {
	files := map[string][]byte{}
	err := fs.WalkDir(FS, "polix", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		files[p], err = FS.ReadFile(p)
		return err
	})
	return files, err
}
