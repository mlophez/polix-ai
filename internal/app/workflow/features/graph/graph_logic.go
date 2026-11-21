package graph

import (
	"fmt"
	"sort"
	"strings"

	"github.com/mlophez/polix-ai/internal/app/workflow"
)

// Output is the decision of the logic of GraphWorkflow: the workflow as a Mermaid flowchart, or that there is no
// polix.yaml to draw.
type Output struct {
	Mermaid string // the flowchart, one line per statement, each ending in a newline
	Missing bool   // there is no polix.yaml: nothing to draw
}

// Decide draws the workflow as a Mermaid flowchart: first a subgraph guardrails with the protections the hook applies
// before every event, then one subgraph per event with entries, in the order of workflow.Events, then one subgraph
// githooks.<event> per event of githooks with entries, in the order of workflow.GithookEvents, and one node per
// entry, linked in the order of the file. Without polix.yaml (found false) it draws nothing and says so.
func Decide(wf workflow.Workflow, found bool) Output {
	if !found {
		return Output{Missing: true}
	}
	var b strings.Builder
	b.WriteString("flowchart TD\n")
	n := 0
	if labels := guardrails(wf); len(labels) > 0 {
		n = subgraph(&b, "guardrails", labels, n)
	}
	for _, ev := range workflow.Events {
		n = subgraph(&b, ev, labelsOf(wf.On[ev]), n)
	}
	for _, ev := range workflow.GithookEvents {
		n = subgraph(&b, "githooks_"+ev+" [githooks."+ev+"]", labelsOf(wf.Githooks[ev]), n)
	}
	return Output{Mermaid: b.String()}
}

// subgraph writes into b the subgraph head, its id and title, with one node per label, numbered from n and each
// linked to the one before, and returns the number of the next node; without labels it writes nothing.
func subgraph(b *strings.Builder, head string, labels []string, n int) int {
	if len(labels) == 0 {
		return n
	}
	fmt.Fprintf(b, "  subgraph %s\n", head)
	for i, l := range labels {
		fmt.Fprintf(b, "    n%d[\"%s\"]\n", n, l)
		if i > 0 {
			fmt.Fprintf(b, "    n%d --> n%d\n", n-1, n)
		}
		n++
	}
	b.WriteString("  end\n")
	return n
}

// labelsOf describes the entries, each as label says, in their order.
func labelsOf(entries []workflow.Entry) []string {
	labels := make([]string, 0, len(entries))
	for _, e := range entries {
		labels = append(labels, label(e))
	}
	return labels
}

// guardrails describes the protections of the workflow: first one label per item of forbidden_commands the presets
// of guardrails.builtin add, then one for the globs of protected_files, then one per item written in
// forbidden_commands, in the order the hook applies them; each item with its pattern, its reason and, for a preset,
// its origin.
func guardrails(wf workflow.Workflow) []string {
	var labels, written []string
	for _, f := range wf.ForbiddenCommands {
		label := workflow.ShortcutForbiddenCommands + ": " + f.Pattern
		if f.Reason != "" {
			label += "<br/>" + f.Reason
		}
		if f.From == "" {
			written = append(written, text(label))
			continue
		}
		labels = append(labels, text(label+"<br/>from "+f.From))
	}
	if len(wf.ProtectedFiles) > 0 {
		labels = append(labels, text(workflow.ShortcutProtectedFiles+": "+strings.Join(wf.ProtectedFiles, ", ")))
	}
	return append(labels, written...)
}

// label describes an entry: its conditions, its action and the shortcut it was expanded from, written as text says.
func label(e workflow.Entry) string {
	var parts []string
	if c := cond(e.When); c != "" {
		parts = append(parts, "when "+c)
	}
	if c := cond(e.Unless); c != "" {
		parts = append(parts, "unless "+c)
	}
	a := e.Action
	act := a.Verb()
	switch act {
	case workflow.VerbBlock:
		act += ": " + a.Block.Message
		if a.Block.Once {
			act += " (once)"
		}
	case workflow.VerbRun:
		if a.Run.Rules != "" {
			act += " rules " + a.Run.Rules
		} else {
			act += " " + strings.Join(a.Run.Command, " ")
		}
		if a.Run.Always {
			act += " (always)"
		}
	case workflow.VerbInject:
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
	case workflow.VerbNotify:
		act += ": " + a.Notify.Message
		if a.Notify.Always {
			act += " (always)"
		}
	}
	parts = append(parts, act)
	if e.From != "" {
		parts = append(parts, "from "+e.From)
	}
	return text(strings.Join(parts, "<br/>"))
}

// text writes a label with single quotes for double ones, which would end it, and the line breaks of a
// message as breaks of the label, so each statement keeps one line.
func text(label string) string {
	return strings.NewReplacer(`"`, "'", "\r\n", "<br/>", "\n", "<br/>").Replace(label)
}

// cond writes the conditions as key=value, ordered by key.
func cond(c workflow.Cond) string {
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
