package rule

import (
	"fmt"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// fileYAML is the shape of a file of ai/rules/; it keeps the yaml out of the entity.
type fileYAML struct {
	Rules []struct {
		ID            string   `yaml:"id"`
		Description   string   `yaml:"description"`
		Scope         []string `yaml:"scope"`
		Deterministic bool     `yaml:"deterministic"`
		Check         string   `yaml:"check"`
	} `yaml:"rules"`
}

// RetiredFile holds retired rules: they keep their id but never apply.
const RetiredFile = "retired.yaml"

// Parse reads the rules of one file of ai/rules/; file is its path relative to the root. It is domain, not an
// adapter: the format of ai/rules/ belongs to polix, and the adapters of every use case that reads rules share it.
// The first invalid rule stops it.
func Parse(file string, data []byte) ([]Rule, error) {
	rules, invalid, err := decode(file, data)
	if err != nil {
		return nil, err
	}
	for _, err := range invalid {
		if err != nil {
			return nil, fmt.Errorf("%s: %w", file, err)
		}
	}
	return rules, nil
}

// ParseAll reads the rules of one file like Parse, but goes on past an invalid rule, so polix config check and polix
// doctor list every problem: it returns every rule of the file, invalid ones included, and one problem for each
// invalid rule in the words of polix config check ("<file> <id>: no description"). Only broken YAML is an error.
func ParseAll(file string, data []byte) ([]Rule, []string, error) {
	rules, invalid, err := decode(file, data)
	if err != nil {
		return nil, nil, err
	}
	var problems []string
	for i, err := range invalid {
		if err != nil {
			problems = append(problems, rules[i].problem())
		}
	}
	return rules, problems, nil
}

// ParseFiles reads the files of the folder of rules dir, relative to the root (by name, as
// internal/platform/file.ReadDir gives them), retired.yaml and files that are not yaml left out, in the order of
// their names, and returns every rule ordered by id, stably. Like Parse, the first invalid rule stops it.
func ParseFiles(dir string, files map[string][]byte) ([]Rule, error) {
	var all []Rule
	for _, name := range ruleFiles(files) {
		rules, err := Parse(dir+"/"+name, files[name])
		if err != nil {
			return nil, err
		}
		all = append(all, rules...)
	}
	sort.SliceStable(all, func(i, j int) bool { return all[i].ID < all[j].ID })
	return all, nil
}

// ParseFilesAll reads the files of the folder of rules dir like ParseFiles, but like ParseAll goes on past an
// invalid rule: it returns every rule, ordered by id, and the problems of the invalid ones. Only broken YAML stops
// it.
func ParseFilesAll(dir string, files map[string][]byte) ([]Rule, []string, error) {
	var all []Rule
	var problems []string
	for _, name := range ruleFiles(files) {
		rules, invalid, err := ParseAll(dir+"/"+name, files[name])
		if err != nil {
			return nil, nil, err
		}
		all, problems = append(all, rules...), append(problems, invalid...)
	}
	sort.SliceStable(all, func(i, j int) bool { return all[i].ID < all[j].ID })
	return all, problems, nil
}

// ruleFiles returns the names of the files of rules among files, sorted: yaml, without RetiredFile.
func ruleFiles(files map[string][]byte) []string {
	var names []string
	for name := range files {
		if strings.HasSuffix(name, ".yaml") && name != RetiredFile {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	return names
}

// decode reads the rules of one file, each with why it is not valid (nil when it is), in the same order.
func decode(file string, data []byte) ([]Rule, []error, error) {
	var doc fileYAML
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, nil, fmt.Errorf("%s: %w", file, err)
	}
	out := make([]Rule, 0, len(doc.Rules))
	invalid := make([]error, 0, len(doc.Rules))
	for _, y := range doc.Rules {
		r := Rule{
			ID:            y.ID,
			Description:   strings.TrimSpace(y.Description),
			Scope:         y.Scope,
			Deterministic: y.Deterministic,
			Check:         y.Check,
			File:          file,
		}
		err := r.Valid()
		out, invalid = append(out, r), append(invalid, err)
	}
	return out, invalid, nil
}

// problem is why an invalid rule is not valid, in the words of polix config check: the file, the id and what is wrong.
func (r Rule) problem() string {
	switch {
	case r.ID == "":
		return r.File + ": a rule has no id"
	case r.Description == "":
		return fmt.Sprintf("%s %s: no description", r.File, r.ID)
	case r.Check != "" && !r.Deterministic:
		return fmt.Sprintf("%s %s: check %s needs deterministic: true", r.File, r.ID, r.Check)
	}
	return fmt.Sprintf("%s %s: check %s is an absolute path, not relative to the root", r.File, r.ID, r.Check)
}
