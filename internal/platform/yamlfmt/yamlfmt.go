// Package yamlfmt formats a YAML document in one layout while it keeps every comment, the blank lines that
// separate its items and the style of each node: two spaces of indentation, sequences indented below their key,
// flow collections on one line ({ a: b } and [a, b]), one space before a line comment and at most one blank line in
// a row. The keys of each mapping follow the order a caller gives; the items of a sequence never move.
//
// The comments and the blank lines are read from the source lines, not from the comments yaml.v3 attaches to the
// nodes, which it spreads between head, line and foot comments in ways that do not survive a reorder. A comment or
// blank line above an item travels with it; a comment below the last line of an item, indented deeper than the next
// item, stays below it. Quoted scalars on one line and block scalars (| and >) keep their source text.
//
// A leading document start marker (---) stays as it is. Format refuses what it cannot rewrite safely: anchors,
// aliases, explicit tags, complex keys, comments inside a flow collection, directives and any other document marker.
// It checks its own result too: the output must decode to the same data and hold the same comments, or Format fails
// instead of returning it.
package yamlfmt

import (
	"bytes"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"unicode/utf8"

	"gopkg.in/yaml.v3"
)

// Order returns the order of the keys of the mapping at path: the keys from the root to the mapping, with "-" for
// the item of a sequence. The keys it lists come first, in its order; the rest follow in their order in the
// source. Nil keeps the order of the source.
type Order func(path []string) []string

// line is a comment or blank line of the source: its text without the indentation (empty for a blank line) and its
// column, relative to the item it belongs to for the lines kept below an item.
type line struct {
	text string
	col  int
}

// body is the source of a block scalar: its header (| or > with its indicators), the explicit indentation of its
// content (0 without it) and its lines without the indentation of the content (empty for a blank line).
type body struct {
	header string
	indent int
	lines  []string
}

// printer formats one document: the source lines, what was read from them and the output being written.
type printer struct {
	src     []string              // the source lines, without line breaks
	claimed []bool                // the source lines already given to an item or to a block scalar
	lead    map[*yaml.Node][]line // the comments and blank lines above each item, at the column of the item
	trail   map[*yaml.Node][]line // the comments below each item, indented deeper than the item after it
	bodies  map[*yaml.Node]body   // the source of each block scalar
	header  []line                // the comments at the top of the document, before its first blank line
	first   *yaml.Node            // the first item of the root in the source
	footer  []line                // the comments at the end of the document
	out     []string              // the lines written
	code    []int                 // per line written, the length of its content before a line comment; -1 for none
	pending string                // a line comment for the next line written
}

// ErrUnsupported is the error of a document Format cannot rewrite safely.
var ErrUnsupported = errors.New("not supported by the formatter")

// Format formats the YAML document src, ordering the keys of each mapping with order (nil keeps them all). A
// document without content (empty, or only comments) comes back as it is. Line breaks \r\n are kept.
func Format(src []byte, order Order) ([]byte, error) {
	crlf := bytes.Contains(src, []byte("\r\n"))
	text := strings.ReplaceAll(string(src), "\r\n", "\n")
	if start, rest, ok := documentStart(text); ok {
		out, err := Format([]byte(rest), order)
		if err != nil {
			return nil, err
		}
		return breaks(append([]byte(start), out...), crlf), nil
	}
	lines, root, err := parse(text)
	if err != nil {
		return nil, err
	}
	if root == nil {
		return src, nil
	}
	out := rewrite(lines, root, order)
	if err := same(text, out); err != nil {
		return nil, err
	}
	return breaks([]byte(out), crlf), nil
}

// breaks turns the line breaks of out back into \r\n when crlf is set.
func breaks(out []byte, crlf bool) []byte {
	if crlf {
		return bytes.ReplaceAll(out, []byte("\n"), []byte("\r\n"))
	}
	return out
}

// parse splits text into its lines and decodes its root collection, after it checks that Format can rewrite it. The
// root is nil for a document without content or whose root is a scalar, which Format leaves as it is.
func parse(text string) ([]string, *yaml.Node, error) {
	lines := strings.Split(strings.TrimSuffix(text, "\n"), "\n")
	if err := markers(lines); err != nil {
		return nil, nil, err
	}
	var doc yaml.Node
	if err := yaml.Unmarshal([]byte(text), &doc); err != nil {
		return nil, nil, err
	}
	if doc.Kind != yaml.DocumentNode || len(doc.Content) == 0 {
		return lines, nil, nil
	}
	root := doc.Content[0]
	if root.Kind != yaml.MappingNode && root.Kind != yaml.SequenceNode {
		return lines, nil, nil
	}
	if err := supported(root); err != nil {
		return nil, nil, err
	}
	if err := flowComments(lines, root); err != nil {
		return nil, nil, err
	}
	return lines, root, nil
}

// markers fails on a document marker or a directive at the start of a line of the source.
func markers(lines []string) error {
	for i, l := range lines {
		if strings.HasPrefix(l, "---") || strings.HasPrefix(l, "...") || strings.HasPrefix(l, "%") {
			return fmt.Errorf("line %d: document markers and directives are %w", i+1, ErrUnsupported)
		}
	}
	return nil
}

// rewrite reads the comments, blank lines and block scalars of the source lines, orders the keys below root and
// writes the document again.
func rewrite(lines []string, root *yaml.Node, order Order) string {
	p := &printer{src: lines, claimed: make([]bool, len(lines)+1), lead: map[*yaml.Node][]line{},
		trail: map[*yaml.Node][]line{}, bodies: map[*yaml.Node]body{}}
	p.readBodies(root, 0)
	p.readTrivia(root, true)
	if order != nil {
		reorder(root, nil, order)
	}
	p.write(root)
	return strings.Join(p.out, "\n") + "\n"
}

// documentStart splits text at a leading document start marker: the comment and blank lines before it and the
// marker itself, kept as they are, and the rest of the document. ok is false without one, or when the marker has
// content on its line.
func documentStart(text string) (start, rest string, ok bool) {
	at := 0
	for at < len(text) {
		end := strings.IndexByte(text[at:], '\n')
		if end < 0 {
			end = len(text) - at
		}
		l := text[at : at+end]
		switch {
		case strings.TrimRight(l, " ") == "---":
			next := min(at+end+1, len(text))
			return text[:at] + "---\n", text[next:], true
		case !trivia(l):
			return "", "", false
		}
		at += end + 1
	}
	return "", "", false
}

// supported fails on what Format cannot rewrite safely: anchors and aliases, which a reorder may break, explicit
// tags and keys that are not scalars.
func supported(n *yaml.Node) error {
	switch {
	case n.Kind == yaml.AliasNode || n.Anchor != "":
		return fmt.Errorf("line %d: anchors and aliases are %w", n.Line, ErrUnsupported)
	case n.Style&yaml.TaggedStyle != 0:
		return fmt.Errorf("line %d: explicit tags are %w", n.Line, ErrUnsupported)
	}
	for i, c := range n.Content {
		if n.Kind == yaml.MappingNode && i%2 == 0 && c.Kind != yaml.ScalarNode {
			return fmt.Errorf("line %d: keys that are not scalars are %w", c.Line, ErrUnsupported)
		}
		if err := supported(c); err != nil {
			return err
		}
	}
	return nil
}

// reorder sorts the keys of every mapping below n, at path, as order says, keeping the rest in source order.
func reorder(n *yaml.Node, path []string, order Order) {
	switch n.Kind {
	case yaml.MappingNode:
		known := order(path)
		type pair struct{ k, v *yaml.Node }
		pairs := make([]pair, 0, len(n.Content)/2)
		for i := 0; i+1 < len(n.Content); i += 2 {
			pairs = append(pairs, pair{n.Content[i], n.Content[i+1]})
		}
		rank := func(p pair) int {
			if i := slices.Index(known, p.k.Value); i >= 0 {
				return i
			}
			return len(known)
		}
		slices.SortStableFunc(pairs, func(a, b pair) int { return rank(a) - rank(b) })
		content := make([]*yaml.Node, 0, len(n.Content))
		for _, p := range pairs {
			content = append(content, p.k, p.v)
			reorder(p.v, append(slices.Clone(path), p.k.Value), order)
		}
		n.Content = content
	case yaml.SequenceNode:
		for _, c := range n.Content {
			reorder(c, append(slices.Clone(path), "-"), order)
		}
	default:
	}
}

// same fails when out does not decode to the data of src or does not hold the same comments.
func same(src, out string) error {
	var a, b any
	if err := yaml.Unmarshal([]byte(src), &a); err != nil {
		return err
	}
	if err := yaml.Unmarshal([]byte(out), &b); err != nil {
		return fmt.Errorf("the formatted document does not parse: %w", err)
	}
	if !reflect.DeepEqual(a, b) {
		return errors.New("the formatted document would change the data: left as it is")
	}
	var na, nb yaml.Node
	_ = yaml.Unmarshal([]byte(src), &na)
	_ = yaml.Unmarshal([]byte(out), &nb)
	ca, cb := comments(&na, nil), comments(&nb, nil)
	slices.Sort(ca)
	slices.Sort(cb)
	if !slices.Equal(ca, cb) {
		return errors.New("the formatted document would lose a comment: left as it is")
	}
	return nil
}

// comments gathers every comment line yaml.v3 read below n.
func comments(n *yaml.Node, acc []string) []string {
	for _, c := range []string{n.HeadComment, n.LineComment, n.FootComment} {
		for l := range strings.SplitSeq(c, "\n") {
			if l = strings.TrimSpace(l); l != "" {
				acc = append(acc, l)
			}
		}
	}
	for _, c := range n.Content {
		acc = comments(c, acc)
	}
	return acc
}

// trivia tells whether the source line s is blank or only a comment.
func trivia(s string) bool {
	t := strings.TrimSpace(s)
	return t == "" || strings.HasPrefix(t, "#")
}

// indentOf returns the number of leading spaces of s.
func indentOf(s string) int {
	return len(s) - len(strings.TrimLeft(s, " "))
}

// offset returns the byte offset of the rune at column col of s, as yaml.v3 counts columns.
func offset(s string, col int) int {
	at := 0
	for range col {
		if at >= len(s) {
			break
		}
		_, size := utf8.DecodeRuneInString(s[at:])
		at += size
	}
	return at
}
