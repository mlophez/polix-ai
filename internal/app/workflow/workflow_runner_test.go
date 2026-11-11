package workflow

import (
	"errors"
	"strconv"
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

func TestCheckCommands(t *testing.T) {
	batch := Runner{Glob: "*_test.go", Argv: []string{"go", "test", "{dir}", "-run", "^Test({names})$"}}
	all := Runner{Glob: "*_test.go", Argv: []string{"go", "test", "./..."}}
	single := Runner{Glob: "*_test.go", Argv: []string{"go", "test", "{dir}", "-run", "^Test{name}$"}}
	plural := Runner{Glob: "*", Argv: []string{"lint", "{files}", "{dirs}"}}
	arch := Runner{Glob: "tests/architecture/*_test.go", Argv: []string{"arch", "{names}"}}
	e2e := Runner{Glob: "tests/e2e/*_test.go", Argv: []string{"e2e", "{names}"}}
	cases := []struct {
		name       string
		w          Workflow
		files, ids []string
		want       []string // each invocation as "argv|positions"
	}{
		{"grouped by dir with names", runners(batch),
			[]string{"a/x_test.go", "b/y_test.go", "a/z_test.go"}, []string{"R-1", "R-2", "R-3"},
			[]string{"go test ./a -run ^Test(R1|R3)$|0 2", "go test ./b -run ^Test(R2)$|1"}},
		{"one invocation without placeholders", runners(all),
			[]string{"a/x_test.go", "b/y_test.go"}, []string{"R-1", "R-2"}, []string{"go test ./...|0 1"}},
		{"one invocation per rule with name", runners(single),
			[]string{"a/x_test.go", "a/y_test.go"}, []string{"R-1", "R-2"},
			[]string{"go test ./a -run ^TestR1$|0", "go test ./a -run ^TestR2$|1"}},
		{"files and dirs deduplicated in rule order", runners(plural),
			[]string{"b/y.sh", "a/x.sh", "b/y.sh"}, []string{"R-1", "R-2", "R-3"},
			[]string{"lint b/y.sh a/x.sh ./b ./a|0 1 2"}},
		{"two runners by path, in order of their first check", runners(e2e, arch),
			[]string{"tests/architecture/a_test.go", "tests/e2e/b_test.go", "tests/architecture/c_test.go"},
			[]string{"A-1", "E-1", "A-2"}, []string{"arch A1|A2|0 2", "e2e E1|1"}},
		{"checks without runner run alone", runners(arch),
			[]string{"x.sh", "tests/architecture/a_test.go", "y.sh"}, []string{"R-1", "A-1", "R-2"},
			[]string{"|0", "arch A1|1", "|2"}},
		{"a glob with / matches only under its folder", runners(arch),
			[]string{"other/a_test.go"}, []string{"R-1"}, []string{"|0"}},
		{"a glob without / matches any folder", runners(batch),
			[]string{"deep/er/a_test.go"}, []string{"R-1"}, []string{"go test ./deep/er -run ^Test(R1)$|0"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var got []string
			for _, inv := range c.w.CheckCommands(c.files, c.ids) {
				if err := inv.Valid(); err != nil {
					t.Fatal(err)
				}
				pos := make([]string, len(inv.Checks))
				for i, p := range inv.Checks {
					pos[i] = strconv.Itoa(p)
				}
				got = append(got, strings.Join(inv.Argv, " ")+"|"+strings.Join(pos, " "))
			}
			if strings.Join(got, "\n") != strings.Join(c.want, "\n") {
				t.Fatalf("got %q, want %q", got, c.want)
			}
		})
	}
}

func TestCheckCommandIsTheSingleCheckCommands(t *testing.T) {
	batch := Runner{Glob: "*_test.go", Argv: []string{"go", "test", "{files}", "{dirs}", "-run", "^Test({names})$"}}
	w := runners(batch)
	got := strings.Join(w.CheckCommand("a/x_test.go", "R-1"), " ")
	want := strings.Join(w.CheckCommands([]string{"a/x_test.go"}, []string{"R-1"})[0].Argv, " ")
	if got != want || got != "go test a/x_test.go ./a -run ^Test(R1)$" {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestInvocationValid(t *testing.T) {
	cases := map[string]Invocation{
		"no checks":                 {Argv: []string{"x"}},
		"a repeated check":          {Argv: []string{"x"}, Checks: []int{0, 0}},
		"a negative position":       {Argv: []string{"x"}, Checks: []int{-1}},
		"several without a command": {Checks: []int{0, 1}},
	}
	for name, inv := range cases {
		if inv.Valid() == nil {
			t.Errorf("%s is valid", name)
		}
	}
	for _, inv := range []Invocation{{Checks: []int{0}}, {Argv: []string{"x"}, Checks: []int{2, 0}}} {
		if err := inv.Valid(); err != nil {
			t.Error(err)
		}
	}
}

// ValidPath tells only the problem of rules.path, in the words of Workflow.Problems; the runners do not count.
func TestRulesValidPath(t *testing.T) {
	broken := []Runner{{Glob: "*"}}
	for _, p := range []string{"", "ai/rules", "docs/rules/"} {
		if err := (Rules{Path: p, Run: broken}).ValidPath(); err != nil {
			t.Errorf("%q: %v", p, err)
		}
	}
	for p, want := range map[string]string{
		"../x": `rules.path "../x" must be below the root`,
		"/etc": `rules.path "/etc" must be relative to the root, not absolute`,
		".":    `rules.path "." cannot be the root itself`,
		"ai/*": `rules.path "ai/*" must be a folder, not a glob`,
	} {
		err := (Rules{Path: p}).ValidPath()
		if _, invalid := errors.AsType[InvalidError](err); !invalid || !strings.Contains(err.Error(), want) {
			t.Errorf("%q: got %v, want %s", p, err, want)
		}
		if w := (Workflow{Version: 1, Rules: Rules{Path: p}}); !strings.Contains(strings.Join(w.Problems(), "\n"), want) {
			t.Errorf("%q: the workflow says it otherwise: %v", p, w.Problems())
		}
	}
}

func TestRunnerValid(t *testing.T) {
	cases := map[string]Runner{
		"the command is empty":   {Glob: "*_test.go"},
		"a glob cannot be empty": {Argv: []string{"x"}},
		"unknown placeholder {other} (file, dir, id, name, files, dirs, names)": {Glob: "*",
			Argv: []string{"x", "--{other}"}},
		"{files} must be a whole argument": {Glob: "*", Argv: []string{"x", "--x={files}"}},
		"{dirs} must be a whole argument":  {Glob: "*", Argv: []string{"x", "{dirs}/y"}},
	}
	for want, r := range cases {
		if err := r.Valid(); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%s: got %v", want, err)
		}
		if err := runners(r).Valid(); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%s in a workflow: got %v", want, err)
		}
	}
	ok := Runner{Glob: "[x", Argv: []string{"go", "test", "{dir}", "{file}", "{id}", "{name}", "{files}", "{dirs}",
		"-run=^({names})$"}}
	if err := ok.Valid(); err != nil {
		t.Fatal(err)
	}
	if err := (Rules{Run: []Runner{ok}, Keys: []string{"run", "x"}}).Valid(); err == nil {
		t.Fatal("an unknown key in rules")
	}
}
