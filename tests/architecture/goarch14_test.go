package architecture

import (
	"fmt"
	"strings"
	"testing"
)

// docSections are the sections every document of a use case has, in this order.
var docSections = []string{"Trigger", "Request", "Logic", "Response", "Port", "Errors", "Flow"}

// GOARCH-14, the part a check can decide: each use case has its document <verb>_<noun>.md, titled with the name
// of the use case and with every section once, in order; a heading inside a fenced code block is not a section.
// That it says what the code does is GOARCH-21.
func TestGOARCH14(t *testing.T) {
	checkRule(t, checkDocument, "use case cancel_order has no document cancel_order.md",
		"internal/order/place_order.md: no section ## Port",
		"internal/stock/count_stock.md: does not start with # CountStock",
		"internal/stock/count_stock.md: section ## Port out of order",
		"internal/stock/count_stock.md: section ## Errors appears twice")
}

func checkDocument(m *module) []string {
	docs := map[string]*mdFile{}
	for _, d := range m.docs {
		docs[d.dir+"/"+d.name] = d
	}
	var out []string
	for _, uc := range m.useCases() {
		d := docs[uc.dir+"/"+uc.stem+".md"]
		if d == nil {
			out = append(out, fmt.Sprintf("%s: use case %s has no document %s.md", uc.dir, uc.stem, uc.stem))
			continue
		}
		lines := strings.Split(d.text, "\n")
		if title := "# " + camel(uc.stem); len(lines) == 0 || strings.TrimSpace(lines[0]) != title {
			out = append(out, fmt.Sprintf("%s: does not start with %s", d.rel, title))
		}
		at := map[string]int{}
		for i, h := range headings(d.text) {
			if _, dup := at[h]; dup {
				out = append(out, fmt.Sprintf("%s: section ## %s appears twice", d.rel, h))
				continue
			}
			at[h] = i
		}
		last := -1
		for _, s := range docSections {
			i, ok := at[s]
			switch {
			case !ok:
				out = append(out, fmt.Sprintf("%s: no section ## %s", d.rel, s))
			case i < last:
				out = append(out, fmt.Sprintf("%s: section ## %s out of order", d.rel, s))
			default:
				last = i
			}
		}
	}
	return out
}

// headings returns the ## headings of a Markdown text in order, leaving out fenced code blocks.
func headings(text string) []string {
	var out []string
	fenced := false
	for l := range strings.SplitSeq(text, "\n") {
		l = strings.TrimSpace(l)
		if strings.HasPrefix(l, "```") {
			fenced = !fenced
			continue
		}
		if h, ok := strings.CutPrefix(l, "## "); ok && !fenced {
			out = append(out, h)
		}
	}
	return out
}
