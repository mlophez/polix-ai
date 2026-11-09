package architecture

import (
	"fmt"
	"path"
	"strings"
	"testing"
)

// GOARCH-9: the direction of every import follows from the depth. A use case imports the domains of any noun and
// its own helpers; it never imports another use case. A domain imports only other domains, never a use case nor a
// helper. A helper is imported only by the files of its use case package, tests included. Only cmd/ and tests/
// import a use case. The files of the tests count as their package; internal/platform/ is GOARCH-42.
func TestGOARCH9(t *testing.T) {
	checkRule(t, checkDirection,
		"internal/app/cart/cart_rebate.go:5: the domain internal/app/cart imports the use case "+
			"internal/app/order/place: only cmd/ and tests/ import a use case",
		"internal/app/cart/cart_rebate.go:6: the domain internal/app/cart imports the helper "+
			"internal/app/order/place/pricing: only the files of its use case internal/app/order/place import it",
		"the use case internal/app/order/cancel imports the use case internal/app/order/place",
		"the use case internal/app/order/cancel imports the use case internal/app/stock/count",
		"the use case internal/app/order/cancel imports the helper internal/app/order/place/pricing",
		"the helper internal/app/order/place/tax imports the helper internal/app/order/place/pricing",
		"internal/app/stock/stock_test.go:4: the domain internal/app/stock imports the use case "+
			"internal/app/order/cancel")
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
			}
		}
	}
	return out
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
	}
	return f.dir
}
