package install

import (
	"bytes"
	"context"
	"errors"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/mlophez/polix-ai/internal/app/setup"
	"github.com/mlophez/polix-ai/internal/platform/testenv"
)

// fakePort answers the port of InstallSetup from memory and records the calls.
type fakePort struct {
	calls     []string
	pluginErr error // of RegisterPlugin
	filesErr  error // of MarketplaceFiles
	refuse    bool  // Confirm refuses
	outside   bool  // Root finds no repository
	// gitHooks records the hooks of git written and listed, apart from calls; gitChanges makes each write a change,
	// unlisted makes git list none of them, and disabled makes git list the first one as disabled.
	gitHooks   []string
	gitChanges bool
	unlisted   bool
	disabled   bool
	// methods records the name of every method of the port called, in order.
	methods []string
}

func (f *fakePort) RegisterGitHook(_ context.Context, root string, h setup.GitHook) ([]setup.Change, error) {
	f.methods = append(f.methods, "RegisterGitHook")
	f.gitHooks = append(f.gitHooks, "RegisterGitHook "+root+" "+string(h))
	if !f.gitChanges {
		return nil, nil
	}
	return []setup.Change{{Path: h.Key(), Action: "created"}}, nil
}

func (f *fakePort) Unlisted(_ context.Context, root string, hooks []setup.GitHook) ([]setup.GitHook, []setup.GitHook) {
	f.methods = append(f.methods, "Unlisted")
	f.gitHooks = append(f.gitHooks, "Unlisted "+root)
	switch {
	case f.unlisted:
		return hooks, nil
	case f.disabled:
		return nil, hooks[:1]
	}
	return nil, nil
}

func (f *fakePort) Confirm(_ context.Context, _ string) error {
	f.methods = append(f.methods, "Confirm")
	if f.refuse {
		return setup.ErrCancelled
	}
	return nil
}

func (f *fakePort) Home(_ context.Context) (string, error) {
	f.methods = append(f.methods, "Home")
	return "/h", nil
}
func (f *fakePort) Root(_ context.Context, _ string) string {
	f.methods = append(f.methods, "Root")
	if f.outside {
		return ""
	}
	return "/r"
}
func (f *fakePort) Plugin(_ context.Context) (map[string][]byte, error) {
	f.methods = append(f.methods, "Plugin")
	return map[string][]byte{"polix/.claude-plugin/plugin.json": []byte(`{"name": "polix", "version": "0"}`)}, nil
}
func (f *fakePort) MarketplaceFiles(_ context.Context, _ setup.Plugin) (map[string][]byte, error) {
	f.methods = append(f.methods, "MarketplaceFiles")
	return map[string][]byte{".claude-plugin/marketplace.json": []byte("{}")}, f.filesErr
}
func (f *fakePort) WritePlugin(_ context.Context, dir string, _ map[string][]byte) ([]setup.Change, error) {
	f.methods = append(f.methods, "WritePlugin")
	f.calls = append(f.calls, "WritePlugin "+dir)
	return []setup.Change{{Path: dir, Action: "created"}}, nil
}
func (f *fakePort) RegisterPlugin(_ context.Context, _ setup.Setup, agent string) ([]setup.Change, error) {
	f.methods = append(f.methods, "RegisterPlugin")
	f.calls = append(f.calls, "RegisterPlugin "+agent)
	return nil, f.pluginErr
}
func (f *fakePort) RegisterHooks(_ context.Context, file string, _ map[string][]setup.HookGroup) ([]setup.Change, error) {
	f.methods = append(f.methods, "RegisterHooks")
	f.calls = append(f.calls, "RegisterHooks "+file)
	return nil, nil
}
func (f *fakePort) Registered(_ context.Context, _ []string) map[string]bool {
	f.methods = append(f.methods, "Registered")
	return nil
}
func (f *fakePort) OnPath(_ context.Context, _ string) error {
	f.methods = append(f.methods, "OnPath")
	return errors.New("no")
}

func TestShell(t *testing.T) {
	port := &fakePort{}
	res, err := run(context.Background(), port, Request{Version: "1.2.3"})
	if err != nil || len(res.Changes) != 1 || !res.NotOnPath || len(res.HooksFiles) != 1 || len(port.calls) != 3 ||
		!strings.HasPrefix(port.calls[2], "RegisterHooks") {
		t.Fatalf("%+v %v %v", res, err, port.calls)
	}
	// The hooks are registered also when the plugin fails, and the error comes back with the changes.
	port = &fakePort{pluginErr: errors.New("refused")}
	res, err = run(context.Background(), port, Request{Version: "1.2.3"})
	if err == nil || len(res.Changes) != 1 || len(port.calls) != 3 {
		t.Fatalf("%+v %v %v", res, err, port.calls)
	}
	// A person that does not confirm stops it before anything is written.
	port = &fakePort{refuse: true}
	_, err = run(context.Background(), port, Request{Version: "1.2.3", Parts: setup.Parts{Skills: true}})
	if !errors.Is(err, setup.ErrCancelled) || len(port.calls) != 0 {
		t.Fatalf("%v %v", err, port.calls)
	}
	// A plugin whose marketplace cannot be written stops it before asking.
	port = &fakePort{filesErr: errors.New("no version"), refuse: true}
	if _, err = run(context.Background(), port, Request{Version: "1.2.3"}); err == nil || err.Error() != "no version" ||
		len(port.calls) != 0 {
		t.Fatalf("%v %v", err, port.calls)
	}
}

// AC11, AC12, AC13: inside a repository the shell writes both hooks of git after the hooks of the agents, reports
// their changes and warns when git lists none of them; with --skills or outside a repository it touches no git
// configuration.
func TestShellGitHooks(t *testing.T) {
	ctx := context.Background()
	port := &fakePort{gitChanges: true}
	res, err := run(ctx, port, Request{Version: "1.2.3"})
	want := []string{"RegisterGitHook /r pre-commit", "RegisterGitHook /r commit-msg", "Unlisted /r"}
	if err != nil || !slices.Equal(port.gitHooks, want) || res.Unlisted != nil ||
		!slices.Contains(res.Changes, setup.Change{Path: "git config hook.polix-pre-commit", Action: "created"}) ||
		!slices.Contains(res.Changes, setup.Change{Path: "git config hook.polix-commit-msg", Action: "created"}) {
		t.Fatalf("%+v %v %q", res, err, port.gitHooks)
	}
	var stdout, stderr bytes.Buffer
	res.print(&stdout, &stderr)
	if strings.Contains(stderr.String(), "git") {
		t.Fatalf("a warning without reason: %s", stderr.String())
	}

	port = &fakePort{unlisted: true}
	res, err = run(ctx, port, Request{Version: "1.2.3", Quiet: true})
	if err != nil || !slices.Equal(res.Unlisted, setup.GitHooks) {
		t.Fatalf("unlisted: %+v %v", res, err)
	}
	stdout.Reset()
	res.print(&stdout, &stderr)
	if !strings.Contains(stderr.String(), "git does not run hooks from its configuration") ||
		!strings.Contains(stderr.String(), "githooks of polix.yaml do not apply until git is updated") {
		t.Fatalf("no warning: %q", stderr.String())
	}

	for name, req := range map[string]Request{"--skills": {Version: "1.2.3", Parts: setup.Parts{Skills: true}}, "outside": {Version: "1.2.3"}} {
		port = &fakePort{outside: name == "outside"}
		if res, err := run(ctx, port, req); err != nil || port.gitHooks != nil || res.Unlisted != nil {
			t.Fatalf("%s: %+v %v %q", name, res, err, port.gitHooks)
		}
	}
}

// AC18, AC19: with --githooks the shell reads only the repository, writes each hook of git and asks which git does
// not list, never asks for confirmation nor touches the plugin or the hooks of the agents; the command prints only
// the changes, nothing when there are none, and exits 1 with the warning of AC12 when git does not list them.
func TestShellGitHooksOnly(t *testing.T) {
	ctx := context.Background()
	want := []string{"Root", "RegisterGitHook", "RegisterGitHook", "Unlisted"}
	port := &fakePort{gitChanges: true, refuse: true}
	res, err := run(ctx, port, Request{Version: "1.2.3", Parts: setup.Parts{GitHooks: true}})
	if err != nil || !slices.Equal(port.methods, want) || !res.Parts.OnlyGitHooks() || len(res.Changes) != 2 || res.Unlisted != nil {
		t.Fatalf("%+v %v %q", res, err, port.methods)
	}
	var stdout, stderr bytes.Buffer
	res.printChanges(&stdout)
	res.print(&stdout, &stderr)
	lines := strings.Split(strings.TrimSpace(stdout.String()), "\n")
	if len(lines) != 2 || !strings.Contains(lines[0], "git config hook.polix-pre-commit") ||
		!strings.Contains(lines[1], "git config hook.polix-commit-msg") || stderr.Len() != 0 || res.exit() != nil {
		t.Fatalf("only the changes, exit 0: %q %q %v", stdout.String(), stderr.String(), res.exit())
	}

	port = &fakePort{}
	res, err = run(ctx, port, Request{Version: "1.2.3", Parts: setup.Parts{GitHooks: true}})
	stdout.Reset()
	res.printChanges(&stdout)
	res.print(&stdout, &stderr)
	if err != nil || !slices.Equal(port.methods, want) || stdout.Len() != 0 || stderr.Len() != 0 || res.exit() != nil {
		t.Fatalf("nothing changed, nothing printed: %v %q %q %q", err, port.methods, stdout.String(), stderr.String())
	}

	port = &fakePort{unlisted: true}
	res, err = run(ctx, port, Request{Version: "1.2.3", Parts: setup.Parts{GitHooks: true}})
	res.print(&stdout, &stderr)
	if err != nil || !slices.Equal(res.Unlisted, setup.GitHooks) || res.exit() == nil ||
		!strings.Contains(stderr.String(), "git does not run hooks from its configuration") {
		t.Fatalf("unlisted: %+v %v %q %v", res, err, stderr.String(), res.exit())
	}

	// polix install without --githooks exits 0 with unlisted hooks.
	port = &fakePort{unlisted: true}
	if res, err := run(ctx, port, Request{Version: "1.2.3"}); err != nil || res.Unlisted == nil || res.exit() != nil {
		t.Fatalf("without --githooks: %+v %v", res, err)
	}
}

// A hook of git that git lists as disabled (hook.<name>.enabled=false) is not unlisted: install, with or without
// --githooks, warns that it is disabled in the git configuration, neither blaming the version of git nor exiting 1,
// since writing the hooks again does not enable it.
func TestShellGitHooksDisabled(t *testing.T) {
	ctx := context.Background()
	for _, req := range []Request{{Version: "1.2.3", Parts: setup.Parts{GitHooks: true}}, {Version: "1.2.3", Quiet: true}} {
		port := &fakePort{disabled: true}
		res, err := run(ctx, port, req)
		var stdout, stderr bytes.Buffer
		res.print(&stdout, &stderr)
		warn := stderr.String()
		if err != nil || res.Unlisted != nil || !slices.Equal(res.Disabled, setup.GitHooks[:1]) || res.exit() != nil ||
			!strings.Contains(warn, "disabled in the git configuration") ||
			!strings.Contains(warn, "hook.polix-pre-commit.enabled") || strings.Contains(warn, "polix-commit-msg") ||
			strings.Contains(warn, "2.54") {
			t.Fatalf("githooks %v: %+v %v %q %v", req.Parts.GitHooks, res, err, warn, res.exit())
		}
	}
}

// AC20: --githooks, and --hooks, outside a repository is an error that writes nothing.
func TestShellOutside(t *testing.T) {
	for _, p := range []setup.Parts{{GitHooks: true}, {Hooks: true}, {Skills: true, GitHooks: true}} {
		port := &fakePort{outside: true}
		_, err := run(context.Background(), port, Request{Version: "1.2.3", Parts: p})
		if err == nil || !strings.Contains(err.Error(), "not inside a git repository") || port.gitHooks != nil ||
			port.calls != nil {
			t.Fatalf("%+v: %v %q %q", p, err, port.gitHooks, port.calls)
		}
	}
}

// --skills writes only the plugin and registers it, without the hooks of the agents nor of git; --hooks registers
// only the hooks of the agents, without reading nor writing the plugin, after a confirmation.
func TestShellParts(t *testing.T) {
	ctx := context.Background()
	port := &fakePort{}
	res, err := run(ctx, port, Request{Version: "1.2.3", Parts: setup.Parts{Skills: true}})
	if err != nil || !slices.Equal(port.calls, []string{"WritePlugin " + setup.Setup{Home: "/h"}.MarketplaceDir(), "RegisterPlugin claude"}) ||
		port.gitHooks != nil || slices.Contains(port.methods, "Root") || slices.Contains(port.methods, "Registered") {
		t.Fatalf("--skills: %+v %v %q %q", res, err, port.calls, port.methods)
	}
	port = &fakePort{}
	res, err = run(ctx, port, Request{Version: "1.2.3", Parts: setup.Parts{Hooks: true}})
	want := []string{"Home", "Root", "Confirm", "RegisterHooks", "OnPath", "Registered"}
	if err != nil || !slices.Equal(port.methods, want) || port.gitHooks != nil || len(res.HooksFiles) != 1 {
		t.Fatalf("--hooks: %+v %v %q", res, err, port.methods)
	}
	var stdout, stderr bytes.Buffer
	res.print(&stdout, &stderr)
	if strings.Contains(stdout.String(), "skills") || !strings.Contains(stdout.String(), "Hooks registered in") {
		t.Fatalf("--hooks prints the hooks, not the skills:\n%s", stdout.String())
	}
	// --hooks --githooks goes through the confirmation, without the plugin, and exits 0 when git does not list
	// the hooks of git, as the whole install.
	port = &fakePort{unlisted: true}
	res, err = run(ctx, port, Request{Version: "1.2.3", Parts: setup.Parts{Hooks: true, GitHooks: true}})
	want = []string{"Home", "Root", "Confirm", "RegisterHooks", "RegisterGitHook", "RegisterGitHook", "Unlisted", "OnPath", "Registered"}
	if err != nil || !slices.Equal(port.methods, want) || !slices.Equal(res.Unlisted, setup.GitHooks) || res.exit() != nil {
		t.Fatalf("--hooks --githooks: %+v %v %q", res, err, port.methods)
	}
}

// The command rejects --githooks alone with --agent before running anything, and takes --user, hidden, as
// --skills, which polix update of 0.6 and earlier runs.
func TestCommandFlags(t *testing.T) {
	testenv.Home(t)
	dir := t.TempDir()
	c := Command()
	c.SilenceUsage, c.SilenceErrors = true, true
	var stdout, stderr bytes.Buffer
	c.SetOut(&stdout)
	c.SetErr(&stderr)
	c.SetArgs([]string{"--githooks", "--agent", "claude", "--yes", "--dir", dir})
	if err := c.Execute(); err == nil || !strings.Contains(err.Error(), "it takes no --agent") || stdout.Len() != 0 {
		t.Fatalf("%v %q", err, stdout.String())
	}
	if entries, err := os.ReadDir(os.Getenv("HOME")); err != nil || len(entries) != 0 {
		t.Fatalf("the home stays empty: %v %v", entries, err)
	}
	c = Command()
	if f := c.Flags().Lookup("user"); f == nil || !f.Hidden {
		t.Fatal("--user is a hidden flag")
	}
	if err := c.ParseFlags([]string{"--user"}); err != nil || c.Flags().Lookup("skills").Value.String() != "true" {
		t.Fatalf("--user: %v", err)
	}
}

// Without the running version there is no setup: nothing is asked nor written.
func TestShellWithoutVersion(t *testing.T) {
	port := &fakePort{}
	if _, err := run(context.Background(), port, Request{}); err == nil || len(port.calls) != 0 {
		t.Fatalf("%v %v", err, port.calls)
	}
}

func TestParseAgents(t *testing.T) {
	req := Request{Version: "1"}
	if err := req.parseAgents([]string{" Claude", "codex", "claude"}); err != nil ||
		!slices.Equal(req.Agents, []string{setup.Claude, setup.Codex}) {
		t.Fatalf("%v %v", req.Agents, err)
	}
	req = Request{Version: "1"}
	if err := req.parseAgents([]string{"cursor"}); err == nil || err.Error() != `unknown agent "cursor" (claude, codex)` {
		t.Fatalf("unknown agent: %v", err)
	}
}

// Codex is set up only when asked, and then the output says its support is not implemented.
func TestShellCodex(t *testing.T) {
	const notImplemented = "Codex support is not implemented in this release and the setup is untested"
	port := &fakePort{}
	res, err := run(context.Background(), port, Request{Version: "1.2.3"})
	var stdout, stderr bytes.Buffer
	res.print(&stdout, &stderr)
	if err != nil || slices.Contains(res.Agents, setup.Codex) || strings.Contains(stdout.String(), notImplemented) {
		t.Fatalf("a default install leaves Codex out: %+v %v\n%s", res, err, stdout.String())
	}
	port = &fakePort{}
	res, err = run(context.Background(), port, Request{Version: "1.2.3", Agents: []string{setup.Codex}})
	stdout.Reset()
	res.print(&stdout, &stderr)
	if err != nil || !slices.Equal(res.Agents, []string{setup.Codex}) || !slices.Contains(port.calls, "RegisterPlugin "+setup.Codex) ||
		!strings.Contains(stdout.String(), notImplemented) {
		t.Fatalf("--agent codex sets up Codex and says it is not implemented: %+v %v %v\n%s", res, err, port.calls, stdout.String())
	}
}
