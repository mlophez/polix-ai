package architecture

import (
	"fmt"
	"path"
	"strings"
	"testing"
)

// GOARCH-35: each test file is named after the file it tests: <stem>_test.go sits next to <stem>.go, so the test
// of a file is found by its name.
func TestGOARCH35(t *testing.T) {
	checkRule(t, checkTestNames, "internal/app/order/misc_test.go: tests no file, there is no misc.go")
}

func checkTestNames(m *module) []string {
	have := map[string]bool{}
	for _, f := range m.files {
		have[f.rel] = true
	}
	var out []string
	for _, f := range m.files {
		stem, ok := strings.CutSuffix(f.name, "_test.go")
		if ok && f.place.level != levelCmd && !have[path.Join(f.dir, stem+".go")] {
			out = append(out, fmt.Sprintf("%s: tests no file, there is no %s.go", f.rel, stem))
		}
	}
	return out
}
