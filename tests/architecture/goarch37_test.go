package architecture

import (
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"strings"
	"testing"
)

// GOARCH-37: the shell decides nothing: every condition of an if, for or switch in the shell <verb><Noun> and in
// the unexported functions of the in file is made of fields of the request, errors (err != nil, errors.Is,
// errors.As), fields or methods of values of the domain or of the output, and len of them, compared only with
// nil or a zero literal and combined with !, && and ||; the arguments of an accepted call are operands too. The
// entry <Verb><Noun>Command and the methods are left out. An operand whose type the checker could not resolve is
// accepted.
func TestGOARCH37(t *testing.T) {
	checkRule(t, checkShellDecidesNothing,
		"countStock decides: the condition `len(req.Shop) > 3` uses `len(req.Shop) > 3`",
		"countStock decides: the condition `req.Shop == \"main\"` uses `\"main\"`",
		"countStockPick decides: the condition `n % 2 == 0` uses `n % 2`",
		"countStockPick decides: the condition `stockLimit` uses `stockLimit`",
		"countStockPick decides: a type switch",
		"countStock decides: the condition `l.Rename(\"x\").Valid() != nil` uses `\"x\"`")
}

func checkShellDecidesNothing(m *module) []string {
	info := m.typed()
	var out []string
	for _, uc := range m.useCases() {
		in := uc.files[kindIn]
		if in == nil {
			continue // GOARCH-3
		}
		c := camel(uc.stem)
		for _, d := range in.ast.Decls {
			fn, ok := d.(*ast.FuncDecl)
			if !ok || fn.Recv != nil || fn.Body == nil || fn.Name.Name == c+"Command" {
				continue
			}
			s := &shellConds{m: m, info: info, f: in, fn: fn, output: uc.dir + "." + c + "Output",
				request: uc.dir + "." + c + "Request"}
			out = append(out, s.check()...)
		}
	}
	return out
}

// shellConds checks the conditions of one function of the in file.
type shellConds struct {
	m               *module
	info            *types.Info
	f               *goFile
	fn              *ast.FuncDecl
	output, request string // folder.Name of the output and the request of the use case
}

// check returns the conditions of the function that decide something.
func (s *shellConds) check() []string {
	var out []string
	report := func(cond ast.Expr, bad ast.Expr) {
		out = append(out, fmt.Sprintf("%s: %s decides: the condition `%s` uses `%s`", s.m.pos(s.f, cond),
			s.fn.Name.Name, types.ExprString(cond), types.ExprString(bad)))
	}
	var conds []ast.Expr
	ast.Inspect(s.fn.Body, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.IfStmt:
			conds = append(conds, x.Cond)
		case *ast.ForStmt:
			if x.Cond != nil {
				conds = append(conds, x.Cond)
			}
		case *ast.SwitchStmt:
			if x.Tag != nil {
				conds = append(conds, x.Tag)
			}
			for _, c := range x.Body.List {
				conds = append(conds, c.(*ast.CaseClause).List...)
			}
		case *ast.TypeSwitchStmt:
			out = append(out, fmt.Sprintf("%s: %s decides: a type switch", s.m.pos(s.f, x), s.fn.Name.Name))
		}
		return true
	})
	for _, cond := range conds {
		if bad := s.condition(cond); bad != nil {
			report(cond, bad)
		}
	}
	return out
}

// condition returns the first part of e that is not allowed in a condition of the shell, or nil.
func (s *shellConds) condition(e ast.Expr) ast.Expr {
	switch x := e.(type) {
	case *ast.ParenExpr:
		return s.condition(x.X)
	case *ast.UnaryExpr:
		if x.Op == token.NOT {
			return s.condition(x.X)
		}
	case *ast.BinaryExpr:
		switch x.Op {
		case token.LAND, token.LOR:
			if bad := s.condition(x.X); bad != nil {
				return bad
			}
			return s.condition(x.Y)
		case token.EQL, token.NEQ:
			switch {
			case zero(x.Y):
				return s.operand(x.X)
			case zero(x.X):
				return s.operand(x.Y)
			}
			for _, side := range []ast.Expr{x.X, x.Y} {
				if _, lit := side.(*ast.BasicLit); lit {
					return side
				}
			}
			return x
		}
		return x
	}
	return s.operand(e)
}

// operand returns the first part of e that is not an allowed operand, or nil: a field of the request, an error,
// a field or a method of a value of the domain or of the output, len of one, or a call to errors.Is or errors.As.
func (s *shellConds) operand(e ast.Expr) ast.Expr {
	switch x := e.(type) {
	case *ast.ParenExpr:
		return s.operand(x.X)
	case *ast.Ident:
		if x.Name == "nil" || x.Name == "false" {
			return nil
		}
		if s.allowedRoot(x) {
			return nil
		}
		return x
	case *ast.SelectorExpr, *ast.IndexExpr:
		if id, ok := rootExpr(e); ok && s.allowedRoot(id) {
			return nil
		}
		return e
	case *ast.CallExpr:
		if id, ok := x.Fun.(*ast.Ident); ok && id.Name == "len" && id.Obj == nil && len(x.Args) == 1 {
			return s.operand(x.Args[0])
		}
		if sel, ok := x.Fun.(*ast.SelectorExpr); ok {
			if pkg, ok := sel.X.(*ast.Ident); ok && pkg.Obj == nil && imports(s.f.ast)[pkg.Name] == "errors" &&
				(sel.Sel.Name == "Is" || sel.Sel.Name == "As") {
				return s.arguments(x)
			}
			if id, ok := rootExpr(sel.X); ok && s.allowedRoot(id) {
				return s.arguments(x) // a method of a value of the domain or of the output
			}
		}
		return x
	}
	return e
}

// arguments returns the first argument of call, or of a call along its chain (a.B(1).C(2)), that is not an
// allowed operand, or nil. A sentinel error of a domain package (rule.ErrNoCheck) is allowed.
func (s *shellConds) arguments(call *ast.CallExpr) ast.Expr {
	for e := ast.Expr(call); ; {
		c, ok := e.(*ast.CallExpr)
		if !ok {
			return nil
		}
		for _, a := range c.Args {
			if s.sentinel(a) {
				continue
			}
			if bad := s.operand(a); bad != nil {
				return bad
			}
		}
		sel, ok := c.Fun.(*ast.SelectorExpr)
		if !ok {
			return nil
		}
		e = sel.X
	}
}

// sentinel reports whether e is pkg.ErrX, an error of a domain package of internal/.
func (s *shellConds) sentinel(e ast.Expr) bool {
	sel, ok := e.(*ast.SelectorExpr)
	if !ok || !strings.HasPrefix(sel.Sel.Name, "Err") {
		return false
	}
	pkg, ok := sel.X.(*ast.Ident)
	return ok && pkg.Obj == nil && s.m.internalDir(imports(s.f.ast)[pkg.Name]) != ""
}

// allowedRoot reports whether the identifier id is a value the shell may look at: the request, an error, or a
// value of the domain or of the output; one whose type did not resolve is accepted.
func (s *shellConds) allowedRoot(id *ast.Ident) bool {
	obj := s.info.Uses[id]
	if obj == nil {
		return true
	}
	if _, isPkg := obj.(*types.PkgName); isPkg {
		return false
	}
	t := obj.Type()
	if t == nil || invalid(t) {
		return true
	}
	if types.Identical(t, types.Universe.Lookup("error").Type()) {
		return true
	}
	if key := s.m.domainStruct(elemOf(t)); key != "" {
		return true
	}
	return s.namedAs(t, s.output) || s.namedAs(t, s.request)
}

// elemOf returns the element type of a slice, an array or a map, through pointers, else t itself.
func elemOf(t types.Type) types.Type {
	for {
		switch x := types.Unalias(t).(type) {
		case *types.Pointer:
			t = x.Elem()
		case *types.Slice:
			t = x.Elem()
		case *types.Array:
			t = x.Elem()
		case *types.Map:
			t = x.Elem()
		default:
			return t
		}
	}
}

// namedAs reports whether t, through pointers, slices, maps and aliases, is the named type key, folder.Name.
func (s *shellConds) namedAs(t types.Type, key string) bool {
	t = types.Unalias(t)
	switch x := t.(type) {
	case *types.Pointer:
		return s.namedAs(x.Elem(), key)
	case *types.Slice:
		return s.namedAs(x.Elem(), key)
	case *types.Map:
		return s.namedAs(x.Elem(), key)
	case *types.Named:
		if x.Obj().Pkg() == nil {
			return false
		}
		return s.m.internalDir(x.Obj().Pkg().Path())+"."+x.Obj().Name() == key
	}
	return false
}

// rootExpr returns the identifier at the root of a chain of selectors, indexes and method calls, or false.
func rootExpr(e ast.Expr) (*ast.Ident, bool) {
	for {
		switch x := e.(type) {
		case *ast.Ident:
			return x, true
		case *ast.SelectorExpr:
			e = x.X
		case *ast.IndexExpr:
			e = x.X
		case *ast.CallExpr:
			e = x.Fun
		case *ast.ParenExpr:
			e = x.X
		default:
			return nil, false
		}
	}
}

// zero reports whether e is nil or a zero literal: "", 0 or false.
func zero(e ast.Expr) bool {
	switch x := e.(type) {
	case *ast.Ident:
		return x.Name == "nil" || x.Name == "false"
	case *ast.BasicLit:
		return x.Value == `""` || x.Value == "``" || strings.Trim(x.Value, "0.") == ""
	}
	return false
}
