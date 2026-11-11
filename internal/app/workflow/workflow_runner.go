package workflow

import (
	"errors"
	"path"
	"slices"
	"strconv"
	"strings"
	"unicode"

	"github.com/mlophez/polix-ai/internal/platform/glob"
)

// Rules is the key `rules` of polix.yaml: the folder of the rules (Workflow.RulesPath) and how their checks run.
type Rules struct {
	Path string // the folder of the rules relative to the root, empty for DefaultRulesPath
	// Run holds the runners in file order; the first whose glob matches a check runs it.
	Run  []Runner
	Keys []string // the keys the file used, so Valid rejects unknown ones
}

// Runner runs the checks that match Glob with Argv. The glob matches as those of scope and protected do
// (internal/platform/glob): without `/` against the base name of the check, with `/` against its path relative
// to the root.
// The singular placeholders stand for one check: {file} (the check, relative to the root, with slashes), {dir}
// (its folder as ./<folder>), {id} (GOARCH-4) and {name} (the id with only its letters and digits, GOARCH4). The
// plural ones stand for every check of a group: {names} (their names joined with |), {files} and {dirs} (one
// argument per distinct file or folder, so each must be a whole argument). The checks of a runner whose argv with
// the singular placeholders expanded is the same run together, in one invocation.
type Runner struct {
	Glob string
	Argv []string
	Line int // line in the file, for messages
}

// Invocation is one run of a runner that covers the checks at Checks, positions in the list given to
// Workflow.CheckCommands. An empty Argv runs its only check as a program, as when no runner matches. It is built
// with a literal followed by Valid.
type Invocation struct {
	Argv   []string
	Checks []int
}

// ruleCheck is a check given to Workflow.CheckCommands: the path of the check, as given, and the id of its rule.
type ruleCheck struct {
	file string
	id   string
}

// ruleChecks are the checks given to Workflow.CheckCommands, in their order.
type ruleChecks []ruleCheck

// RunnerPlaceholders are the placeholders the argv of a runner may use: the singular ones, then the plural ones.
var RunnerPlaceholders = []string{"file", "dir", "id", "name", "files", "dirs", "names"}

// runnerWhole are the plural placeholders that expand into several arguments, so they must be a whole argument.
var runnerWhole = []string{"files", "dirs"}

// Valid tells why the rules are not valid: an unknown key or a runner that is not valid.
func (r Rules) Valid() error {
	v := &validator{}
	v.rules(r)
	if len(v.problems) > 0 {
		return InvalidError(v.problems)
	}
	return nil
}

// ValidPath tells why rules.path cannot be the folder of the rules (absolute, the root itself, outside the root or a
// glob), in the words of Workflow.Problems; the runners and the keys of rules are left to Valid.
func (r Rules) ValidPath() error {
	v := &validator{}
	v.rulesPath(r)
	if len(v.problems) > 0 {
		return InvalidError(v.problems)
	}
	return nil
}

// Valid tells why the runner is not valid: an empty glob or argv, an unknown placeholder, or {files} or {dirs}
// inside a longer argument.
func (r Runner) Valid() error {
	v := &validator{}
	v.runner(r)
	if len(v.problems) > 0 {
		return InvalidError(v.problems)
	}
	return nil
}

// Valid tells why the invocation is not one: it covers no check, repeats one, or covers several without argv,
// since only a single check runs as a program.
func (i Invocation) Valid() error {
	switch {
	case len(i.Checks) == 0:
		return errors.New("an invocation covers at least one check")
	case len(i.Checks) > 1 && len(i.Argv) == 0:
		return errors.New("an invocation of several checks needs a command")
	}
	seen := map[int]bool{}
	for _, c := range i.Checks {
		if c < 0 || seen[c] {
			return errors.New("an invocation covers each check once, by its position")
		}
		seen[c] = true
	}
	return nil
}

// CheckCommand returns the argv that runs the check of the rule id alone, from the first runner whose glob
// matches check, with its placeholders expanded for that rule; nil when no runner matches, and the check then
// runs as a program. It equals the argv CheckCommands gives for that check alone.
func (w Workflow) CheckCommand(check, id string) []string {
	inv := w.CheckCommands([]string{check}, []string{id})
	if len(inv[0].Argv) == 0 {
		return nil
	}
	return inv[0].Argv
}

// CheckCommands returns the invocations that run the checks of several rules, files[i] the check of the rule
// ids[i]: each check goes to the first runner whose glob matches it; the checks of the same runner whose argv
// with the singular placeholders expanded is equal share one invocation, where the plural placeholders cover
// them all. Invocations come in the order of their first check; a check without runner is an invocation of its
// own with an empty argv.
func (w Workflow) CheckCommands(files, ids []string) []Invocation {
	var argvs [][]string
	var groups [][]int
	keys := map[string]int{}
	checks := make(ruleChecks, len(files))
	for i, f := range files {
		checks[i] = ruleCheck{file: f, id: ids[i]}
		check := checks[i].clean()
		at := slices.IndexFunc(w.Rules.Run, func(r Runner) bool { return glob.Path(r.Glob, check) })
		if at < 0 {
			argvs, groups = append(argvs, nil), append(groups, []int{i})
			continue
		}
		repl := strings.NewReplacer("{file}", check, "{dir}", checks[i].dir(), "{id}", ids[i],
			"{name}", checks[i].name())
		argv := make([]string, len(w.Rules.Run[at].Argv))
		for k, a := range w.Rules.Run[at].Argv {
			argv[k] = repl.Replace(a)
		}
		key := strconv.Itoa(at) + "\x00" + strings.Join(argv, "\x00")
		if g, ok := keys[key]; ok {
			groups[g] = append(groups[g], i)
			continue
		}
		keys[key] = len(groups)
		argvs, groups = append(argvs, argv), append(groups, []int{i})
	}
	out := make([]Invocation, 0, len(groups))
	for g, positions := range groups {
		inv := Invocation{Argv: checks.plurals(argvs[g], positions), Checks: positions}
		if err := inv.Valid(); err != nil {
			continue // never happens: every group has its checks and a runner when it has several
		}
		out = append(out, inv)
	}
	return out
}

// plurals expands the plural placeholders of argv with the checks at positions: {names} joined with |, {files} and
// {dirs} into one argument per distinct value, in order. A nil argv stays nil.
func (cs ruleChecks) plurals(argv []string, positions []int) []string {
	if argv == nil {
		return nil
	}
	var names, files, dirs []string
	for _, p := range positions {
		check := cs[p].clean()
		names = append(names, cs[p].name())
		if !slices.Contains(files, check) {
			files = append(files, check)
		}
		if d := cs[p].dir(); !slices.Contains(dirs, d) {
			dirs = append(dirs, d)
		}
	}
	out := make([]string, 0, len(argv))
	for _, a := range argv {
		switch a {
		case "{files}":
			out = append(out, files...)
		case "{dirs}":
			out = append(out, dirs...)
		default:
			out = append(out, strings.ReplaceAll(a, "{names}", strings.Join(names, "|")))
		}
	}
	return out
}

// clean is the path of the check cleaned: slashes, no ./ prefix.
func (c ruleCheck) clean() string {
	return strings.TrimPrefix(path.Clean(strings.ReplaceAll(c.file, `\`, "/")), "./")
}

// dir is the folder of the clean check as {dir} gives it: ./<folder>, ./ at the root, absolute as it is.
func (c ruleCheck) dir() string {
	dir := path.Dir(c.clean())
	switch {
	case dir == ".":
		return "./"
	case !path.IsAbs(dir):
		return "./" + dir
	}
	return dir
}

// name is the id of the rule with only its letters and digits, as {name} gives it.
func (c ruleCheck) name() string {
	return strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return r
		}
		return -1
	}, c.id)
}

func (v *validator) rules(r Rules) {
	v.unknownKeys(0, "rules", r.Keys, []string{"path", "run"})
	v.rulesPath(r)
	for _, run := range r.Run {
		v.runner(run)
	}
}

// rulesPath reports the problem of rules.path, the one text of it.
func (v *validator) rulesPath(r Rules) {
	if p := folderPath(r.Path).problem(); p != "" {
		v.add(0, "rules.path %q %s", r.Path, p)
	}
}

func (v *validator) runner(r Runner) {
	if strings.TrimSpace(r.Glob) == "" {
		v.add(r.Line, "rules.run: a glob cannot be empty")
	}
	if len(r.Argv) == 0 {
		v.add(r.Line, "rules.run %q: the command is empty", r.Glob)
	}
	for _, a := range r.Argv {
		for _, m := range placeholder.FindAllStringSubmatch(a, -1) {
			switch {
			case !slices.Contains(RunnerPlaceholders, m[1]):
				v.add(r.Line, "rules.run %q: unknown placeholder {%s} (%s)", r.Glob, m[1],
					strings.Join(RunnerPlaceholders, ", "))
			case slices.Contains(runnerWhole, m[1]) && a != m[0]:
				v.add(r.Line, "rules.run %q: {%s} must be a whole argument, not part of %q", r.Glob, m[1], a)
			}
		}
	}
}
