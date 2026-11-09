package architecture

import (
	"fmt"
	"go/ast"
	"strings"
	"testing"
)

// GOARCH-36, the part of GOARCH-21 a check can decide: the document of a use case names, between backquotes, every
// field of Request in ## Request, every field of Response and of the structs of in.go it holds in ## Response, and
// every method of Port in ## Port, so a field or a method added to the code without its documentation fails. A
// heading inside a fenced code block is not a section.
func TestGOARCH36(t *testing.T) {
	checkRule(t, checkDocumentNamesCode,
		"internal/app/stock/count/count.md: ## Request does not name `Shop` of Request",
		"internal/app/stock/count/count.md: ## Response does not name `Count` of Response",
		"internal/app/stock/count/count.md: ## Response does not name `Aisle` of Shelf",
		"internal/app/stock/count/count.md: ## Port does not name `Load` of Port")
}

func checkDocumentNamesCode(m *module) []string {
	docs := map[string]*mdFile{}
	for _, d := range m.docs {
		docs[d.dir+"/"+d.name] = d
	}
	var out []string
	for _, uc := range m.useCases() {
		d, in := docs[uc.dir+"/"+uc.verb+".md"], uc.files[kindIn]
		if d == nil || in == nil {
			continue // GOARCH-14, GOARCH-3
		}
		sections := markdownSections(d.text)
		for _, part := range []struct {
			section, typ string
			nested       bool
		}{
			{"Request", "Request", false}, {"Response", "Response", true}, {"Port", "Port", false},
		} {
			text, ok := sections[part.section]
			if !ok {
				continue // GOARCH-14
			}
			typs := []string{part.typ}
			if part.nested {
				typs = heldStructs(in.ast, part.typ)
			}
			for _, typ := range typs {
				for _, name := range memberNames(in.ast, typ) {
					if !strings.Contains(text, "`"+name+"`") {
						out = append(out, fmt.Sprintf("%s: ## %s does not name `%s` of %s", d.rel, part.section, name, typ))
					}
				}
			}
		}
	}
	return out
}

// heldStructs returns name and the structs declared in f that its fields hold, directly or through slices, maps
// and pointers, each once, in order of discovery.
func heldStructs(f *ast.File, name string) []string {
	out := []string{name}
	seen := map[string]bool{name: true}
	for i := 0; i < len(out); i++ {
		spec := typeSpec(f, out[i])
		if spec == nil {
			continue
		}
		st, ok := spec.Type.(*ast.StructType)
		if !ok {
			continue
		}
		for _, field := range st.Fields.List {
			ast.Inspect(field.Type, func(n ast.Node) bool {
				if id, ok := n.(*ast.Ident); ok && !seen[id.Name] && typeSpec(f, id.Name) != nil {
					seen[id.Name] = true
					out = append(out, id.Name)
				}
				return true
			})
		}
	}
	return out
}

// markdownSections maps each heading ## X of text to the text under it, leaving out fenced code blocks.
func markdownSections(text string) map[string]string {
	out := map[string]string{}
	current := ""
	fenced := false
	for l := range strings.SplitSeq(text, "\n") {
		if strings.HasPrefix(strings.TrimSpace(l), "```") {
			fenced = !fenced
		}
		if h, ok := strings.CutPrefix(strings.TrimSpace(l), "## "); ok && !fenced {
			current = h
			if _, dup := out[current]; !dup {
				out[current] = ""
			}
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
