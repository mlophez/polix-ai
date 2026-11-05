package tests

import (
	"fmt"
	"go/ast"
	"strings"
	"testing"
)

// GOARCH-36, the part of GOARCH-21 a check can decide: the document of a use case names, between backquotes, every
// field of <V>Request in ## Request, every field of <V>Response in ## Response and every method of <V>Port in
// ## Port, so a field or a method added to the code without its documentation fails.
func TestGOARCH36DocumentNamesTheCode(t *testing.T) {
	checkRule(t, checkDocumentNamesCode,
		"internal/stock/count_stock.md: ## Request does not name `Shop` of CountStockRequest",
		"internal/stock/count_stock.md: ## Response does not name `Count` of CountStockResponse",
		"internal/stock/count_stock.md: ## Port does not name `Load` of CountStockPort")
}

func checkDocumentNamesCode(m *module) []string {
	docs := map[string]*mdFile{}
	for _, d := range m.docs {
		docs[d.dir+"/"+d.name] = d
	}
	var out []string
	for _, uc := range m.useCases() {
		d, in := docs[uc.dir+"/"+uc.stem+".md"], uc.files[kindIn]
		if d == nil || in == nil {
			continue // GOARCH-14, GOARCH-3
		}
		sections := markdownSections(d.text)
		c := camel(uc.stem)
		for _, part := range []struct{ section, typ string }{
			{"Request", c + "Request"}, {"Response", c + "Response"}, {"Port", c + "Port"},
		} {
			text, ok := sections[part.section]
			if !ok {
				continue // GOARCH-14
			}
			for _, name := range memberNames(in.ast, part.typ) {
				if !strings.Contains(text, "`"+name+"`") {
					out = append(out, fmt.Sprintf("%s: ## %s does not name `%s` of %s", d.rel, part.section, name,
						part.typ))
				}
			}
		}
	}
	return out
}

// markdownSections maps each heading ## X of text to the text under it.
func markdownSections(text string) map[string]string {
	out := map[string]string{}
	current := ""
	for l := range strings.SplitSeq(text, "\n") {
		if h, ok := strings.CutPrefix(strings.TrimSpace(l), "## "); ok {
			current = h
			out[current] = ""
			continue
		}
		if current != "" {
			out[current] += l + "\n"
		}
	}
	return out
}

// memberNames returns the fields of the struct, or the methods of the interface, name declared in f.
func memberNames(f *ast.File, name string) []string {
	spec := typeSpec(f, name)
	if spec == nil {
		return nil
	}
	var list *ast.FieldList
	switch t := spec.Type.(type) {
	case *ast.StructType:
		list = t.Fields
	case *ast.InterfaceType:
		list = t.Methods
	default:
		return nil
	}
	var out []string
	for _, field := range list.List {
		for _, id := range fieldNames(field) {
			out = append(out, id.Name)
		}
	}
	return out
}
