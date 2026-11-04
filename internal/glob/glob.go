// Package glob matches repository paths and command lines against the patterns of polix.yaml and
// ai/rules/.
//
// Path patterns use slash separated, repository relative paths: `*` matches inside one path segment, `**`
// matches any number of segments and `?` one character. A pattern without `/` also matches the base name of a
// path anywhere in the tree, like .gitignore, so `*.md` matches `docs/guide.md`.
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

// Command reports whether a command line matches pattern, where `*` matches any text, spaces and slashes
// included. Surrounding spaces of the command are ignored.
func Command(pattern, command string) bool {
	return compile(pattern, false).MatchString(strings.TrimSpace(command))
}

// compile turns a pattern into an anchored regular expression; paths decides whether `*` stops at `/`.
func compile(pattern string, paths bool) *regexp.Regexp {
	key := "c:" + pattern
	if paths {
		key = "p:" + pattern
	}
	if re, ok := cache.Load(key); ok {
		return re.(*regexp.Regexp)
	}
	var b strings.Builder
	b.WriteString("^")
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
