package architecture

import (
	"slices"
	"sort"
	"strings"
	"testing"
)

// GOARCH-7: the packages of internal/ have no import cycle (the compiler would catch it too, but only once the
// module compiles). That internal/platform/ never imports internal/app/ is GOARCH-42, which depguard decides.
func TestGOARCH7(t *testing.T) {
	checkRule(t, checkDAG,
		"import cycle: internal/app/order -> internal/app/stock -> internal/app/order",
		"import cycle: internal/platform/text -> internal/platform/util -> internal/platform/text")
}

func checkDAG(m *module) []string {
	graph := map[string]map[string]bool{}
	for _, f := range m.files {
		if f.place.level == levelCmd || f.kind == kindTest {
			continue
		}
		for _, s := range f.ast.Imports {
			p := strings.Trim(s.Path.Value, `"`)
			dir := m.moduleDir(p)
			if dir == "" || dir == f.dir {
				continue
			}
			if graph[f.dir] == nil {
				graph[f.dir] = map[string]bool{}
			}
			graph[f.dir][dir] = true
		}
	}
	return cycles(graph)
}

// cycles returns each import cycle of graph once, starting at its smallest package.
func cycles(graph map[string]map[string]bool) []string {
	var nodes []string
	for n := range graph {
		nodes = append(nodes, n)
	}
	sort.Strings(nodes)
	seen := map[string]bool{}
	var out []string
	var walk func(start, n string, trail []string)
	walk = func(start, n string, trail []string) {
		var next []string
		for to := range graph[n] {
			next = append(next, to)
		}
		sort.Strings(next)
		for _, to := range next {
			if to == start {
				c := strings.Join(append(trail, start), " -> ")
				if !seen[c] {
					seen[c] = true
					out = append(out, "import cycle: "+c)
				}
				continue
			}
			if to > start && !contains(trail, to) {
				walk(start, to, append(trail, to))
			}
		}
	}
	for _, n := range nodes {
		walk(n, n, []string{n})
	}
	return out
}

func contains(list []string, s string) bool {
	return slices.Contains(list, s)
}
