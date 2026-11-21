// Package adapters reads the files of the rules: Folder loads the folder of rules (rule.Load) and keeps what it read
// for the life of the process, so cmd/polix builds it once and passes its method Load to every use case that reads the
// rules. It reads, decodes and caches; what the rules mean is decided by rule.Rules.Loaded.
package adapters

import (
	"context"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"gopkg.in/yaml.v3"

	"github.com/mlophez/polix-ai/internal/app/rule"
	"github.com/mlophez/polix-ai/internal/platform/file"
)

// Folder loads the rules of a folder of rules. Its zero value works: the first Load of a root and a folder reads
// them, and every later one returns the same result.
type Folder struct {
	mu   sync.Mutex
	seen map[folderKey]rule.Loaded
}

// folderKey is the arguments of a Load, which the cache of Folder tells apart.
type folderKey struct {
	root string // the root of the repository, with the separators of the system
	dir  string // the folder of rules, relative to the root, with slashes (rules/ below path of polix.yaml)
}

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

// Load loads the rules of the folder of rules dir (relative to root, with slashes) of the repository at root: it
// reads the rules and then the retired ones, and has rule.Rules.Loaded decide what they load. It never fails: what
// stops the reading is data of its result. The first call for a root and a folder reads them, and every later call
// returns the same result.
func (f *Folder) Load(_ context.Context, root, dir string) rule.Loaded {
	f.mu.Lock()
	defer f.mu.Unlock()
	key := folderKey{root: root, dir: dir}
	if loaded, ok := f.seen[key]; ok {
		return loaded
	}
	rules, exists, err := f.readRules(root, dir)
	loaded := rules.Loaded(f.readRetired(root, dir), exists, err)
	if f.seen == nil {
		f.seen = map[folderKey]rule.Loaded{}
	}
	f.seen[key] = loaded
	return loaded
}

// readRules reads the yaml files of the folder dir of root through internal/platform/file, retired.yaml and the
// files that are not yaml left out, and decodes them in the order of their names, until the first that cannot be
// decoded. A missing folder has no rules and no error.
func (f *Folder) readRules(root, dir string) (rule.Rules, bool, error) {
	folder := filepath.Join(root, filepath.FromSlash(dir))
	exists := file.IsDir(folder)
	files, err := file.ReadDir(folder, ".yaml")
	if err != nil {
		return nil, exists, err
	}
	var rules rule.Rules
	for _, name := range f.names(files) {
		decoded, err := f.decode(dir+"/"+name, files[name])
		if err != nil {
			return rules, exists, err
		}
		rules = append(rules, decoded...)
	}
	return rules, exists, nil
}

// readRetired reads rule.RetiredFile of the folder dir of root through internal/platform/file and decodes its rules,
// each with its file. A file that is missing, cannot be read or cannot be decoded has no rules, and is no error:
// the retired rules only take their ids.
func (f *Folder) readRetired(root, dir string) rule.Rules {
	path := filepath.Join(root, filepath.FromSlash(dir), rule.RetiredFile)
	data, found, err := file.Read(path)
	if err != nil || !found {
		return nil
	}
	rules, err := f.decode(dir+"/"+rule.RetiredFile, data)
	if err != nil {
		return nil
	}
	return rules
}

// names returns the names of the files of rules among files, sorted: yaml, without rule.RetiredFile.
func (f *Folder) names(files map[string][]byte) []string {
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
func (f *Folder) decode(path string, data []byte) (rule.Rules, error) {
	var doc fileYAML
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("%s: %s", path, doc.names().Replace(err.Error()))
	}
	rules := make(rule.Rules, 0, len(doc.Rules))
	for _, y := range doc.Rules {
		y.Description, y.File = strings.TrimSpace(y.Description), path
		rules = append(rules, rule.Rule(y))
	}
	return rules, nil
}

// names translates the names of the decoding types in the errors of yaml into those the messages always named,
// from the package load where the decoding lived, so moving it to adapters changes no message.
func (fileYAML) names() *strings.Replacer {
	return strings.NewReplacer("adapters.fileYAML", "load.fileYAML", "adapters.ruleYAML", "load.ruleYAML")
}
