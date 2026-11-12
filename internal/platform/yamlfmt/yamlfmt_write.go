package yamlfmt

import (
	"strconv"
	"strings"
	"unicode/utf8"

	"gopkg.in/yaml.v3"
)

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
// the indentation of its first line. A spaced sequence gets one blank line before each item but the first, above
// its comments, unless the item before it ends in a block scalar that keeps its final blank lines or both items
// are written on one line.
func (p *printer) sequence(n *yaml.Node, indent int, first string) {
	if n.Style&yaml.FlowStyle != 0 || len(n.Content) == 0 {
		p.line(prefix(indent, first)+p.inline(n), p.comment(n))
		return
	}
	for i, it := range n.Content {
		if i > 0 && p.spaced[n] && !p.keeps(n.Content[i-1]) &&
			(!p.oneLine(n.Content[i-1]) || !p.oneLine(it)) {
			p.lines([]line{{}}, indent, true)
		}
		p.lines(p.lead[it], leadIndent(indent, first, i), i > 0)
		dash := spaces(indent) + "- "
		if i == 0 {
			dash = prefix(indent, first) + "- "
		}
		p.item(it, indent, dash)
		p.lines(p.trail[it], indent, true)
	}
}

// item writes the item it of a block sequence at indent after its dash, which ends in a space and may follow other
// dashes. An anchored block collection has its anchor alone after the dash and its content on the lines below, since
// its first key after the anchor would be read as the anchor of that key.
func (p *printer) item(it *yaml.Node, indent int, dash string) {
	switch {
	case block(it) && it.Anchor != "":
		p.line(dash+"&"+it.Anchor, it.LineComment)
		p.collection(it, indent+2, "")
	case block(it):
		p.pending = join(p.pending, it.LineComment)
		p.collection(it, indent+2, dash)
	case p.isBody(it):
		p.line(dash+p.head(it), p.comment(it))
		p.body(it, indent)
	default:
		p.line(dash+p.inline(it), p.comment(it))
	}
}

// collection writes the block collection n at indent, as mapping or sequence says.
func (p *printer) collection(n *yaml.Node, indent int, first string) {
	if n.Kind == yaml.MappingNode {
		p.mapping(n, indent, first)
	} else {
		p.sequence(n, indent, first)
	}
}

// head returns the header of the block scalar n, after its anchor when it has one.
func (p *printer) head(n *yaml.Node) string {
	return anchored(n, p.bodies[n].header)
}

// value writes the value v of the key whose line is head, at the indent of the key, with the line comment lc of
// the key. The anchor of a block collection goes alone at the end of that line.
func (p *printer) value(head string, v *yaml.Node, indent int, lc string) {
	switch {
	case block(v):
		if v.Anchor != "" {
			head += " &" + v.Anchor
		}
		p.line(head, join(lc, v.LineComment))
		p.collection(v, indent+2, "")
	case p.isBody(v):
		p.line(head+" "+p.head(v), join(lc, p.comment(v)))
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

// inline writes n on one line, after its anchor when it has one: a scalar, an alias or a flow collection with its
// children.
func (p *printer) inline(n *yaml.Node) string {
	if n.Kind == yaml.AliasNode {
		return "*" + n.Value
	}
	return anchored(n, p.bare(n))
}

// bare writes n on one line without its anchor.
func (p *printer) bare(n *yaml.Node) string {
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
	default:
		return p.scalar(n)
	}
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
	at := skipAnchor(src, offset(src, n.Column-1), n)
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

// keeps tells whether n ends in a block scalar with the indicator + (|+, >+), its last value as written: a blank line
// below it would be part of its content, so it counts as separated from the item after it.
func (p *printer) keeps(n *yaml.Node) bool {
	for block(n) {
		n = n.Content[len(n.Content)-1]
	}
	b, ok := p.bodies[n]
	return ok && strings.Contains(b.header, "+")
}

// oneLine tells whether the item n of a sequence is written on one line: no comment above or below it (a line
// comment is allowed), and a scalar that is not a block scalar, a flow collection, or a block mapping of one key
// whose value is one of those two. The key of such a mapping starts on the line of the dash, so no comment of its
// own goes above or below it. An anchored block collection takes more lines: its anchor goes alone on the line of the
// dash.
func (p *printer) oneLine(n *yaml.Node) bool {
	if hasComment(p.lead[n]) || hasComment(p.trail[n]) || (block(n) && n.Anchor != "") {
		return false
	}
	if n.Kind == yaml.MappingNode && block(n) && len(n.Content) == 2 {
		n = n.Content[1]
	}
	return !block(n) && !p.isBody(n)
}

// hasComment tells whether ls holds a comment, not only blank lines.
func hasComment(ls []line) bool {
	for _, l := range ls {
		if l.text != "" {
			return true
		}
	}
	return false
}

// block tells whether n is a block collection with items.
func block(n *yaml.Node) bool {
	return (n.Kind == yaml.MappingNode || n.Kind == yaml.SequenceNode) && n.Style&yaml.FlowStyle == 0 &&
		len(n.Content) > 0
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
