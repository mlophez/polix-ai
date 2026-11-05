package tests

import (
	"fmt"
	"go/ast"
	"path"
	"testing"
)

// GOARCH-2: each domain package has its entity file <noun>.go, which declares the entity <Noun>, a struct.
func TestGOARCH02Entity(t *testing.T) {
	checkRule(t, checkEntity, "internal/order/order.go: does not declare the entity type Order",
		"internal/stock/stock.go: the entity Stock is not a struct", "internal/cart: no entity file cart.go")
}

func checkEntity(m *module) []string {
	var out []string
	for dir, files := range m.packages() {
		pkg := path.Base(dir)
		var entity *goFile
		for _, f := range files {
			if f.name == pkg+".go" {
				entity = f
			}
		}
		switch {
		case entity == nil:
			out = append(out, fmt.Sprintf("%s: no entity file %s.go", dir, pkg))
		case decls(entity.ast)[camel(pkg)] != "type":
			out = append(out, fmt.Sprintf("%s: does not declare the entity type %s", entity.rel, camel(pkg)))
		default:
			if _, ok := typeSpec(entity.ast, camel(pkg)).Type.(*ast.StructType); !ok {
				out = append(out, fmt.Sprintf("%s: the entity %s is not a struct", entity.rel, camel(pkg)))
			}
		}
	}
	return out
}
