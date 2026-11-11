package format

import (
	"context"
	"errors"
	"path/filepath"

	"github.com/mlophez/polix-ai/internal/app/rule"
	"github.com/mlophez/polix-ai/internal/app/workflow"
	"github.com/mlophez/polix-ai/internal/platform/file"
)

// Adapter implements Port on the disk of a repository. The entry fills it and passes it as the port; there is no
// constructor.
type Adapter struct {
	Root     string        // the root of the repository, with the separators of the system
	Rules    rule.Load     // the loader of the rules (internal/app/rule/load), which cmd/polix builds once
	Workflow workflow.Load // the loader of polix.yaml (internal/app/workflow/load), which cmd/polix builds once too
}

// LoadWorkflow loads polix.yaml of the repository through the loader of polix.yaml. Without polix.yaml it is the
// default workflow; a polix.yaml that cannot be read, cannot be parsed or requires a newer polix is an error, never
// a silent default. The problems of a workflow that parses (Invalid) are not errors here: the shell checks only its
// rules.path, and polix config check reports the rest.
func (a Adapter) LoadWorkflow(ctx context.Context, running string) (workflow.Workflow, error) {
	loaded := a.Workflow(ctx, filepath.Join(a.Root, workflow.File), running)
	if _, invalid := errors.AsType[workflow.InvalidError](loaded.Err); loaded.Err != nil && !invalid {
		return workflow.Workflow{}, loaded.Err
	}
	return loaded.Workflow, nil
}

// ReadRuleFiles reads the yaml files of the folder dir through internal/platform/file, by name, retired.yaml
// included and subfolders left out; a missing folder has no files and no error.
func (a Adapter) ReadRuleFiles(_ context.Context, dir string) (map[string][]byte, error) {
	files, err := file.ReadDir(a.folder(dir), ".yaml")
	if err != nil || len(files) == 0 {
		return nil, err
	}
	return files, nil
}

// WriteRuleFile writes the file name of the folder dir through internal/platform/file, keeping its mode and a
// symlink to it.
func (a Adapter) WriteRuleFile(_ context.Context, dir, name string, data []byte) error {
	return file.Write(filepath.Join(a.folder(dir), name), data)
}

// RepeatedIDs returns the ids repeated among the rules of the folder dir through the loader of the rules
// (rule.Loaded.Repeated), retired ones included. When the loader stops at Err, a file that cannot be read, is not
// yaml or does not decode as rules, it returns that error instead: the rules after it are not read, so no repeat can
// be ruled out.
func (a Adapter) RepeatedIDs(ctx context.Context, dir string) []string {
	loaded := a.Rules(ctx, a.Root, dir)
	if loaded.Err != nil {
		return []string{loaded.Err.Error()}
	}
	return loaded.Repeated
}

// folder is the path of the folder dir, relative to the root with slashes, on the disk.
func (a Adapter) folder(dir string) string {
	return filepath.Join(a.Root, filepath.FromSlash(dir))
}
