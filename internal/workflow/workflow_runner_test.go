package workflow

import (
	"strings"
	"testing"
)

func runners(rs ...Runner) Workflow { return Workflow{Version: 1, Rules: Rules{Run: rs}} }

func TestCheckCommand(t *testing.T) {
	test := Runner{Glob: "*_test.go", Argv: []string{"go", "test", "{dir}", "-run", "^Test{name}$"}}
	all := Runner{Glob: "*", Argv: []string{"run", "{file}", "{id}"}}
	cases := []struct {
		name, check, id, want string
		w                     Workflow
	}{
		{"glob matches the base name", "tests/architecture/goarch04_test.go", "GOARCH-4",
			"go test ./tests/architecture -run ^TestGOARCH4$", runners(test)},
		{"the first matching runner wins", "tests/a_test.go", "GO-2", "go test ./tests -run ^TestGO2$", runners(test, all)},
		{"file and id", "scripts/check.sh", "XPLAT-1", "run scripts/check.sh XPLAT-1", runners(test, all)},
		{"dir of a check at the root", "a_test.go", "R-1", "go test ./ -run ^TestR1$", runners(test)},
		{"backslashes", `tests\arch\x_test.go`, "R-1", "go test ./tests/arch -run ^TestR1$", runners(test)},
		{"absolute dir keeps its path", "/abs/x_test.go", "R-1", "go test /abs -run ^TestR1$", runners(test)},
		{"no runner matches", "scripts/check.sh", "R-1", "", runners(test)},
		{"no runners", "a_test.go", "R-1", "", Workflow{}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := c.w.CheckCommand(c.check, c.id)
			if strings.Join(got, " ") != c.want || (c.want == "") != (got == nil) {
				t.Fatalf("got %q, want %q", got, c.want)
			}
		})
	}
}

func TestRunnerValid(t *testing.T) {
	cases := map[string]Runner{
		"the command is empty":        {Glob: "*_test.go"},
		"a glob cannot be empty":      {Argv: []string{"x"}},
		"invalid glob":                {Glob: "[", Argv: []string{"x"}},
		"unknown placeholder {other}": {Glob: "*", Argv: []string{"x", "--{other}"}},
	}
	for want, r := range cases {
		if err := r.Valid(); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%s: got %v", want, err)
		}
		if err := runners(r).Valid(); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%s in a workflow: got %v", want, err)
		}
	}
	ok := Runner{Glob: "*_test.go", Argv: []string{"go", "test", "{dir}", "{file}", "{id}", "{name}"}}
	if err := ok.Valid(); err != nil {
		t.Fatal(err)
	}
	if err := (Rules{Run: []Runner{ok}, Keys: []string{"run", "x"}}).Valid(); err == nil {
		t.Fatal("an unknown key in rules")
	}
}
