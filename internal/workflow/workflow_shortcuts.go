package workflow

import (
	"errors"
	"fmt"
	"maps"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

// guardrailsYAML is the section guardrails: the shortcuts of the common cases, each in its one form and each item
// with its line. protected_files and forbidden_commands are protections the workflow holds as lists (see
// protectedFiles and forbiddenCommands); the rest are only sugar, which expand turns into entries of on.
type guardrailsYAML struct {
	protectedFiles    []itemYAML
	forbiddenCommands []forbiddenYAML
	protectedBranches []itemYAML
	formatOnEdit      []checkYAML
	runBeforeDone     []checkYAML
}

// itemYAML is an item of a shortcut that takes a list of strings: a path glob of protected_files or a branch name of
// protected_branches.
type itemYAML struct {
	text string
	line int
}

// forbiddenYAML is an item of forbidden_commands: the pattern of a command and its reason, optional. problems holds
// what is wrong with its values, reported at its line.
type forbiddenYAML struct {
	pattern  string
	reason   string
	line     int
	problems []string
}

// checkYAML is an item of format_on_edit or run_before_done: the command to run, the globs that scope it (the path
// of format_on_edit or the changed of run_before_done, nil without them) and the message before the output of a
// failure. problems holds what is wrong with its values, reported at its line.
type checkYAML struct {
	run      argsYAML
	scope    any
	message  string
	line     int
	problems []string
}

// The shortcuts of the section guardrails.
const (
	shortcutProtectedFiles    = "protected_files"
	shortcutForbiddenCommands = "forbidden_commands"
	shortcutProtectedBranches = "protected_branches"
	shortcutFormatOnEdit      = "format_on_edit"
	shortcutRunBeforeDone     = "run_before_done"
)

// branchMessage is the message of the entry of protected_branches, in English since polix writes it.
const branchMessage = "{branch} is protected in " + File + ": create a branch for this change and commit there."

// guardrailsKeys are the shortcuts of the section guardrails, in the order they expand.
var guardrailsKeys = []string{shortcutProtectedFiles, shortcutForbiddenCommands, shortcutProtectedBranches,
	shortcutFormatOnEdit, shortcutRunBeforeDone}

// forbiddenKeys, formatKeys and doneKeys are the keys of an item of forbidden_commands, format_on_edit and
// run_before_done.
var (
	forbiddenKeys = []string{"pattern", "reason"}
	formatKeys    = []string{"run", "path", "message"}
	doneKeys      = []string{"run", "changed", "message"}
)

// UnmarshalYAML reads the section guardrails, a map of shortcuts. It checks the kind of each node before reading it,
// so a value in another form than the one of its shortcut is an error that shows that form, not an error of yaml;
// the problems of the values of an item are kept with it, for Parse to report them with the rest.
func (y *guardrailsYAML) UnmarshalYAML(n *yaml.Node) error {
	if n.Kind != yaml.MappingNode {
		return fmt.Errorf("line %d: guardrails must be a map with %s", n.Line, strings.Join(guardrailsKeys, ", "))
	}
	var out guardrailsYAML
	seen := map[string]int{}
	for i := 0; i+1 < len(n.Content); i += 2 {
		k, val := n.Content[i], n.Content[i+1]
		if first, ok := seen[k.Value]; ok {
			return fmt.Errorf("line %d: mapping key %q already defined at line %d", k.Line, k.Value, first)
		}
		seen[k.Value] = k.Line
		var err error
		switch k.Value {
		case shortcutProtectedFiles:
			out.protectedFiles, err = shortcutItems(val, k.Value, `path globs, as [".env", "secrets/**"]`)
		case shortcutProtectedBranches:
			out.protectedBranches, err = shortcutItems(val, k.Value, `branch names, as [main, "release/*"]`)
		case shortcutForbiddenCommands:
			var items []*yaml.Node
			items, err = shortcutMaps(val, k.Value, `{ pattern: "git push --force*", reason: "..." }`)
			for _, item := range items {
				out.forbiddenCommands = append(out.forbiddenCommands, forbiddenItem(item))
			}
		case shortcutFormatOnEdit:
			var items []*yaml.Node
			items, err = shortcutMaps(val, k.Value, `{ run: "gofmt -w {path}", path: "*.go" }`)
			for _, item := range items {
				out.formatOnEdit = append(out.formatOnEdit, checkItem(item, k.Value, formatKeys))
			}
		case shortcutRunBeforeDone:
			var items []*yaml.Node
			items, err = shortcutMaps(val, k.Value, `{ run: "go test ./...", changed: "*.go" }`)
			for _, item := range items {
				out.runBeforeDone = append(out.runBeforeDone, checkItem(item, k.Value, doneKeys))
			}
		default:
			return fmt.Errorf("line %d: unknown key %q in guardrails (%s)", k.Line, k.Value,
				strings.Join(guardrailsKeys, ", "))
		}
		if err != nil {
			return err
		}
	}
	*y = out
	return nil
}

// expand returns the entries of on with those the shortcuts protected_branches, format_on_edit and run_before_done
// expand into, before the entries written in on in each event, each group in file order. An item with problems
// expands into nothing: Parse reports them.
//
//   - protected_branches: an entry on pre_commit with branch, an anchored regular expression of the names.
//   - format_on_edit: an entry on post_edit per item that runs its command, on the edits of its path when it has one.
//   - run_before_done: an entry on stop per item that runs its command, when its changed holds if it has one.
//
// Each entry takes the line of its item, the first one for an entry of several items, and the path of its shortcut.
// protected_files and forbidden_commands expand into nothing: they are protections, not entries.
func (y guardrailsYAML) expand(on map[string][]Entry) map[string][]Entry {
	regular := map[string][]Entry{}
	if names, line := shortcutTexts(y.protectedBranches); len(names) > 0 {
		e, _ := shortcutBlock("guardrails."+shortcutProtectedBranches, line, Cond{"branch": branchExpr(names)},
			branchMessage)
		regular[PreCommit] = append(regular[PreCommit], e)
	}
	for _, c := range y.formatOnEdit {
		if e, ok := c.entry(shortcutFormatOnEdit, "path"); ok {
			regular[PostEdit] = append(regular[PostEdit], e)
		}
	}
	for _, c := range y.runBeforeDone {
		if e, ok := c.entry(shortcutRunBeforeDone, "changed"); ok {
			regular[Stop] = append(regular[Stop], e)
		}
	}
	if len(regular) == 0 {
		return on
	}
	out := make(map[string][]Entry, len(on)+len(regular))
	maps.Copy(out, on)
	for ev, entries := range regular {
		out[ev] = append(entries, on[ev]...)
	}
	return out
}

// protected returns the globs of protected_files that are not empty, in file order; nil without them. The empty ones
// are problems of the shortcut, which Parse reports.
func (y guardrailsYAML) protected() []string {
	var out []string
	for _, it := range y.protectedFiles {
		if strings.TrimSpace(it.text) != "" {
			out = append(out, it.text)
		}
	}
	return out
}

// forbidden returns the items of forbidden_commands without problems, in file order; nil without them. The items
// with problems are left out: Parse reports them.
func (y guardrailsYAML) forbidden() []ForbiddenCommand {
	var out []ForbiddenCommand
	for _, f := range y.forbiddenCommands {
		if len(f.problems) > 0 {
			continue
		}
		c := ForbiddenCommand{Pattern: f.pattern, Reason: f.reason, Line: f.line}
		if c.Valid() != nil {
			continue
		}
		out = append(out, c)
	}
	return out
}

// problems returns the problems of the values of the shortcuts, each at the line of its item, together with those
// of the workflow w they expand into, in the order of the file; nil when the shortcuts have none.
func (y guardrailsYAML) problems(w Workflow) []string {
	v := &validator{w: &w}
	for _, it := range y.protectedFiles {
		if strings.TrimSpace(it.text) == "" {
			v.add(it.line, "guardrails.%s: a path glob cannot be empty", shortcutProtectedFiles)
		}
	}
	for _, f := range y.forbiddenCommands {
		for _, p := range f.problems {
			v.add(f.line, "%s", p)
		}
	}
	for _, it := range y.protectedBranches {
		if strings.TrimSpace(it.text) == "" {
			v.add(it.line, "guardrails.%s: a branch name cannot be empty", shortcutProtectedBranches)
		}
	}
	for _, c := range append(append([]checkYAML(nil), y.formatOnEdit...), y.runBeforeDone...) {
		for _, p := range c.problems {
			v.add(c.line, "%s", p)
		}
	}
	if len(v.problems) == 0 {
		return nil
	}
	v.workflow(w)
	return v.byLine()
}

// entry returns the entry of the item c of the shortcut key, whose scope goes in the condition cond; false when the
// item has problems.
func (c checkYAML) entry(key, cond string) (Entry, bool) {
	if len(c.problems) > 0 {
		return Entry{}, false
	}
	var when Cond
	if c.scope != nil {
		when = Cond{cond: c.scope}
	}
	e, _ := shortcutRun("guardrails."+key, c.line, when, []string(c.run), c.message)
	return e, true
}

// shortcutItems reads the value n of the shortcut key, a list of strings described by form: null is an empty list,
// and any other node is an error that shows the form.
func shortcutItems(n *yaml.Node, key, form string) ([]itemYAML, error) {
	if n.ShortTag() == "!!null" {
		return nil, nil
	}
	if n.Kind != yaml.SequenceNode {
		return nil, fmt.Errorf("line %d: guardrails.%s takes a list of %s", n.Line, key, form)
	}
	out := make([]itemYAML, 0, len(n.Content))
	for _, item := range n.Content {
		if item.Kind != yaml.ScalarNode {
			return nil, fmt.Errorf("line %d: guardrails.%s takes a list of %s", item.Line, key, form)
		}
		out = append(out, itemYAML{text: item.Value, line: item.Line})
	}
	return out, nil
}

// shortcutMaps returns the items of the value n of the shortcut key, a list of maps like example: null is an empty
// list, and any other node is an error that shows the form.
func shortcutMaps(n *yaml.Node, key, example string) ([]*yaml.Node, error) {
	if n.ShortTag() == "!!null" {
		return nil, nil
	}
	if n.Kind != yaml.SequenceNode {
		return nil, fmt.Errorf("line %d: guardrails.%s takes a list of maps, each as %s", n.Line, key, example)
	}
	for _, item := range n.Content {
		if item.Kind != yaml.MappingNode {
			return nil, fmt.Errorf("line %d: an item of guardrails.%s must be a map, as %s", item.Line, key, example)
		}
	}
	return n.Content, nil
}

// forbiddenItem reads the item n of forbidden_commands, a map, with the problems of its values.
func forbiddenItem(n *yaml.Node) forbiddenYAML {
	y := forbiddenYAML{line: n.Line}
	badPattern := false
	y.problems = shortcutFields(n, shortcutForbiddenCommands, forbiddenKeys, func(k string, val *yaml.Node) string {
		s, ok := shortcutText(val)
		if !ok {
			badPattern = badPattern || k == "pattern"
			return k + " must be a text"
		}
		if k == "pattern" {
			y.pattern = s
		} else {
			y.reason = s
		}
		return ""
	})
	if strings.TrimSpace(y.pattern) == "" && !badPattern {
		y.problems = append(y.problems, fmt.Sprintf("guardrails.%s: an item needs a pattern", shortcutForbiddenCommands))
	}
	return y
}

// checkItem reads the item n of format_on_edit or run_before_done (the shortcut key, whose items take keys), a
// map, with the problems of its values. The command of run is a string split on spaces or a list of arguments, and
// the scope keeps the value as written, which the validator checks as the condition it becomes.
func checkItem(n *yaml.Node, key string, keys []string) checkYAML {
	y := checkYAML{line: n.Line}
	badRun := false
	y.problems = shortcutFields(n, key, keys, func(k string, val *yaml.Node) string {
		switch k {
		case "run":
			if !shortcutCommand(val) || val.Decode(&y.run) != nil {
				badRun = true
				return "run takes a command, as a string or a list of arguments"
			}
		case "message":
			s, ok := shortcutText(val)
			if !ok {
				return "message must be a text"
			}
			y.message = s
		default:
			var scope any
			if err := val.Decode(&scope); err != nil {
				return fmt.Sprintf("%s: %v", k, err)
			}
			y.scope = scope
		}
		return ""
	})
	if len(y.run) == 0 && !badRun {
		y.problems = append(y.problems, fmt.Sprintf("guardrails.%s: an item needs run", key))
	}
	return y
}

// shortcutFields walks the keys of the map item n of the shortcut key: read takes the value of each of the known
// keys and returns its problem, or "". It returns the problems of the item: those of read, of an unknown key and of
// a key written twice, each with the path of the shortcut.
func shortcutFields(n *yaml.Node, key string, known []string, read func(k string, val *yaml.Node) string) []string {
	var out []string
	seen := map[string]int{}
	for i := 0; i+1 < len(n.Content); i += 2 {
		k, val := n.Content[i], n.Content[i+1]
		if first, ok := seen[k.Value]; ok {
			out = append(out, fmt.Sprintf("guardrails.%s: key %q already defined at line %d", key, k.Value, first))
			continue
		}
		seen[k.Value] = k.Line
		if !contains(known, k.Value) {
			out = append(out, fmt.Sprintf("unknown key %q in guardrails.%s (%s)", k.Value, key, strings.Join(known, ", ")))
			continue
		}
		if p := read(k.Value, val); p != "" {
			out = append(out, fmt.Sprintf("guardrails.%s: %s", key, p))
		}
	}
	return out
}

// shortcutText reads a text of an item: a scalar, empty when null.
func shortcutText(n *yaml.Node) (string, bool) {
	if n.Kind != yaml.ScalarNode {
		return "", false
	}
	if n.ShortTag() == "!!null" {
		return "", true
	}
	return n.Value, true
}

// shortcutCommand reports whether n is a command: a string, or a list of strings.
func shortcutCommand(n *yaml.Node) bool {
	switch n.Kind {
	case yaml.ScalarNode:
		return true
	case yaml.SequenceNode:
		for _, a := range n.Content {
			if a.Kind != yaml.ScalarNode {
				return false
			}
		}
		return true
	}
	return false
}

// shortcutTexts returns the texts of the items that are not empty, as the value of a condition, and the line of the
// first one.
func shortcutTexts(items []itemYAML) ([]any, int) {
	var out []any
	line := 0
	for _, it := range items {
		if strings.TrimSpace(it.text) == "" {
			continue
		}
		if line == 0 {
			line = it.line
		}
		out = append(out, it.text)
	}
	return out, line
}

// branchExpr returns the regular expression of the branch condition of protected_branches: any of the names, whole,
// where `*` matches any text.
func branchExpr(names []any) string {
	alts := make([]string, 0, len(names))
	for _, n := range names {
		parts := strings.Split(fmt.Sprint(n), "*")
		for i := range parts {
			parts[i] = regexp.QuoteMeta(parts[i])
		}
		alts = append(alts, strings.Join(parts, ".*"))
	}
	return "^(?:" + strings.Join(alts, "|") + ")$"
}

// shortcutBlock returns the entry of a shortcut from at line that blocks with message when the conditions of when
// hold.
func shortcutBlock(from string, line int, when Cond, message string) (Entry, error) {
	b := Block{Message: message}
	a := Action{Block: &b, Keys: []string{VerbBlock}}
	e := Entry{When: when, Action: a, From: from, Line: line, Keys: shortcutKeys(when)}
	return e, errors.Join(b.Valid(), a.Valid(), e.Valid())
}

// shortcutRun returns the entry of a shortcut from at line that runs command, with message before the output of a
// failure, when the conditions of when hold.
func shortcutRun(from string, line int, when Cond, command []string, message string) (Entry, error) {
	keys := []string{"command"}
	if message != "" {
		keys = append(keys, "message")
	}
	r := Run{Command: command, Message: message, Keys: keys}
	a := Action{Run: &r, Keys: []string{VerbRun}}
	e := Entry{When: when, Action: a, From: from, Line: line, Keys: shortcutKeys(when)}
	return e, errors.Join(r.Valid(), a.Valid(), e.Valid())
}

// shortcutKeys returns the keys an entry of a shortcut would have written in on.
func shortcutKeys(when Cond) []string {
	var keys []string
	if when != nil {
		keys = append(keys, "when")
	}
	return append(keys, "action")
}
