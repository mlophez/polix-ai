package load

import (
	"fmt"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/mlophez/polix-ai/internal/app/workflow"
)

// guardrailsYAML is the section guardrails: the shortcuts of the common cases, each in its one form and each item
// with its line, which it turns into a workflow.Guardrails.
type guardrailsYAML struct {
	protectedFiles    []itemYAML
	forbiddenCommands []forbiddenYAML
	protectedBranches []itemYAML
	formatOnEdit      []checkYAML
	runOnStop         []checkYAML
}

// itemYAML is an item of a shortcut that takes a list of strings, with the fields of workflow.GuardrailItem in its
// order, so it converts to one.
type itemYAML struct {
	Text string
	Line int
}

// forbiddenYAML is an item of forbidden_commands, with the fields of workflow.GuardrailForbidden in its order, so it
// converts to one: Problems holds what is wrong with its values.
type forbiddenYAML struct {
	Pattern  string
	Reason   string
	Line     int
	Problems []string
}

// checkYAML is an item of format_on_edit or run_on_stop, with the fields of workflow.GuardrailCheck in its order,
// so it converts to one: Problems holds what is wrong with its values.
type checkYAML struct {
	Run      []string
	Scope    any
	Message  string
	Line     int
	Problems []string
}

// UnmarshalYAML reads the section guardrails, a map of shortcuts. It checks the kind of each node before reading it,
// so a value in another form than the one of its shortcut is an error that shows that form, not an error of yaml;
// the problems of the values of an item are kept with it, for Guardrails.Problems to report them with the rest.
func (y *guardrailsYAML) UnmarshalYAML(n *yaml.Node) error {
	if n.Kind != yaml.MappingNode {
		return fmt.Errorf("line %d: guardrails must be a map with %s", n.Line,
			strings.Join(workflow.GuardrailsKeys, ", "))
	}
	var out guardrailsYAML
	seen := map[string]int{}
	for i := 0; i+1 < len(n.Content); i += 2 {
		k, val := n.Content[i], n.Content[i+1]
		if first, ok := seen[k.Value]; ok {
			return fmt.Errorf("line %d: mapping key %q already defined at line %d", k.Line, k.Value, first)
		}
		seen[k.Value] = k.Line
		if err := out.shortcut(k, nodeYAML{val}); err != nil {
			return err
		}
	}
	*y = out
	return nil
}

// shortcut reads the value val of the shortcut whose key is k into y; an unknown key is an error.
func (y *guardrailsYAML) shortcut(k *yaml.Node, val nodeYAML) error {
	var err error
	switch k.Value {
	case workflow.ShortcutProtectedFiles:
		y.protectedFiles, err = val.items(k.Value, `path globs, as [".env", "secrets/**"]`)
	case workflow.ShortcutProtectedBranches:
		y.protectedBranches, err = val.items(k.Value, `branch names, as [main, "release/*"]`)
	case workflow.ShortcutForbiddenCommands:
		var items []*yaml.Node
		items, err = val.maps(k.Value, `{ pattern: "git push --force*", reason: "..." }`)
		for _, item := range items {
			y.forbiddenCommands = append(y.forbiddenCommands, nodeYAML{item}.forbidden())
		}
	case workflow.ShortcutFormatOnEdit:
		var items []*yaml.Node
		items, err = val.maps(k.Value, `{ run: "gofmt -w {path}", path: "*.go" }`)
		for _, item := range items {
			y.formatOnEdit = append(y.formatOnEdit, nodeYAML{item}.check(k.Value, workflow.FormatOnEditKeys))
		}
	case workflow.ShortcutRunOnStop:
		var items []*yaml.Node
		items, err = val.maps(k.Value, `{ run: "go test ./...", changed: "*.go" }`)
		for _, item := range items {
			y.runOnStop = append(y.runOnStop, nodeYAML{item}.check(k.Value, workflow.RunOnStopKeys))
		}
	default:
		return fmt.Errorf("line %d: unknown key %q in guardrails (%s)", k.Line, k.Value,
			strings.Join(workflow.GuardrailsKeys, ", "))
	}
	return err
}

// guardrails builds the section of the domain, with the error of its Valid: the items with problems.
func (y *guardrailsYAML) guardrails() (workflow.Guardrails, error) {
	g := workflow.Guardrails{
		ProtectedFiles:    y.items(y.protectedFiles),
		ForbiddenCommands: y.forbidden(),
		ProtectedBranches: y.items(y.protectedBranches),
		FormatOnEdit:      y.checks(y.formatOnEdit),
		RunOnStop:         y.checks(y.runOnStop),
	}
	return g, g.Valid()
}

// items converts the items of a shortcut that takes a list of strings; nil without them.
func (*guardrailsYAML) items(items []itemYAML) []workflow.GuardrailItem {
	var out []workflow.GuardrailItem
	for _, it := range items {
		out = append(out, workflow.GuardrailItem(it))
	}
	return out
}

// forbidden converts the items of forbidden_commands; nil without them.
func (y *guardrailsYAML) forbidden() []workflow.GuardrailForbidden {
	var out []workflow.GuardrailForbidden
	for _, f := range y.forbiddenCommands {
		out = append(out, workflow.GuardrailForbidden(f))
	}
	return out
}

// checks converts the items of format_on_edit or run_on_stop; nil without them.
func (*guardrailsYAML) checks(items []checkYAML) []workflow.GuardrailCheck {
	var out []workflow.GuardrailCheck
	for _, c := range items {
		out = append(out, workflow.GuardrailCheck(c))
	}
	return out
}

// items reads the value n of the shortcut key, a list of strings described by form: null is an empty list, and any
// other node is an error that shows the form.
func (n nodeYAML) items(key, form string) ([]itemYAML, error) {
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
		out = append(out, itemYAML{Text: item.Value, Line: item.Line})
	}
	return out, nil
}

// maps returns the items of the value n of the shortcut key, a list of maps like example: null is an empty list,
// and any other node is an error that shows the form.
func (n nodeYAML) maps(key, example string) ([]*yaml.Node, error) {
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

// forbidden reads the item n of forbidden_commands, a map, with the problems of its values.
func (n nodeYAML) forbidden() forbiddenYAML {
	y := forbiddenYAML{Line: n.Line}
	badPattern := false
	y.Problems = n.fields(workflow.ShortcutForbiddenCommands, workflow.ForbiddenKeys, func(k string, val nodeYAML) string {
		s, ok := val.text()
		if !ok {
			badPattern = badPattern || k == "pattern"
			return k + " must be a text"
		}
		if k == "pattern" {
			y.Pattern = s
		} else {
			y.Reason = s
		}
		return ""
	})
	if strings.TrimSpace(y.Pattern) == "" && !badPattern {
		y.Problems = append(y.Problems, fmt.Sprintf("guardrails.%s: an item needs a pattern",
			workflow.ShortcutForbiddenCommands))
	}
	return y
}

// check reads the item n of format_on_edit or run_on_stop (the shortcut key, whose items take keys), a map, with
// the problems of its values. The command of run is a string split on spaces or a list of arguments, and the scope
// keeps the value as written, which the validator checks as the condition it becomes.
func (n nodeYAML) check(key string, keys []string) checkYAML {
	y := checkYAML{Line: n.Line}
	badRun := false
	y.Problems = n.fields(key, keys, func(k string, val nodeYAML) string {
		switch k {
		case "run":
			var run argsYAML
			if !val.command() || val.Decode(&run) != nil {
				badRun = true
				return "run takes a command, as a string or a list of arguments"
			}
			y.Run = run
		case "message":
			s, ok := val.text()
			if !ok {
				return "message must be a text"
			}
			y.Message = s
		default:
			var scope any
			if err := val.Decode(&scope); err != nil {
				return fmt.Sprintf("%s: %v", k, err)
			}
			y.Scope = scope
		}
		return ""
	})
	if len(y.Run) == 0 && !badRun {
		y.Problems = append(y.Problems, fmt.Sprintf("guardrails.%s: an item needs run", key))
	}
	return y
}

// fields walks the keys of the map item n of the shortcut key: read takes the value of each of the known keys and
// returns its problem, or "". It returns the problems of the item: those of read, of an unknown key and of a key
// written twice, each with the path of the shortcut.
func (n nodeYAML) fields(key string, known []string, read func(k string, val nodeYAML) string) []string {
	var out []string
	seen := map[string]int{}
	for i := 0; i+1 < len(n.Content); i += 2 {
		k, val := n.Content[i], n.Content[i+1]
		if first, ok := seen[k.Value]; ok {
			out = append(out, fmt.Sprintf("guardrails.%s: key %q already defined at line %d", key, k.Value, first))
			continue
		}
		seen[k.Value] = k.Line
		if !slices.Contains(known, k.Value) {
			out = append(out, fmt.Sprintf("unknown key %q in guardrails.%s (%s)", k.Value, key, strings.Join(known, ", ")))
			continue
		}
		if p := read(k.Value, nodeYAML{val}); p != "" {
			out = append(out, fmt.Sprintf("guardrails.%s: %s", key, p))
		}
	}
	return out
}

// text reads a text of an item: a scalar, empty when null.
func (n nodeYAML) text() (string, bool) {
	if n.Kind != yaml.ScalarNode {
		return "", false
	}
	if n.ShortTag() == "!!null" {
		return "", true
	}
	return n.Value, true
}

// command reports whether n is a command: a string, or a list of strings.
func (n nodeYAML) command() bool {
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
	default:
		return false
	}
}
