package rule

import (
	"fmt"
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
func Parse(file string, data []byte) ([]Rule, error) {
	var doc fileYAML
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("%s: %w", file, err)
	}
	out := make([]Rule, 0, len(doc.Rules))
	for _, y := range doc.Rules {
		r := Rule{
			ID:            y.ID,
			Description:   strings.TrimSpace(y.Description),
			Scope:         y.Scope,
			Deterministic: y.Deterministic,
			Check:         y.Check,
			File:          file,
		}
		if err := r.Valid(); err != nil {
			return nil, fmt.Errorf("%s: %w", file, err)
		}
		out = append(out, r)
	}
	return out, nil
}
