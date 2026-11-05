package tests

import (
	"fmt"
	"sort"
	"strings"
	"testing"
)

// GOARCH-7: the packages form a DAG: pkg/ never imports internal/, and the packages of internal/ have no import
// cycle (the compiler would catch it too, but only once the module compiles).
func TestGOARCH07DAG(t *testing.T) {
	checkRule(t, checkDAG, "pkg/util/util.go:3: imports example.com/app/internal/order",
		"import cycle: internal/order -> internal/stock -> internal/order")
}

func checkDAG(m *module) []string {
	var out []string
	graph := map[string]map[string]bool{}
	for _, f := range m.files {
		for _, s := range f.ast.Imports {
			dir := internalDir(strings.Trim(s.Path.Value, `"`))
			if dir == "" {
				continue
			}
			switch f.top {
			case "pkg":
				out = append(out, fmt.Sprintf("%s: imports %s: pkg/ never imports internal/", m.pos(f, s), strings.Trim(s.Path.Value, `"`)))
			case "internal":
				if f.kind != kindTest && dir != f.dir {
					if graph[f.dir] == nil {
						graph[f.dir] = map[string]bool{}
					}
					graph[f.dir][dir] = true
				}
			}
		}
	}
	return append(out, cycles(graph)...)
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
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}
