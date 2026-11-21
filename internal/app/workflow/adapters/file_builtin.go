package adapters

import (
	"fmt"

	"gopkg.in/yaml.v3"

	"github.com/mlophez/polix-ai/internal/app/workflow"
)

// builtinYAML is guardrails.builtin as written: the switches the file sets, by key, the line of the key builtin, 0
// when the file leaves it out, and what is wrong with its values, each at the line of its key.
type builtinYAML struct {
	switches map[string]bool
	line     int
	problems []problemYAML
}

// problemYAML is what the decoding finds wrong with a value of guardrails, at the line of its key, with the fields of
// workflow.GuardrailProblem in its order, so it converts to one.
type problemYAML struct {
	Text string
	Line int
}

// builtinForm is the form of guardrails.builtin, which the error of another kind of value shows.
const builtinForm = "a map of true or false, as { forbid_git_destructive_commands: false }"

// builtin reads the value n of guardrails.builtin, whose key is at line, an alias as the value of its anchor: a map of
// true or false, with the problems of an unknown key or another value, or null, every built-in good practice on.
// Any other node is an error that shows the form.
func (n nodeYAML) builtin(line int) (builtinYAML, error) {
	y := builtinYAML{line: line}
	m := n.resolved()
	if m.ShortTag() == "!!null" {
		return y, nil
	}
	if m.Kind != yaml.MappingNode {
		return y, fmt.Errorf("line %d: guardrails.%s takes %s", n.Line, workflow.ShortcutBuiltin, builtinForm)
	}
	y.switches = map[string]bool{}
	y.problems = n.fieldsAt(m, workflow.ShortcutBuiltin, workflow.BuiltinKeys, func(k string, val nodeYAML) string {
		var on bool
		if val.Kind != yaml.ScalarNode || val.ShortTag() != "!!bool" || val.Decode(&on) != nil {
			return k + " must be true or false"
		}
		y.switches[k] = on
		return ""
	})
	return y, nil
}

// builtin returns the built-in good practices of the domain: each switch the file sets, and true for those it leaves
// out or writes wrong, with the line of builtin and the problems of its values, and the error of its Valid.
func (y builtinYAML) builtin() (workflow.Builtin, error) {
	on := func(key string) bool {
		v, ok := y.switches[key]
		return !ok || v
	}
	var problems []workflow.GuardrailProblem
	for _, p := range y.problems {
		problems = append(problems, workflow.GuardrailProblem(p))
	}
	b := workflow.Builtin{
		ForbidCommitNoVerify:         on(workflow.BuiltinForbidCommitNoVerify),
		ForbidGitDestructiveCommands: on(workflow.BuiltinForbidGitDestructiveCommands),
		FormatPolixFiles:             on(workflow.BuiltinFormatPolixFiles),
		CheckPolixFiles:              on(workflow.BuiltinCheckPolixFiles),
		Line:                         y.line,
		Problems:                     problems,
	}
	return b, b.Valid()
}
