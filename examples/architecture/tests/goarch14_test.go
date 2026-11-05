package tests

import (
	"fmt"
	"strings"
	"testing"
)

// docSections are the sections every document of a use case has, in this order.
var docSections = []string{"Trigger", "Request", "Logic", "Response", "Port", "Errors", "Flow"}

// GOARCH-14, the part a check can decide: each use case has its document <verb>_<noun>.md, titled with the name
// of the use case and with every section, in order. That it says what the code does is GOARCH-21.
func TestGOARCH14Document(t *testing.T) {
	checkRule(t, checkDocument, "use case cancel_order has no document cancel_order.md",
		"internal/order/place_order.md: no section ## Port")
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
		for i, l := range lines {
			if h, ok := strings.CutPrefix(strings.TrimSpace(l), "## "); ok {
				at[h] = i
			}
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
