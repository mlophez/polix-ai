package load

import (
	"errors"
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
	protectedBranches []itemYAML
	forbiddenCommands []forbiddenYAML
	execOnStart       []checkYAML
	formatOnEdit      []checkYAML
	formatOnSave      []checkYAML
	checkOnStop       []checkYAML
	checkOnSave       []checkYAML
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

// checkYAML is an item of a lifecycle shortcut, with the fields of workflow.GuardrailCheck in its order,
// so it converts to one: Problems holds what is wrong with its values.
type checkYAML struct {
	Run      []string
	Scope    any
	Message  string
	Waits    bool
	Line     int
	Problems []string
}

// placedYAML is an item of a shortcut that takes a list of maps, an alias read as the node of its anchor, with the
// line where the shortcut writes it (see nodeYAML.itemLine).
type placedYAML struct {
	nodeYAML
	line int
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

// shortcut reads the value val of the shortcut whose key is k into y, an alias as the value of its anchor; an unknown
// key is an error, with the hint of its new spelling when it is an old name of a shortcut.
func (y *guardrailsYAML) shortcut(k *yaml.Node, val nodeYAML) error {
	var err error
	switch k.Value {
	case workflow.ShortcutProtectedFiles:
		y.protectedFiles, err = val.items(k.Value, `path globs, as [".env", "secrets/**"]`)
	case workflow.ShortcutProtectedBranches:
		y.protectedBranches, err = val.items(k.Value, `branch names, as [main, "release/*"]`)
	case workflow.ShortcutForbiddenCommands:
		var items []placedYAML
		items, err = val.maps(k.Value, `{ pattern: "git push --force*", reason: "..." }`)
		for _, item := range items {
			y.forbiddenCommands = append(y.forbiddenCommands, item.forbidden())
		}
	case workflow.ShortcutExecOnStart:
		y.execOnStart, err = val.checkItems(k.Value, workflow.ExecOnStartKeys, `{ run: "uv sync --locked" }`)
	case workflow.ShortcutFormatOnEdit:
		y.formatOnEdit, err = val.checkItems(k.Value, workflow.FormatOnEditKeys, `{ run: "gofmt -w {path}", path: "*.go" }`)
	case workflow.ShortcutFormatOnSave:
		y.formatOnSave, err = val.checkItems(k.Value, workflow.FormatOnSaveKeys,
			`{ run: "npx eslint --fix", changed: "*.ts" }`)
	case workflow.ShortcutCheckOnStop:
		y.checkOnStop, err = val.checkItems(k.Value, workflow.CheckOnStopKeys, `{ run: "go test ./...", changed: "*.go" }`)
	case workflow.ShortcutCheckOnSave:
		y.checkOnSave, err = val.checkItems(k.Value, workflow.CheckOnSaveKeys, `{ run: "go test ./...", changed: "*.go" }`)
	default:
		unknown := workflow.Problem(fmt.Sprintf("line %d: unknown key %q in guardrails (%s)", k.Line, k.Value,
			strings.Join(workflow.GuardrailsKeys, ", ")))
		return errors.New(string(unknown.Hinted("guardrails", k.Value)))
	}
	return err
}

// guardrails builds the section of the domain, with the error of its Valid: the items with problems.
func (y *guardrailsYAML) guardrails() (workflow.Guardrails, error) {
	g := workflow.Guardrails{
		ProtectedFiles:    y.items(y.protectedFiles),
		ProtectedBranches: y.items(y.protectedBranches),
		ForbiddenCommands: y.forbidden(),
		ExecOnStart:       y.checks(y.execOnStart),
		FormatOnEdit:      y.checks(y.formatOnEdit),
		FormatOnSave:      y.checks(y.formatOnSave),
		CheckOnStop:       y.checks(y.checkOnStop),
		CheckOnSave:       y.checks(y.checkOnSave),
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

// checks converts the items of a lifecycle shortcut; nil without them.
func (*guardrailsYAML) checks(items []checkYAML) []workflow.GuardrailCheck {
	var out []workflow.GuardrailCheck
	for _, c := range items {
		out = append(out, workflow.GuardrailCheck(c))
	}
	return out
}

// items reads the value n of the shortcut key, a list of strings described by form, each alias, of the list or of an
// item, as the value of its anchor and each item at the line of itemLine: null is an empty list, and any other node
// is an error that shows the form.
func (n nodeYAML) items(key, form string) ([]itemYAML, error) {
	list := n.resolved()
	if list.ShortTag() == "!!null" {
		return nil, nil
	}
	if list.Kind != yaml.SequenceNode {
		return nil, fmt.Errorf("line %d: guardrails.%s takes a list of %s", n.Line, key, form)
	}
	out := make([]itemYAML, 0, len(list.Content))
	for _, node := range list.Content {
		item := nodeYAML{node}.resolved()
		if item.Kind != yaml.ScalarNode {
			return nil, fmt.Errorf("line %d: guardrails.%s takes a list of %s", n.itemLine(node), key, form)
		}
		out = append(out, itemYAML{Text: item.Value, Line: n.itemLine(node)})
	}
	return out, nil
}

// maps returns the items of the value n of the shortcut key, a list of maps like example, each alias, of the list or
// of an item, as the value of its anchor and each item at the line of itemLine: null is an empty list, and any other
// node is an error that shows the form.
func (n nodeYAML) maps(key, example string) ([]placedYAML, error) {
	list := n.resolved()
	if list.ShortTag() == "!!null" {
		return nil, nil
	}
	if list.Kind != yaml.SequenceNode {
		return nil, fmt.Errorf("line %d: guardrails.%s takes a list of maps, each as %s", n.Line, key, example)
	}
	out := make([]placedYAML, 0, len(list.Content))
	for _, node := range list.Content {
		item := nodeYAML{node}.resolved()
		if item.Kind != yaml.MappingNode {
			return nil, fmt.Errorf("line %d: an item of guardrails.%s must be a map, as %s", n.itemLine(node), key,
				example)
		}
		out = append(out, placedYAML{item, n.itemLine(node)})
	}
	return out, nil
}

// itemLine returns the line where the shortcut whose value is n, as written, writes its item node, so an item read
// through an alias is reported in its shortcut, never at its anchor in another one: the alias of the list, since every
// node of an aliased list, an alias among them too, sits in another shortcut; else the item, an alias at its own line.
func (n nodeYAML) itemLine(node *yaml.Node) int {
	if n.Kind == yaml.AliasNode {
		return n.Line
	}
	return node.Line
}

// resolved returns the node an alias stands for, so a value written as *name reads as the value of its anchor, as
// the decoding of yaml does for the entries of on; any other node as it is.
func (n nodeYAML) resolved() nodeYAML {
	for n.Node != nil && n.Kind == yaml.AliasNode && n.Alias != nil {
		n = nodeYAML{n.Alias}
	}
	return n
}

// checkItems reads the value n of the lifecycle shortcut key, a list of maps like example whose items take keys: each
// item with the problems of its values (check), or the error of maps that shows the form.
func (n nodeYAML) checkItems(key string, keys []string, example string) ([]checkYAML, error) {
	items, err := n.maps(key, example)
	var out []checkYAML
	for _, item := range items {
		out = append(out, item.check(key, keys))
	}
	return out, err
}

// forbidden reads the item n of forbidden_commands, a map at its line, with the problems of its values.
func (n placedYAML) forbidden() forbiddenYAML {
	y := forbiddenYAML{Line: n.line}
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

// check reads the item n of a lifecycle shortcut (the shortcut key, whose items take keys), a map at its line, with
// the problems of its values. The command of run is a string split on spaces or a list of arguments, and the scope
// keeps the value as written, which the validator checks as the condition it becomes. An item of check_on_stop waits
// for the subagents still running unless its wait_agents, true or false, is false.
func (n placedYAML) check(key string, keys []string) checkYAML {
	y := checkYAML{Line: n.line, Waits: key == workflow.ShortcutCheckOnStop}
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
		case "wait_agents":
			var waits bool
			if val.Kind != yaml.ScalarNode || val.ShortTag() != "!!bool" || val.Decode(&waits) != nil {
				return "wait_agents must be true or false"
			}
			y.Waits = waits
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
		if p := read(k.Value, nodeYAML{val}.resolved()); p != "" {
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

// command reports whether n is a command: a string, or a list of strings, each alias among them as the string of its
// anchor, as in an entry of on.
func (n nodeYAML) command() bool {
	switch n.Kind {
	case yaml.ScalarNode:
		return true
	case yaml.SequenceNode:
		for _, a := range n.Content {
			if (nodeYAML{a}).resolved().Kind != yaml.ScalarNode {
				return false
			}
		}
		return true
	default:
		return false
	}
}
