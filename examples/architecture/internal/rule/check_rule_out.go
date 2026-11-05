package rule

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"time"

	"github.com/mlophez/polix-ai/internal/workflow"
	"github.com/mlophez/polix-ai/pkg/git"
)

// CheckRuleAdapter implements CheckRulePort on the real disk, git and processes of a repository. The entry fills
// it and passes it as the port; there is no constructor.
type CheckRuleAdapter struct {
	Repo *git.Repo // the repository whose rules are checked
}

const (
	// checkRuleTimeout bounds one check script, within the deadline of the caller.
	checkRuleTimeout = 5 * time.Minute
	// checkRuleWaitDelay bounds the wait for the output of a command killed by its deadline, held open by a
	// grandchild that outlives it.
	checkRuleWaitDelay = 5 * time.Second
)

// LoadRules reads the rules from the files of ai/rules/.
func (a CheckRuleAdapter) LoadRules(_ context.Context) ([]Rule, error) {
	root := a.Repo.Root
	// The folder is listed, not globbed: the root may hold [, ], * or ?, which a glob would read as a pattern.
	entries, err := os.ReadDir(filepath.Join(root, Dir))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var all []Rule
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".yaml" || e.Name() == RetiredFile {
			continue
		}
		data, err := os.ReadFile(filepath.Join(root, Dir, e.Name()))
		if err != nil {
			return nil, err
		}
		rules, err := Parse(Dir+"/"+e.Name(), data)
		if err != nil {
			return nil, err
		}
		all = append(all, rules...)
	}
	sort.SliceStable(all, func(i, j int) bool { return all[i].ID < all[j].ID })
	return all, nil
}

// BaseBranches reads the base branches of polix.yaml with the domain of workflow, as a use case of workflow would
// break the vertical slices. Without polix.yaml they are those of workflow by default; a polix.yaml that cannot
// be read or parsed is an error, never a silent default.
func (a CheckRuleAdapter) BaseBranches(_ context.Context) ([]string, error) {
	data, err := os.ReadFile(filepath.Join(a.Repo.Root, workflow.File))
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, err
	}
	wf, err := workflow.Parse(data)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", workflow.File, err)
	}
	return wf.BaseBranches(), nil
}

// Changed asks git for the files changed against the first of candidates that exists.
func (a CheckRuleAdapter) Changed(ctx context.Context, candidates []string) ([]string, error) {
	ref, err := a.Repo.ResolveBase(ctx, candidates)
	if err != nil {
		return nil, err
	}
	return a.Repo.Changed(ctx, ref)
}

// RunScript runs a check script as a process. On Windows a shebang does nothing, so scripts are started through
// their interpreter by extension; elsewhere they run directly, as their shebang says.
func (a CheckRuleAdapter) RunScript(ctx context.Context, check string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, checkRuleTimeout)
	defer cancel()
	script := filepath.Join(a.Repo.Root, filepath.FromSlash(check))
	argv := []string{script}
	if runtime.GOOS == "windows" {
		interp, err := a.interpreter(check)
		if err != nil {
			return "", err
		}
		argv = append(interp, script)
	}
	cmd := exec.CommandContext(ctx, argv[0], argv[1:]...)
	cmd.Dir = a.Repo.Root
	cmd.WaitDelay = checkRuleWaitDelay
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	err := cmd.Run()
	return out.String(), err
}

// interpreter returns the interpreter that runs the script check on Windows. Interpreter, fileExists, winDir,
// winExt, normWin, inWindowsDir and isStoreAlias of internal/rules/rules.go become unexported methods of the
// adapter, since the out file holds only the adapter, or move to pkg/ when hook needs them too (open decision 5).
func (a CheckRuleAdapter) interpreter(check string) ([]string, error)
