package workflow

import (
	"fmt"
	"sort"
	"strings"
)

// GraphWorkflowOutput is the decision of the logic of GraphWorkflow: the workflow as a Mermaid flowchart, or that
// there is no polix.yaml to draw.
type GraphWorkflowOutput struct {
	Mermaid string // the flowchart, one line per statement, each ending in a newline
	Missing bool   // there is no polix.yaml: nothing to draw
}

// GraphWorkflow draws the workflow as a Mermaid flowchart: one subgraph per event with entries, in the order of
// Events, and one node per entry, linked in the order of the file. Without polix.yaml (found false) it draws
// nothing and says so.
func GraphWorkflow(wf Workflow, found bool) GraphWorkflowOutput {
	if !found {
		return GraphWorkflowOutput{Missing: true}
	}
	var b strings.Builder
	b.WriteString("flowchart TD\n")
	n := 0
	for _, ev := range Events {
		entries := wf.On[ev]
		if len(entries) == 0 {
			continue
		}
		fmt.Fprintf(&b, "  subgraph %s\n", ev)
		prev := ""
		for _, e := range entries {
			id := fmt.Sprintf("n%d", n)
			n++
			fmt.Fprintf(&b, "    %s[\"%s\"]\n", id, graphWorkflowLabel(e))
			if prev != "" {
				fmt.Fprintf(&b, "    %s --> %s\n", prev, id)
			}
			prev = id
		}
		b.WriteString("  end\n")
	}
	return GraphWorkflowOutput{Mermaid: b.String()}
}

// graphWorkflowLabel describes an entry: its conditions and its action, with single quotes for double ones,
// which would end the label.
func graphWorkflowLabel(e Entry) string {
	var parts []string
	if c := graphWorkflowCond(e.When); c != "" {
		parts = append(parts, "when "+c)
	}
	if c := graphWorkflowCond(e.Unless); c != "" {
		parts = append(parts, "unless "+c)
	}
	a := e.Action
	act := a.Verb()
	switch act {
	case VerbBlock:
		act += ": " + a.Block.Message
		if a.Block.Once {
			act += " (once)"
		}
	case VerbRun:
		if a.Run.Rules != "" {
			act += " rules " + a.Run.Rules
		} else {
			act += " " + strings.Join(a.Run.Command, " ")
		}
		if a.Run.Always {
			act += " (always)"
		}
	case VerbInject:
		switch {
		case a.Inject.File != "":
			act += " file " + a.Inject.File
		case a.Inject.Rules != "":
			act += " rules " + a.Inject.Rules
		default:
			act += ": " + a.Inject.Text
		}
		if a.Inject.Always {
			act += " (always)"
		}
	}
	parts = append(parts, act)
	return strings.ReplaceAll(strings.Join(parts, "<br/>"), `"`, "'")
}

// graphWorkflowCond writes the conditions as key=value, ordered by key.
func graphWorkflowCond(c Cond) string {
	keys := make([]string, 0, len(c))
	for k := range c {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make([]string, 0, len(keys))
	for _, k := range keys {
		out = append(out, fmt.Sprintf("%s=%v", k, c[k]))
	}
	return strings.Join(out, ", ")
}
