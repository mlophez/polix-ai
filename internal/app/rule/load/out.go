package load

import (
	"context"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/mlophez/polix-ai/internal/app/rule"
	"github.com/mlophez/polix-ai/internal/platform/file"
)

// Adapter implements Port on the disk. The entry passes it as the port; there is no constructor.
type Adapter struct{}

// fileYAML is the shape of a file of the folder of rules; it keeps the yaml out of the entity.
type fileYAML struct {
	Rules []ruleYAML `yaml:"rules"`
}

// ruleYAML is the shape of one rule of a file, with the fields of rule.Rule in its order, so it converts to one;
// File is not in the yaml, but where it was read from.
type ruleYAML struct {
	ID            string   `yaml:"id"`
	Description   string   `yaml:"description"`
	Scope         []string `yaml:"scope"`
	Deterministic bool     `yaml:"deterministic"`
	Check         string   `yaml:"check"`
	File          string   `yaml:"-"`
}

// ReadRules reads the yaml files of the folder dir of root through internal/platform/file, retired.yaml and the
// files that are not yaml left out, and decodes them in the order of their names, until the first that cannot be
// decoded. A missing folder has no rules and no error.
func (a Adapter) ReadRules(_ context.Context, root, dir string) ([]rule.Rule, bool, error) {
	folder := filepath.Join(root, filepath.FromSlash(dir))
	exists := file.IsDir(folder)
	files, err := file.ReadDir(folder, ".yaml")
	if err != nil {
		return nil, exists, err
	}
	var rules []rule.Rule
	for _, name := range a.names(files) {
		decoded, err := a.decode(dir+"/"+name, files[name])
		if err != nil {
			return rules, exists, err
		}
		rules = append(rules, decoded...)
	}
	return rules, exists, nil
}

// names returns the names of the files of rules among files, sorted: yaml, without rule.RetiredFile.
func (a Adapter) names(files map[string][]byte) []string {
	var names []string
	for name := range files {
		if strings.HasSuffix(name, ".yaml") && name != rule.RetiredFile {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	return names
}

// decode reads the rules of the file of rules at path (relative to the root, with slashes), in their order, each
// with its description trimmed and its file; yaml that cannot be decoded is the error, with the file.
func (a Adapter) decode(path string, data []byte) ([]rule.Rule, error) {
	var doc fileYAML
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	rules := make([]rule.Rule, 0, len(doc.Rules))
	for _, y := range doc.Rules {
		y.Description, y.File = strings.TrimSpace(y.Description), path
		rules = append(rules, rule.Rule(y))
	}
	return rules, nil
}
