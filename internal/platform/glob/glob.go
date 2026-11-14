// Package glob matches slash separated paths and command lines against patterns.
//
// Path patterns use slash separated, repository relative paths: `*` matches inside one path segment, `**`
// matches any number of segments and `?` one character. A pattern without `/` also matches the base name of a
// path anywhere in the tree, like .gitignore, so `*.md` matches `docs/guide.md`. A leading `/` anchors the pattern
// to the root, also like .gitignore: `/polix.yaml` matches `polix.yaml`, not `examples/polix.yaml`.
package glob

import (
	"path"
	"regexp"
	"strings"
	"sync"
)

// cache keeps compiled patterns: the hook evaluates the same few patterns on every event.
var cache sync.Map

// Path reports whether the repository relative path p matches pattern.
func Path(pattern, p string) bool {
	p = strings.TrimPrefix(p, "./")
	if rooted, ok := strings.CutPrefix(pattern, "/"); ok {
		return compile(rooted, true).MatchString(strings.TrimPrefix(p, "/"))
	}
	if compile(pattern, true).MatchString(p) {
		return true
	}
	if !strings.Contains(pattern, "/") {
		return compile(pattern, true).MatchString(path.Base(p))
	}
	return false
}

// AnyPath reports whether p matches at least one of the patterns.
func AnyPath(patterns []string, p string) bool {
	for _, pattern := range patterns {
		if Path(pattern, p) {
			return true
		}
	}
	return false
}

// Folder returns the folder a path pattern looks inside: the longest leading run of its segments without
// wildcards, slash separated and relative to the root (`ai/plans` for `ai/plans/{key}-*.md`). A leading `/` is
// trimmed first, and a pattern without wildcards returns the folder that holds its file (`.git` for `.git/HEAD`).
// It returns "" when the pattern can match anywhere in the tree: its first segment has a wildcard, or it has no
// folder.
func Folder(pattern string) string {
	segs := strings.Split(strings.TrimPrefix(pattern, "/"), "/")
	n := 0
	for n < len(segs)-1 && !strings.ContainsAny(segs[n], "*?") {
		n++
	}
	return strings.Join(segs[:n], "/")
}

// Command reports whether a command line matches pattern, where `*` matches any text, spaces, slashes and line
// breaks included, so a pattern reaches the lines of a quoted message or a here-document. Surrounding spaces of the
// command are ignored.
func Command(pattern, command string) bool {
	return compile(pattern, false).MatchString(strings.TrimSpace(command))
}

// compile turns a pattern into an anchored regular expression; paths decides whether `*` stops at `/`. A command
// spans lines, so in a command pattern `*` and `?` match a line break too.
func compile(pattern string, paths bool) *regexp.Regexp {
	key, start := "c:"+pattern, "(?s)^"
	if paths {
		key, start = "p:"+pattern, "^"
	}
	if v, ok := cache.Load(key); ok {
		if re, ok := v.(*regexp.Regexp); ok {
			return re
		}
	}
	var b strings.Builder
	b.WriteString(start)
	for i := 0; i < len(pattern); i++ {
		c := pattern[i]
		switch {
		case paths && strings.HasPrefix(pattern[i:], "**/"):
			b.WriteString("(?:.*/)?")
			i += 2
		case paths && strings.HasPrefix(pattern[i:], "**"):
			b.WriteString(".*")
			i++
		case c == '*' && paths:
			b.WriteString("[^/]*")
		case c == '*':
			b.WriteString(".*")
		case c == '?' && paths:
			b.WriteString("[^/]")
		case c == '?':
			b.WriteString(".")
		default:
			b.WriteString(regexp.QuoteMeta(string(c)))
		}
	}
	b.WriteString("$")
	re := regexp.MustCompile(b.String())
	cache.Store(key, re)
	return re
}
