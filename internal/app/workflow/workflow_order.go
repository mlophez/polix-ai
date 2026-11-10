package workflow

import (
	"fmt"
	"slices"
)

// order reports the problems that depend on the order of the entries of the event ev: each entry that never runs
// (see unreachable) and each notice a later entry may hide (see notices).
func (v *validator) order(ev string, entries []Entry) {
	v.unreachable(ev, entries)
	v.notices(ev, entries)
}

// unreachable reports each entry of the event ev that follows a block that always blocks the event, which never runs.
// stop and subagent_stop are left out, since their loop guard lets the later entries run once a block has blocked
// max_stop_retries times.
func (v *validator) unreachable(ev string, entries []Entry) {
	if ev == Stop || ev == SubagentStop {
		return
	}
	for i, gate := range entries {
		if !gate.alwaysBlocks() {
			continue
		}
		for _, e := range entries[i+1:] {
			v.add(e.Line, "%snever runs: %s always blocks the event", e.prefix(), gate.name())
		}
		return
	}
}

// notices reports each notify of the event ev followed in it by an entry that may block the event (see mayBlock),
// at the line of the notify and naming the first such entry: a blocked event shows no message to the user, so the
// gate would swallow the notice whenever it acts, on every event, stop and subagent_stop included.
func (v *validator) notices(ev string, entries []Entry) {
	for i, n := range entries {
		if n.Action.Notify == nil {
			continue
		}
		for _, gate := range entries[i+1:] {
			if gate.mayBlock(ev) {
				v.add(n.Line, "%sthe notice is not shown when %s blocks the event: move it after that entry or into "+
					"its message", n.prefix(), gate.name())
				break
			}
		}
	}
}

// mayBlock reports whether the entry may block its event ev: a block, or a run on an event that has not happened yet
// (outside Informed), whose failure blocks it.
func (e Entry) mayBlock(ev string) bool {
	return e.Action.Block != nil || e.Action.Run != nil && !slices.Contains(Informed, ev)
}

// alwaysBlocks reports whether the entry blocks every time its event comes: a block without when, unless nor once.
// The conditions under the old key if count as conditions, since the entry is already reported for them.
func (e Entry) alwaysBlocks() bool {
	return e.Action.Block != nil && !e.Action.Block.Once && len(e.When) == 0 && len(e.Unless) == 0 &&
		len(e.OldIf) == 0
}

// name names the entry in the problem of another one: by its line, and by its shortcut when it was expanded from one.
func (e Entry) name() string {
	if e.From != "" {
		return fmt.Sprintf("%s of line %d", e.From, e.Line)
	}
	return fmt.Sprintf("the entry of line %d", e.Line)
}

// prefix starts a problem of the entry with its shortcut, as the other problems of a shortcut start, when it was
// expanded from one; "" for an entry written in on.
func (e Entry) prefix() string {
	if e.From != "" {
		return e.From + ": "
	}
	return ""
}
