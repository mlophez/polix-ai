package agentconf

import (
	"slices"
	"strings"
)

// CodexEnabled reports whether the configuration of Codex, config (~/.codex/config.toml), enables the plugin of
// id: its table [plugins."<id>"] sets enabled = true.
func CodexEnabled(config []byte, id string) bool {
	lines := strings.Split(string(config), "\n")
	start, end := codexBlock(lines, id)
	for _, l := range lines[max(start, 0):end] {
		if k, v, ok := strings.Cut(l, "="); ok && strings.TrimSpace(k) == "enabled" {
			return strings.TrimSpace(v) == "true"
		}
	}
	return false
}

// CodexPlugin returns the configuration of Codex (data, which existed or not) with the plugin of id enabled (on)
// or its table removed, leaving the rest of the file as it is, and what that does to the file: "" when nothing
// changes, else created, updated or removed.
func CodexPlugin(data []byte, existed bool, id string, on bool) ([]byte, string) {
	text := string(data)
	lines := strings.Split(text, "\n")
	start, end := codexBlock(lines, id)
	switch {
	case on && start < 0:
		text = addTable(text, id)
	case on:
		text = enableTable(lines, start, end)
	case start >= 0:
		text = dropTable(lines, start, end, strings.HasSuffix(text, "\n"))
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

// codexTable is the table of the configuration of Codex that enables the plugin of id.
func codexTable(id string) string { return `[plugins."` + id + `"]` }

// codexBlock returns the lines of the table of the plugin of id, [start, end); start is -1 when it is missing.
func codexBlock(lines []string, id string) (int, int) {
	table := codexTable(id)
	start := slices.IndexFunc(lines, func(l string) bool { return strings.TrimSpace(l) == table })
	if start < 0 {
		return -1, 0
	}
	end := start + 1
	for end < len(lines) && !strings.HasPrefix(strings.TrimSpace(lines[end]), "[") {
		end++
	}
	return start, end
}

// addTable appends the table of the plugin of id, enabled, to the configuration text, after a blank line.
func addTable(text, id string) string {
	if text != "" && !strings.HasSuffix(text, "\n") {
		text += "\n"
	}
	if text != "" {
		text += "\n"
	}
	return text + codexTable(id) + "\nenabled = true\n"
}

// enableTable sets enabled = true in the table of lines [start, end), adding the key when it is missing.
func enableTable(lines []string, start, end int) string {
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
	return strings.Join(lines, "\n")
}

// dropTable removes the table of lines [start, end) and the blank line before it; nothing is left of a file the
// table was all it held, and a file that ended in a newline still does.
func dropTable(lines []string, start, end int, newline bool) string {
	if start > 0 && strings.TrimSpace(lines[start-1]) == "" {
		start--
	}
	text := strings.Join(slices.Delete(lines, start, end), "\n")
	switch {
	case strings.TrimSpace(text) == "":
		return ""
	case newline && !strings.HasSuffix(text, "\n"):
		return text + "\n"
	}
	return text
}
