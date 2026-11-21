// Package adapters reads polix.yaml: File loads it (workflow.Load) and keeps what it read for the life of the process,
// so cmd/polix builds it once and passes its method Load to every use case that reads polix.yaml. It reads, decodes and
// caches; what the workflow means is decided by workflow.Workflow.Loaded. The decoding types, with their yaml tags, are
// in the files file_*.go.
package adapters

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"sync"

	"gopkg.in/yaml.v3"

	"github.com/mlophez/polix-ai/internal/app/workflow"
	"github.com/mlophez/polix-ai/internal/platform/file"
	"github.com/mlophez/polix-ai/internal/platform/semver"
)

// File loads polix.yaml, or another workflow file. Its zero value works: the first Load of a file and a version
// reads it, and every later one returns the same result.
type File struct {
	mu   sync.Mutex
	seen map[fileKey]workflow.Loaded
}

// fileKey is the arguments of a Load, which the cache of File tells apart.
type fileKey struct {
	path    string // the file of the workflow, polix.yaml of a repository or another one, with its separators
	running string // the version of the running polix, which decides whether polix.yaml requires a newer one
}

// Load loads the workflow file at path for the polix of version running: it reads and decodes the file and has
// workflow.Workflow.Loaded decide what it loads. It never fails: what stops the reading is data of its result. The
// first call for a file and a version reads the file, and every later call returns the same result.
func (f *File) Load(_ context.Context, path, running string) workflow.Loaded {
	f.mu.Lock()
	defer f.mu.Unlock()
	key := fileKey{path: path, running: running}
	if loaded, ok := f.seen[key]; ok {
		return loaded
	}
	wf, guardrails, exists, err := f.read(path, running)
	loaded := wf.Loaded(guardrails, exists, err)
	if f.seen == nil {
		f.seen = map[fileKey]workflow.Loaded{}
	}
	f.seen[key] = loaded
	return loaded
}

// read reads the workflow file at path through internal/platform/file and decodes it for the polix of version
// running: the workflow, with the protections and the entries its shortcuts expand into, and the section guardrails
// as written, and whether the file exists. A missing file has no workflow and no error; one that cannot be read, or
// that decode rejects, is the error.
func (f *File) read(path, running string) (workflow.Workflow, workflow.Guardrails, bool, error) {
	data, exists, err := file.Read(path)
	if err != nil {
		return workflow.Workflow{}, workflow.Guardrails{}, true, err
	}
	if !exists {
		return workflow.Workflow{}, workflow.Guardrails{}, false, nil
	}
	wf, guardrails, err := f.decode(data, running)
	return wf, guardrails, true, err
}

// decode decodes polix.yaml (data) for the polix of version running. It returns a TooOldError when the workflow
// requires a newer polix than running, read before the rest, which an older polix may not know (a development build is
// never too old); an error for broken YAML, or a UnknownKeyError for an unknown top level key; and else the workflow
// with the error of its Valid, and the section guardrails as written.
func (f *File) decode(data []byte, running string) (workflow.Workflow, workflow.Guardrails, error) {
	if err := f.tooOld(data, running); err != nil {
		return workflow.Workflow{}, workflow.Guardrails{}, err
	}
	var root yaml.Node
	if err := yaml.Unmarshal(data, &root); err != nil {
		return workflow.Workflow{}, workflow.Guardrails{}, fmt.Errorf("%s: %w", workflow.File, err)
	}
	lines, err := f.top(&root)
	if err != nil {
		return workflow.Workflow{}, workflow.Guardrails{}, err
	}
	var y workflowYAML
	if err := root.Decode(&y); err != nil {
		return workflow.Workflow{}, workflow.Guardrails{}, fmt.Errorf("%s: %s", workflow.File,
			y.names().Replace(err.Error()))
	}
	if y.StopRetries == 0 {
		y.StopRetries = workflow.DefaultStopRetries
	}
	return y.workflow(lines["on"], lines["githooks"])
}

// tooOld returns the TooOldError of a workflow (data) that requires a newer polix than running; nil when it does not,
// when running is a development build, which is never too old, or when what the workflow requires is not a version.
func (f *File) tooOld(data []byte, running string) error {
	req := f.requires(data)
	if !semver.IsRelease(running) || !semver.Valid(req) || semver.Compare(running, req) >= 0 {
		return nil
	}
	old := workflow.TooOldError{Requires: strings.TrimPrefix(req, "v"), Running: running}
	if err := old.Valid(); err != nil {
		return err
	}
	return old
}

// requires reads the top level `requires` of a workflow without decoding the rest; "" when it has none or the
// YAML is broken (decode reports it then).
func (f *File) requires(data []byte) string {
	var root yaml.Node
	if yaml.Unmarshal(data, &root) != nil || len(root.Content) != 1 || root.Content[0].Kind != yaml.MappingNode {
		return ""
	}
	top := root.Content[0]
	for i := 0; i+1 < len(top.Content); i += 2 {
		if top.Content[i].Value == "requires" && top.Content[i+1].Kind == yaml.ScalarNode {
			return strings.TrimSpace(top.Content[i+1].Value)
		}
	}
	return ""
}

// top checks the top level keys of the document root, a map: the first one that is not one of workflow.TopKeys is
// a UnknownKeyError, with the new spelling of a renamed one. It returns the line of each event of the sections on
// and githooks, by section.
func (f *File) top(root *yaml.Node) (map[string]map[string]int, error) {
	if len(root.Content) != 1 || root.Content[0].Kind != yaml.MappingNode {
		return nil, nil
	}
	lines := map[string]map[string]int{}
	top := root.Content[0]
	for i := 0; i < len(top.Content); i += 2 {
		k := top.Content[i]
		if !slices.Contains(workflow.TopKeys, k.Value) {
			unknown := workflow.UnknownKeyError{Key: k.Value, Line: k.Line}
			if err := unknown.Valid(); err != nil {
				return nil, err
			}
			return nil, unknown
		}
		if (k.Value == "on" || k.Value == "githooks") && i+1 < len(top.Content) {
			lines[k.Value] = nodeYAML{top.Content[i+1]}.lines()
		}
	}
	return lines, nil
}
