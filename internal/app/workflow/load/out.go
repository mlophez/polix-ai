package load

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/mlophez/polix-ai/internal/app/workflow"
	"github.com/mlophez/polix-ai/internal/platform/file"
	"github.com/mlophez/polix-ai/internal/platform/semver"
)

// Adapter implements Port on the disk. The entry passes it as the port; there is no constructor.
type Adapter struct{}

// ReadWorkflow reads the workflow file at path through internal/platform/file and decodes it for the polix of version
// running. A missing file has no workflow and no error; one that cannot be read is the error.
func (a Adapter) ReadWorkflow(_ context.Context, path, running string) (workflow.Workflow, workflow.Guardrails,
	bool, error) {
	data, exists, err := file.Read(path)
	if err != nil {
		return workflow.Workflow{}, workflow.Guardrails{}, true, err
	}
	if !exists {
		return workflow.Workflow{}, workflow.Guardrails{}, false, nil
	}
	wf, guardrails, err := a.decode(data, running)
	return wf, guardrails, true, err
}

// decode decodes polix.yaml (data) for the polix of version running. It returns a TooOldError when the workflow requires
// a newer polix than running, read before the rest, which an older polix may not know (a development build is never
// too old); an error for broken YAML, or a UnknownKeyError for an unknown top level key; and else the workflow with the
// error of its Valid, and the section guardrails as written.
func (a Adapter) decode(data []byte, running string) (workflow.Workflow, workflow.Guardrails, error) {
	if err := a.tooOld(data, running); err != nil {
		return workflow.Workflow{}, workflow.Guardrails{}, err
	}
	var root yaml.Node
	if err := yaml.Unmarshal(data, &root); err != nil {
		return workflow.Workflow{}, workflow.Guardrails{}, fmt.Errorf("%s: %w", workflow.File, err)
	}
	lines, err := a.top(&root)
	if err != nil {
		return workflow.Workflow{}, workflow.Guardrails{}, err
	}
	var y workflowYAML
	if err := root.Decode(&y); err != nil {
		return workflow.Workflow{}, workflow.Guardrails{}, fmt.Errorf("%s: %s", workflow.File,
			y.names().Replace(err.Error()))
	}
	if y.MaxStopRetries == 0 {
		y.MaxStopRetries = workflow.DefaultMaxStopRetries
	}
	return y.workflow(lines)
}

// tooOld returns the TooOldError of a workflow (data) that requires a newer polix than running; nil when it does not, when
// running is a development build, which is never too old, or when what the workflow requires is not a version.
func (a Adapter) tooOld(data []byte, running string) error {
	req := a.requires(data)
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
func (a Adapter) requires(data []byte) string {
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
// a UnknownKeyError, with the new spelling of a renamed one. It returns the line of each event of on.
func (a Adapter) top(root *yaml.Node) (map[string]int, error) {
	if len(root.Content) != 1 || root.Content[0].Kind != yaml.MappingNode {
		return nil, nil
	}
	var lines map[string]int
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
		if k.Value == "on" && i+1 < len(top.Content) {
			lines = nodeYAML{top.Content[i+1]}.lines()
		}
	}
	return lines, nil
}
