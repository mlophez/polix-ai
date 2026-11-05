package rule

import "fmt"

// Run is the check of a rule ready to run: the rule and the argv its runner of rules.run in polix.yaml gives,
// with the placeholders expanded. An empty argv runs the check itself as a program, as when no runner matches.
// It is built with a literal followed by Valid.
type Run struct {
	Rule Rule
	Argv []string // the command that runs the check, from the root; empty runs the file as a program
}

// Valid reports why the run is not the run of a check: its rule must be valid and have a check.
func (r Run) Valid() error {
	if err := r.Rule.Valid(); err != nil {
		return err
	}
	if !r.Rule.HasCheck() {
		return fmt.Errorf("rule %s has no check to run", r.Rule.ID)
	}
	return nil
}
