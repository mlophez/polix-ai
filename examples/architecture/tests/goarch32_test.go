package tests

import (
	"fmt"
	"go/ast"
	"go/types"
	"strings"
	"testing"
)

// GOARCH-32, the part of GOARCH-20 a check can decide: the in file declares the shell <verb><Noun>, of type
// func(context.Context, <V>Port, <V>Request) (<V>Response, error); outside the tests, the shell is the only
// function of its package that calls the logic <Verb><Noun>, from one call site; and it is the only one that calls
// methods of the port or of the adapter, apart from the methods of the adapter itself.
func TestGOARCH32ShellShape(t *testing.T) {
	checkRule(t, checkShellShape,
		"the shell placeOrder is not func(ctx context.Context, port PlaceOrderPort, req PlaceOrderRequest) "+
			"(PlaceOrderResponse, error)",
		"internal/order/cancel_order_in.go: declares no shell func cancelOrder",
		"Helper calls the logic PlaceOrder: only the shell placeOrder calls it",
		"CancelOrderCommand calls CancelOrderAdapter.Refund: only the shell cancelOrder uses the port and the adapter",
		"countStockLoad calls CountStockPort.Load: only the shell countStock uses the port and the adapter",
		"the shell countStock calls the logic CountStock 2 times, not once")
}

func checkShellShape(m *module) []string {
	info := m.typed()
	pkgs := m.packages()
	var out []string
	for _, uc := range m.useCases() {
		in := uc.files[kindIn]
		if in == nil {
			continue // GOARCH-3
		}
		c, lc := camel(uc.stem), lowerCamel(uc.stem)
		want := fmt.Sprintf("func(ctx context.Context, port %sPort, req %sRequest) (%sResponse, error)", c, c, c)
		shell := funcDecl(in.ast, lc)
		switch {
		case shell == nil:
			out = append(out, fmt.Sprintf("%s: declares no shell func %s%s", in.rel, lc, strings.TrimPrefix(want, "func")))
		case !shellSignature(shell, c):
			out = append(out, fmt.Sprintf("%s: the shell %s is not %s", m.pos(in, shell), lc, want))
		}
		portTypes := map[string]bool{uc.dir + "." + c + "Port": true, uc.dir + "." + c + "Adapter": true}
		for _, f := range pkgs[uc.dir] {
			for _, d := range f.ast.Decls {
				fn, ok := d.(*ast.FuncDecl)
				if !ok || fn.Body == nil || (fn.Recv == nil && fn.Name.Name == c) || receiver(fn) == c+"Adapter" {
					continue
				}
				calls := logicCalls(fn, c)
				if fn == shell {
					if uc.files[kindLogic] != nil && calls != 1 {
						out = append(out, fmt.Sprintf("%s: the shell %s calls the logic %s %d times, not once",
							m.pos(f, fn), lc, c, calls))
					}
					continue
				}
				if calls > 0 {
					out = append(out, fmt.Sprintf("%s: %s calls the logic %s: only the shell %s calls it",
						m.pos(f, fn), fn.Name.Name, c, lc))
				}
				out = append(out, portCalls(m, info, f, fn, portTypes, lc)...)
			}
		}
	}
	return out
}

// shellSignature reports whether fn is func(context.Context, <c>Port, <c>Request) (<c>Response, error).
func shellSignature(fn *ast.FuncDecl, c string) bool {
	return strings.Join(fieldTypes(fn.Type.Params), ", ") == "context.Context, "+c+"Port, "+c+"Request" &&
		strings.Join(fieldTypes(fn.Type.Results), ", ") == c+"Response, error"
}

// fieldTypes returns the type of each parameter or result of a list, one per name.
func fieldTypes(list *ast.FieldList) []string {
	var out []string
	if list == nil {
		return out
	}
	for _, f := range list.List {
		for range max(1, len(f.Names)) {
			out = append(out, types.ExprString(f.Type))
		}
	}
	return out
}

// logicCalls counts the calls of fn to the function name of its package.
func logicCalls(fn *ast.FuncDecl, name string) int {
	n := 0
	ast.Inspect(fn.Body, func(node ast.Node) bool {
		if call, ok := node.(*ast.CallExpr); ok {
			if id, ok := call.Fun.(*ast.Ident); ok && id.Name == name && id.Obj == nil {
				n++
			}
		}
		return true
	})
	return n
}

// portCalls returns the calls of fn to methods of the types of portTypes, folder.Name.
func portCalls(m *module, info *types.Info, f *goFile, fn *ast.FuncDecl, portTypes map[string]bool,
	shell string) []string {
	var out []string
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		sel, ok := n.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		s := info.Selections[sel]
		if s == nil || s.Kind() != types.MethodVal {
			return true
		}
		t := s.Recv()
		if p, ok := t.(*types.Pointer); ok {
			t = p.Elem()
		}
		named, ok := t.(*types.Named)
		if !ok || named.Obj().Pkg() == nil {
			return true
		}
		if portTypes[internalDir(named.Obj().Pkg().Path())+"."+named.Obj().Name()] {
			out = append(out, fmt.Sprintf("%s: %s calls %s.%s: only the shell %s uses the port and the adapter",
				m.pos(f, sel), fn.Name.Name, named.Obj().Name(), sel.Sel.Name, shell))
		}
		return true
	})
	return out
}
