package rule

// CheckRuleOutput is the decision of the logic of CheckRule, which the shell carries out through the port: the
// rules whose check runs, in their order.
type CheckRuleOutput struct {
	Run []Rule
}

// CheckRule decides which checks run: those of the deterministic rules, all of them or, with onlyChanged, only
// those whose scope matches the changed files.
func CheckRule(rules []Rule, onlyChanged bool, changed []string) CheckRuleOutput {
	if onlyChanged {
		rules = Matching(rules, changed)
	}
	var out CheckRuleOutput
	for _, r := range rules {
		if r.HasCheck() {
			out.Run = append(out.Run, r)
		}
	}
	return out
}
