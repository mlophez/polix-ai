// Package text shortens texts for logs and reports: a string cut to a byte limit with a marker, and the last
// lines of a text, each one cut.
package text

import (
	"strings"
	"unicode/utf8"
)

// Mark ends a text that Cut shortened.
const Mark = "… [recortado]"

// Cut shortens s to at most limit bytes, on a rune boundary, marking the cut.
func Cut(s string, limit int) string {
	if len(s) <= limit {
		return s
	}
	n := limit - len(Mark)
	if n < 0 {
		n = 0
	}
	for n > 0 && !utf8.RuneStart(s[n]) {
		n--
	}
	return s[:n] + Mark
}

// Tail keeps the last n non-empty lines of s, each cut to width bytes.
func Tail(s string, n, width int) string {
	var lines []string
	for l := range strings.SplitSeq(s, "\n") {
		if strings.TrimSpace(l) != "" {
			lines = append(lines, Cut(l, width))
		}
	}
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.Join(lines, "\n")
}

// LastLines keeps the last n lines of s, without its trailing newlines, each cut to width bytes; empty lines
// inside the text are kept.
func LastLines(s string, n, width int) string {
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	for i, l := range lines {
		lines[i] = Cut(l, width)
	}
	return strings.Join(lines, "\n")
}
