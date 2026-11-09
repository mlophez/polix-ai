package setup

import (
	"errors"
	"fmt"
	"slices"
)

// Change is what install, uninstall or a refresh did with one path, or with the registration of the plugin in an
// agent. It is built with a literal followed by Valid.
type Change struct {
	Path   string // the path, the plugin in an agent, or the command to run by hand
	Action string // one of Actions
}

// Changes are the changes of a run of install, uninstall or a refresh, in the order they happened.
type Changes []Change

// ErrCancelled is the error of a person that did not confirm an install or an uninstall.
var ErrCancelled = errors.New("cancelled")

// Actions a change may tell: todo is a command the user has to run by hand.
var Actions = []string{"created", "updated", "removed", "registered", "unregistered", "todo"}

// Valid reports why c is not a change: it names a path and one of Actions.
func (c Change) Valid() error {
	switch {
	case c.Path == "":
		return errors.New("a change needs a path")
	case !slices.Contains(Actions, c.Action):
		return fmt.Errorf("unknown action %q of a change of %s", c.Action, c.Path)
	}
	return nil
}

// Todo reports whether the change is a command the user has to run by hand.
func (c Change) Todo() bool { return c.Action == "todo" }

// Removal reports whether the change took something away.
func (c Change) Removal() bool { return c.Action == "removed" || c.Action == "unregistered" }

// Add returns a copy of cs followed by the change of each path with action, validated; nil when there is none.
func (cs Changes) Add(action string, paths ...string) (Changes, error) {
	var out Changes
	out = append(out, cs...)
	for _, p := range paths {
		ch := Change{Path: p, Action: action}
		if err := ch.Valid(); err != nil {
			return nil, err
		}
		out = append(out, ch)
	}
	return out, nil
}

// Synced returns a copy of cs followed by the changes of a folder synced to the files of the plugin: created, updated and
// removed paths.
func (cs Changes) Synced(created, updated, removed []string) (Changes, error) {
	out := cs
	for _, c := range []struct {
		action string
		paths  []string
	}{{"created", created}, {"updated", updated}, {"removed", removed}} {
		var err error
		if out, err = out.Add(c.action, c.paths...); err != nil {
			return nil, err
		}
	}
	return out, nil
}
