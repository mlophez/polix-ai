package setup

// Parts are the parts of a setup that install writes and uninstall removes, each asked with its flag: the plugin of
// the user (--skills), the hooks of the agents in the repository (--hooks) and the hooks of git in its local git
// configuration (--githooks). A command that asks for none of them asks for all (Defaulted).
type Parts struct {
	Skills   bool // the plugin polix, with its skills and agents, in the local marketplace and registered with each agent
	Hooks    bool // the hooks of polix of each agent in the hooks file of the repository
	GitHooks bool // the hooks of git of polix (GitHooks) in the local git configuration of the repository
}

// Valid reports why p is not a choice of parts: never, since the parts combine freely and none is every one.
func (p Parts) Valid() error { return nil }

// None reports whether p asks for no part.
func (p Parts) None() bool { return p == Parts{} }

// Defaulted returns every part when p asks for none, else p.
func (p Parts) Defaulted() Parts {
	if p.None() {
		p.Skills, p.Hooks, p.GitHooks = true, true, true
	}
	return p
}

// WithoutGitHooks returns p without the hooks of git: the parts left when a repository keeps them.
func (p Parts) WithoutGitHooks() Parts {
	p.GitHooks = false
	return p
}

// OnlyGitHooks reports whether p is only the hooks of git: no plugin, no hooks of the agents, and so no agent.
func (p Parts) OnlyGitHooks() bool { return p.GitHooks && !p.Skills && !p.Hooks }

// Repository reports whether p has a part that lives in a repository: the hooks of the agents or those of git.
func (p Parts) Repository() bool { return p.Hooks || p.GitHooks }
