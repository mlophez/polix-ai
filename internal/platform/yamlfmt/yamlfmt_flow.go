package yamlfmt

import (
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

// flowScan is the state of the scan of the source of a flow collection: the depth of its brackets and the quote of
// the scalar it is inside, 0 outside quotes.
type flowScan struct {
	depth int
	quote byte
}

// flowComments fails on a comment inside a flow collection below n, which a flow written on one line cannot keep
// and yaml.v3 does not always report: it scans the source of each outermost flow collection, from its opening
// bracket to its closing one, outside quotes.
func flowComments(src []string, n *yaml.Node) error {
	if n.Style&yaml.FlowStyle != 0 && (n.Kind == yaml.MappingNode || n.Kind == yaml.SequenceNode) {
		if line, ok := flowComment(src, n.Line, offset(src[n.Line-1], n.Column-1)); ok {
			return fmt.Errorf("line %d: comments inside a flow collection are %w", line, ErrUnsupported)
		}
		return nil
	}
	for _, c := range n.Content {
		if err := flowComments(src, c); err != nil {
			return err
		}
	}
	return nil
}

// flowComment scans the flow collection that opens at byte at of the line ln (1-based) up to its closing bracket,
// and returns the line of the first comment inside it.
func flowComment(src []string, ln, at int) (int, bool) {
	var f flowScan
	for ; ln <= len(src); ln, at = ln+1, 0 {
		comment, closed := f.line(src[ln-1], at)
		if comment {
			return ln, true
		}
		if closed {
			return 0, false
		}
	}
	return 0, false
}

// opensQuote tells whether the byte i of s opens a quoted scalar: a quote opens a scalar only at its start, not
// inside a plain one (it's).
func opensQuote(s string, i int) bool {
	c := s[i]
	return (c == '"' || c == '\'') && (i == 0 || strings.IndexByte(" \t[{,:", s[i-1]) >= 0)
}

// opensComment tells whether the byte i of s starts a comment: a # at the start of the line or after a blank.
func opensComment(s string, i int) bool {
	return s[i] == '#' && (i == 0 || s[i-1] == ' ' || s[i-1] == '\t')
}

// line scans the line s from byte at: comment tells whether a comment starts on it inside the collection, closed
// whether the collection closes on it first.
func (f *flowScan) line(s string, at int) (comment, closed bool) {
	for i := at; i < len(s); i++ {
		if f.quote != 0 {
			i = f.quoted(s, i)
			continue
		}
		switch c := s[i]; {
		case opensQuote(s, i):
			f.quote = c
		case opensComment(s, i):
			return true, false
		case c == '[' || c == '{':
			f.depth++
		case c == ']' || c == '}':
			if f.depth--; f.depth == 0 {
				return false, true
			}
		}
	}
	return false, false
}

// quoted scans the byte i of s inside a quoted scalar and returns the last byte it took: an escape of a double
// quoted scalar and a doubled quote of a single quoted one take two bytes, and the closing quote ends the scalar.
func (f *flowScan) quoted(s string, i int) int {
	c := s[i]
	switch {
	case f.quote == '"' && c == '\\':
		return i + 1
	case f.quote == '\'' && c == '\'' && i+1 < len(s) && s[i+1] == '\'':
		return i + 1
	case c == f.quote:
		f.quote = 0
	}
	return i
}
