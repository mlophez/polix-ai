package architecture

import (
	"fmt"
	"go/ast"
	"path"
	"strings"
	"testing"
)

// GOARCH-9: the direction of every import follows from the depth. A use case imports the domains of any noun and its
// own helpers; it never imports another use case. A domain imports only other domains, never a use case nor a helper. A
// helper is imported only by the files of its use case package, tests included. Only cmd/ and tests/ import a use case.
// Only cmd/, tests/ and the test files of internal/app/ import an adapters package, which imports neither a use case
// nor another adapters package. The files of the tests count as their package; internal/platform/ is GOARCH-42.
func TestGOARCH9(t *testing.T) {
	checkRule(t, checkDirection,
		"internal/app/cart/rebate.go:5: the domain internal/app/cart imports the use case "+
			"internal/app/order/features/place: only cmd/ and tests/ import a use case",
		"internal/app/cart/rebate.go:6: the domain internal/app/cart imports the helper "+
			"internal/app/order/features/place/pricing: only the files of its use case internal/app/order/features/place import it",
		"the use case internal/app/order/features/cancel imports the use case internal/app/order/features/place",
		"the use case internal/app/order/features/cancel imports the use case internal/app/stock/features/count",
		"the use case internal/app/order/features/cancel imports the helper internal/app/order/features/place/pricing",
		"the helper internal/app/order/features/place/tax imports the helper internal/app/order/features/place/pricing",
		"internal/app/stock/stock_test.go:4: the domain internal/app/stock imports the use case "+
			"internal/app/order/features/cancel",
		"internal/app/order/features/cancel/cancel_out_receipt_test.go:6: the use case internal/app/order/features/cancel imports the use "+
			"case internal/app/order/features/place: only cmd/ and tests/ import a use case",
		"internal/app/cart/io.go:4: the domain internal/app/cart imports the adapters package internal/app/order/adapters: "+
			"only cmd/, tests/ and the test files of internal/app/ import an adapters package",
		"internal/app/order/features/ship/ship_out_files.go:4: the use case internal/app/order/features/ship imports the adapters "+
			"package internal/app/order/adapters",
		"internal/app/order/adapters/file_imports.go:5: the adapters package internal/app/order/adapters imports the use case "+
			"internal/app/stock/features/count",
		"internal/app/order/adapters/file_imports.go:6: the adapters package internal/app/order/adapters imports the adapters package "+
			"internal/app/stock/adapters: an adapters package imports neither a use case nor another adapters package")
	checkAccepts(t, checkDirection, "internal/app/order/features/cancel/cancel_out_receipt_test.go:7:", "cmd/app/files.go",
		"internal/app/stock/features/count/count_out_files_test.go", "internal/app/stock/io_test.go")
}

func checkDirection(m *module) []string {
	var out []string
	for _, f := range m.files {
		if f.place.level == levelLibrary || f.place.level == levelPlatform {
			continue // GOARCH-42
		}
		for _, s := range f.ast.Imports {
			dir := m.moduleDir(strings.Trim(s.Path.Value, `"`))
			if dir == "" || dir == f.dir {
				continue
			}
			switch placeOf(dir).level {
			case levelUseCase:
				if f.place.level != levelCmd {
					out = append(out, fmt.Sprintf("%s: %s imports the use case %s: only cmd/ and tests/ import a use case",
						m.pos(f, s), who(f), dir))
				}
			case levelHelper:
				if owner := path.Dir(dir); f.dir != owner {
					out = append(out, fmt.Sprintf("%s: %s imports the helper %s: only the files of its use case %s "+
						"import it", m.pos(f, s), who(f), dir, owner))
				}
			case levelAdapters:
				out = append(out, adaptersImport(m, f, s, dir)...)
			default:
			}
		}
	}
	return out
}

// adaptersImport reports the import s of the adapters package dir by f: only cmd/ and the test files of internal/app/
// import one, and an adapters package, its tests included, imports no other adapters package.
func adaptersImport(m *module, f *goFile, s *ast.ImportSpec, dir string) []string {
	switch {
	case f.place.level == levelAdapters:
		return []string{fmt.Sprintf("%s: %s imports the adapters package %s: an adapters package imports neither a use case "+
			"nor another adapters package", m.pos(f, s), who(f), dir)}
	case f.place.level == levelCmd || f.kind == kindTest:
		return nil
	}
	return []string{fmt.Sprintf("%s: %s imports the adapters package %s: only cmd/, tests/ and the test files of "+
		"internal/app/ import an adapters package", m.pos(f, s), who(f), dir)}
}

// who names the package of f by the kind of its folder: the domain internal/app/rule.
func who(f *goFile) string {
	switch f.place.level {
	case levelDomain:
		return "the domain " + f.dir
	case levelUseCase:
		return "the use case " + f.dir
	case levelHelper:
		return "the helper " + f.dir
	case levelAdapters:
		return "the adapters package " + f.dir
	default:
		return f.dir
	}
}
