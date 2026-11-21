package setup

// GitHook is a hook of git that polix declares in the local configuration of a repository (hook.<name>.event and
// hook.<name>.command, which git 2.54 and later run without a script), named after the event of git it runs on:
// pre-commit or commit-msg.
type GitHook string

// GitHooks are the hooks of git polix registers, in the order git runs them: install writes them in every
// repository, also without githooks in polix.yaml, so the gates apply once they are written.
var GitHooks = []GitHook{"pre-commit", "commit-msg"}

// Name returns the name of the hook in the configuration of git: polix- and its event, since git forbids a name
// equal to an event.
func (h GitHook) Name() string { return "polix-" + string(h) }

// Command returns the command git runs for the hook: plain polix, found on the PATH, as the hooks of the agents.
func (h GitHook) Command() string { return "polix githook " + string(h) }

// Key returns how a change of the hook names it, as a path of setup.Change: its section of the git configuration.
func (h GitHook) Key() string { return "git config hook." + h.Name() }
