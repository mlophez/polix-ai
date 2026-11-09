package architecture

import (
	"fmt"
	"strings"
	"testing"
)

// GOARCH-1: the depth of a folder gives the kind of its code, and each level admits a closed set of children:
// internal/ holds app/ and platform/; internal/app/ holds domain folders; a domain folder internal/app/<noun>/ holds
// <noun>.go, <noun>_*.go, their tests and use case folders; a use case folder internal/app/<noun>/<verb>/ holds
// in.go, logic.go, out.go, their siblings in_<topic>.go, logic_<topic>.go and out_<topic>.go, <verb>.md, their
// tests and helper folders; a helper folder holds <helper>.go, <helper>_*.go and their tests, and no folder;
// internal/platform/ holds library folders; a library folder holds Go files and their tests, and no folder.
// testdata/ is admitted at any level. Every package clause equals the name of its folder (of the tests,
// GOARCH-38).
func TestGOARCH1(t *testing.T) {
	checkRule(t, checkLayout, "internal/app/order/helpers.go: neither domain (order.go, order_*.go) nor a test",
		"internal/app/order/notes.md: not the document of a use case",
		"internal/app/order/ship/notes.go: a use case folder holds only in.go, logic.go, out.go",
		"internal/app/order/ship/in.go: declares package shipping, not ship",
		"internal/app/order/place/pricing/rates: a folder below the helper internal/app/order/place/pricing",
		"internal/platform/text/unicode: a folder below the library internal/platform/text")
	checkAccepts(t, checkLayout, "internal/app/stock/take/", "internal/app/stock/count/in_scan.go",
		"internal/app/stock/count/logic_audit.go", "internal/app/order/cancel/out_receipt.go")
	t.Run("siblings", func(t *testing.T) {
		uc := placeOf("internal/app/rule/check")
		for name, want := range map[string]kind{
			"in_claude.go": kindIn, "logic_session.go": kindLogic, "out_yaml.go": kindOut,
			"in_claude_test.go": kindTest, "in_.go": kindUnknown, "inbox.go": kindUnknown, "logic.go": kindLogic,
		} {
			if got := classify(uc, name); got != want {
				t.Errorf("classify(%q) = %d, want %d", name, got, want)
			}
		}
	})
	t.Run("testdata", func(t *testing.T) {
		for _, v := range checkLayout(mustLoad(t, badRoot)) {
			if where, _, _ := strings.Cut(v, ":"); strings.Contains(where, "testdata") {
				t.Errorf("testdata/ is admitted at any level: %s", v)
			}
		}
	})
}

func checkLayout(m *module) []string {
	var out []string
	for _, dir := range m.dirs {
		if at := placeOf(dir); at.level == levelOutside {
			out = append(out, fmt.Sprintf("%s: %s", dir, at.why))
		}
	}
	for _, f := range m.files {
		if f.place.level == levelCmd || f.place.level == levelOutside {
			continue // cmd/ is GOARCH-6; a folder out of the layout is reported once, above
		}
		if f.kind == kindUnknown {
			out = append(out, f.rel+": "+admits(f.place, f.dir))
		}
		if f.kind != kindTest && f.ast.Name.Name != f.pkg {
			out = append(out, fmt.Sprintf("%s: declares package %s, not %s: a package is named after its folder",
				f.rel, f.ast.Name.Name, f.pkg))
		}
	}
	for _, d := range m.docs {
		if d.place.level != levelUseCase || d.name != d.place.verb+".md" {
			out = append(out, d.rel+": not the document of a use case: only a use case folder holds one, "+
				"<verb>.md (GOARCH-14)")
		}
	}
	return out
}

// admits tells what the folder dir at a level holds, for the message of a file it does not admit.
func admits(at place, dir string) string {
	switch at.level {
	case levelInternal:
		return "internal/ holds only the folders app/ and platform/"
	case levelApp:
		return "internal/app/ holds only domain folders"
	case levelPlatform:
		return "internal/platform/ holds only library folders"
	case levelDomain:
		return fmt.Sprintf("neither domain (%s.go, %s_*.go) nor a test: a domain folder holds only its domain "+
			"files, their tests and use case folders", at.noun, at.noun)
	case levelUseCase:
		return fmt.Sprintf("a use case folder holds only in.go, logic.go, out.go, their siblings in_<topic>.go, "+
			"logic_<topic>.go and out_<topic>.go, %s.md, their tests and helper folders", at.verb)
	case levelHelper:
		return fmt.Sprintf("a helper folder holds only %s.go, %s_*.go and their tests", at.helper, at.helper)
	default:
		return "out of the layout of " + dir
	}
}
