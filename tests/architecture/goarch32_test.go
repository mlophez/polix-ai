package architecture

import (
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"strings"
	"testing"
)

// GOARCH-32, the part of GOARCH-20 a check can decide: in.go declares the shell run, of type
// func(context.Context, Port, Request) (Response, error); outside the tests, the shell is the only function or
// method of its use case package that calls the logic Decide, from one call site, in a function declaration or in
// the value of a package variable; and it is the only one that calls methods of the port or of the adapter,
// declared or promoted, apart from the methods of the adapter itself, which never call the logic.
func TestGOARCH32(t *testing.T) {
	checkRule(t, checkShellShape,
		"internal/app/order/place/in.go:17: the shell run is not func(ctx context.Context, port Port, req Request) "+
			"(Response, error)",
		"internal/app/order/cancel/in.go: declares no shell func run",
		"place/in.go:13: Helper calls the logic Decide: only the shell run calls it",
		"place/logic.go:62: twice calls the logic Decide: only the shell run calls it",
		"cancel/out.go:16: Adapter.Refund calls the logic Decide: only the shell run calls it",
		"cancel/in.go:41: again calls the logic Decide: only the shell run calls it",
		"cancel/in.go:26: Command calls Adapter.Refund: only the shell run uses the port and the adapter",
		"count/in.go:51: load calls Port.Load: only the shell run uses the port and the adapter",
		"count/in.go:54: closeAll calls Port.Close: only the shell run uses the port and the adapter",
		"count/in.go:36: the shell run calls the logic Decide 2 times, not once")
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
		const c, lc = "Decide", "run"
		const want = "func(ctx context.Context, port Port, req Request) (Response, error)"
		shell := funcDecl(in.ast, lc)
		switch {
		case shell == nil:
			out = append(out, fmt.Sprintf("%s: declares no shell func %s%s", in.rel, lc, strings.TrimPrefix(want, "func")))
		case !shellSignature(shell):
			out = append(out, fmt.Sprintf("%s: the shell %s is not %s", m.pos(in, shell), lc, want))
		}
		portTypes := map[string]bool{uc.dir + ".Port": true, uc.dir + ".Adapter": true}
		for _, f := range pkgs[uc.dir] {
			for _, d := range f.ast.Decls {
				switch d := d.(type) {
				case *ast.FuncDecl:
					if d.Body == nil || (d.Recv == nil && d.Name.Name == c) {
						continue
					}
					who := d.Name.Name
					if r := receiver(d); r != "" {
						who = r + "." + who
					}
					calls := logicCalls(info, d.Body, c)
					if d == shell {
						if uc.files[kindLogic] != nil && calls != 1 {
							out = append(out, fmt.Sprintf("%s: the shell %s calls the logic %s %d times, not once",
								m.pos(f, d), lc, c, calls))
						}
						continue
					}
					if calls > 0 {
						out = append(out, fmt.Sprintf("%s: %s calls the logic %s: only the shell %s calls it",
							m.pos(f, d), who, c, lc))
					}
					if receiver(d) != "Adapter" {
						out = append(out, portCalls(m, info, f, d.Body, who, portTypes, lc)...)
					}
				case *ast.GenDecl:
					if d.Tok != token.VAR {
						continue
					}
					for _, s := range d.Specs {
						v := s.(*ast.ValueSpec)
						for i, value := range v.Values {
							if logicCalls(info, value, c) > 0 {
								out = append(out, fmt.Sprintf("%s: %s calls the logic %s: only the shell %s calls it",
									m.pos(f, value), v.Names[min(i, len(v.Names)-1)].Name, c, lc))
							}
							out = append(out, portCalls(m, info, f, value, v.Names[min(i, len(v.Names)-1)].Name, portTypes,
								lc)...)
						}
					}
				}
			}
		}
	}
	return out
}

// shellSignature reports whether fn is func(context.Context, Port, Request) (Response, error).
func shellSignature(fn *ast.FuncDecl) bool {
	return strings.Join(fieldTypes(fn.Type.Params), ", ") == "context.Context, Port, Request" &&
		strings.Join(fieldTypes(fn.Type.Results), ", ") == "Response, error"
}

// fieldTypes returns the type of each parameter or result of a list, one per name, as source.
func fieldTypes(list *ast.FieldList) []string {
	var out []string
	for _, t := range flatTypes(list) {
		out = append(out, types.ExprString(t))
	}
	return out
}

// logicCalls counts the calls under n to the function name of its package: the callee is resolved by the
// checker when it can, else a name declared at the top level of its file or in another file counts.
func logicCalls(info *types.Info, n ast.Node, name string) int {
	count := 0
	ast.Inspect(n, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		id, ok := call.Fun.(*ast.Ident)
		if !ok || id.Name != name {
			return true
		}
		if obj, ok := info.Uses[id]; ok {
			if fn, ok := obj.(*types.Func); ok && fn.Pkg() != nil && fn.Parent() == fn.Pkg().Scope() {
				count++
			}
			return true
		}
		if id.Obj == nil {
			count++
		} else if _, topLevel := id.Obj.Decl.(*ast.FuncDecl); topLevel {
			count++
		}
		return true
	})
	return count
}

// portCalls returns the calls under n, in who, to methods of the types of portTypes, folder.Name, declared or
// promoted through an embedded field.
func portCalls(m *module, info *types.Info, f *goFile, n ast.Node, who string, portTypes map[string]bool,
	shell string) []string {
	var out []string
	ast.Inspect(n, func(n ast.Node) bool {
		sel, ok := n.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		s := info.Selections[sel]
		if s == nil || s.Kind() != types.MethodVal {
			return true
		}
		t := types.Unalias(declaringType(s))
		if p, ok := t.(*types.Pointer); ok {
			t = types.Unalias(p.Elem())
		}
		named, ok := t.(*types.Named)
		if !ok || named.Obj().Pkg() == nil {
			return true
		}
		if portTypes[m.moduleDir(named.Obj().Pkg().Path())+"."+named.Obj().Name()] {
			out = append(out, fmt.Sprintf("%s: %s calls %s.%s: only the shell %s uses the port and the adapter",
				m.pos(f, sel), who, named.Obj().Name(), sel.Sel.Name, shell))
		}
		return true
	})
	return out
}
