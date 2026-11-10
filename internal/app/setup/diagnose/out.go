package diagnose

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/mlophez/polix-ai/internal/app/rule"
	"github.com/mlophez/polix-ai/internal/app/setup"
	"github.com/mlophez/polix-ai/internal/app/state"
	"github.com/mlophez/polix-ai/internal/app/version"
	"github.com/mlophez/polix-ai/internal/app/workflow"
	"github.com/mlophez/polix-ai/internal/platform/agentconf"
	"github.com/mlophez/polix-ai/internal/platform/file"
	"github.com/mlophez/polix-ai/internal/platform/git"
	"github.com/mlophez/polix-ai/internal/platform/github"
	"github.com/mlophez/polix-ai/internal/platform/proc"
	"github.com/mlophez/polix-ai/plugins"
)

// Adapter implements Port on the real machine: disk, git, network and the CLI of Claude
// Code. The entry fills it and passes it as the port; there is no constructor.
type Adapter struct {
	OS       string        // the system the checks run on (runtime.GOOS): on windows they need an interpreter
	Look     proc.LookFunc // how a program is found on the PATH (exec.LookPath)
	Rules    rule.Load     // the loader of the rules (internal/app/rule/load), which cmd/polix builds once
	Cache    version.Load  // the loader of the cache of polix update (internal/app/version/load), built once too
	Workflow workflow.Load // the loader of polix.yaml (internal/app/workflow/load), built once too
}

// claudePluginJSON is one plugin of `claude plugin list --json`.
type claudePluginJSON struct {
	ID             string `json:"id"`
	Scope          string `json:"scope"`
	Enabled        bool   `json:"enabled"`
	Version        string `json:"version"`
	ReadFromFolder string `json:"readFromFolder"`
}

// cacheJSON is the shape of the file of the cache of polix update, with the fields of version.Cache in its order, so
// one converts to it; LoadVersion (internal/app/version/load) decodes the same shape.
type cacheJSON struct {
	CheckedAt time.Time `json:"checked_at"`
	Latest    string    `json:"latest"`
	FailedAt  time.Time `json:"failed_at,omitzero"`
}

const (
	// releasesURL is the releases page of polix; POLIX_RELEASES_URL overrides it.
	releasesURL = "https://github.com/mlophez/polix-ai/releases"
	// latestTimeout bounds the question for the latest release.
	latestTimeout = 5 * time.Second
	// cliTimeout bounds the call to the CLI of Claude Code.
	cliTimeout = time.Minute
)

// Home returns the home folder of the user, or "".
func (a Adapter) Home(_ context.Context) string {
	home, _ := os.UserHomeDir()
	return home
}

// Executable returns the real path of the running binary.
func (a Adapter) Executable(_ context.Context) (string, error) {
	return github.Executable()
}

// LoadCache loads the cache of polix update through the loader of the cache, empty when there is none or it cannot
// be read.
func (a Adapter) LoadCache(ctx context.Context) version.Cache {
	path, err := file.CacheFile("POLIX_CACHE_DIR", "polix", "update.json")
	if err != nil {
		return version.Cache{}
	}
	return a.Cache(ctx, path).Cache
}

// SaveCache writes the cache of polix update, encoded with cacheJSON.
func (a Adapter) SaveCache(_ context.Context, c version.Cache) error {
	path, err := file.CacheFile("POLIX_CACHE_DIR", "polix", "update.json")
	if err != nil {
		return err
	}
	data, err := json.Marshal(cacheJSON(c))
	if err != nil {
		return err
	}
	return file.Write(path, data)
}

// Latest asks the releases page (POLIX_RELEASES_URL, else the one of polix) for the latest release, within 5
// seconds.
func (a Adapter) Latest(ctx context.Context) (string, error) {
	releases := os.Getenv("POLIX_RELEASES_URL")
	if releases == "" {
		releases = releasesURL
	}
	ctx, cancel := context.WithTimeout(ctx, latestTimeout)
	defer cancel()
	return github.New(releases, "polix-update").Latest(ctx)
}

// LookPath finds name on the PATH, with its symlinks resolved.
func (a Adapter) LookPath(_ context.Context, name string) (string, error) {
	found, err := a.Look(name)
	if err != nil {
		return "", err
	}
	if resolved, err := filepath.EvalSymlinks(found); err == nil {
		found = resolved
	}
	return found, nil
}

// SameFile reports whether both paths exist and are the same file.
func (a Adapter) SameFile(_ context.Context, x, y string) bool {
	ix, errX := os.Stat(x)
	iy, errY := os.Stat(y)
	return x != "" && y != "" && errX == nil && errY == nil && os.SameFile(ix, iy)
}

// OpenRepo asks git for the repository of dir.
func (a Adapter) OpenRepo(ctx context.Context, dir string) (string, string, error) {
	repo, err := git.Open(ctx, dir)
	if err != nil {
		return "", "", err
	}
	return repo.Root, repo.GitDir, nil
}

// LoadWorkflow loads polix.yaml of root through the loader of polix.yaml: the workflow, whether the file exists,
// and the error of loading it, a workflow.InvalidError with the workflow when it parses.
// It asks git for the first of its base branches that exists, locally or on origin: baseErr tells why none does.
func (a Adapter) LoadWorkflow(ctx context.Context, root, running string) (wf workflow.Workflow, found bool,
	err, baseErr error) {
	loaded := a.Workflow(ctx, filepath.Join(root, workflow.File), running)
	return loaded.Workflow, loaded.Exists, loaded.Err, a.resolveBase(ctx, root, loaded.Workflow.BaseBranches())
}

// LoadRules loads the rules of the folder dir of root through the loader of the rules: every rule, the problems of
// the invalid ones, and the error of a file that cannot be read or is not yaml. For the check of each rule that has
// one it tells whether its file is missing below root and why it cannot run.
func (a Adapter) LoadRules(ctx context.Context, root, dir string) (loaded rule.Loaded, missing map[string]bool,
	runnable map[string]error) {
	loaded = a.Rules(ctx, root, dir)
	missing, runnable = map[string]bool{}, map[string]error{}
	for _, r := range loaded.Rules {
		if r.HasCheck() {
			missing[r.Check] = !file.Exists(filepath.Join(root, filepath.FromSlash(r.Check)))
			runnable[r.Check] = a.runnable(root, r.Check)
		}
	}
	return loaded, missing, runnable
}

// runnable tells why a check that exists cannot run as a program: on Windows, where a shebang does nothing, when
// its interpreter is not found; elsewhere, when it lacks the permission to execute. A missing check is not its
// business.
func (a Adapter) runnable(root, check string) error {
	info, err := os.Stat(filepath.Join(root, filepath.FromSlash(check)))
	switch {
	case err != nil:
		return nil //nolint:nilerr // a missing check is reported as missing
	case a.OS == "windows":
		_, err := proc.Interpreter(check, a.Look, proc.FileExists)
		return err
	case info.Mode()&0o111 == 0:
		return fmt.Errorf("check %s is not executable (chmod +x)", check)
	}
	return nil
}

// resolveBase asks git for the first of candidates that exists, locally or on origin.
func (a Adapter) resolveBase(ctx context.Context, root string, candidates []string) error {
	repo, err := git.Open(ctx, root)
	if err != nil {
		return err
	}
	_, err = repo.ResolveBase(ctx, candidates)
	return err
}

// WriteState creates the folder of the state and a temporary file in it, which it removes.
func (a Adapter) WriteState(_ context.Context, gitDir string) (string, error) {
	dir := filepath.Join(gitDir, state.DirName)
	err := os.MkdirAll(dir, 0o755)
	if err == nil {
		var f *os.File
		if f, err = os.CreateTemp(dir, "doctor-*"); err == nil {
			f.Close()
			os.Remove(f.Name())
		}
	}
	return dir, err
}

// HooksFiles reads the hooks files of agent of paths that exist: the hooks files of Codex with
// agentconf.ReadHooks, for which an empty one has no hooks, and else the settings of Claude Code with
// agentconf.ParseHooks, for which an empty one is not JSON.
func (a Adapter) HooksFiles(_ context.Context, agent string, paths []string) map[string]setup.HooksFile {
	if agent == setup.Codex {
		return a.hooksFiles(paths, agentconf.ReadHooks)
	}
	return a.hooksFiles(paths, agentconf.ParseHooks)
}

// CodexEnabled reads the configuration of Codex at path (agentconf.CodexEnabled); one that cannot be read enables
// nothing.
func (a Adapter) CodexEnabled(_ context.Context, path string) bool {
	data, _, err := file.Read(path)
	return err == nil && agentconf.CodexEnabled(data, setup.PluginID)
}

// Plugin reads the plugin embedded in the binary and the manifest of the one written at the path manifest.
func (a Adapter) Plugin(_ context.Context, manifest string) (embedded map[string][]byte, version string,
	written bool, err error) {
	if embedded, err = plugins.Files(); err != nil {
		return nil, "", false, err
	}
	version, written = a.readManifest(manifest)
	return embedded, version, written, nil
}

// readManifest reads the manifest of the plugin at path (agentconf.ReadManifest): its version, and whether it is
// the plugin polix.
func (a Adapter) readManifest(path string) (string, bool) {
	data, _, err := file.Read(path)
	if err != nil {
		return "", false
	}
	name, v, err := agentconf.ReadManifest(data)
	if err != nil || name != setup.Name {
		return "", false
	}
	return v, true
}

// ClaudePlugin runs `claude plugin list --json` within a minute and reads, among the plugins it prints, polix for
// the user; setup.ErrNoCLI without the CLI.
func (a Adapter) ClaudePlugin(ctx context.Context) (bool, bool, string, error) {
	if _, err := a.Look("claude"); err != nil {
		return false, false, "", setup.ErrNoCLI
	}
	out, err := proc.Stdout(ctx, "", []string{"claude", "plugin", "list", "--json"}, cliTimeout)
	if err != nil {
		return false, false, "", err
	}
	return a.claudePlugin([]byte(out))
}

// claudePlugin reads what `claude plugin list --json` printed: whether the plugin polix is installed for the user,
// whether it is enabled, and the folder Claude Code reads it from.
func (a Adapter) claudePlugin(list []byte) (bool, bool, string, error) {
	var all []claudePluginJSON
	if err := json.Unmarshal(list, &all); err != nil {
		return false, false, "", fmt.Errorf("claude plugin list: %w", err)
	}
	for _, p := range all {
		if p.ID == setup.PluginID && p.Scope == "user" {
			return true, p.Enabled, p.ReadFromFolder, nil
		}
	}
	return false, false, "", nil
}

// hooksFiles reads the hooks files of paths that exist and decodes them with decode: the groups of hooks that run
// a command, or why the file cannot be read or decoded.
func (a Adapter) hooksFiles(paths []string, decode func([]byte) (map[string][]agentconf.Group, error)) map[string]setup.HooksFile {
	out := map[string]setup.HooksFile{}
	for _, p := range paths {
		data, existed, readErr := file.Read(p)
		if readErr == nil && !existed {
			continue
		}
		var hooks map[string][]setup.HookGroup
		var err error
		if readErr == nil {
			var groups map[string][]agentconf.Group
			groups, err = decode(data)
			hooks = a.hookGroups(groups)
		}
		f := setup.HooksFile{Hooks: hooks, ReadErr: readErr, Err: err}
		if f.Valid() == nil {
			out[p] = f
		}
	}
	return out
}

// hookGroups returns the groups of hooks of a file, by event, as the domain reads them: a handler without a
// command (a hook of another type, such as a prompt) and a group left without handlers are left out.
func (a Adapter) hookGroups(groups map[string][]agentconf.Group) map[string][]setup.HookGroup {
	out := map[string][]setup.HookGroup{}
	for ev, gs := range groups {
		for _, g := range gs {
			var handlers []setup.HookHandler
			for _, h := range g.Handlers {
				handler := setup.HookHandler{Command: h.Command, Timeout: h.Timeout, Status: h.Status}
				if handler.Valid() == nil {
					handlers = append(handlers, handler)
				}
			}
			group := setup.HookGroup{Matcher: g.Matcher, Handlers: handlers}
			if group.Valid() == nil {
				out[ev] = append(out[ev], group)
			}
		}
	}
	return out
}
