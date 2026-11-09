package yamlfmt

import (
	"math"
	"strings"

	"gopkg.in/yaml.v3"
)

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
	b, content := p.bodyHeader(n, parent)
	last, content := p.bodyEnd(n.Line, parent, content)
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

// bodyHeader reads the header of the block scalar n, whose parent starts at column parent, from its line. It returns
// the body with its header and its explicit indentation, and the column of its content when the header gives it,
// else -1.
func (p *printer) bodyHeader(n *yaml.Node, parent int) (body, int) {
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
	return b, content
}

// bodyEnd returns the last line (1-based) of the content of the block scalar whose header is on the line ln and whose
// parent starts at column parent, and the column of its content: content when the header gave it (not -1), else the
// indentation of its first line that is not blank.
func (p *printer) bodyEnd(ln, parent, content int) (int, int) {
	last := ln
	for i := ln + 1; i <= len(p.src); i++ {
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
	return last, content
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
			lines = p.splitTrail(items[i-1], lines, col)
		case top:
			lines = p.splitHeader(it, lines)
		}
		p.lead[it] = relative(lines, math.MaxInt)
	}
	if top && len(items) > 0 {
		p.footer = p.splitTrail(items[len(items)-1], p.above(len(p.src)+1), col)
	}
	for _, c := range n.Content {
		if c.Kind == yaml.MappingNode || c.Kind == yaml.SequenceNode {
			p.readTrivia(c, false)
		}
	}
}

// splitTrail gives to the item prev of a collection at column col the lines up to the last comment indented deeper
// than col, as the comments below it, and returns the rest.
func (p *printer) splitTrail(prev *yaml.Node, lines []line, col int) []line {
	split := 0
	for j, l := range lines {
		if l.text != "" && l.col > col {
			split = j + 1
		}
	}
	p.trail[prev] = relative(lines[:split], col)
	return lines[split:]
}

// splitHeader gives the lines up to the last blank one to the top of the document, marks first as the first item of
// the root in the source, and returns the rest.
func (p *printer) splitHeader(first *yaml.Node, lines []line) []line {
	split := 0
	for j, l := range lines {
		if l.text == "" {
			split = j + 1
		}
	}
	p.header = lines[:split]
	p.first = first
	return lines[split:]
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
