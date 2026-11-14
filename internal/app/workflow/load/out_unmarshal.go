package load

import (
	"fmt"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/mlophez/polix-ai/internal/app/workflow"
)

// nodeYAML is a node of polix.yaml, with the readings the decoding types share.
type nodeYAML struct {
	*yaml.Node
}

// UnmarshalYAML accepts a string or a list of strings.
func (ns *stringsYAML) UnmarshalYAML(n *yaml.Node) error {
	if n.Kind == yaml.ScalarNode {
		*ns = stringsYAML{n.Value}
		return nil
	}
	var list []string
	if err := n.Decode(&list); err != nil {
		return err
	}
	*ns = list
	return nil
}

// UnmarshalYAML accepts a string, split on spaces, or a list of strings.
func (a *argsYAML) UnmarshalYAML(n *yaml.Node) error {
	if n.Kind == yaml.ScalarNode {
		*a = strings.Fields(n.Value)
		return nil
	}
	var list []string
	if err := n.Decode(&list); err != nil {
		return err
	}
	*a = list
	return nil
}

// UnmarshalYAML keeps the line of the entry and the keys it uses, so Valid can reject unknown ones.
func (y *entryYAML) UnmarshalYAML(n *yaml.Node) error {
	if n.Kind != yaml.MappingNode {
		return fmt.Errorf("line %d: an entry must be a map with %s", n.Line, strings.Join(workflow.EntryKeys, ", "))
	}
	type plain entryYAML
	var p plain
	if err := n.Decode(&p); err != nil {
		return err
	}
	*y = entryYAML(p)
	y.line = n.Line
	y.keys = nodeYAML{n}.keys()
	return nil
}

// UnmarshalYAML reads the conditions of the old key if when it is a map, and ignores any other value.
func (y *ifYAML) UnmarshalYAML(n *yaml.Node) error {
	if n.Kind != yaml.MappingNode {
		return nil
	}
	var c map[string]any
	if err := n.Decode(&c); err != nil {
		return err
	}
	*y = c
	return nil
}

// UnmarshalYAML keeps the keys of the variable, so Valid can reject unknown ones.
func (y *varYAML) UnmarshalYAML(n *yaml.Node) error {
	if n.Kind != yaml.MappingNode {
		return fmt.Errorf("line %d: a variable of vars must be a map with branch", n.Line)
	}
	type plain varYAML
	var p plain
	if err := n.Decode(&p); err != nil {
		return err
	}
	*y = varYAML(p)
	y.keys = nodeYAML{n}.keys()
	return nil
}

// UnmarshalYAML reads the action, a map, and keeps its keys, so Valid can reject unknown ones.
func (y *actionYAML) UnmarshalYAML(n *yaml.Node) error {
	if n.Kind != yaml.MappingNode {
		return fmt.Errorf("line %d: action must be a map with one of %s", n.Line, strings.Join(workflow.Actions, ", "))
	}
	type plain actionYAML
	var p plain
	if err := n.Decode(&p); err != nil {
		return err
	}
	*y = actionYAML(p)
	y.keys = nodeYAML{n}.keys()
	return nil
}

// UnmarshalYAML reads the run: a string split on spaces or a list of arguments is the command, and a map keeps its
// keys, so Valid can reject unknown ones.
func (y *runYAML) UnmarshalYAML(n *yaml.Node) error {
	switch n.Kind {
	case yaml.ScalarNode, yaml.SequenceNode:
		var args argsYAML
		if err := n.Decode(&args); err != nil {
			return err
		}
		*y = runYAML{Command: args}
		return nil
	case yaml.MappingNode:
		if err := (nodeYAML{n}).bools(workflow.VerbRun, "always"); err != nil {
			return err
		}
		type plain runYAML
		var p plain
		if err := n.Decode(&p); err != nil {
			return err
		}
		*y = runYAML(p)
		y.keys = nodeYAML{n}.keys()
		return nil
	default:
		return fmt.Errorf("line %d: run takes a command, a list of arguments or a map with command or rules", n.Line)
	}
}

// UnmarshalYAML reads the inject: a string is the text, and a map keeps its keys, so Valid can reject unknown ones.
func (y *injectYAML) UnmarshalYAML(n *yaml.Node) error {
	switch n.Kind {
	case yaml.ScalarNode:
		*y = injectYAML{Text: n.Value}
		return nil
	case yaml.MappingNode:
		if err := (nodeYAML{n}).bools(workflow.VerbInject, "always"); err != nil {
			return err
		}
		type plain injectYAML
		var p plain
		if err := n.Decode(&p); err != nil {
			return err
		}
		*y = injectYAML(p)
		y.keys = nodeYAML{n}.keys()
		return nil
	default:
		return fmt.Errorf("line %d: inject takes a text or a map with one of file, text or rules", n.Line)
	}
}

// UnmarshalYAML reads the block: a string is the message, and a map keeps its keys, so Valid can reject unknown
// ones.
func (y *blockYAML) UnmarshalYAML(n *yaml.Node) error {
	switch n.Kind {
	case yaml.ScalarNode:
		*y = blockYAML{Message: n.Value}
		return nil
	case yaml.MappingNode:
		if err := (nodeYAML{n}).bools(workflow.VerbBlock, "once"); err != nil {
			return err
		}
		type plain blockYAML
		var p plain
		if err := n.Decode(&p); err != nil {
			return err
		}
		*y = blockYAML(p)
		y.keys = nodeYAML{n}.keys()
		return nil
	default:
		return fmt.Errorf("line %d: block takes a message or a map with message and once", n.Line)
	}
}

// UnmarshalYAML reads the notify: a string is the message, and a map keeps its keys, so Valid can reject unknown
// ones.
func (y *notifyYAML) UnmarshalYAML(n *yaml.Node) error {
	switch n.Kind {
	case yaml.ScalarNode:
		*y = notifyYAML{Message: n.Value}
		return nil
	case yaml.MappingNode:
		if err := (nodeYAML{n}).bools(workflow.VerbNotify, "always"); err != nil {
			return err
		}
		type plain notifyYAML
		var p plain
		if err := n.Decode(&p); err != nil {
			return err
		}
		*y = notifyYAML(p)
		y.keys = nodeYAML{n}.keys()
		return nil
	default:
		return fmt.Errorf("line %d: notify takes a message or a map with message and always", n.Line)
	}
}

// UnmarshalYAML keeps the keys of rules, so Valid can reject unknown ones.
func (y *rulesYAML) UnmarshalYAML(n *yaml.Node) error {
	if n.Kind != yaml.MappingNode {
		return fmt.Errorf("line %d: rules must be a map with run", n.Line)
	}
	type plain rulesYAML
	var p plain
	if err := n.Decode(&p); err != nil {
		return err
	}
	*y = rulesYAML(p)
	y.keys = nodeYAML{n}.keys()
	return nil
}

// UnmarshalYAML reads the map of rules.run in file order: a glob to a command, a string split on spaces or a
// list of strings.
func (rs *runnersYAML) UnmarshalYAML(n *yaml.Node) error {
	if n.Kind != yaml.MappingNode {
		return fmt.Errorf("line %d: rules.run must map a glob to a command", n.Line)
	}
	var out runnersYAML
	seen := map[string]int{}
	for i := 0; i+1 < len(n.Content); i += 2 {
		key := n.Content[i]
		if key.Kind != yaml.ScalarNode {
			return fmt.Errorf("line %d: a glob of rules.run must be a string", key.Line)
		}
		if first, ok := seen[key.Value]; ok {
			return fmt.Errorf("line %d: mapping key %q already defined at line %d", key.Line, key.Value, first)
		}
		seen[key.Value] = key.Line
		var argv argsYAML
		if err := n.Content[i+1].Decode(&argv); err != nil {
			return err
		}
		out = append(out, runnerYAML{glob: n.Content[i].Value, argv: argv, line: n.Content[i].Line})
	}
	*rs = out
	return nil
}

// UnmarshalYAML reads the section risk, a map whose only key is weights.
func (y *riskYAML) UnmarshalYAML(n *yaml.Node) error {
	if n.Kind != yaml.MappingNode {
		return fmt.Errorf("line %d: risk must be a map with weights", n.Line)
	}
	for i := 0; i+1 < len(n.Content); i += 2 {
		if k := n.Content[i]; !slices.Contains(workflow.RiskKeys, k.Value) {
			return fmt.Errorf("line %d: unknown key %q in risk (%s)", k.Line, k.Value, strings.Join(workflow.RiskKeys, ", "))
		}
	}
	type plain riskYAML
	var p plain
	if err := n.Decode(&p); err != nil {
		return err
	}
	*y = riskYAML(p)
	return nil
}

// UnmarshalYAML reads the map of risk.weights in file order: a glob to a number.
func (ws *weightsYAML) UnmarshalYAML(n *yaml.Node) error {
	if n.Kind != yaml.MappingNode {
		return fmt.Errorf("line %d: risk.weights must map a glob to a number", n.Line)
	}
	var out weightsYAML
	seen := map[string]int{}
	for i := 0; i+1 < len(n.Content); i += 2 {
		key, val := n.Content[i], n.Content[i+1]
		if key.Kind != yaml.ScalarNode {
			return fmt.Errorf("line %d: a glob of risk.weights must be a string", key.Line)
		}
		if first, ok := seen[key.Value]; ok {
			return fmt.Errorf("line %d: mapping key %q already defined at line %d", key.Line, key.Value, first)
		}
		seen[key.Value] = key.Line
		var f float64
		if val.Kind != yaml.ScalarNode || (val.Tag != "!!int" && val.Tag != "!!float") || val.Decode(&f) != nil {
			return fmt.Errorf("line %d: the weight of %q must be a number", val.Line, key.Value)
		}
		out = append(out, weightYAML{glob: key.Value, factor: f})
	}
	*ws = out
	return nil
}

// keys returns the keys of a mapping node, in file order.
func (n nodeYAML) keys() []string {
	var keys []string
	if n.Kind == yaml.MappingNode {
		for i := 0; i < len(n.Content); i += 2 {
			keys = append(keys, n.Content[i].Value)
		}
	}
	return keys
}

// lines returns the line of each key of a mapping node.
func (n nodeYAML) lines() map[string]int {
	if n.Kind != yaml.MappingNode {
		return nil
	}
	lines := make(map[string]int, len(n.Content)/2)
	for i := 0; i < len(n.Content); i += 2 {
		lines[n.Content[i].Value] = n.Content[i].Line
	}
	return lines
}

// bools returns the error of a key of the map n, of the verb what, among keys whose value is not true or false.
func (n nodeYAML) bools(what string, keys ...string) error {
	for i := 0; i+1 < len(n.Content); i += 2 {
		k, val := n.Content[i], n.Content[i+1]
		if slices.Contains(keys, k.Value) && (val.Kind != yaml.ScalarNode || val.ShortTag() != "!!bool") {
			return fmt.Errorf("line %d: %s.%s must be true or false", val.Line, what, k.Value)
		}
	}
	return nil
}
