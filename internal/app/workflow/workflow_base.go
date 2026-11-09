package workflow

import "slices"

// DefaultBase is the branch a change merges into when the workflow does not say.
const DefaultBase = "main"

// BaseBranches returns the branches a change may merge into, DefaultBase when the workflow names none.
func (w Workflow) BaseBranches() []string {
	if len(w.Base) == 0 {
		return []string{DefaultBase}
	}
	return slices.Clone(w.Base)
}

// Bases returns the base asked (`--base`) when there is one, else the base branches of the workflow (BaseBranches).
func (w Workflow) Bases(base string) []string {
	if base != "" {
		return []string{base}
	}
	return w.BaseBranches()
}
