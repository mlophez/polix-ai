package git

import (
	"context"
	"errors"
	"os/exec"
	"regexp"
	"strings"
)

// SetHook writes the hook name into the local configuration of the repository (hook.<name>), as git 2.54 and later
// read it: its events replaced by event and its command set. It returns "created" when the hook had no key,
// "updated" when it had other values, and "" when it already had these ones.
func (r *Repo) SetHook(ctx context.Context, name, event, command string) (string, error) {
	key := "hook." + name
	events, _ := r.git(ctx, "config", "--local", "--get-all", key+".event")
	commands, _ := r.git(ctx, "config", "--local", "--get-all", key+".command")
	if events == event && commands == command {
		return "", nil
	}
	if _, err := r.git(ctx, "config", "--local", "--replace-all", key+".event", event); err != nil {
		return "", err
	}
	if _, err := r.git(ctx, "config", "--local", "--replace-all", key+".command", command); err != nil {
		return "", err
	}
	if events == "" && commands == "" {
		return "created", nil
	}
	return "updated", nil
}

// RemoveHook removes the section of the hook name from the local configuration of the repository, every key of it,
// and tells whether it was there; the other keys stay.
func (r *Repo) RemoveHook(ctx context.Context, name string) (bool, error) {
	pattern := "^hook\\." + regexp.QuoteMeta(name) + "\\."
	if _, err := r.git(ctx, "config", "--local", "--get-regexp", pattern); err != nil {
		return false, nil //nolint:nilerr // git config exits with 1 when no key matches: there is nothing to remove
	}
	if _, err := r.git(ctx, "config", "--local", "--remove-section", "hook."+name); err != nil {
		return false, err
	}
	return true, nil
}

// Hooks returns the hooks git runs for event, as `git hook list` names them (a hook of the configuration by its
// name), apart from the hooks of the configuration it lists as disabled (hook.<name>.enabled=false, listed as
// "disabled<TAB><name>"), which it does not run; and whether this git lists them: a git older than 2.54, without
// `git hook list`, neither lists nor runs the hooks of its configuration, and returns false without error. An event
// without hooks has none.
func (r *Repo) Hooks(ctx context.Context, event string) (names, disabled []string, ok bool, err error) {
	// The messages of git in English, which tell an event without hooks from a git without the command.
	out, err := runRaw(ctx, r.Root, []string{"LC_ALL=C"}, "hook", "list", event)
	if err == nil {
		names, disabled = hookList(out)
		return names, disabled, true, nil
	}
	exit, isExit := errors.AsType[*exec.ExitError](err)
	msg := err.Error()
	switch {
	case !isExit:
		return nil, nil, false, err
	case strings.Contains(msg, "no hooks found"):
		return nil, nil, true, nil
	case exit.ExitCode() == 129 || strings.Contains(msg, "is not a git command"):
		return nil, nil, false, nil
	}
	return nil, nil, false, err
}

// hookList reads the output of `git hook list`, one hook per line: the names of the hooks git runs, and apart those
// of the lines "disabled<TAB><name>", which it lists and does not run.
func hookList(out string) (names, disabled []string) {
	for line := range strings.SplitSeq(out, "\n") {
		name, off := strings.CutPrefix(line, "disabled\t")
		if name = strings.TrimSpace(name); name == "" {
			continue
		}
		if off {
			disabled = append(disabled, name)
		} else {
			names = append(names, name)
		}
	}
	return names, disabled
}
