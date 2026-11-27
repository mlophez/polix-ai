package architecture

import (
	"fmt"
	"go/ast"
	"go/token"
	"path"
	"strings"
	"testing"
)

// GOARCH-1: the depth of a folder gives the kind of its code, and each level admits a closed set of children:
// internal/ holds app/ and platform/; internal/app/ holds domain folders; a domain folder internal/app/<noun>/ holds
// Go files (the entity <noun>.go and a <topic>.go per part of the domain), their tests and the folders features/
// and adapters/, and no other folder; features/ holds use case folders, at least one, and no Go file; a use case
// folder internal/app/<noun>/features/<verb>/ holds <verb>_in.go, <verb>_logic.go, <verb>_out.go, their siblings
// <verb>_in_<topic>.go, <verb>_logic_<topic>.go and <verb>_out_<topic>.go, <verb>.md, their tests and helper
// folders; a helper folder holds <helper>.go, <helper>_*.go and their tests, and no folder; adapters/ holds
// <concept>.go, <concept>_*.go and their tests, where <concept> is the lower case name of an exported struct of the
// package, and no folder; internal/platform/ holds library folders; a library folder holds Go files and their tests,
// and no folder. testdata/ is admitted at any level. The package clause is GOARCH-49.
func TestGOARCH1(t *testing.T) {
	checkRule(t, checkLayout,
		"internal/app/order/refund: a domain folder holds only its domain files, their tests, features/ and "+
			"adapters/: use case folders go in features/, as internal/app/order/features/refund",
		"internal/app/order/adapters/notes.go: an adapters file is named after no exported struct of its package",
		"internal/app/order/adapters/convert: a folder below the adapters package internal/app/order/adapters",
		"internal/app/order/features/shared.go: features/ holds only use case folders",
		"internal/app/order/features/shared_test.go: features/ holds only use case folders",
		"internal/app/cart/features: features/ holds no use case",
		"internal/app/order/notes.md: not the document of a use case",
		"internal/app/order/order_extra.go: a domain file is named after its topic without the noun",
		"internal/app/order/features/ship/notes.go: a use case folder holds only ship_in.go, ship_logic.go, "+
			"ship_out.go",
		"internal/app/order/features/ship/in.go: a use case folder holds only ship_in.go",
		"internal/app/order/features/ship/ship_input.go: a use case folder holds only ship_in.go",
		"internal/app/order/features/place/pricing/rates: a folder below the helper internal/app/order/features/place/pricing",
		"internal/platform/text/unicode: a folder below the library internal/platform/text")
	checkAccepts(t, checkLayout, "internal/app/stock/features/take/", "internal/app/stock/features/count/count_in_scan.go",
		"internal/app/stock/features/count/count_logic_audit.go", "internal/app/order/features/cancel/cancel_out_receipt.go",
		"internal/app/order/adapters/file.go", "internal/app/order/adapters/file_faults.go", "internal/app/stock/adapters/log.go",
		"internal/app/order/helpers.go")
	t.Run("siblings", func(t *testing.T) {
		uc := placeOf("internal/app/rule/features/check")
		for name, want := range map[string]kind{
			"check_in_claude.go": kindIn, "check_logic_session.go": kindLogic, "check_out_yaml.go": kindOut,
			"check_in_claude_test.go": kindTest, "check_in_.go": kindUnknown, "check_inbox.go": kindUnknown,
			"check_logic.go": kindLogic, "logic.go": kindUnknown, "in_claude.go": kindUnknown,
			"format_in.go": kindUnknown,
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

// GOARCH-49: every package clause of internal/ equals the name of its folder (of the tests, GOARCH-38).
func TestGOARCH49(t *testing.T) {
	checkRule(t, checkPackageClause,
		"internal/app/order/features/ship/ship_in.go: declares package shipping, not ship")
}

// checkPackageClause reports the files of internal/ whose package clause is not the name of their folder; cmd/ is
// GOARCH-6 and the tests are GOARCH-38.
func checkPackageClause(m *module) []string {
	var out []string
	for _, f := range m.files {
		if f.place.level == levelCmd || f.place.level == levelOutside || f.kind == kindTest {
			continue
		}
		if f.ast.Name.Name != f.pkg {
			out = append(out, fmt.Sprintf("%s: declares package %s, not %s: a package is named after its folder",
				f.rel, f.ast.Name.Name, f.pkg))
		}
	}
	return out
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
	}
	out = append(out, checkAdaptersNames(m)...)
	out = append(out, checkFeaturesNotEmpty(m)...)
	for _, d := range m.docs {
		if d.place.level != levelUseCase || d.name != d.place.verb+".md" {
			out = append(out, d.rel+": not the document of a use case: only a use case folder holds one, "+
				"<verb>.md (GOARCH-14)")
		}
	}
	return out
}

// checkAdaptersNames reports the files of an adapters package that are named after no exported struct of it: its files
// are <concept>.go and <concept>_*.go, with their tests, where <concept> is the lower case name of an exported struct.
func checkAdaptersNames(m *module) []string {
	concepts := map[string][]string{} // folder -> lower case names of its exported structs
	for _, f := range m.files {
		if f.place.level != levelAdapters || f.kind == kindTest {
			continue
		}
		for _, d := range f.ast.Decls {
			if g, ok := d.(*ast.GenDecl); ok && g.Tok == token.TYPE {
				for _, t := range typeSpecs(g) {
					if _, isStruct := t.Type.(*ast.StructType); isStruct && t.Name.IsExported() {
						concepts[f.dir] = append(concepts[f.dir], strings.ToLower(t.Name.Name))
					}
				}
			}
		}
	}
	var out []string
	for _, f := range m.files {
		if f.place.level != levelAdapters {
			continue
		}
		stem := strings.TrimSuffix(strings.TrimSuffix(f.name, ".go"), "_test")
		named := false
		for _, c := range concepts[f.dir] {
			named = named || stem == c || strings.HasPrefix(stem, c+"_")
		}
		if !named {
			out = append(out, f.rel+": an adapters file is named after no exported struct of its package: an adapters "+
				"package "+
				"holds only <concept>.go and <concept>_*.go, with their tests, where <concept> is the lower case "+
				"name of an exported struct")
		}
	}
	return out
}

// checkFeaturesNotEmpty reports the folders features/ that hold no use case folder with a Go file: a noun without
// commands has no features/.
func checkFeaturesNotEmpty(m *module) []string {
	withGo := map[string]bool{}
	for _, f := range m.files {
		withGo[f.dir] = true
	}
	used := map[string]bool{}
	for _, dir := range m.dirs {
		if at := placeOf(dir); at.level == levelUseCase && withGo[dir] {
			used[path.Dir(dir)] = true
		}
	}
	var out []string
	for _, dir := range m.dirs {
		if placeOf(dir).level == levelFeatures && !used[dir] {
			out = append(out, dir+": features/ holds no use case: a features/ holds at least one use case folder "+
				"with a Go file, and a noun without commands has no features/")
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
		return fmt.Sprintf("a domain file is named after its topic without the noun: %s.go or <topic>.go, never "+
			"%s_<topic>.go", at.noun, at.noun)
	case levelFeatures:
		return "features/ holds only use case folders, each in its own folder internal/app/<noun>/features/<verb>/"
	case levelUseCase:
		return fmt.Sprintf("a use case folder holds only %[1]s_in.go, %[1]s_logic.go, %[1]s_out.go, their siblings "+
			"%[1]s_in_<topic>.go, %[1]s_logic_<topic>.go and %[1]s_out_<topic>.go, %[1]s.md, their tests and "+
			"helper folders", at.verb)
	case levelHelper:
		return fmt.Sprintf("a helper folder holds only %s.go, %s_*.go and their tests", at.helper, at.helper)
	default:
		return "out of the layout of " + dir
	}
}
