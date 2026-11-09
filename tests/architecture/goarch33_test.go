package architecture

import (
	"fmt"
	"go/ast"
	"go/types"
	"testing"
)

// GOARCH-33: the port speaks in domain values: every method of Port takes context.Context first, and the rest of
// its parameters and its results are domain types of any noun or predeclared types (error among them), so the
// types of the infrastructure never leave the adapter; it embeds no interface. A name reserved to the use cases
// (GOARCH-23) is never domain, whatever its package.
func TestGOARCH33(t *testing.T) {
	checkRule(t, checkPortInDomain,
		"internal/app/order/cancel/in.go:16: Port.Cancel does not take context.Context first",
		"internal/app/stock/count/in.go:27: Port.Load takes io.Reader, which is not a domain type",
		"internal/app/stock/count/in.go:27: Port.Load returns chan int, which is not a domain type",
		"internal/app/stock/count/in.go:28: Port.Plan returns place.Output, which is not a domain type",
		"internal/app/stock/count/in.go:29: Port.Open takes stock.StockPort, which is not a domain type",
		"internal/app/stock/count/in.go:26: Port embeds io.Closer: a port lists its own methods")
}

func checkPortInDomain(m *module) []string {
	domain := m.domainTypes()
	var out []string
	for _, uc := range m.useCases() {
		in := uc.files[kindIn]
		if in == nil {
			continue
		}
		const port = "Port"
		spec := typeSpec(in.ast, port)
		if spec == nil {
			continue // GOARCH-11
		}
		iface, ok := spec.Type.(*ast.InterfaceType)
		if !ok {
			continue // GOARCH-12
		}
		imps := imports(in.ast)
		for _, field := range iface.Methods.List {
			ft, ok := field.Type.(*ast.FuncType)
			if !ok || len(field.Names) == 0 {
				out = append(out, fmt.Sprintf("%s: %s embeds %s: a port lists its own methods", m.pos(in, field), port,
					types.ExprString(field.Type)))
				continue
			}
			name := port + "." + field.Names[0].Name
			params := flatTypes(ft.Params)
			if len(params) == 0 || types.ExprString(params[0]) != "context.Context" {
				out = append(out, fmt.Sprintf("%s: %s does not take context.Context first", m.pos(in, field), name))
			} else {
				params = params[1:]
			}
			for _, p := range params {
				if !m.domainType(p, in, domain, imps) {
					out = append(out, fmt.Sprintf("%s: %s takes %s, which is not a domain type", m.pos(in, field), name,
						types.ExprString(p)))
				}
			}
			for _, r := range flatTypes(ft.Results) {
				if !m.domainType(r, in, domain, imps) {
					out = append(out, fmt.Sprintf("%s: %s returns %s, which is not a domain type", m.pos(in, field), name,
						types.ExprString(r)))
				}
			}
		}
	}
	return out
}

// flatTypes returns the type expression of each parameter or result of a list, one per name.
func flatTypes(list *ast.FieldList) []ast.Expr {
	var out []ast.Expr
	if list == nil {
		return out
	}
	for _, f := range list.List {
		for range max(1, len(f.Names)) {
			out = append(out, f.Type)
		}
	}
	return out
}
