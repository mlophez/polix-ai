package workflow

import (
	"fmt"
	"maps"
	"regexp"
	"slices"
	"strings"
)

// Measure is the value of a measure condition (changed_lines, changed_files, risk_score): it holds when the
// measure is at least AtLeast.
type Measure struct {
	AtLeast int
}

// Valid tells why the measure is not valid: a negative minimum.
func (m Measure) Valid() error {
	if m.AtLeast < 0 {
		return fmt.Errorf("needs a non-negative integer, got %d", m.AtLeast)
	}
	return nil
}

// Measured reads the value of the measure condition key of c, a non-negative integer: the measure holds from that
// number up. The map of the old vocabulary, { at_least: N }, is reported with the number to write instead.
func (c Cond) Measured(key string) (Measure, error) {
	v := c[key]
	if m, ok := c.Nested(key); ok {
		if n, ok := m["at_least"]; ok {
			return Measure{}, fmt.Errorf("needs a non-negative integer, got a map: use %v instead of { at_least: %v }",
				n, n)
		}
		return Measure{}, fmt.Errorf("needs a non-negative integer, got %v", v)
	}
	n, ok := v.(int)
	if !ok {
		return Measure{}, fmt.Errorf("needs a non-negative integer, got %v", v)
	}
	measure := Measure{AtLeast: n}
	if err := measure.Valid(); err != nil {
		return Measure{}, err
	}
	return measure, nil
}

// Nested reads the value of the condition key of c, a `not` or an `any`, as a Cond; a nested mapping may come as a
// Cond or as a map[string]any, and both are accepted.
func (c Cond) Nested(key string) (Cond, bool) {
	switch t := c[key].(type) {
	case Cond:
		return t, true
	case map[string]any:
		return Cond(t), true
	}
	return nil, false
}

// Strings reads the value of the condition key of c, which may be a string or a list of strings.
func (c Cond) Strings(key string) ([]string, error) {
	switch t := c[key].(type) {
	case string:
		return []string{t}, nil
	case []any:
		out := make([]string, 0, len(t))
		for _, x := range t {
			s, ok := x.(string)
			if !ok {
				return nil, fmt.Errorf("expected strings, got %v", x)
			}
			out = append(out, s)
		}
		return out, nil
	case []string:
		return t, nil
	default:
		return nil, fmt.Errorf("expected a string or a list of strings, got %v", t)
	}
}

// appendFinished appends to out the subagent types the `finished` of c names, in c and the maps nested in it.
func (c Cond) appendFinished(out []string) []string {
	for k := range c {
		switch k {
		case "finished":
			names, _ := c.Strings(k)
			out = append(out, names...)
		case "not", "any":
			if nested, ok := c.Nested(k); ok {
				out = nested.appendFinished(out)
			}
		}
	}
	return out
}

// cond reports the problems of the conditions c on event ev, in the order of their names; an empty ev skips the
// events they hold on. inAny tells that c is the map of an `any`, where contains is rejected.
func (v *validator) cond(ev string, c Cond, line int, inAny bool) {
	for _, k := range slices.Sorted(maps.Keys(c)) {
		if !slices.Contains(Conditions, k) {
			v.unknownCondition(line, k)
			continue
		}
		if only, ok := conditionEvents[k]; ok && ev != "" && !slices.Contains(only, ev) {
			v.add(line, "%s never holds on %s (only on %s)", k, ev, strings.Join(only, ", "))
		}
		v.condition(ev, c, k, line, inAny)
	}
}

// unknownCondition reports the condition k that is not one of Conditions, with its new spelling when it is a name of
// the old vocabulary.
func (v *validator) unknownCondition(line int, k string) {
	if h, ok := renamed["condition"][k]; ok {
		v.add(line, "unknown condition %q: %s", k, h)
		return
	}
	v.add(line, "unknown condition %q (conditions: %s)", k, strings.Join(Conditions, ", "))
}

// condition reports the problems of the value of the known condition k of c (see cond).
func (v *validator) condition(ev string, c Cond, k string, line int, inAny bool) {
	switch k {
	case "not":
		v.condNot(ev, c, line)
	case "any":
		v.condAny(ev, c, line)
	case "changed_lines", "changed_files", "risk_score":
		if _, err := c.Measured(k); err != nil {
			v.add(line, "%s: %v", k, err)
		}
	case "branch":
		v.condBranch(c, line)
	case "exit":
		if _, ok := c[k].(int); !ok {
			v.add(line, "exit needs a number")
		}
	case "contains":
		v.condContains(c, line, inAny)
	default:
		v.condStrings(c, k, line)
	}
}

// condNot reports the problems of the `not` of c: a value that is not a map, or the problems of its conditions.
func (v *validator) condNot(ev string, c Cond, line int) {
	nested, ok := c.Nested("not")
	if !ok {
		v.add(line, "not needs a map of conditions")
		return
	}
	v.cond(ev, nested, line, false)
}

// condAny reports the problems of the `any` of c: a value that is not a map, an empty map, or the problems of its
// conditions.
func (v *validator) condAny(ev string, c Cond, line int) {
	nested, ok := c.Nested("any")
	if !ok {
		v.add(line, "any needs a map of conditions")
		return
	}
	if len(nested) == 0 {
		v.add(line, "any needs at least one condition")
	}
	v.cond(ev, nested, line, true)
}

// condBranch reports the problems of the `branch` of c: a value that is not a regular expression that compiles.
func (v *validator) condBranch(c Cond, line int) {
	s, ok := c["branch"].(string)
	if !ok {
		v.add(line, "branch needs a regular expression")
		return
	}
	if _, err := regexp.Compile(s); err != nil {
		v.add(line, "branch: %v", err)
	}
}

// condContains reports the problems of the `contains` of c: a value that is not a text, no file next to it, or a
// place inside an any (inAny).
func (v *validator) condContains(c Cond, line int, inAny bool) {
	s, ok := c["contains"].(string)
	if !ok {
		v.add(line, "contains needs a text")
	}
	if _, ok := c["file"]; !ok {
		v.add(line, "contains only works next to file")
	}
	if inAny {
		v.add(line, "contains cannot go inside any, where it holds on its own: put file and contains "+
			"next to the any, or in a not inside it")
	}
	v.placeholders(line, s)
}

// condStrings reports the problems of the condition k of c whose value is a string or a list of strings: another
// value, a placeholder that is not known, an empty finished or a var not defined in vars.
func (v *validator) condStrings(c Cond, k string, line int) {
	list, err := c.Strings(k)
	if err != nil {
		v.add(line, "%s: %v", k, err)
		return
	}
	for _, s := range list {
		v.placeholders(line, s)
		if k == "finished" && strings.TrimSpace(s) == "" {
			v.add(line, "finished needs the type of a subagent")
		}
		if k == "var" && v.w != nil {
			if _, ok := v.w.Vars[s]; !ok {
				v.add(line, "var %s is not defined in vars", s)
			}
		}
	}
}
