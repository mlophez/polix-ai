package agent

import (
	"errors"
	"slices"
)

// Answer is the answer of polix to one or more events.
type Answer struct {
	Block    bool
	Reasons  []string // why it is blocked, or what is required
	Contexts []string // text to inject into the session
	Warnings []string // messages for the user that do not block
}

// texts is a list of the texts of an answer, kept without repeating one.
type texts []string

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
	a.Reasons = texts(a.Reasons).with(o.Reasons)
	a.Contexts = texts(a.Contexts).with(o.Contexts)
	a.Warnings = texts(a.Warnings).with(o.Warnings)
	return a
}

// with returns a copy of the list with the texts of more it does not hold yet appended.
func (t texts) with(more []string) []string {
	list := slices.Clone([]string(t))
	for _, s := range more {
		if !slices.Contains(list, s) {
			list = append(list, s)
		}
	}
	return list
}
