package diagnose

import (
	"slices"
	"strings"

	"github.com/mlophez/polix-ai/internal/app/setup"
)

// githooks checks that git runs polix githook on each event polix registers (setup.GitHooks), as `git hook list`
// names them: one ok line when it runs both, naming the git version githooks need; else a warning per event without its hook, saying that the hook is
// disabled in the git configuration when git lists it as disabled (setup.Inspection.DisabledGitHooks), which polix
// install does not change, and asking to run polix install otherwise; or one warning when git does not list the
// hooks of its configuration, older than 2.54, which ignores them. It never fails, and says nothing when git was not
// asked (setup.Inspection.GitHooks nil).
func githooks(i setup.Inspection) []setup.Finding {
	if i.GitHooks == nil {
		return nil
	}
	if !i.ListsHooks {
		return found(setup.LevelWarn, "githooks", "git does not run hooks from its configuration (it needs git 2.54 "+
			"or later): the githooks of polix.yaml do not apply until git is updated")
	}
	var f []setup.Finding
	events := make([]string, 0, len(setup.GitHooks))
	for _, h := range setup.GitHooks {
		events = append(events, string(h))
		switch {
		case slices.Contains(i.GitHooks[string(h)], h.Name()):
		case slices.Contains(i.DisabledGitHooks[string(h)], h.Name()):
			f = append(f, found(setup.LevelWarn, "githooks", "git does not run polix githook on %s: hook.%s is "+
				"disabled in the git configuration (hook.%[2]s.enabled false)", h, h.Name())...)
		default:
			f = append(f, found(setup.LevelWarn, "githooks", "git does not run polix githook on %s: run polix install",
				h)...)
		}
	}
	if len(f) > 0 {
		return f
	}
	return found(setup.LevelOK, "githooks", "git runs polix githook on %s (githooks need git 2.54 or later)",
		strings.Join(events, ", "))
}
