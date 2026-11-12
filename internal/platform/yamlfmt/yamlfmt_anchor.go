package yamlfmt

import (
	"fmt"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"
)

// refusal is an error of Format with a text of its own that is also ErrUnsupported.
type refusal string

// Error returns the text of the refusal.
func (r refusal) Error() string { return string(r) }

// Is tells whether target is ErrUnsupported.
func (r refusal) Is(target error) bool { return target == ErrUnsupported }

// supportedAnchor fails on an anchor that Format cannot keep: one on the root collection and one whose name was
// defined before, in source order. root tells whether n is the root; seen holds the names defined so far and gets the
// one of n.
func supportedAnchor(n *yaml.Node, root bool, seen map[string]bool) error {
	switch {
	case n.Anchor == "":
		return nil
	case root:
		return fmt.Errorf("line %d: an anchor on the whole document is %w", n.Line, ErrUnsupported)
	case seen[n.Anchor]:
		return refusal(fmt.Sprintf("line %d: the anchor &%s is defined twice: rename one", n.Line, n.Anchor))
	}
	seen[n.Anchor] = true
	return nil
}

// supportedKey fails on a key of a mapping, with value v, that Format cannot keep: one with an anchor or an alias,
// which would anchor the word and not the entry, one that is not a scalar and a merge key that uses an anchor or an
// alias.
func supportedKey(k, v *yaml.Node) error {
	switch {
	case k.Anchor != "" || k.Kind == yaml.AliasNode:
		return fmt.Errorf("line %d: anchors and aliases on keys are %w", k.Line, ErrUnsupported)
	case k.Kind != yaml.ScalarNode:
		return fmt.Errorf("line %d: keys that are not scalars are %w", k.Line, ErrUnsupported)
	case k.Tag == "!!merge" && usesAnchors(v):
		return fmt.Errorf("line %d: anchors and aliases are %w", k.Line, ErrUnsupported)
	}
	return nil
}

// usesAnchors tells whether n or a node below it has an anchor or is an alias.
func usesAnchors(n *yaml.Node) bool {
	return n.Anchor != "" || n.Kind == yaml.AliasNode || slices.ContainsFunc(n.Content, usesAnchors)
}

// anchored returns text, the text of n, after its anchor when it has one.
func anchored(n *yaml.Node, text string) string {
	switch {
	case n.Anchor == "":
		return text
	case text == "":
		return "&" + n.Anchor
	}
	return "&" + n.Anchor + " " + text
}

// skipAnchor returns the byte offset in the source line src after the anchor of n, and the blanks that follow it,
// when the anchor starts at the byte at; else at. yaml.v3 gives an anchored node the position of its anchor.
func skipAnchor(src string, at int, n *yaml.Node) int {
	if n.Anchor == "" || !strings.HasPrefix(src[at:], "&"+n.Anchor) {
		return at
	}
	at += 1 + len(n.Anchor)
	return at + len(src[at:]) - len(strings.TrimLeft(src[at:], " \t"))
}

// column returns the column (0-based) of the first item of the block collection n: the one of n, except when it is
// anchored, since yaml.v3 then gives it the column of its anchor. The first key of an anchored mapping has it; the
// first dash of an anchored sequence is on the first line below its anchor that is not a comment or blank.
func (p *printer) column(n *yaml.Node) int {
	switch {
	case n.Anchor == "":
		return n.Column - 1
	case n.Kind == yaml.MappingNode && len(n.Content) > 0:
		return n.Content[0].Column - 1
	}
	for i := n.Line; i < len(p.src); i++ {
		if !trivia(p.src[i]) {
			return indentOf(p.src[i])
		}
	}
	return n.Column - 1
}

// anchorsFirst fails on the first alias below n, in output order (the keys of a mapping and then their values, in
// their order after reorder; the items of a sequence in order), whose anchor it has not reached yet. defined holds the
// anchors reached so far. The error names the line of the alias in the source.
func anchorsFirst(n *yaml.Node, defined map[string]bool) error {
	if n.Kind == yaml.AliasNode && !defined[n.Value] {
		return refusal(fmt.Sprintf("line %d: the alias *%s would come before its anchor &%s: move the anchor up",
			n.Line, n.Value, n.Value))
	}
	if n.Anchor != "" {
		defined[n.Anchor] = true
	}
	for _, c := range n.Content {
		if err := anchorsFirst(c, defined); err != nil {
			return err
		}
	}
	return nil
}
