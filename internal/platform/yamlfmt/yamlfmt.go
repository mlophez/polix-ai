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
	"math"
	"reflect"
	"slices"
	"strconv"
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
		out = append([]byte(start), out...)
		if crlf {
			out = bytes.ReplaceAll(out, []byte("\n"), []byte("\r\n"))
		}
		return out, nil
	}
	lines := strings.Split(strings.TrimSuffix(text, "\n"), "\n")
	for i, l := range lines {
		if strings.HasPrefix(l, "---") || strings.HasPrefix(l, "...") || strings.HasPrefix(l, "%") {
			return nil, fmt.Errorf("line %d: document markers and directives are %w", i+1, ErrUnsupported)
		}
	}
	var doc yaml.Node
	if err := yaml.Unmarshal([]byte(text), &doc); err != nil {
		return nil, err
	}
	if doc.Kind != yaml.DocumentNode || len(doc.Content) == 0 {
		return src, nil
	}
	root := doc.Content[0]
	if root.Kind != yaml.MappingNode && root.Kind != yaml.SequenceNode {
		return src, nil
	}
	if err := supported(root); err != nil {
		return nil, err
	}
	if err := flowComments(lines, root); err != nil {
		return nil, err
	}
	p := &printer{src: lines, claimed: make([]bool, len(lines)+1), lead: map[*yaml.Node][]line{},
		trail: map[*yaml.Node][]line{}, bodies: map[*yaml.Node]body{}}
	p.readBodies(root, 0)
	p.readTrivia(root, true)
	if order != nil {
		reorder(root, nil, order)
	}
	p.write(root)
	out := strings.Join(p.out, "\n") + "\n"
	if err := same(text, out); err != nil {
		return nil, err
	}
	if crlf {
		out = strings.ReplaceAll(out, "\n", "\r\n")
	}
	return []byte(out), nil
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
	depth, quote := 0, byte(0)
	for ; ln <= len(src); ln, at = ln+1, 0 {
		s := src[ln-1]
		for i := at; i < len(s); i++ {
			c := s[i]
			switch {
			case quote == '"' && c == '\\':
				i++
			case quote == '\'' && c == '\'' && i+1 < len(s) && s[i+1] == '\'':
				i++
			case quote != 0:
				if c == quote {
					quote = 0
				}
			case (c == '"' || c == '\'') && (i == 0 || strings.IndexByte(" \t[{,:", s[i-1]) >= 0):
				quote = c // a quote opens a scalar only at its start, not inside a plain one (it's)
			case c == '#' && (i == 0 || s[i-1] == ' ' || s[i-1] == '\t'):
				return ln, true
			case c == '[' || c == '{':
				depth++
			case c == ']' || c == '}':
				if depth--; depth == 0 {
					return 0, false
				}
			}
		}
	}
	return 0, false
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

// readBodies reads the source of every block scalar below n, whose parent starts at column parent, and claims its
// lines so no comment or blank line inside it is taken for one between items.
func (p *printer) readBodies(n *yaml.Node, parent int) {
	if n.Kind == yaml.ScalarNode && n.Style&(yaml.LiteralStyle|yaml.FoldedStyle) != 0 {
		p.readBody(n, parent)
		return
	}
	for i, c := range n.Content {
		parent := n.Column - 1 // the dash of a sequence item
		if n.Kind == yaml.MappingNode {
			parent = n.Content[i-i%2].Column - 1 // the key of the pair
		}
		p.readBodies(c, parent)
	}
}

// readBody reads the block scalar n, whose parent starts at column parent: its header from its line and its content
// from the lines below, up to the first line indented less than the content. The blank lines that end it are part of
// it only with the indicator +.
func (p *printer) readBody(n *yaml.Node, parent int) {
	src := p.src[n.Line-1]
	at := offset(src, n.Column-1)
	end := at + 1
	for end < len(src) && strings.ContainsRune("-+0123456789", rune(src[end])) {
		end++
	}
	b := body{header: src[at:end]}
	content := -1
	if i := strings.IndexAny(b.header, "123456789"); i >= 0 {
		b.indent = int(b.header[i] - '0')
		content = parent + b.indent
	}
	last := n.Line
	for i := n.Line + 1; i <= len(p.src); i++ {
		l := p.src[i-1]
		ind := indentOf(l)
		if strings.TrimSpace(l) == "" {
			continue
		}
		if content < 0 {
			if ind <= parent {
				break
			}
			content = ind
		}
		if ind < content {
			break
		}
		last = i
	}
	if strings.Contains(b.header, "+") {
		for last < len(p.src) && strings.TrimSpace(p.src[last]) == "" {
			last++
		}
	}
	for i := n.Line + 1; i <= last; i++ {
		l := p.src[i-1]
		p.claimed[i] = true
		if len(l) > content {
			b.lines = append(b.lines, l[content:])
		} else {
			b.lines = append(b.lines, "")
		}
	}
	p.bodies[n] = b
}

// readTrivia gives the comment and blank lines of the source to the items of the block collections below n, in
// source order, outer items first: each item takes the lines right above it; those indented deeper than the item
// go below the item before it. top tells whether n is the root, whose first item leaves the lines up to the last
// blank one to the top of the document, and whose last item takes the deeper comments that end the document.
func (p *printer) readTrivia(n *yaml.Node, top bool) {
	if n.Style&yaml.FlowStyle != 0 {
		return
	}
	items := p.items(n)
	col := n.Column - 1
	for i, it := range items {
		lines := p.above(it.Line)
		switch {
		case i > 0:
			split := 0
			for j, l := range lines {
				if l.text != "" && l.col > col {
					split = j + 1
				}
			}
			p.trail[items[i-1]] = relative(lines[:split], col)
			lines = lines[split:]
		case top:
			split := 0
			for j, l := range lines {
				if l.text == "" {
					split = j + 1
				}
			}
			p.header = lines[:split]
			p.first = it
			lines = lines[split:]
		}
		p.lead[it] = relative(lines, math.MaxInt)
	}
	if top && len(items) > 0 {
		lines := p.above(len(p.src) + 1)
		split := 0
		for j, l := range lines {
			if l.text != "" && l.col > col {
				split = j + 1
			}
		}
		p.trail[items[len(items)-1]] = relative(lines[:split], col)
		p.footer = lines[split:]
	}
	for _, c := range n.Content {
		if c.Kind == yaml.MappingNode || c.Kind == yaml.SequenceNode {
			p.readTrivia(c, false)
		}
	}
}

// items returns the items of the block collection n in source order, as the nodes the lines are given to and whose
// line each item starts on: the key of a pair, the node of a sequence item.
func (p *printer) items(n *yaml.Node) []*yaml.Node {
	var items []*yaml.Node
	switch n.Kind {
	case yaml.MappingNode:
		for i := 0; i < len(n.Content); i += 2 {
			items = append(items, n.Content[i])
		}
	case yaml.SequenceNode:
		items = append(items, n.Content...)
	}
	return items
}

// above returns the unclaimed comment and blank lines right above the line at (1-based), top to bottom, and claims
// them.
func (p *printer) above(at int) []line {
	first := at
	for first > 1 && !p.claimed[first-1] && trivia(p.src[first-2]) {
		first--
	}
	var lines []line
	for i := first; i < at; i++ {
		p.claimed[i] = true
		l := p.src[i-1]
		lines = append(lines, line{text: strings.TrimSpace(l), col: indentOf(l)})
	}
	return lines
}

// write writes the root collection n, between the comments at the top and at the end of the document. When the
// order moved the first item of the source and no blank line set the top of the document apart, the comments right
// above that item stay at the top: they most often speak of the whole document.
func (p *printer) write(n *yaml.Node) {
	if p.first != nil && n.Content[0] != p.first && len(p.header) == 0 {
		p.header = append(p.header, p.lead[p.first]...)
		p.lead[p.first] = nil
	}
	p.lines(p.header, 0, false)
	p.pending = n.LineComment
	if n.Kind == yaml.MappingNode {
		p.mapping(n, 0, "")
	} else {
		p.sequence(n, 0, "")
	}
	p.lines(p.footer, 0, true)
	for len(p.out) > 0 && p.out[len(p.out)-1] == "" {
		p.out, p.code = p.out[:len(p.out)-1], p.code[:len(p.code)-1]
	}
	p.align()
}

// align lines up the line comments of consecutive lines with the same indentation, one space after the longest
// content among them.
func (p *printer) align() {
	for i := 0; i < len(p.out); {
		j := i
		for j < len(p.out) && p.code[j] >= 0 && indentOf(p.out[j]) == indentOf(p.out[i]) {
			j++
		}
		if j == i {
			i++
			continue
		}
		width := 0
		for k := i; k < j; k++ {
			width = max(width, utf8.RuneCountInString(p.out[k][:p.code[k]]))
		}
		for k := i; k < j; k++ {
			code, comment := p.out[k][:p.code[k]], p.out[k][p.code[k]+1:]
			p.out[k] = code + spaces(width-utf8.RuneCountInString(code)+1) + comment
		}
		i = j
	}
}

// mapping writes the block mapping n at indent; a non-empty first replaces the indentation of its first line (the
// dash of a sequence item).
func (p *printer) mapping(n *yaml.Node, indent int, first string) {
	if n.Style&yaml.FlowStyle != 0 || len(n.Content) == 0 {
		p.line(prefix(indent, first)+p.inline(n), p.comment(n))
		return
	}
	for i := 0; i+1 < len(n.Content); i += 2 {
		k, v := n.Content[i], n.Content[i+1]
		p.lines(p.lead[k], leadIndent(indent, first, i), i > 0)
		head := spaces(indent)
		if i == 0 {
			head = prefix(indent, first)
		}
		p.value(head+p.scalar(k)+":", v, indent, p.comment(k))
		p.lines(p.trail[k], indent, true)
	}
}

// sequence writes the block sequence n at indent, its items indented below their key; a non-empty first replaces
// the indentation of its first line.
func (p *printer) sequence(n *yaml.Node, indent int, first string) {
	if n.Style&yaml.FlowStyle != 0 || len(n.Content) == 0 {
		p.line(prefix(indent, first)+p.inline(n), p.comment(n))
		return
	}
	for i, it := range n.Content {
		p.lines(p.lead[it], leadIndent(indent, first, i), i > 0)
		dash := spaces(indent) + "- "
		if i == 0 {
			dash = prefix(indent, first) + "- "
		}
		switch {
		case block(it):
			p.pending = join(p.pending, it.LineComment)
			if it.Kind == yaml.MappingNode {
				p.mapping(it, indent+2, dash)
			} else {
				p.sequence(it, indent+2, dash)
			}
		case p.isBody(it):
			p.line(dash+p.bodies[it].header, p.comment(it))
			p.body(it, indent)
		default:
			p.line(dash+p.inline(it), p.comment(it))
		}
		p.lines(p.trail[it], indent, true)
	}
}

// value writes the value v of the key whose line is head, at the indent of the key, with the line comment lc of
// the key.
func (p *printer) value(head string, v *yaml.Node, indent int, lc string) {
	switch {
	case block(v):
		p.line(head, join(lc, v.LineComment))
		if v.Kind == yaml.MappingNode {
			p.mapping(v, indent+2, "")
		} else {
			p.sequence(v, indent+2, "")
		}
	case p.isBody(v):
		p.line(head+" "+p.bodies[v].header, join(lc, p.comment(v)))
		p.body(v, indent)
	default:
		if text := p.inline(v); text != "" {
			head += " " + text
		}
		p.line(head, join(lc, p.comment(v)))
	}
}

// body writes the content of the block scalar n whose parent is at indent: at its explicit indentation, else two
// spaces deeper.
func (p *printer) body(n *yaml.Node, indent int) {
	b := p.bodies[n]
	at := indent + 2
	if b.indent > 0 {
		at = indent + b.indent
	}
	for _, l := range b.lines {
		if l == "" {
			p.raw("")
		} else {
			p.raw(spaces(at) + l)
		}
	}
}

// inline writes n on one line: a scalar, or a flow collection with its children.
func (p *printer) inline(n *yaml.Node) string {
	switch n.Kind {
	case yaml.MappingNode:
		if len(n.Content) == 0 {
			return "{}"
		}
		parts := make([]string, 0, len(n.Content)/2)
		for i := 0; i+1 < len(n.Content); i += 2 {
			part := p.scalar(n.Content[i]) + ":"
			if v := p.inline(n.Content[i+1]); v != "" {
				part += " " + v
			}
			parts = append(parts, part)
		}
		return "{ " + strings.Join(parts, ", ") + " }"
	case yaml.SequenceNode:
		parts := make([]string, 0, len(n.Content))
		for _, c := range n.Content {
			parts = append(parts, p.inline(c))
		}
		return "[" + strings.Join(parts, ", ") + "]"
	}
	return p.scalar(n)
}

// scalar writes the scalar n in its style: a plain one as its value (nothing for an empty null), a quoted one as its
// source when it fits on one line, else quoted again.
func (p *printer) scalar(n *yaml.Node) string {
	switch {
	case n.Style&yaml.DoubleQuotedStyle != 0:
		if s, ok := p.quoted(n, '"'); ok {
			return s
		}
		return strconv.Quote(n.Value)
	case n.Style&yaml.SingleQuotedStyle != 0:
		if s, ok := p.quoted(n, '\''); ok {
			return s
		}
		if strings.Contains(n.Value, "\n") {
			return strconv.Quote(n.Value)
		}
		return "'" + strings.ReplaceAll(n.Value, "'", "''") + "'"
	case strings.Contains(n.Value, "\n"):
		return strconv.Quote(n.Value)
	case n.Value == "" && n.Tag == "!!null":
		return ""
	}
	return n.Value
}

// quoted returns the source of the scalar n quoted with q when it opens and closes on its line.
func (p *printer) quoted(n *yaml.Node, q byte) (string, bool) {
	if n.Line < 1 || n.Line > len(p.src) {
		return "", false
	}
	src := p.src[n.Line-1]
	at := offset(src, n.Column-1)
	if at >= len(src) || src[at] != q {
		return "", false
	}
	for i := at + 1; i < len(src); i++ {
		switch {
		case q == '"' && src[i] == '\\':
			i++
		case src[i] == q && q == '\'' && i+1 < len(src) && src[i+1] == '\'':
			i++
		case src[i] == q:
			return src[at : i+1], true
		}
	}
	return "", false
}

// comment returns the line comment of n and, for a flow collection, of every node inside it, each after a space.
func (p *printer) comment(n *yaml.Node) string {
	lc := ""
	if n.Style&yaml.FlowStyle != 0 {
		for _, c := range n.Content {
			lc = join(lc, p.comment(c))
		}
	}
	return join(lc, n.LineComment)
}

// isBody tells whether n is a block scalar read from the source.
func (p *printer) isBody(n *yaml.Node) bool {
	_, ok := p.bodies[n]
	return ok
}

// lines writes comment and blank lines at indent plus their column: never a blank line first in a collection
// (inner false) nor two in a row.
func (p *printer) lines(ls []line, indent int, inner bool) {
	for _, l := range ls {
		if l.text == "" {
			if inner && len(p.out) > 0 && p.out[len(p.out)-1] != "" {
				p.raw("")
			}
			continue
		}
		inner = true
		p.raw(spaces(indent+l.col) + l.text)
	}
}

// line writes a line of content code with its line comment, after the pending one if any; both may be empty.
func (p *printer) line(code, comment string) {
	comment = strings.TrimSpace(join(p.pending, comment))
	p.pending = ""
	code = strings.TrimRight(code, " ")
	if comment == "" {
		p.raw(code)
		return
	}
	p.out = append(p.out, code+" "+comment)
	p.code = append(p.code, len(code))
}

// raw writes a line without a line comment to align.
func (p *printer) raw(s string) {
	p.out = append(p.out, s)
	p.code = append(p.code, -1)
}

// block tells whether n is a block collection with items.
func block(n *yaml.Node) bool {
	return (n.Kind == yaml.MappingNode || n.Kind == yaml.SequenceNode) && n.Style&yaml.FlowStyle == 0 &&
		len(n.Content) > 0
}

// relative makes the column of each line relative to col, the column of the item they go below, never below 0; a
// blank line keeps column 0. With math.MaxInt every line lines up with its item.
func relative(ls []line, col int) []line {
	out := make([]line, 0, len(ls))
	for _, l := range ls {
		if l.text != "" {
			l.col = max(l.col-col, 0)
		} else {
			l.col = 0
		}
		out = append(out, l)
	}
	return out
}

// join appends the comment c to s after a space; either may be empty.
func join(s, c string) string {
	switch {
	case c == "":
		return s
	case s == "":
		return " " + strings.TrimSpace(c)
	}
	return s + " " + strings.TrimSpace(c)
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

// leadIndent returns the indentation of the comments above the item i of a collection at indent: those of the first
// item of a collection that starts on the line of a dash (first set) go above the dash, at its column, where a
// second pass reads them again as comments of the sequence item, so the output is stable.
func leadIndent(indent int, first string, i int) int {
	if i == 0 && first != "" {
		return indent - 2
	}
	return indent
}

// prefix returns first when it is set, else the indentation of indent.
func prefix(indent int, first string) string {
	if first != "" {
		return first
	}
	return spaces(indent)
}

// spaces returns n spaces.
func spaces(n int) string {
	return strings.Repeat(" ", n)
}
