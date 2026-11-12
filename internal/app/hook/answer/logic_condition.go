package answer

import (
	"fmt"
	"regexp"
	"slices"
	"sort"

	"github.com/mlophez/polix-ai/internal/app/agent"
	"github.com/mlophez/polix-ai/internal/app/diff"
	"github.com/mlophez/polix-ai/internal/app/state"
	"github.com/mlophez/polix-ai/internal/app/workflow"
	"github.com/mlophez/polix-ai/internal/platform/glob"
)

// matches reports whether every condition of c holds for the event, cheapest first, then in the order of their
// names. scope holds the globs of the nearest `changed` of the enclosing maps, which the measures take; a
// `changed` of c replaces it for c and the maps nested in it.
func (r *pass) matches(c workflow.Cond, scope []string) bool {
	if _, ok := c["changed"]; ok {
		scope = r.list(c, "changed")
	}
	for _, k := range keys(c) {
		if !r.condition(k, c, scope) || r.waiting() {
			return false
		}
	}
	return true
}

// condition evaluates the condition k of all; scope holds the globs the measures take (see matches).
func (r *pass) condition(k string, all workflow.Cond, scope []string) bool {
	switch k {
	case "not":
		return r.condNot(all, scope)
	case "any":
		return r.condAny(all, scope)
	case "changed_lines", "changed_files", "risk_score":
		return r.condMeasure(k, all, scope)
	case "branch":
		s, _ := all[k].(string)
		return branchMatches(s, r.branch())
	case "var":
		return r.condVar(all)
	case "path":
		return r.condPath(all)
	case "changed":
		return r.condChanged(all)
	case "file":
		return r.fileExists(r.list(all, k), all["contains"])
	case "contains":
		return true // evaluated together with file
	case "command":
		return len(matchingCommands(r.list(all, k), r.ev.Event.Command, r.ev.Windows())) > 0
	case "writes":
		_, _, ok := r.writes(r.list(all, k))
		return ok
	case "exit":
		return r.condExit(all)
	}
	return r.condAgents(k, all, scope)
}

// condAgents evaluates the condition k of all that is about the agents: the actor, the subagents that finished and
// those still running; any other k does not hold.
func (r *pass) condAgents(k string, all workflow.Cond, scope []string) bool {
	switch k {
	case "actor":
		return slices.Contains(r.list(all, k), r.ev.Event.Actor)
	case "finished":
		return r.condFinished(all, scope)
	case "agents_are_running":
		want, _ := all[k].(bool)
		running := r.agentsRunning()
		return !r.waiting() && running == want
	}
	return false
}

// condNot holds when the nested conditions of the not of all do not.
func (r *pass) condNot(all workflow.Cond, scope []string) bool {
	nested, _ := all.Nested("not")
	m := r.matches(nested, scope)
	return !r.waiting() && !m
}

// condAny holds when one of the nested conditions of the any of all does, cheapest first.
func (r *pass) condAny(all workflow.Cond, scope []string) bool {
	nested, _ := all.Nested("any")
	// The `changed` of the nested map scopes its measures, as in matches.
	if _, ok := nested["changed"]; ok {
		scope = r.list(nested, "changed")
	}
	for _, nk := range keys(nested) {
		holds := r.condition(nk, nested, scope)
		if r.waiting() {
			return false
		}
		if holds {
			return true
		}
	}
	return false
}

// condMeasure holds when the measure k of the diff, over the globs of scope, is at least the value of k in all. It
// fails closed: a diff that cannot be measured never skips what the measure guards.
func (r *pass) condMeasure(k string, all workflow.Cond, scope []string) bool {
	m, err := all.Measured(k)
	if err != nil {
		return false
	}
	d, measured := r.diff()
	if r.waiting() {
		return false
	}
	if !measured {
		return true
	}
	return measureHolds(k, m, d.Narrow(scope), r.ev.Workflow.Weights)
}

// condVar holds when every variable of the var of all has a value on the current branch.
func (r *pass) condVar(all workflow.Cond) bool {
	vars := branchVars(r.ev.Workflow, r.branch())
	for _, name := range r.list(all, "var") {
		if vars[name] == "" {
			return false
		}
	}
	return true
}

// condPath holds when the path of the event matches one of the globs of the path of all, without case where file
// names ignore it, as writes: `.ENV` is `.env` there.
func (r *pass) condPath(all workflow.Cond) bool {
	return r.ev.Event.Path != "" && anyPathFold(r.list(all, "path"), r.ev.Event.Path, r.ev.Fold())
}

// condChanged holds when a file not committed yet matches one of the globs of the changed of all.
func (r *pass) condChanged(all workflow.Cond) bool {
	patterns := r.list(all, "changed")
	for _, f := range r.changed() {
		if glob.AnyPath(patterns, f) {
			return true
		}
	}
	return false
}

// condExit holds when the command line of the event exited with the exit of all. The exit code of the line only
// speaks for a command whose failure propagates to it, so with a condition command of all, one of its commands must
// propagate.
func (r *pass) condExit(all workflow.Cond) bool {
	ev := r.ev.Event
	n, _ := all["exit"].(int)
	if _, ok := all["command"]; ok && !propagates(matchingCommands(r.list(all, "command"), ev.Command, r.ev.Windows())) {
		return false
	}
	return ev.ExitCode == n
}

// condFinished holds when every subagent type of the finished of all finished on the content of the scope of the
// entry: the nearest changed, else the one of its when (see finished).
func (r *pass) condFinished(all workflow.Cond, scope []string) bool {
	if scope == nil {
		scope = r.whenScope()
	}
	for _, name := range r.list(all, "finished") {
		if !r.finished(name, scope) || r.waiting() {
			return false
		}
	}
	return true
}

// list reads the value of the condition k of c as a list of strings, expanded.
func (r *pass) list(c workflow.Cond, k string) []string {
	s, _ := c.Strings(k)
	out := make([]string, len(s))
	for i := range s {
		out[i] = r.expand(s[i])
	}
	return out
}

// fileExists reports whether a file of the repository matches one of the patterns and, with contains, holds
// that text.
func (r *pass) fileExists(patterns []string, contains any) bool {
	want := ""
	if s, ok := contains.(string); ok {
		want = r.expand(s)
	}
	if want != "" {
		return r.contains(patterns, want)
	}
	for _, f := range r.files() {
		if glob.AnyPath(patterns, f) {
			return true
		}
	}
	return false
}

// writes returns the first path a command of the event writes that matches one of the patterns (see lineWrites).
func (r *pass) writes(patterns []string) (p, held string, ok bool) {
	if len(patterns) == 0 {
		return "", "", false
	}
	ev := r.ev.Event
	paths := append([]string{r.ev.Root}, writePaths(ev.Command, ev.Cwd, r.ev.Root, r.ev.Windows())...)
	reals := r.reals(paths...)
	if r.waiting() {
		return "", "", false
	}
	return lineWrites(patterns, ev.Command, ev.Cwd, r.ev.Root, reals, r.ev.Fold(), r.ev.Windows())
}

// finished reports whether the subagent type name finished in the session, whose log alone the state holds, on the
// current branch or on the same HEAD commit, on a content whose files in scope did not change since: the tree of its latest observed finished with a hash is
// compared with the current one (see sameScope). The log is read first, so without such a line the content hash is
// never computed. A log that cannot be read, with a warning, an unknown hash, a tree git no longer has or a
// comparison that fails never holds, the last two with the error in the log.
func (r *pass) finished(name string, scope []string) bool {
	branch, head := r.branch(), r.head()
	st, err := r.currentState()
	if r.waiting() {
		return false
	}
	if err != nil {
		r.warnLog(err)
		return false
	}
	q := state.Query{Outcome: state.OutcomeFinished, Branch: branch, Head: head, ByBranch: true, Key: name}
	if q.Valid() != nil {
		return false
	}
	var last state.Entry
	for _, e := range st.Observed(q) {
		if e.Hash != "" {
			last = e
		}
	}
	if last.Hash == "" {
		return false
	}
	hash, known := r.hash()
	if r.waiting() || !known {
		return false
	}
	same, err := r.sameScope(last.Hash, hash, scope)
	if err != nil {
		r.log("finished", r.entry.Line, "warn", fmt.Sprintf(
			"the content %s finished on cannot be compared with the current one (%v): finished does not hold", name, err))
		return false
	}
	return same
}

// agentsRunning reports whether subagents of the session other than the one that acts are still running: by the
// subagents in flight the agent listed on a stop, when it gave the list (othersInFlight), else by the starts and ends
// of subagents in the log (state.State.RunningOthers), without reading it when the list is known. The actor is left
// out by its id, else by its type, except on its subagent_start, whose own start is not written yet. A log that
// cannot be read counts as no subagent running, with a warning.
func (r *pass) agentsRunning() bool {
	ev := r.ev.Event
	if ev.InFlightKnown {
		return len(othersInFlight(ev)) > 0
	}
	st, err := r.currentState()
	if r.waiting() {
		return false
	}
	if err != nil {
		r.warnLog(err)
		return false
	}
	if ev.Actor == agent.ActorSession || ev.Name == workflow.SubagentStart {
		return len(st.Running(ev.Session)) > 0
	}
	return len(st.RunningOthers(ev.Session, ev.AgentID, ev.Actor)) > 0
}

// othersInFlight returns the subagents in flight of the event ev without the one that acts: the one of its id, or,
// when it has none, the first of its type; all of them for the main session.
func othersInFlight(ev agent.Event) []agent.Subagent {
	if ev.Actor == agent.ActorSession {
		return ev.InFlight
	}
	var i int
	if ev.AgentID != "" {
		i = slices.IndexFunc(ev.InFlight, func(s agent.Subagent) bool { return s.ID == ev.AgentID })
	} else {
		i = slices.IndexFunc(ev.InFlight, func(s agent.Subagent) bool { return s.Type == ev.Actor })
	}
	if i < 0 {
		return ev.InFlight
	}
	return slices.Delete(slices.Clone(ev.InFlight), i, i+1)
}

// keys returns the conditions of c, cheapest first, then in the order of their names.
func keys(c workflow.Cond) []string {
	keys := make([]string, 0, len(c))
	for k := range c {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		ri, rj := cost(keys[i]), cost(keys[j])
		return ri < rj || ri == rj && keys[i] < keys[j]
	})
	return keys
}

// cost ranks a condition by what it reads: the event first, then what git or the log give, so a cheap
// condition that fails spares the content hash.
func cost(k string) int {
	switch k {
	case "path", "actor", "command", "exit", "contains":
		return 0
	case "writes":
		return 1
	case "branch", "var":
		return 2
	case "changed", "file":
		return 3
	case "changed_lines", "changed_files", "risk_score":
		return 4
	case "finished", "agents_are_running":
		return 5
	}
	return 6 // not and any, which may read anything
}

// branchMatches reports whether the branch matches the regular expression of a condition branch; an expression that
// does not compile never holds.
func branchMatches(expr, branch string) bool {
	re, err := regexp.Compile(expr)
	return err == nil && re.MatchString(branch)
}

// measureHolds reports whether the measure condition k holds on d: its lines, its files or its risk score with the
// weights, at least m.AtLeast.
func measureHolds(k string, m workflow.Measure, d diff.Diff, weights []workflow.Weight) bool {
	switch k {
	case "changed_lines":
		return d.Lines() >= m.AtLeast
	case "changed_files":
		return d.Count() >= m.AtLeast
	case "risk_score":
		return d.Risk(weights) >= m.AtLeast
	}
	return false
}
