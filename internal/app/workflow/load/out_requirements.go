package load

import (
	"fmt"

	"gopkg.in/yaml.v3"
)

// requirementsYAML is the key requirements: the names of the programs the repository requires on the PATH, in file
// order.
type requirementsYAML []string

// requirementsForm is the form of the key requirements, which the error of any other form shows.
const requirementsForm = "requirements takes a list of program names, as [go, golangci-lint]"

// UnmarshalYAML reads the key requirements, a list of program names. It checks the kind of the node and of each item
// before reading them, so another form (a single name, a map, an item that is not a name) is an error at the line of
// its node that shows the form, not an error of yaml. A null item is an empty name, which Workflow.Valid reports; an
// empty list is no requirement.
func (rs *requirementsYAML) UnmarshalYAML(n *yaml.Node) error {
	if n.Kind != yaml.SequenceNode {
		return fmt.Errorf("line %d: %s", n.Line, requirementsForm)
	}
	var out requirementsYAML
	for _, item := range n.Content {
		name, ok := nodeYAML{item}.text()
		if !ok {
			return fmt.Errorf("line %d: %s", item.Line, requirementsForm)
		}
		out = append(out, name)
	}
	*rs = out
	return nil
}
