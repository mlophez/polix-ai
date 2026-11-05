package architecture

import (
	"fmt"
	"strings"
	"testing"
)

// GOARCH-1: each file of a domain package is classified by its name alone; a file that fits no kind fails, and so
// does a Markdown file that is not the document of a use case of its package.
func TestGOARCH1(t *testing.T) {
	checkRule(t, checkFileNames, "internal/order/helpers.go", "internal/order/notes.md")
}

func checkFileNames(m *module) []string {
	var out []string
	cases := map[string]bool{}
	for _, f := range m.sources() {
		if f.kind == kindUnknown {
			out = append(out, fmt.Sprintf("%s: neither domain (%s.go, %s_*.go) nor a file of a use case "+
				"(<verb>_%s_in.go, _logic.go, _out.go)", f.rel, f.pkg, f.pkg, f.pkg))
		}
		if f.useCase != "" {
			cases[f.dir+"/"+f.useCase] = true
		}
	}
	for _, d := range m.docs {
		if !cases[d.dir+"/"+strings.TrimSuffix(d.name, ".md")] {
			out = append(out, d.rel+": not the document of a use case of its package")
		}
	}
	return out
}
