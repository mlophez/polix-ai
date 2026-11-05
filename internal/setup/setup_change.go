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

// Changes returns the change of each path with action, validated.
func Changes(action string, paths ...string) ([]Change, error) {
	var out []Change
	for _, p := range paths {
		ch := Change{Path: p, Action: action}
		if err := ch.Valid(); err != nil {
			return nil, err
		}
		out = append(out, ch)
	}
	return out, nil
}

// Synced returns the changes of a folder synced to the files of the plugin: created, updated and removed paths.
func Synced(created, updated, removed []string) ([]Change, error) {
	var out []Change
	for _, c := range []struct {
		action string
		paths  []string
	}{{"created", created}, {"updated", updated}, {"removed", removed}} {
		ch, err := Changes(c.action, c.paths...)
		if err != nil {
			return nil, err
		}
		out = append(out, ch...)
	}
	return out, nil
}
