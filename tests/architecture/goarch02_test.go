package architecture

import (
	"fmt"
	"go/ast"
	"testing"
)

// GOARCH-2: each domain package, internal/app/<noun>/, has its entity file <noun>.go, which declares the entity
// <Noun>, a struct. Use cases, helpers and libraries have no entity.
func TestGOARCH2(t *testing.T) {
	checkRule(t, checkEntity, "internal/app/order/order.go: does not declare the entity type Order",
		"internal/app/stock/stock.go: the entity Stock is not a struct", "internal/app/cart: no entity file cart.go")
}

func checkEntity(m *module) []string {
	pkgs := m.packages()
	var out []string
	for _, dir := range m.dirs {
		at := placeOf(dir)
		if at.level != levelDomain {
			continue
		}
		var entity *goFile
		for _, f := range pkgs[dir] {
			if f.name == at.noun+".go" {
				entity = f
			}
		}
		name := camel(at.noun)
		switch {
		case entity == nil:
			out = append(out, fmt.Sprintf("%s: no entity file %s.go", dir, at.noun))
		case decls(entity.ast)[name] != "type":
			out = append(out, fmt.Sprintf("%s: does not declare the entity type %s", entity.rel, name))
		default:
			if _, ok := typeSpec(entity.ast, name).Type.(*ast.StructType); !ok {
				out = append(out, fmt.Sprintf("%s: the entity %s is not a struct", entity.rel, name))
			}
		}
	}
	return out
}
