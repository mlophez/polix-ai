// Package tests checks the architecture rules GOARCH-* of rules.yaml on the Go AST of a module: its internal/,
// pkg/ and cmd/ folders. It only parses the files, so the module does not need to compile.
package tests

import (
	"errors"
	"flag"
	"fmt"
	"go/ast"
	"go/importer"
	"go/parser"
	"go/token"
	"go/types"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"testing"
)

// kind is what a file of a domain package is, from its name alone (GOARCH-1).
type kind int

// goFile is one parsed Go file of the module.
type goFile struct {
	rel     string // path relative to the root, with slashes
	top     string // internal, pkg or cmd
	dir     string // folder relative to the root, with slashes (internal/rule)
	pkg     string // name of the folder of the package (rule)
	name    string // base name (check_rule_in.go)
	kind    kind
	useCase string // stem of the use case (check_rule); empty for domain and unknown files
	ast     *ast.File
}

// mdFile is one Markdown file of a domain package.
type mdFile struct {
	rel, dir, name, text string
}

// module is the parsed module under check.
type module struct {
	path   string // module path of go.mod, or "" without one
	fset   *token.FileSet
	files  []*goFile
	docs   []*mdFile
	broken []string // files that do not parse, with their error

	typesOnce sync.Once
	info      *types.Info // types of the sources of internal/ and pkg/, filled by typed
}

// sourceImporter type-checks the packages of internal/ and pkg/ of the module from their sources, on demand, and
// takes the standard library from the compiler. Any other import, and a package of an import cycle (GOARCH-7),
// is an empty package: its names do not resolve, which the checker reports and typed ignores.
type sourceImporter struct {
	m    *module
	std  types.Importer
	pkgs map[string][]*goFile // sources by folder
	done map[string]*types.Package
	busy map[string]bool
}

// useCase holds the files of one use case of a package, by kind.
type useCase struct {
	dir, stem string
	files     map[kind]*goFile
}

// ref is a use of a top-level name declared in another file of the same package.
type ref struct {
	name string
	node ast.Node
	decl *goFile
}

// pkgRef is a use of a name of another package of internal/: pkg.Name.
type pkgRef struct {
	dir, pkg, name string
	node           ast.Node
}

// badRoot is a module that breaks every rule with a deterministic check, to prove that each check detects it.
const badRoot = "testdata/bad"

const (
	kindUnknown kind = iota
	kindDomain
	kindIn
	kindLogic
	kindOut
	kindTest
)

// root is the module under check; by default the example, one folder up.
var root = flag.String("root", "..", "root of the module whose architecture is checked")

// loaded caches the parsed modules by root, shared by the tests of every rule.
var loaded sync.Map

// useCaseSuffixes are the suffixes of the files of a use case, from input to output (GOARCH-1, GOARCH-3).
var useCaseSuffixes = []struct {
	suffix string
	kind   kind
}{{"_in", kindIn}, {"_logic", kindLogic}, {"_out", kindOut}}

// versionSuffix is the major version at the end of an import path, which is not the name of the package.
var versionSuffix = regexp.MustCompile(`[./]v[0-9]+$`)

// TestParses reports the files of the module under check that do not parse, once, instead of failing every rule.
func TestParses(t *testing.T) {
	for _, b := range mustLoad(t, *root).broken {
		t.Error(b)
	}
}

// checkRule runs check on the module under check, where it must find nothing, and on testdata/bad, where every
// string of want must appear in some violation it finds.
func checkRule(t *testing.T, check func(*module) []string, want ...string) {
	t.Helper()
	t.Run("module", func(t *testing.T) {
		got := check(mustLoad(t, *root))
		sort.Strings(got)
		for _, v := range got {
			t.Error(v)
		}
	})
	t.Run("detects", func(t *testing.T) {
		got := strings.Join(check(mustLoad(t, badRoot)), "\n")
		for _, w := range want {
			if !strings.Contains(got, w) {
				t.Errorf("no violation mentions %q; found:\n%s", w, got)
			}
		}
	})
}

// mustLoad parses the module at root once per run.
func mustLoad(t *testing.T, root string) *module {
	t.Helper()
	if m, ok := loaded.Load(root); ok {
		return m.(*module)
	}
	m, err := load(root)
	if err != nil {
		t.Fatal(err)
	}
	if root == badRoot && len(m.broken) > 0 {
		t.Fatal(strings.Join(m.broken, "\n"))
	}
	loaded.Store(root, m)
	return m
}

// load parses every Go file of internal/, pkg/ and cmd/ under root, and the Markdown files of internal/. A file
// that does not parse is listed in broken and left out.
func load(root string) (*module, error) {
	m := &module{fset: token.NewFileSet(), path: modulePath(root)}
	for _, top := range []string{"internal", "pkg", "cmd"} {
		base := filepath.Join(root, top)
		if _, err := os.Stat(base); errors.Is(err, fs.ErrNotExist) {
			continue
		}
		err := filepath.WalkDir(base, func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				if d.Name() == "testdata" {
					return filepath.SkipDir
				}
				return nil
			}
			rel, err := filepath.Rel(root, p)
			if err != nil {
				return err
			}
			rel = filepath.ToSlash(rel)
			dir := path.Dir(rel)
			switch filepath.Ext(p) {
			case ".go":
				f, err := parser.ParseFile(m.fset, p, nil, parser.ParseComments)
				if err != nil {
					m.broken = append(m.broken, fmt.Sprintf("%s: does not parse: %v", rel, err))
					return nil
				}
				g := &goFile{rel: rel, top: top, dir: dir, pkg: path.Base(dir), name: d.Name(), ast: f}
				g.kind, g.useCase = classify(top, g.pkg, g.name)
				m.files = append(m.files, g)
			case ".md":
				if top == "internal" {
					data, err := os.ReadFile(p)
					if err != nil {
						return err
					}
					m.docs = append(m.docs, &mdFile{rel: rel, dir: dir, name: d.Name(), text: string(data)})
				}
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	return m, nil
}

// modulePath reads the module path of the go.mod at root, or "" without one.
func modulePath(root string) string {
	data, err := os.ReadFile(filepath.Join(root, "go.mod"))
	if err != nil {
		return ""
	}
	for l := range strings.SplitSeq(string(data), "\n") {
		if p, ok := strings.CutPrefix(strings.TrimSpace(l), "module "); ok {
			return strings.TrimSpace(p)
		}
	}
	return ""
}

// classify tells what a file of the package pkg under top is from its name (GOARCH-1): a test under any top; a
// file of a use case or domain only under internal/. The suffixes of a use case take precedence and need a verb:
// <pkg>_in.go is neither domain nor a use case.
func classify(top, pkg, name string) (kind, string) {
	if strings.HasSuffix(name, "_test.go") {
		return kindTest, ""
	}
	if top != "internal" {
		return kindUnknown, ""
	}
	stem := strings.TrimSuffix(name, ".go")
	for _, s := range useCaseSuffixes {
		if uc, ok := strings.CutSuffix(stem, s.suffix); ok {
			if uc == "" || uc == pkg {
				return kindUnknown, ""
			}
			return s.kind, uc
		}
	}
	if stem == pkg || strings.HasPrefix(stem, pkg+"_") {
		return kindDomain, ""
	}
	return kindUnknown, ""
}

// pos renders the position of n in f as path:line.
func (m *module) pos(f *goFile, n ast.Node) string {
	return fmt.Sprintf("%s:%d", f.rel, m.fset.Position(n.Pos()).Line)
}

// sources returns the files of internal/ that are not tests.
func (m *module) sources() []*goFile {
	var out []*goFile
	for _, f := range m.files {
		if f.top == "internal" && f.kind != kindTest {
			out = append(out, f)
		}
	}
	return out
}

// packages groups the sources of internal/ by folder.
func (m *module) packages() map[string][]*goFile {
	out := map[string][]*goFile{}
	for _, f := range m.sources() {
		out[f.dir] = append(out[f.dir], f)
	}
	return out
}

// compiled groups the files of internal/ and pkg/ that are not tests by folder, for the type checker.
func (m *module) compiled() map[string][]*goFile {
	out := map[string][]*goFile{}
	for _, f := range m.files {
		if f.top != "cmd" && f.kind != kindTest {
			out[f.dir] = append(out[f.dir], f)
		}
	}
	return out
}

// useCases returns the use cases of the module, from the names of their files.
func (m *module) useCases() []*useCase {
	byKey := map[string]*useCase{}
	var out []*useCase
	for _, f := range m.sources() {
		if f.useCase == "" {
			continue
		}
		key := f.dir + "/" + f.useCase
		uc, ok := byKey[key]
		if !ok {
			uc = &useCase{dir: f.dir, stem: f.useCase, files: map[kind]*goFile{}}
			byKey[key] = uc
			out = append(out, uc)
		}
		uc.files[f.kind] = f
	}
	return out
}

// moduleDir returns the folder of an import of the module (internal/rule, pkg/git), or "". The checker names the
// packages of the module module/<folder>. Without a module path, an import is of the module when it holds
// /internal/ or /pkg/.
func (m *module) moduleDir(importPath string) string {
	if rest, ok := strings.CutPrefix(importPath, "module/"); ok {
		return rest
	}
	if m.path != "" {
		rest, ok := strings.CutPrefix(importPath, m.path+"/")
		if !ok || (!strings.HasPrefix(rest, "internal/") && !strings.HasPrefix(rest, "pkg/")) {
			return ""
		}
		return rest
	}
	for _, top := range []string{"/internal/", "/pkg/"} {
		if _, rest, ok := strings.Cut(importPath, top); ok {
			return strings.Trim(top, "/") + "/" + rest
		}
	}
	return ""
}

// internalDir returns the folder of an import of internal/ (internal/rule), or "".
func (m *module) internalDir(importPath string) string {
	if dir := m.moduleDir(importPath); strings.HasPrefix(dir, "internal/") {
		return dir
	}
	return ""
}

// symbols maps each top-level name of a package (not methods) to the file that declares it.
func symbols(files []*goFile) map[string]*goFile {
	out := map[string]*goFile{}
	for _, f := range files {
		for name := range decls(f.ast) {
			out[name] = f
		}
	}
	return out
}

// decls returns the top-level names of a file, functions without receiver included, as name -> func, type, const
// or var.
func decls(f *ast.File) map[string]string {
	out := map[string]string{}
	for _, d := range f.Decls {
		what := "func"
		if g, ok := d.(*ast.GenDecl); ok {
			what = strings.ToLower(g.Tok.String())
		}
		for _, id := range topLevelNames(d) {
			out[id.Name] = what
		}
	}
	return out
}

// topLevelNames returns the names a declaration adds to its package; methods and imports add none.
func topLevelNames(d ast.Decl) []*ast.Ident {
	var out []*ast.Ident
	switch d := d.(type) {
	case *ast.FuncDecl:
		if d.Recv == nil {
			out = append(out, d.Name)
		}
	case *ast.GenDecl:
		for _, s := range d.Specs {
			switch s := s.(type) {
			case *ast.TypeSpec:
				out = append(out, s.Name)
			case *ast.ValueSpec:
				out = append(out, s.Names...)
			}
		}
	}
	return out
}

// typeSpec returns the declaration of the type name in f, or nil.
func typeSpec(f *ast.File, name string) *ast.TypeSpec {
	for _, d := range f.Decls {
		if g, ok := d.(*ast.GenDecl); ok && g.Tok == token.TYPE {
			for _, s := range g.Specs {
				if t := s.(*ast.TypeSpec); t.Name.Name == name {
					return t
				}
			}
		}
	}
	return nil
}

// funcDecl returns the function without receiver name in f, or nil.
func funcDecl(f *ast.File, name string) *ast.FuncDecl {
	for _, d := range f.Decls {
		if fn, ok := d.(*ast.FuncDecl); ok && fn.Recv == nil && fn.Name.Name == name {
			return fn
		}
	}
	return nil
}

// imports maps the local name of each import of f to its path; a dot import is under ".".
func imports(f *ast.File) map[string]string {
	out := map[string]string{}
	for _, s := range f.Imports {
		p := strings.Trim(s.Path.Value, `"`)
		name := packageName(p)
		if s.Name != nil {
			name = s.Name.Name
		}
		out[name] = p
	}
	return out
}

// packageName guesses the name of the package of an import path: its last element without a major version
// (gopkg.in/yaml.v3 is yaml, math/rand/v2 is rand).
func packageName(importPath string) string {
	p := versionSuffix.ReplaceAllString(importPath, "")
	return path.Base(p)
}

// importedAs returns the local name under which f imports the package of path p, or "".
func importedAs(f *ast.File, p string) string {
	for name, ip := range imports(f) {
		if ip == p {
			return name
		}
	}
	return ""
}

// localRefs returns the uses in f of top-level names that other files of its package declare. Names resolved in f
// itself (its own declarations, locals that shadow) are left out, and so are selectors, field names and keys of
// struct literals, which are not top-level names; the keys of a map literal are.
func (m *module) localRefs(f *goFile, syms map[string]*goFile) []ref {
	skip := map[*ast.Ident]bool{}
	var out []ref
	ast.Inspect(f.ast, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.SelectorExpr:
			skip[x.Sel] = true
		case *ast.CompositeLit:
			if m.structLiteral(x) {
				for _, e := range x.Elts {
					if kv, ok := e.(*ast.KeyValueExpr); ok {
						if id, ok := kv.Key.(*ast.Ident); ok {
							skip[id] = true
						}
					}
				}
			}
		case *ast.Field:
			for _, id := range x.Names {
				skip[id] = true
			}
		case *ast.FuncDecl:
			skip[x.Name] = true
		case *ast.TypeSpec:
			skip[x.Name] = true
		case *ast.ValueSpec:
			for _, id := range x.Names {
				skip[id] = true
			}
		case *ast.Ident:
			if skip[x] || x.Obj != nil {
				return true
			}
			if d, ok := syms[x.Name]; ok && d != f {
				out = append(out, ref{name: x.Name, node: x, decl: d})
			}
		}
		return true
	})
	return out
}

// structLiteral reports whether the composite literal lit builds a struct, so its keys are field names: by its
// type when the checker resolved it, else by the syntax of its type expression.
func (m *module) structLiteral(lit *ast.CompositeLit) bool {
	if tv, ok := m.typed().Types[lit]; ok && tv.Type != nil {
		_, isStruct := tv.Type.Underlying().(*types.Struct)
		return isStruct
	}
	switch lit.Type.(type) {
	case *ast.MapType, *ast.ArrayType:
		return false
	}
	return true
}

// packageRefs returns the uses in f of names of other packages of internal/.
func (m *module) packageRefs(f *goFile) []pkgRef {
	imps := imports(f.ast)
	var out []pkgRef
	ast.Inspect(f.ast, func(n ast.Node) bool {
		sel, ok := n.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		id, ok := sel.X.(*ast.Ident)
		if !ok || id.Obj != nil {
			return true
		}
		if dir := m.internalDir(imps[id.Name]); dir != "" && dir != f.dir {
			out = append(out, pkgRef{dir: dir, pkg: id.Name, name: sel.Sel.Name, node: sel})
		}
		return true
	})
	return out
}

// typed returns the types of the sources of internal/ and pkg/. The module need not compile: the checker goes on
// after each error, and what does not resolve has no type.
func (m *module) typed() *types.Info {
	m.typesOnce.Do(func() {
		m.info = &types.Info{
			Types:      map[ast.Expr]types.TypeAndValue{},
			Defs:       map[*ast.Ident]types.Object{},
			Uses:       map[*ast.Ident]types.Object{},
			Selections: map[*ast.SelectorExpr]*types.Selection{},
		}
		imp := &sourceImporter{m: m, std: importer.ForCompiler(m.fset, "gc", nil), pkgs: m.compiled(),
			done: map[string]*types.Package{}, busy: map[string]bool{}}
		var dirs []string
		for dir := range imp.pkgs {
			dirs = append(dirs, dir)
		}
		sort.Strings(dirs)
		for _, dir := range dirs {
			imp.check(dir)
		}
	})
	return m.info
}

// Import resolves an import of the module from its sources, one of the standard library from the compiler, and
// any other as an empty package.
func (s *sourceImporter) Import(importPath string) (*types.Package, error) {
	if dir := s.m.moduleDir(importPath); s.pkgs[dir] != nil && !s.busy[dir] {
		return s.check(dir), nil
	}
	if first, _, _ := strings.Cut(importPath, "/"); !strings.Contains(first, ".") {
		if pkg, err := s.std.Import(importPath); err == nil {
			return pkg, nil
		}
	}
	pkg := types.NewPackage(importPath, packageName(importPath))
	pkg.MarkComplete()
	return pkg, nil
}

// check type-checks the package of the folder dir once. Its path is the folder, so a type tells its package by
// internalDir.
func (s *sourceImporter) check(dir string) *types.Package {
	if pkg, ok := s.done[dir]; ok {
		return pkg
	}
	s.busy[dir] = true
	var files []*ast.File
	for _, f := range s.pkgs[dir] {
		files = append(files, f.ast)
	}
	conf := types.Config{Importer: s, Error: func(error) {}}
	pkg, _ := conf.Check("module/"+dir, s.m.fset, files, s.m.info)
	s.done[dir] = pkg
	s.busy[dir] = false
	return pkg
}

// domainStruct returns the exported struct of the domain t is, through aliases and pointers, as folder.Name, or
// "".
func (m *module) domainStruct(t types.Type) string {
	if t == nil {
		return ""
	}
	t = types.Unalias(t)
	for {
		p, ok := t.(*types.Pointer)
		if !ok {
			break
		}
		t = types.Unalias(p.Elem())
	}
	named, ok := t.(*types.Named)
	if !ok || named.Obj().Pkg() == nil {
		return ""
	}
	key := m.internalDir(named.Obj().Pkg().Path()) + "." + named.Obj().Name()
	if m.domainStructs()[key] {
		return key
	}
	return ""
}

// domainStructs returns the exported structs of the domain files, as folder.Name.
func (m *module) domainStructs() map[string]bool {
	out := map[string]bool{}
	for _, f := range m.sources() {
		if f.kind != kindDomain {
			continue
		}
		for _, d := range f.ast.Decls {
			g, ok := d.(*ast.GenDecl)
			if !ok || g.Tok != token.TYPE {
				continue
			}
			for _, sp := range g.Specs {
				t := sp.(*ast.TypeSpec)
				if _, isStruct := t.Type.(*ast.StructType); isStruct && t.Name.IsExported() {
					out[f.dir+"."+t.Name.Name] = true
				}
			}
		}
	}
	return out
}

// receiverOf returns the domain struct whose method fn is, as folder.Name, or "".
func (m *module) receiverOf(f *goFile, fn *ast.FuncDecl) string {
	if fn.Recv == nil || len(fn.Recv.List) == 0 {
		return ""
	}
	return m.domainStruct(m.typed().TypeOf(fn.Recv.List[0].Type))
}

// domainSymbols returns, for each package of internal/, the top-level names its domain files declare.
func (m *module) domainSymbols() map[string]map[string]bool {
	out := map[string]map[string]bool{}
	for dir, files := range m.packages() {
		out[dir] = map[string]bool{}
		for _, f := range files {
			if f.kind == kindDomain {
				for name := range decls(f.ast) {
					out[dir][name] = true
				}
			}
		}
	}
	return out
}

// domainTypes returns, for each package of internal/, the types its domain files declare: the domain types of
// GOARCH-23 and GOARCH-33.
func (m *module) domainTypes() map[string]map[string]bool {
	out := map[string]map[string]bool{}
	for dir, files := range m.packages() {
		out[dir] = map[string]bool{}
		for _, f := range files {
			if f.kind == kindDomain {
				for name, what := range decls(f.ast) {
					if what == "type" {
						out[dir][name] = true
					}
				}
			}
		}
	}
	return out
}

// camel turns a snake case stem into CamelCase: check_rule -> CheckRule.
func camel(stem string) string {
	var b strings.Builder
	for w := range strings.SplitSeq(stem, "_") {
		if w != "" {
			b.WriteString(strings.ToUpper(w[:1]))
			b.WriteString(w[1:])
		}
	}
	return b.String()
}

// lowerCamel turns a snake case stem into lowerCamelCase: check_rule -> checkRule.
func lowerCamel(stem string) string {
	c := camel(stem)
	if c == "" {
		return c
	}
	return strings.ToLower(c[:1]) + c[1:]
}
