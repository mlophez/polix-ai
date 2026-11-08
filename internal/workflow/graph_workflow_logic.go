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

// GraphWorkflow draws the workflow as a Mermaid flowchart: first a subgraph guardrails with the protections the hook
// applies before every event, then one subgraph per event with entries, in the order of Events, and one node per
// entry, linked in the order of the file. Without polix.yaml (found false) it draws nothing and says so.
func GraphWorkflow(wf Workflow, found bool) GraphWorkflowOutput {
	if !found {
		return GraphWorkflowOutput{Missing: true}
	}
	var b strings.Builder
	b.WriteString("flowchart TD\n")
	n := 0
	if labels := graphWorkflowGuardrails(wf); len(labels) > 0 {
		b.WriteString("  subgraph guardrails\n")
		for i, label := range labels {
			fmt.Fprintf(&b, "    n%d[\"%s\"]\n", n, label)
			if i > 0 {
				fmt.Fprintf(&b, "    n%d --> n%d\n", n-1, n)
			}
			n++
		}
		b.WriteString("  end\n")
	}
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

// graphWorkflowGuardrails describes the protections of the workflow, in the order the hook applies them: one label for
// the globs of protected_files, and one per item of forbidden_commands with its pattern and its reason.
func graphWorkflowGuardrails(wf Workflow) []string {
	var labels []string
	if len(wf.ProtectedFiles) > 0 {
		labels = append(labels, graphWorkflowText(shortcutProtectedFiles+": "+strings.Join(wf.ProtectedFiles, ", ")))
	}
	for _, f := range wf.ForbiddenCommands {
		label := shortcutForbiddenCommands + ": " + f.Pattern
		if f.Reason != "" {
			label += "<br/>" + f.Reason
		}
		labels = append(labels, graphWorkflowText(label))
	}
	return labels
}

// graphWorkflowLabel describes an entry: its conditions, its action and the shortcut it was expanded from, written as
// graphWorkflowText says.
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
	if e.From != "" {
		parts = append(parts, "from "+e.From)
	}
	return graphWorkflowText(strings.Join(parts, "<br/>"))
}

// graphWorkflowText writes a label with single quotes for double ones, which would end it, and the line breaks of a
// message as breaks of the label, so each statement keeps one line.
func graphWorkflowText(label string) string {
	return strings.NewReplacer(`"`, "'", "\r\n", "<br/>", "\n", "<br/>").Replace(label)
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
