package workflow

import (
	"path"
	"strings"
	"unicode"
)

// Rules is the key `rules` of polix.yaml: how the checks of the rules of ai/rules/ run.
type Rules struct {
	// Run holds the runners in file order; the first whose glob matches a check runs it.
	Run  []Runner
	Keys []string // the keys the file used, so Valid rejects unknown ones
}

// Runner runs the checks whose base name matches Glob with Argv, after expanding the placeholders {file} (the
// check, relative to the root, with slashes), {dir} (its folder as ./<folder>), {id} (GOARCH-4) and {name} (the
// id without separators, GOARCH4).
type Runner struct {
	Glob string
	Argv []string
	Line int // line in the file, for messages
}

// RunnerPlaceholders are the placeholders the argv of a runner may use.
var RunnerPlaceholders = []string{"file", "dir", "id", "name"}

// Valid tells why the rules are not valid: an unknown key or a runner that is not valid.
func (r Rules) Valid() error {
	v := &validator{}
	v.rules(r)
	if len(v.problems) > 0 {
		return Invalid(v.problems)
	}
	return nil
}

// Valid tells why the runner is not valid: an empty argv, an invalid glob or an unknown placeholder.
func (r Runner) Valid() error {
	v := &validator{}
	v.runner(r)
	if len(v.problems) > 0 {
		return Invalid(v.problems)
	}
	return nil
}

// CheckCommand returns the argv that runs the check of the rule id, from the first runner whose glob matches the
// base name of check, with its placeholders expanded; nil when no runner matches, and the check then runs as a
// program.
func (w Workflow) CheckCommand(check, id string) []string {
	check = strings.TrimPrefix(path.Clean(strings.ReplaceAll(check, `\`, "/")), "./")
	for _, r := range w.Rules.Run {
		if ok, err := path.Match(r.Glob, path.Base(check)); err != nil || !ok {
			continue
		}
		dir := path.Dir(check)
		switch {
		case dir == ".":
			dir = "./"
		case !path.IsAbs(dir):
			dir = "./" + dir
		}
		name := strings.Map(func(c rune) rune {
			if unicode.IsLetter(c) || unicode.IsDigit(c) {
				return c
			}
			return -1
		}, id)
		repl := strings.NewReplacer("{file}", check, "{dir}", dir, "{id}", id, "{name}", name)
		out := make([]string, len(r.Argv))
		for i, a := range r.Argv {
			out[i] = repl.Replace(a)
		}
		return out
	}
	return nil
}

func (v *validator) rules(r Rules) {
	v.unknownKeys(0, "rules", r.Keys, []string{"run"})
	for _, run := range r.Run {
		v.runner(run)
	}
}

func (v *validator) runner(r Runner) {
	if strings.TrimSpace(r.Glob) == "" {
		v.add(r.Line, "rules.run: a glob cannot be empty")
	}
	if len(r.Argv) == 0 {
		v.add(r.Line, "rules.run %q: the command is empty", r.Glob)
	}
	if _, err := path.Match(r.Glob, ""); err != nil {
		v.add(r.Line, "rules.run %q: invalid glob: %v", r.Glob, err)
	}
	for _, a := range r.Argv {
		for _, m := range placeholder.FindAllStringSubmatch(a, -1) {
			if !contains(RunnerPlaceholders, m[1]) {
				v.add(r.Line, "rules.run %q: unknown placeholder {%s} (%s)", r.Glob, m[1],
					strings.Join(RunnerPlaceholders, ", "))
			}
		}
	}
}
