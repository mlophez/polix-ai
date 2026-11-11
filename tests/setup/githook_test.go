package setup

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"github.com/mlophez/polix-ai/internal/platform/testenv"
)

// githooksWorkflow blocks a commit that stages a file of secrets/, and on a ticket branch a message that does not
// start with the key of the ticket.
const githooksWorkflow = `version: 1
vars:
  key: { branch: "^([A-Z][A-Z0-9]+-[0-9]+)" }
githooks:
  pre_commit:
    - when: { changed: "secrets/**" }
      action: { block: "No se commitean secretos" }
  commit_msg:
    - when: { branch: "^[A-Z]+-[0-9]+" }
      unless: { message: "^{key}: " }
      action: { block: "El mensaje empieza por la clave {key}" }
`

// AC16: with the hooks of git installed, a real commit runs polix githook on pre-commit and commit-msg: a commit
// of a staged file the githooks block fails with the reason, one with a wrong message too, and one that meets both
// is created. It runs the polix built from this tree on the PATH of the test, and skips, naming the version of git,
// when git does not run the hooks of its configuration.
func TestGithookCommit(t *testing.T) {
	bin := buildPolix(t)
	testenv.Machine(t, false, false)
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_SYSTEM", os.DevNull)
	r := testenv.GitRepo(t)
	if code, stdout, stderr := runSetup("install", "--yes", "--agent", "claude", "--dir", r); code != 0 ||
		!strings.Contains(stdout, "git config hook.polix-commit-msg") {
		t.Fatalf("install: %d\n%s%s", code, stdout, stderr)
	}
	if out, _ := gitOut(r, "hook", "list", "commit-msg"); !strings.Contains(out, "polix-commit-msg") {
		version, _ := gitOut(r, "--version")
		t.Skipf("%s does not run the hooks of its configuration (git hook list: %q)", strings.TrimSpace(version), out)
	}
	testenv.Write(t, filepath.Join(r, "polix.yaml"), githooksWorkflow)
	mustGit(t, r, "checkout", "--quiet", "-b", "PROJ-1-login")

	testenv.Write(t, filepath.Join(r, "secrets", "token.txt"), "x")
	mustGit(t, r, "add", "secrets/token.txt")
	out, err := gitOut(r, "commit", "-m", "PROJ-1: con secretos")
	if err == nil || !strings.Contains(out, "polix: No se commitean secretos") {
		t.Fatalf("a staged secret: %v\n%s", err, out)
	}
	mustGit(t, r, "rm", "--quiet", "--cached", "secrets/token.txt")

	testenv.Write(t, filepath.Join(r, "src", "login.go"), "package src")
	mustGit(t, r, "add", "src/login.go")
	out, err = gitOut(r, "commit", "-m", "arregla el login")
	if err == nil || !strings.Contains(out, "polix: El mensaje empieza por la clave PROJ-1") {
		t.Fatalf("a message without the key: %v\n%s", err, out)
	}
	if out, err := gitOut(r, "commit", "-m", "PROJ-1: arregla el login"); err != nil {
		t.Fatalf("a commit that meets both: %v\n%s", err, out)
	}
	if subject, _ := gitOut(r, "log", "-1", "--format=%s"); strings.TrimSpace(subject) != "PROJ-1: arregla el login" {
		t.Fatalf("last commit: %q", subject)
	}
}

// AC18, AC20: polix install --githooks writes the four keys of the hooks of git into the local configuration of
// the repository and nothing else, no plugin nor settings of the agents, printing only what changed, and a second
// run prints nothing; with --agent or outside a repository it fails and writes nothing. It skips,
// naming the version of git, when git does not run the hooks of its configuration, where the install exits 1.
func TestGithookInstall(t *testing.T) {
	testenv.Machine(t, true, false)
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_SYSTEM", os.DevNull)
	r := testenv.GitRepo(t)
	for _, args := range [][]string{{"--agent", "claude"}, {"--dir", t.TempDir()}} {
		code, stdout, stderr := runSetup(append([]string{"install", "--githooks", "--dir", r}, args...)...)
		if code != 1 || stderr == "" || stdout != "" {
			t.Fatalf("install --githooks %v: %d\n%s%s", args, code, stdout, stderr)
		}
	}
	if keys := polixKeys(t, r); keys != "" {
		t.Fatalf("a refused install wrote %q", keys)
	}

	code, stdout, stderr := runSetup("install", "--githooks", "--dir", r)
	want := "hook.polix-pre-commit.event=pre-commit\nhook.polix-pre-commit.command=polix githook pre-commit\n" +
		"hook.polix-commit-msg.event=commit-msg\nhook.polix-commit-msg.command=polix githook commit-msg"
	if keys := polixKeys(t, r); keys != want {
		t.Fatalf("keys of git: %q", keys)
	}
	if lines := strings.Split(strings.TrimSpace(stdout), "\n"); len(lines) != 2 ||
		!strings.Contains(lines[0], "git config hook.polix-pre-commit") || !strings.Contains(lines[1], "git config hook.polix-commit-msg") {
		t.Fatalf("only the changes: %q", stdout)
	}
	for _, p := range []string{filepath.Join(r, ".claude"), filepath.Join(os.Getenv("HOME"), ".polix")} {
		if _, err := os.Stat(p); err == nil {
			t.Fatalf("%s written", p)
		}
	}
	if calls := testenv.Calls(os.Getenv("FAKE_STATE")); len(calls) != 1 || calls[0] != "" {
		t.Fatalf("the CLI of the agent was run: %q", calls)
	}
	if out, _ := gitOut(r, "hook", "list", "commit-msg"); !strings.Contains(out, "polix-commit-msg") {
		version, _ := gitOut(r, "--version")
		t.Skipf("%s does not run the hooks of its configuration (git hook list: %q)", strings.TrimSpace(version), out)
	}
	if code != 0 || stderr != "" {
		t.Fatalf("install --githooks: %d\n%s%s", code, stdout, stderr)
	}
	if code, stdout, stderr := runSetup("install", "--githooks", "--yes", "--quiet", "--dir", r); code != 0 ||
		stdout != "" || stderr != "" {
		t.Fatalf("second install --githooks: %d\n%s%s", code, stdout, stderr)
	}
}

// A hook of git a person disabled with hook.<name>.enabled=false is listed by git as disabled: polix install
// --githooks warns that it is disabled in the git configuration and exits 0, without blaming the version of git nor
// enabling it, and polix doctor says so instead of asking for polix install. It skips, naming the version of git,
// when git does not run the hooks of its configuration.
func TestGithookDisabled(t *testing.T) {
	testenv.Machine(t, false, false)
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_SYSTEM", os.DevNull)
	r := testenv.GitRepo(t)
	if code, stdout, stderr := runSetup("install", "--githooks", "--dir", r); code != 0 && code != 1 {
		t.Fatalf("install --githooks: %d\n%s%s", code, stdout, stderr)
	}
	if out, _ := gitOut(r, "hook", "list", "pre-commit"); !strings.Contains(out, "polix-pre-commit") {
		version, _ := gitOut(r, "--version")
		t.Skipf("%s does not run the hooks of its configuration (git hook list: %q)", strings.TrimSpace(version), out)
	}
	mustGit(t, r, "config", "--local", "hook.polix-pre-commit.enabled", "false")
	if out, _ := gitOut(r, "hook", "list", "pre-commit"); !strings.Contains(out, "disabled\tpolix-pre-commit") {
		version, _ := gitOut(r, "--version")
		t.Skipf("%s does not list a disabled hook as disabled (git hook list: %q)", strings.TrimSpace(version), out)
	}
	code, stdout, stderr := runSetup("install", "--githooks", "--dir", r)
	if code != 0 || stdout != "" || !strings.Contains(stderr, "hook.polix-pre-commit.enabled") ||
		!strings.Contains(stderr, "disabled in the git configuration") || strings.Contains(stderr, "2.54") {
		t.Fatalf("install --githooks of a disabled hook: %d\n%s%s", code, stdout, stderr)
	}
	if out, _ := gitOut(r, "config", "--local", "hook.polix-pre-commit.enabled"); strings.TrimSpace(out) != "false" {
		t.Fatalf("the install enabled the hook: %q", out)
	}
	_, stdout, _ = runSetup("doctor", "--offline", "--dir", r)
	var githooks []string
	for line := range strings.SplitSeq(stdout, "\n") {
		if strings.Contains(line, "githooks") {
			githooks = append(githooks, strings.TrimSpace(line))
		}
	}
	if len(githooks) != 1 || !strings.Contains(githooks[0],
		"git does not run polix githook on pre-commit: hook.polix-pre-commit is disabled in the git configuration") {
		t.Fatalf("doctor of a disabled hook:\n%s", stdout)
	}
}

// AC23: with a githooks entry and no start entry written for it, a session start of Claude Code runs the entry
// githooks expand into, polix install --githooks from the polix on the PATH, which writes the hooks of git into the
// local configuration of a repository that had none. The keys are written whatever the version of git; that the run
// passes, and so tells the agent nothing, is skipped, naming the version, when git does not run the hooks of its
// configuration.
func TestGithookSessionStart(t *testing.T) {
	bin := buildPolix(t)
	testenv.Machine(t, false, false)
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_SYSTEM", os.DevNull)
	r := testenv.GitRepo(t)
	testenv.Write(t, filepath.Join(r, "polix.yaml"), githooksWorkflow)
	if keys := polixKeys(t, r); keys != "" {
		t.Fatalf("hooks of git before the session: %q", keys)
	}
	cmd := exec.Command(filepath.Join(bin, "polix"), "hook", "claude")
	cmd.Dir = r
	cmd.Stdin = strings.NewReader(`{"hook_event_name":"SessionStart","source":"startup","session_id":"s","cwd":` +
		strconv.Quote(r) + `}`)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("polix hook claude: %v\n%s", err, out)
	}
	want := "hook.polix-pre-commit.event=pre-commit\nhook.polix-pre-commit.command=polix githook pre-commit\n" +
		"hook.polix-commit-msg.event=commit-msg\nhook.polix-commit-msg.command=polix githook commit-msg"
	if keys := polixKeys(t, r); keys != want {
		t.Fatalf("keys of git after the session start: %q\n%s", keys, out)
	}
	if list, _ := gitOut(r, "hook", "list", "commit-msg"); !strings.Contains(list, "polix-commit-msg") {
		version, _ := gitOut(r, "--version")
		t.Skipf("%s does not run the hooks of its configuration (git hook list: %q)", strings.TrimSpace(version), list)
	}
	if strings.Contains(string(out), "hooks of git of githooks are not installed") {
		t.Fatalf("the install of the hooks of git failed:\n%s", out)
	}
}

// polixKeys returns the keys of the hooks of polix in the local git configuration of the repository r, one per line.
func polixKeys(t *testing.T, r string) string {
	t.Helper()
	out, _ := gitOut(r, "config", "--local", "--list")
	var keys []string
	for line := range strings.SplitSeq(out, "\n") {
		if strings.HasPrefix(line, "hook.polix-") {
			keys = append(keys, line)
		}
	}
	return strings.Join(keys, "\n")
}

// buildPolix builds polix from this tree into a folder of the test and returns the folder, before the test changes
// HOME, so the build cache of the user is used.
func buildPolix(t *testing.T) string {
	t.Helper()
	gotool, err := exec.LookPath("go")
	if err != nil {
		t.Skip("the go tool is not on the PATH")
	}
	bin := t.TempDir()
	name := "polix"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	cmd := exec.Command(gotool, "build", "-o", filepath.Join(bin, name), "./cmd/polix")
	cmd.Dir = filepath.Join("..", "..")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("go build: %v\n%s", err, out)
	}
	return bin
}

// gitOut runs git in dir as a person would, with an author, and returns its output, stdout and stderr together.
func gitOut(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", append([]string{"-c", "user.name=t", "-c", "user.email=t@x"}, args...)...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// mustGit runs git in dir, failing the test on an error.
func mustGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	if out, err := gitOut(dir, args...); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}
