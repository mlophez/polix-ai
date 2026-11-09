package agent

import (
	"encoding/json"
	"errors"
	"slices"
	"strings"
)

// Answer is the answer of polix to one or more events.
type Answer struct {
	Block    bool
	Reasons  []string // why it is blocked, or what is required
	Contexts []string // text to inject into the session
	Warnings []string // messages for the user that do not block
}

// Valid tells why the answer cannot be given: a block needs a reason.
func (a Answer) Valid() error {
	if a.Block && len(a.Reasons) == 0 {
		return errors.New("a block needs a reason")
	}
	return nil
}

// Refuse returns the answer blocked for reason, after the reasons it already has.
func (a Answer) Refuse(reason string) Answer {
	a.Block = true
	a.Reasons = append(slices.Clone(a.Reasons), reason)
	return a
}

// Inform returns the answer with text injected into the session, after the contexts it already has.
func (a Answer) Inform(text string) Answer {
	a.Contexts = append(slices.Clone(a.Contexts), text)
	return a
}

// Warn returns the answer with a warning for the user, after the warnings it already has.
func (a Answer) Warn(warning string) Answer {
	a.Warnings = append(slices.Clone(a.Warnings), warning)
	return a
}

// Merge returns the answer with o added, skipping texts it already has: the events of one call (an edit of
// several paths, a command and its commit) often give the same message.
func (a Answer) Merge(o Answer) Answer {
	a.Block = a.Block || o.Block
	a.Reasons = appendNew(a.Reasons, o.Reasons)
	a.Contexts = appendNew(a.Contexts, o.Contexts)
	a.Warnings = appendNew(a.Warnings, o.Warnings)
	return a
}

// render writes a block as exit 2 with the reasons on stderr, and the contexts as the JSON both agents read on
// the events of contextEvents.
func (a Answer) render(native string, contextEvents map[string]bool) (stdout, stderr string, code int) {
	if a.Block {
		msg := strings.Join(a.Reasons, "\n\n")
		if len(a.Contexts) > 0 {
			msg += "\n\n" + strings.Join(a.Contexts, "\n\n")
		}
		return "", msg + "\n", ExitBlock
	}
	var errb strings.Builder
	for _, w := range a.Warnings {
		errb.WriteString("polix: " + w + "\n")
	}
	if len(a.Contexts) > 0 && contextEvents[native] {
		out := map[string]any{"hookSpecificOutput": map[string]any{
			"hookEventName":     native,
			"additionalContext": strings.Join(a.Contexts, "\n\n"),
		}}
		data, _ := json.Marshal(out)
		stdout = string(data) + "\n"
	}
	return stdout, errb.String(), ExitOK
}

// appendNew appends to a copy of list the texts of more it does not hold yet.
func appendNew(list, more []string) []string {
	list = slices.Clone(list)
	for _, s := range more {
		if !slices.Contains(list, s) {
			list = append(list, s)
		}
	}
	return list
}
