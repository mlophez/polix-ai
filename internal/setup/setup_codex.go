package setup

import (
	"slices"
	"strings"
)

// codexTable is the table of ~/.codex/config.toml that enables the plugin.
const codexTable = `[plugins."` + PluginID + `"]`

// CodexEnabled reports whether the configuration of Codex, config, enables the plugin polix.
func CodexEnabled(config []byte) bool {
	lines := strings.Split(string(config), "\n")
	start, end := codexBlock(lines)
	for _, l := range lines[max(start, 0):end] {
		if k, v, ok := strings.Cut(l, "="); ok && strings.TrimSpace(k) == "enabled" {
			return strings.TrimSpace(v) == "true"
		}
	}
	return false
}

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

// CodexPlugin returns the configuration of Codex (data, which existed or not) with the plugin enabled (on) or its
// table removed, leaving the rest of the file as it is, and what that does to the file: "" when nothing changes,
// else created, updated or removed.
func CodexPlugin(data []byte, existed, on bool) ([]byte, string) {
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
	switch {
	case text == string(data):
		return nil, ""
	case text == "" && existed:
		return nil, "removed"
	case existed:
		return []byte(text), "updated"
	}
	return []byte(text), "created"
}
