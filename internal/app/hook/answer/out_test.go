package answer

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/mlophez/polix-ai/internal/app/diff"
	"github.com/mlophez/polix-ai/internal/app/hook"
	"github.com/mlophez/polix-ai/internal/app/state"
	"github.com/mlophez/polix-ai/internal/platform/build"
	"github.com/mlophez/polix-ai/internal/platform/cli"
)

// These tests run the command polix hook end to end, adapter included, against a temporary repository.

// repo is a throwaway git repository for one test.
type repo struct {
	t   *testing.T
	dir string
}

// newRepo creates a repository with one commit on main and switches to branch.
func newRepo(t *testing.T, branch string) *repo {
	t.Helper()
	r := &repo{t: t, dir: t.TempDir()}
	r.git("init", "--quiet", "--initial-branch=main")
	r.write("README.md", "test\n")
	r.git("add", "--all")
	r.git("commit", "--quiet", "-m", "init")
	if branch != "main" {
		r.git("checkout", "--quiet", "-b", branch)
	}
	return r
}

func (r *repo) git(args ...string) {
	r.t.Helper()
	cmd := exec.Command("git", append([]string{"-c", "user.name=test", "-c", "user.email=test@example.com",
		"-c", "commit.gpgsign=false"}, args...)...)
	cmd.Dir = r.dir
	cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL="+os.DevNull, "GIT_CONFIG_SYSTEM="+os.DevNull)
	if out, err := cmd.CombinedOutput(); err != nil {
		r.t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func (r *repo) write(rel, content string) {
	r.t.Helper()
	p := filepath.Join(r.dir, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		r.t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o755); err != nil {
		r.t.Fatal(err)
	}
}

// log returns the log of polix of the repository.
func (r *repo) log() string {
	data, _ := os.ReadFile(filepath.Join(r.dir, ".git", "polix", "log.jsonl"))
	return string(data)
}

// call runs `polix hook` with args and stdin through the command of the use case, as cmd/polix does.
func call(args []string, stdin []byte) (code int, stdout, stderr string) {
	root := cli.Root("polix", "", "", "")
	root.AddCommand(Command())
	var out, errb bytes.Buffer
	code = cli.Run(root, append([]string{"hook"}, args...), bytes.NewReader(stdin), &out, &errb)
	return code, out.String(), errb.String()
}

// hook calls `polix hook <agent>` with input, as the agent would.
func (r *repo) hook(agent string, input map[string]any) (code int, stdout, stderr string) {
	r.t.Helper()
	if _, ok := input["cwd"]; !ok {
		input["cwd"] = r.dir
	}
	if _, ok := input["session_id"]; !ok {
		input["session_id"] = "s1"
	}
	data, _ := json.Marshal(input)
	return call([]string{agent}, data)
}

func (r *repo) edit(path, agentType string) (int, string, string) {
	in := map[string]any{"hook_event_name": "PreToolUse", "tool_name": "Edit",
		"tool_input": map[string]any{"file_path": filepath.Join(r.dir, filepath.FromSlash(path))}}
	if agentType != "" {
		in["agent_type"] = agentType
	}
	return r.hook("claude", in)
}

func (r *repo) stop() (int, string, string) {
	return r.hook("claude", map[string]any{"hook_event_name": "Stop"})
}

func (r *repo) subagentStop(agentType string) (int, string, string) {
	return r.hook("claude", map[string]any{"hook_event_name": "SubagentStop", "agent_type": agentType})
}

func (r *repo) bash(command string, post bool) (int, string, string) {
	return r.bashAs(command, post, "")
}

func (r *repo) bashAs(command string, post bool, agentType string) (int, string, string) {
	event := "PreToolUse"
	if post {
		event = "PostToolUse"
	}
	in := map[string]any{"hook_event_name": event, "tool_name": "Bash",
		"tool_input": map[string]any{"command": command}, "tool_response": map[string]any{"stdout": "ok"}}
	if agentType != "" {
		in["agent_type"] = agentType
	}
	return r.hook("claude", in)
}

// stops runs a stop (or a subagent stop of agentType) once per wanted exit code and checks each.
func (r *repo) stops(agentType string, want ...int) {
	r.t.Helper()
	for i, w := range want {
		var code int
		var stderr string
		if agentType == "" {
			code, _, stderr = r.stop()
		} else {
			code, _, stderr = r.subagentStop(agentType)
		}
		expect(r.t, fmt.Sprintf("stop %d of %s", i+1, agentType), code, w, stderr)
	}
}

func expect(t *testing.T, what string, got, want int, stderr string) {
	t.Helper()
	if got != want {
		t.Fatalf("%s: exit %d, want %d (stderr: %s)", what, got, want, stderr)
	}
}

// skipFileModes skips a test that relies on file modes denying access: they do not on Windows nor as
// root.
func skipFileModes(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("file modes do not deny access here")
	}
}

const reviewOnStop = `version: 1
on:
  stop:
    - when: { changed: "**" }
      unless: { finished: reviewer }
      action: { block: "Lanza el subagente reviewer sobre el diff actual" }
`

// Example 1 of the design: a review is required before stopping, and a new edit requires a new one. polix observes
// the end of the reviewer by itself, since a finished names it.
func TestReviewOnStop(t *testing.T) {
	r := newRepo(t, "main")
	r.write("polix.yaml", reviewOnStop)
	r.git("add", "--all")
	r.git("commit", "--quiet", "-m", "workflow")

	code, _, stderr := r.stop()
	expect(t, "stop without changes", code, 0, stderr)

	r.write("src/app.go", "package app\n")
	code, _, stderr = r.stop()
	expect(t, "stop with unreviewed changes", code, 2, stderr)
	if !strings.Contains(stderr, "reviewer") {
		t.Fatalf("the reason should ask for the reviewer: %q", stderr)
	}

	code, _, stderr = r.subagentStop("reviewer")
	expect(t, "reviewer stops", code, 0, stderr)
	code, _, stderr = r.stop()
	expect(t, "stop after the review", code, 0, stderr)

	r.write("src/app.go", "package app\n\nfunc A() {}\n")
	code, _, stderr = r.stop()
	expect(t, "stop after a new edit", code, 2, stderr)
}

// The loop guard lets the agent stop after max_stop_retries blocked stops on the same content.
func TestStopLoopGuard(t *testing.T) {
	r := newRepo(t, "main")
	r.write("polix.yaml", "version: 1\nmax_stop_retries: 2\n"+strings.TrimPrefix(reviewOnStop, "version: 1\n"))
	r.write("src/app.go", "package app\n")
	for i, want := range []int{2, 2, 0} {
		code, _, stderr := r.stop()
		expect(t, "stop "+string(rune('1'+i)), code, want, stderr)
	}
}

const teamFlow = `version: 1
vars:
  key: { branch: "^([A-Z][A-Z0-9]+-[0-9]+)" }
on:
  pre_edit:
    - unless: { var: key }
      action: { block: "No hay ticket" }
    - when: { not: { path: "ai/plans/{key}-*.md" } }
      unless: { file: "ai/plans/{key}-*.md", contains: "Ticket: {key}" }
      action: { block: "No hay plan" }
    - when: { not: { path: "ai/plans/{key}-*.md" } }
      unless: { actor: builder }
      action: { block: "Solo el builder edita" }
    - action: { inject: { rules: "{path}" } }
  pre_commit:
    - action: { run: { command: [git, rev-parse, --verify, --quiet, tested], message: "Faltan los tests" } }
    - unless: { finished: reviewer }
      action: { block: "Falta la revisión" }
`

const archRules = `rules:
  - id: ARCH-1
    description: When in src/domain/, never import infrastructure.
    scope: ["src/domain/**"]
    deterministic: false
`

// Example 2 of the design: ticket, plan, builder, rules as context, tests and review before committing. The tests
// are a run of polix, which passes once the ref tested exists.
func TestTeamFlow(t *testing.T) {
	r := newRepo(t, "main")
	r.write("polix.yaml", teamFlow)
	r.write("ai/rules/architecture.yaml", archRules)
	r.git("add", "--all")
	r.git("commit", "--quiet", "-m", "workflow")

	code, _, stderr := r.edit("src/domain/order.go", "builder")
	expect(t, "edit without a ticket branch", code, 2, stderr)
	if !strings.Contains(stderr, "No hay ticket") {
		t.Fatalf("reason: %q", stderr)
	}

	r.git("checkout", "--quiet", "-b", "PLAT-7-orders")
	code, _, stderr = r.edit("ai/plans/PLAT-7-orders.md", "")
	expect(t, "the session writes the plan", code, 0, stderr)
	code, _, stderr = r.edit("src/domain/order.go", "builder")
	expect(t, "edit without a plan", code, 2, stderr)

	r.write("ai/plans/PLAT-7-orders.md", "# Plan\n\nTicket: PLAT-7\n")
	code, _, stderr = r.edit("src/domain/order.go", "")
	expect(t, "the session edits code", code, 2, stderr)

	code, stdout, stderr := r.edit("src/domain/order.go", "builder")
	expect(t, "the builder edits code", code, 0, stderr)
	if !strings.Contains(stdout, "ARCH-1") || !strings.Contains(stdout, "additionalContext") {
		t.Fatalf("the rules of the path should be injected as context: %q", stdout)
	}
	_, stdout, _ = r.edit("src/domain/order.go", "builder")
	if strings.Contains(stdout, "ARCH-1") {
		t.Fatalf("a rule is injected once per session: %q", stdout)
	}

	r.write("src/domain/order.go", "package domain\n")
	code, _, stderr = r.bash("git commit -m 'PLAT-7 orders'", false)
	expect(t, "commit without tests", code, 2, stderr)
	if !strings.HasPrefix(stderr, "Faltan los tests\n\nFalla git rev-parse") {
		t.Fatalf("reason: %q", stderr)
	}

	r.git("branch", "tested")
	code, _, stderr = r.bash("git commit -m 'PLAT-7 orders'", false)
	expect(t, "commit without review", code, 2, stderr)
	if !strings.Contains(stderr, "Falta la revisión") {
		t.Fatalf("reason: %q", stderr)
	}

	r.subagentStop("reviewer")
	code, _, stderr = r.bash("cd . && git commit -m 'PLAT-7 orders'", false)
	expect(t, "commit with tests and review", code, 0, stderr)
}

// A command chained with others still matches, but its exit code is only trusted when its failure propagates.
func TestChainedCommands(t *testing.T) {
	r := newRepo(t, "main")
	r.write("polix.yaml", `version: 1
on:
  post_command:
    - when: { command: "just test*", exit: 0 }
      action: { inject: "Los tests pasan" }
  pre_command:
    - when: { command: "git push*origin main*" }
      action: { block: "push a main" }
`)
	r.write("src/a.go", "package a\n")

	_, stdout, _ := r.bash("just test 2>&1 | tail -3", true)
	if strings.Contains(stdout, "Los tests pasan") {
		t.Fatalf("the exit of piped tests is decided by tail: %q", stdout)
	}
	_, stdout, _ = r.bash("cd . && just test -k sync", true)
	if !strings.Contains(stdout, "Los tests pasan") {
		t.Fatalf("the exit of chained tests is theirs: %q", stdout)
	}

	code, _, stderr := r.bash("git fetch && git push origin main", false)
	expect(t, "chained push to main", code, 2, stderr)
	code, _, stderr = r.bash(`echo "git push origin main"`, false)
	expect(t, "the text of a push inside quotes", code, 0, stderr)
}

// The protection of the state applies whatever the workflow says, even without one; the context, the rules and
// the workflow are not protected unless the workflow says so.
func TestProtections(t *testing.T) {
	r := newRepo(t, "main")

	code, _, stderr := r.edit("ai/rules/security.yaml", "builder")
	expect(t, "a subagent edits the rules", code, 0, stderr)
	code, _, stderr = r.edit("polix.yaml", "builder")
	expect(t, "a subagent edits the workflow", code, 0, stderr)
	code, _, stderr = r.edit("AGENTS.md", "reviewer")
	expect(t, "a subagent edits AGENTS.md", code, 0, stderr)
	code, _, stderr = r.edit(".git/polix/log.jsonl", "")
	expect(t, "anyone edits the state", code, 2, stderr)
	code, _, stderr = r.bash("rm .git/polix/log.jsonl", false)
	expect(t, "a command deletes the state", code, 2, stderr)
	code, _, stderr = r.bash("echo x > .git/polix/log.jsonl", false)
	expect(t, "a command overwrites the state", code, 2, stderr)
	code, _, stderr = r.bash("jq . .git/polix/log.jsonl", false)
	expect(t, "a command reads the state", code, 0, stderr)
}

// Edits outside the repository or of ignored files are not part of the change.
func TestOutsideAndIgnored(t *testing.T) {
	r := newRepo(t, "main")
	r.write("polix.yaml", "version: 1\non:\n  pre_edit:\n    - action: { block: \"bloqueado\" }\n")
	r.write(".gitignore", "build/\n")

	code, _, stderr := r.edit("src/a.go", "")
	expect(t, "edit in the repository", code, 2, stderr)
	code, _, stderr = r.edit("build/out.txt", "")
	expect(t, "edit of an ignored file", code, 0, stderr)
	in := map[string]any{"hook_event_name": "PreToolUse", "tool_name": "Write",
		"tool_input": map[string]any{"file_path": filepath.Join(t.TempDir(), "notes.md")}}
	code, _, stderr = r.hook("claude", in)
	expect(t, "edit outside the repository", code, 0, stderr)
}

// A warning is an inject, and a failing run after an edit reaches the AI as context, with its message, instead of
// blocking.
func TestFeedbackAfterAnEdit(t *testing.T) {
	r := newRepo(t, "main")
	r.write("polix.yaml", `version: 1
on:
  pre_edit:
    - when: { path: "a.txt" }
      action: { inject: "cuidado con a.txt" }
  post_edit:
    - when: { path: "b.txt" }
      action: { run: { command: [git, rev-parse, --verify, --quiet, no-such-ref], message: "b.txt no pasa el check" } }
`)
	code, stdout, stderr := r.edit("a.txt", "")
	expect(t, "inject", code, 0, stderr)
	if !strings.Contains(stdout, "cuidado con a.txt") {
		t.Fatalf("the inject adds context: %q", stdout)
	}
	code, stdout, stderr = r.hook("claude", map[string]any{"hook_event_name": "PostToolUse", "tool_name": "Edit",
		"tool_input": map[string]any{"file_path": filepath.Join(r.dir, "b.txt")}})
	expect(t, "a failing run after an edit", code, 0, stderr)
	if !strings.Contains(stdout, `"additionalContext":"b.txt no pasa el check\n\nFalla git rev-parse`) {
		t.Fatalf("the failure reaches the AI as context: %q", stdout)
	}
	if !strings.Contains(r.log(), `"outcome":"inform"`) {
		t.Fatalf("the failure is logged: %s", r.log())
	}
}

// finished holds once the reviewer ended and after an edit outside the scope of the entry, through the real trees
// of git; an edit inside the scope or a tree git does not have asks for a new review.
func TestFinishedOnTheScope(t *testing.T) {
	r := newRepo(t, "main")
	r.write("polix.yaml", `version: 1
on:
  pre_commit:
    - when: { changed: "*.go" }
      unless: { finished: polix:reviewer }
      action: { block: "Falta la revisión" }
`)
	r.write("src/a.go", "package a\n")
	commit := func(what string, want int) {
		t.Helper()
		code, _, stderr := r.bash("git commit -m x", false)
		expect(t, what, code, want, stderr)
	}
	commit("commit before the review", 2)
	code, _, stderr := r.subagentStop("polix:reviewer")
	expect(t, "the reviewer stops", code, 0, stderr)
	if !strings.Contains(r.log(), `"kind":"observed","agent":"claude","session":"s1","event":"subagent_stop",`+
		`"actor":"polix:reviewer","branch":"main","hash":`) || !strings.Contains(r.log(), `"key":"polix:reviewer",`+
		`"outcome":"finished"`) {
		t.Fatalf("the end of the reviewer is observed: %s", r.log())
	}
	commit("commit after the review", 0)
	r.write("README.md", "docs\n")
	r.write("docs/guide.md", "docs\n")
	commit("commit after an edit of the docs", 0)
	r.write("src/a.go", "package a\n\nvar x = 1\n")
	commit("commit after an edit of a Go file", 2)

	r.subagentStop("polix:reviewer")
	commit("commit after a new review", 0)
	r.write(".git/polix/log.jsonl", `{"kind":"observed","outcome":"finished","key":"polix:reviewer","branch":"main",`+
		`"hash":"`+strings.Repeat("1", 40)+`"}`+"\n")
	commit("commit with the tree of the review missing", 2)
	if !strings.Contains(r.log(), "no se puede comparar el contenido en que terminó polix:reviewer") {
		t.Fatalf("the failed comparison is logged: %s", r.log())
	}
}

// A reminder with once blocks the first commit on a content and lets the next through; an edit outside its scope
// keeps it given, one inside it reminds again, and so does a new session.
func TestReminderOnTheScope(t *testing.T) {
	r := newRepo(t, "main")
	r.write("polix.yaml", `version: 1
on:
  pre_commit:
    - when: { changed: "*.go" }
      action: { block: { message: "Actualiza la documentación", once: true } }
`)
	r.write("src/a.go", "package a\n")
	commit := func(what string, want int) {
		t.Helper()
		code, _, stderr := r.bash("git commit -m x", false)
		expect(t, what, code, want, stderr)
	}
	commit("first commit", 2)
	if !strings.Contains(r.log(), `"line":4,"outcome":"blocked"`) {
		t.Fatalf("the reminder is observed: %s", r.log())
	}
	commit("second commit", 0)
	r.write("README.md", "docs\n")
	commit("commit after an edit of the docs", 0)
	r.write("src/a.go", "package a\n\nvar x = 1\n")
	commit("commit after an edit of a Go file", 2)
	code, _, stderr := r.hook("claude", map[string]any{"hook_event_name": "PreToolUse", "tool_name": "Bash",
		"tool_input": map[string]any{"command": "git commit -m x"}, "session_id": "s2"})
	expect(t, "commit of a new session", code, 2, stderr)
}

// A run that passed is not run again on the same content: the next stop logs the skip, and an edit runs it again.
func TestRunRepeatOnTheContent(t *testing.T) {
	r := newRepo(t, "main")
	r.write("polix.yaml", "version: 1\non:\n  stop:\n    - action: { run: [git, status, --short] }\n")
	r.write("src/app.go", "package app\n")
	r.stops("", 0, 0)
	if n := strings.Count(r.log(), `"key":"git status --short","outcome":"passed"`); n != 1 {
		t.Fatalf("passed %d times: %s", n, r.log())
	}
	if !strings.Contains(r.log(), `"action":"run","line":4,"outcome":"skip"`) {
		t.Fatalf("the skip is logged: %s", r.log())
	}
	r.write("src/app.go", "package app\n\nfunc A() {}\n")
	r.stops("", 0)
	if n := strings.Count(r.log(), `"outcome":"passed"`); n != 2 {
		t.Fatalf("an edit runs it again: %s", r.log())
	}
}

// run: { rules: changed } on stop runs the checks of the rules of the changed files and names the rule.
func TestRuleChecksOnStop(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the test check is a shell script")
	}
	r := newRepo(t, "main")
	r.write("polix.yaml", "version: 1\non:\n  stop:\n    - when: { changed: \"**\" }\n      action: { run: { rules: changed } }\n")
	r.write("ai/rules/style.yaml", `rules:
  - id: STYLE-1
    description: Files under src/ end with a newline.
    scope: ["src/**"]
    deterministic: true
    check: tests/rules/style-1.sh
`)
	r.write("tests/rules/style-1.sh", "#!/bin/sh\nif grep -q TODO src/*; then echo 'TODO found in src'; exit 1; fi\n")
	r.write("src/a.txt", "ok\n")
	code, _, stderr := r.stop()
	expect(t, "stop with the rule passing", code, 0, stderr)
	r.write("src/a.txt", "TODO\n")
	code, _, stderr = r.stop()
	expect(t, "stop with the rule failing", code, 2, stderr)
	if !strings.Contains(stderr, "STYLE-1") || !strings.Contains(stderr, "TODO found") {
		t.Fatalf("the reason should name the rule and its output: %q", stderr)
	}
}

// An invalid workflow blocks edits and commands but never a stop.
func TestInvalidWorkflow(t *testing.T) {
	r := newRepo(t, "main")
	r.write("polix.yaml", "version: 1\non:\n  pre_edit:\n    - when: { pth: \"x\" }\n      action: { block: \"x\" }\n")

	code, _, stderr := r.edit("src/a.go", "")
	expect(t, "edit with an invalid workflow", code, 2, stderr)
	if !strings.Contains(stderr, `unknown condition "pth"`) {
		t.Fatalf("reason: %q", stderr)
	}
	code, _, stderr = r.stop()
	expect(t, "stop with an invalid workflow", code, 0, stderr)

}

// not holds unless every nested condition holds.
func TestNot(t *testing.T) {
	r := newRepo(t, "PLAT-1-x")
	r.write("polix.yaml", `version: 1
on:
  pre_command:
    - when: { command: "terraform apply*", not: { branch: "^main$" } }
      action: { block: "apply solo desde main" }
    - when: { command: "deploy*", not: { branch: "^PLAT-", file: "ai/plans/*.md" } }
      action: { block: "deploy sin plan" }
`)
	code, _, stderr := r.bash("terraform apply", false)
	expect(t, "apply outside main", code, 2, stderr)
	code, _, stderr = r.bash("deploy", false)
	expect(t, "deploy without a plan: only one nested condition holds", code, 2, stderr)
	r.write("ai/plans/PLAT-1-x.md", "Ticket: PLAT-1\n")
	code, _, stderr = r.bash("deploy", false)
	expect(t, "deploy with a plan: every nested condition holds", code, 0, stderr)

	r.git("checkout", "--quiet", "main")
	code, _, stderr = r.bash("terraform apply", false)
	expect(t, "apply on main", code, 0, stderr)
}

// Codex: apply_patch paths and commands given as a list of arguments.
func TestCodex(t *testing.T) {
	r := newRepo(t, "main")
	r.write("polix.yaml", `version: 1
on:
  pre_edit:
    - when: { path: "src/**" }
      action: { block: "src bloqueado" }
  pre_command:
    - when: { command: "terraform apply*" }
      action: { block: "apply solo desde el pipeline" }
`)
	patch := "*** Begin Patch\n*** Update File: src/a.go\n@@\n-a\n+b\n*** End Patch\n"
	code, _, stderr := r.hook("codex", map[string]any{"hook_event_name": "PreToolUse", "tool_name": "apply_patch",
		"tool_input": map[string]any{"command": []any{"apply_patch", patch}}})
	expect(t, "codex patches src", code, 2, stderr)
	code, _, stderr = r.hook("codex", map[string]any{"hook_event_name": "PreToolUse", "tool_name": "shell",
		"tool_input": map[string]any{"command": []any{"terraform", "apply", "-auto-approve"}}})
	expect(t, "codex runs terraform apply", code, 2, stderr)
	code, _, stderr = r.hook("codex", map[string]any{"hook_event_name": "PreToolUse", "tool_name": "shell",
		"tool_input": map[string]any{"command": []any{"terraform", "plan"}}})
	expect(t, "codex runs terraform plan", code, 0, stderr)
}

// The state is found by its path, not by the name alone, which is also the name of the binary: commands that
// only mention polix are not taken for writes of the state.
func TestStateProtectionByPath(t *testing.T) {
	r := newRepo(t, "main")
	for _, c := range []string{
		"go install ./cmd/polix",
		"cd ~/Code/polix-ai && just test 2>&1 | tail -3",
		"polix log > /tmp/out.txt",
		"cp build/polix ~/bin/",
		"cat .git/polix/log.jsonl >&2",
	} {
		code, _, stderr := r.bash(c, false)
		expect(t, c, code, 0, stderr)
	}
	for _, c := range []string{
		"rm -rf .git/polix",
		"cp /tmp/x ./.git/polix/log.jsonl",
		"echo x >> " + filepath.Join(r.dir, ".git", "polix", "log.jsonl"),
		"cd .. && rm repo/.git/worktrees/w1/polix/log.jsonl",
		"rm -rf .git/polix;true",
		"rm -rf .git/polix&&true",
		"(rm -rf .git/polix)",
		"rm .git/polix* x",
		"echo x >&.git/polix/log.jsonl",
		"echo x >& .git/polix/log.jsonl",
	} {
		code, _, stderr := r.bash(c, false)
		expect(t, c, code, 2, stderr)
	}
}

// On the systems whose file names ignore case, the state is found in any case.
func TestStateProtectionFoldsCase(t *testing.T) {
	if runtime.GOOS != "darwin" && runtime.GOOS != "windows" {
		t.Skip("file names keep their case on " + runtime.GOOS)
	}
	r := newRepo(t, "main")
	code, _, stderr := r.edit(".git/POLIX/log.jsonl", "")
	expect(t, "edit of the state in another case", code, 2, stderr)
	code, _, stderr = r.bash("rm -rf .GIT/Polix", false)
	expect(t, "a command on the state in another case", code, 2, stderr)
}

// protected_files keeps every actor, the main session included, from writing a path, also an ignored one; reads
// pass.
func TestProtectedPaths(t *testing.T) {
	r := newRepo(t, "main")
	r.write(".gitignore", ".env\n")
	r.write("polix.yaml", "version: 1\nguardrails:\n  protected_files: [\".env\", \"AGENTS.md\", \"ai/rules/**\"]\n")
	for _, actor := range []string{"", "builder"} {
		code, _, stderr := r.edit(".env", actor)
		expect(t, actor+" edits .env", code, 2, stderr)
		if !strings.Contains(stderr, ".env is protected in polix.yaml: only a person changes it.") {
			t.Fatalf("the reason says who changes it: %q", stderr)
		}
		code, _, stderr = r.bashAs("echo x >AGENTS.md", false, actor)
		expect(t, actor+" writes AGENTS.md", code, 2, stderr)
		code, _, stderr = r.bashAs("cat a.txt>>AGENTS.md", false, actor)
		expect(t, actor+" appends to AGENTS.md without spaces", code, 2, stderr)
		code, _, stderr = r.bashAs("cat AGENTS.md", false, actor)
		expect(t, actor+" reads AGENTS.md", code, 0, stderr)
		for _, cmd := range []string{"rm -rf ai/rules", "rm -rf ai/rules/", "rm -rf ai", "mv ai/rules x", "rm -rf .",
			"rm -rf ./", "rm -rf *", "rm -rf ai/*", "rm -rf ai/r*", "rm -f ai/rules/*", "cat a.txt > ai/rules/x.yaml",
			"echo x >&AGENTS.md", `rm -rf "a>b" ai`} {
			code, _, stderr = r.bashAs(cmd, false, actor)
			expect(t, actor+" runs "+cmd, code, 2, stderr)
		}
		code, _, stderr = r.bashAs("rm -rf ai", false, actor)
		if code != 2 || !strings.Contains(stderr, "ai holds ai/rules/**, protected in polix.yaml: only a person changes it.") {
			t.Fatalf("the reason names the protected glob the folder holds: %q", stderr)
		}
		// Reading a folder that holds protected paths is fine, also with its output redirected away from it.
		for _, cmd := range []string{"rm -rf ai/plans", "rm -f ai/plans/*", "ls ai 2>/dev/null", "rg foo ai > /tmp/out",
			"ls ai > out.txt", "cp a.txt .", "cat AGENTS.md 2>&1 >/dev/null", "rg install ai", "grep -rn rm ai",
			`grep -n "x>y" AGENTS.md`, "mv /tmp/foo.txt .", "cat AGENTS.md > notes.txt"} {
			code, _, stderr = r.bashAs(cmd, false, actor)
			expect(t, actor+" runs "+cmd, code, 0, stderr)
		}
		code, _, stderr = r.edit("README.md", actor)
		expect(t, actor+" edits README.md", code, 0, stderr)
	}
	// Where file names ignore case, so does protected_files: .ENV is .env there.
	if runtime.GOOS == "darwin" || runtime.GOOS == "windows" {
		code, _, stderr := r.edit(".ENV", "")
		expect(t, "edits .ENV", code, 2, stderr)
		code, _, stderr = r.bash("echo x > ./agents.md", false)
		expect(t, "writes ./agents.md", code, 2, stderr)
	}
}

// forbidden_commands denies a line wherever the command comes in it, and protected_branches a commit on the branch,
// through the adapter; another line and a commit on another branch pass.
func TestShortcuts(t *testing.T) {
	r := newRepo(t, "main")
	r.write("polix.yaml", "version: 1\nguardrails:\n  forbidden_commands:\n    - pattern: \"rm -rf /*\"\n"+
		"  protected_branches: [main]\n")
	code, _, stderr := r.bash("cd tmp && rm -rf /", false)
	expect(t, "a forbidden command after cd", code, 2, stderr)
	if !strings.Contains(stderr, "This command is forbidden by polix.yaml: a person must run it.\ncd tmp && rm -rf /") {
		t.Fatalf("the reason: %q", stderr)
	}
	code, _, stderr = r.bash("rm -rf tmp", false)
	expect(t, "another rm", code, 0, stderr)
	code, _, stderr = r.bash("git commit -m x", false)
	expect(t, "a commit on main", code, 2, stderr)
	r.git("checkout", "--quiet", "-b", "feature")
	code, _, stderr = r.bash("git commit -m x", false)
	expect(t, "a commit on feature", code, 0, stderr)
}

// A protected glob with a leading slash is anchored to the root: /polix.yaml keeps polix.yaml, not
// examples/polix.yaml, which a glob without slash would keep too.
func TestProtectedRootedPath(t *testing.T) {
	r := newRepo(t, "main")
	r.write("polix.yaml", "version: 1\nguardrails:\n  protected_files: [\"/polix.yaml\", \"AGENTS.md\"]\n")
	r.write("examples/polix.yaml", "version: 1\n")
	r.write("docs/AGENTS.md", "x\n")
	code, _, stderr := r.edit("polix.yaml", "")
	expect(t, "edits polix.yaml", code, 2, stderr)
	if !strings.Contains(stderr, "polix.yaml is protected in polix.yaml") {
		t.Fatalf("the reason: %q", stderr)
	}
	for _, cmd := range []string{"echo x > polix.yaml", "rm -f polix.yaml", "rm -f ./polix.yaml", "rm -f *.yaml"} {
		code, _, stderr = r.bash(cmd, false)
		expect(t, "runs "+cmd, code, 2, stderr)
	}
	code, _, stderr = r.edit("examples/polix.yaml", "")
	expect(t, "edits examples/polix.yaml", code, 0, stderr)
	for _, cmd := range []string{"echo x > examples/polix.yaml", "rm -f examples/polix.yaml", "rm -rf examples"} {
		code, _, stderr = r.bash(cmd, false)
		expect(t, "runs "+cmd, code, 0, stderr)
	}
	// Without the slash the glob still matches the name in any folder.
	code, _, stderr = r.edit("docs/AGENTS.md", "")
	expect(t, "edits docs/AGENTS.md", code, 2, stderr)
}

// writes lets an entry keep only subagents away from a path.
func TestWritesCondition(t *testing.T) {
	r := newRepo(t, "main")
	r.write("polix.yaml", `version: 1
on:
  pre_command:
    - when: { writes: ["AGENTS.md", "ai/rules/**"], not: { actor: session } }
      action: { block: "Solo la sesión principal cambia AGENTS.md" }
`)
	code, _, stderr := r.bashAs("rm -rf ai/rules", false, "builder")
	expect(t, "builder removes ai/rules", code, 2, stderr)
	code, _, stderr = r.bashAs("ls ai 2>/dev/null", false, "builder")
	expect(t, "builder lists ai", code, 0, stderr)
	code, _, stderr = r.bashAs("sed -i s/a/b/ AGENTS.md", false, "builder")
	expect(t, "builder writes AGENTS.md", code, 2, stderr)
	code, _, stderr = r.bashAs("cat AGENTS.md", false, "builder")
	expect(t, "builder reads AGENTS.md", code, 0, stderr)
	code, _, stderr = r.bashAs("sed -i s/a/b/ AGENTS.md", false, "")
	expect(t, "the session writes AGENTS.md", code, 0, stderr)
}

// Rules that cannot be loaded fail run: { rules }, instead of passing with no checks.
func TestRunRulesFailsOnBadRules(t *testing.T) {
	r := newRepo(t, "main")
	r.write("polix.yaml", "version: 1\non:\n  stop:\n    - action: { run: { rules: changed } }\n")
	r.write("ai/rules/bad.yaml", "rules: [\n")
	code, _, stderr := r.stop()
	expect(t, "stop with rules that do not load", code, 2, stderr)
	if !strings.Contains(stderr, "No se pueden cargar las rules") {
		t.Fatalf("reason: %q", stderr)
	}
}

// The loop guard covers every block of a stop: a block or a failing run cannot trap the agent.
func TestStopLoopGuardOnAnyBlock(t *testing.T) {
	r := newRepo(t, "main")
	r.write("polix.yaml", `version: 1
on:
  stop:
    - action: { block: "nunca paras" }
  subagent_stop:
    - action: { run: [git, rev-parse, --verify, --quiet, no-such-ref] }
`)
	r.stops("", 2, 2, 2, 0)
	r.stops("builder", 2, 2, 2, 0)
}

// Each entry has its own guard, and subagents do not use up the guard of the main session: once the guard lets
// the first block through, the next one blocks on its own.
func TestStopLoopGuardPerEntry(t *testing.T) {
	r := newRepo(t, "main")
	r.write("polix.yaml", `version: 1
on:
  stop:
    - action: { block: "A" }
    - action: { block: "B" }
  subagent_stop:
    - action: { block: "C" }
`)
	r.stops("builder", 2, 2, 2, 0)
	for i := range 3 {
		code, _, stderr := r.stop()
		expect(t, fmt.Sprintf("stop %d", i+1), code, 2, stderr)
		if stderr != "A\n" {
			t.Fatalf("the first block ends the evaluation: %q", stderr)
		}
	}
	for i := range 3 {
		code, _, stderr := r.stop()
		expect(t, fmt.Sprintf("stop %d", i+4), code, 2, stderr)
		if stderr != "B\n" {
			t.Fatalf("A is let through and B blocks: %q", stderr)
		}
	}
	r.stops("", 0)
}

// An invalid workflow fails closed, but the main session can still fix it, and the protections still apply.
func TestInvalidWorkflowCanBeFixed(t *testing.T) {
	r := newRepo(t, "main")
	r.write("polix.yaml", "version: 1\non:\n  pre_edit:\n    - action: { block: x }\n      foo: 1\n")
	r.write(".gitignore", "build/\n")

	code, _, stderr := r.edit("polix.yaml", "")
	expect(t, "the session edits the workflow", code, 0, stderr)
	code, _, stderr = r.edit("polix.yaml", "builder")
	expect(t, "a subagent edits the workflow", code, 2, stderr)
	code, _, stderr = r.edit("src/a.go", "")
	expect(t, "the session edits code", code, 2, stderr)
	code, _, stderr = r.edit("build/out.txt", "")
	expect(t, "edit of an ignored file", code, 0, stderr)
	code, _, stderr = r.hook("claude", map[string]any{"hook_event_name": "PreToolUse", "tool_name": "Write",
		"tool_input": map[string]any{"file_path": filepath.Join(t.TempDir(), "notes.md")}})
	expect(t, "edit outside the repository", code, 0, stderr)
	code, _, stderr = r.edit(".git/polix/log.jsonl", "")
	expect(t, "edit of the state", code, 2, stderr)
	if !strings.Contains(stderr, "estado de polix") {
		t.Fatalf("the protection should answer, not the invalid workflow: %q", stderr)
	}
	for c, want := range map[string]int{"polix config check": 0, "cd . && polix config check --dir .": 0, "ls": 2,
		"polix config check && ls": 2} {
		code, _, stderr = r.bash(c, false)
		expect(t, c, code, want, stderr)
	}
	code, _, stderr = r.bash("git commit -m x", false)
	expect(t, "commit", code, 2, stderr)
	if n := strings.Count(stderr, "no es válido"); n != 1 {
		t.Fatalf("the command and its commit give one message, got %d: %q", n, stderr)
	}
}

// A workflow that requires a newer polix asks to update polix, not to fix the file: not even the main session may
// edit it, lest it take out options of the newer polix, and the commands that update polix stay open.
func TestWorkflowAheadOfPolix(t *testing.T) {
	old := build.Version
	build.Version = "0.1.0"
	t.Cleanup(func() { build.Version = old })
	r := newRepo(t, "main")
	r.write("polix.yaml", "version: 1\nrequires: 0.2.0\nan_option_of_0_2: true\n")

	code, _, stderr := r.edit("polix.yaml", "")
	expect(t, "the session edits a workflow ahead", code, 2, stderr)
	if !strings.Contains(stderr, "necesita polix 0.2.0") || !strings.Contains(stderr, "polix update") ||
		strings.Contains(stderr, "corrígelo") {
		t.Fatalf("the reason asks to update polix: %q", stderr)
	}
	code, _, stderr = r.edit("src/a.go", "")
	expect(t, "the session edits code", code, 2, stderr)
	for c, want := range map[string]int{"polix update": 0, "polix update --version 0.2.0": 0, "polix version": 0,
		"polix doctor": 0, "ls": 2, "polix update && ls": 2} {
		code, _, stderr = r.bash(c, false)
		expect(t, c, code, want, stderr)
	}
	code, _, stderr = r.stop()
	expect(t, "stop", code, 0, stderr)

}

// Unknown options may belong to a newer polix: the reason says so, so the agent updates instead of removing them.
func TestUnknownOptionsHintAnUpdate(t *testing.T) {
	r := newRepo(t, "main")
	r.write("polix.yaml", "version: 1\non:\n  pre_edit:\n    - action: { block: x }\n      an_option_of_0_2: 1\n")
	code, _, stderr := r.edit("src/a.go", "")
	expect(t, "edit", code, 2, stderr)
	if !strings.Contains(stderr, "no las quites: ejecuta polix update") {
		t.Fatalf("the reason hints an update: %q", stderr)
	}
	// A name of the old vocabulary comes with its new spelling, not with an update.
	r.write("polix.yaml", "version: 1\non:\n  pre_edit:\n    - deny: x\n")
	code, _, stderr = r.edit("src/a.go", "")
	expect(t, "edit with the old vocabulary", code, 2, stderr)
	if !strings.Contains(stderr, `unknown key "deny": use action: { block: <message> }`) ||
		strings.Contains(stderr, "polix update") {
		t.Fatalf("the reason gives the new spelling: %q", stderr)
	}
	r.write("polix.yaml", "version: 2\n")
	if _, _, stderr = r.edit("src/a.go", ""); !strings.Contains(stderr, "polix update") {
		t.Fatalf("a later version of the schema hints an update: %q", stderr)
	}
	r.write("polix.yaml", "version: 1\nmax_stop_retries: -1\n")
	if _, _, stderr = r.edit("src/a.go", ""); strings.Contains(stderr, "polix update") {
		t.Fatalf("no hint for a plain mistake: %q", stderr)
	}
}

// Codex gives relative paths in apply_patch: they are resolved from the cwd of the call, not of the process.
func TestCodexRelativeStatePath(t *testing.T) {
	r := newRepo(t, "main")
	patch := "*** Begin Patch\n*** Update File: .git/polix/log.jsonl\n@@\n-a\n+b\n*** End Patch\n"
	code, _, stderr := r.hook("codex", map[string]any{"hook_event_name": "PreToolUse", "tool_name": "apply_patch",
		"tool_input": map[string]any{"input": patch}})
	expect(t, "codex patches the state", code, 2, stderr)
}

// Paths that are not plain ASCII match globs.
func TestNonASCIIChangedFiles(t *testing.T) {
	r := newRepo(t, "main")
	r.write("polix.yaml", "version: 1\non:\n  stop:\n    - when: { changed: \"*.md\" }\n      action: { block: \"md cambiado\" }\n")
	r.git("add", "--all")
	r.git("commit", "--quiet", "-m", "workflow")
	r.write("diseño/año.md", "x\n")
	code, _, stderr := r.stop()
	expect(t, "stop with a changed año.md", code, 2, stderr)
}

// Diff measures a tracked change and an untracked file of the repository.
func TestAdapterDiff(t *testing.T) {
	r := newRepo(t, "main")
	r.write("README.md", "test\nmore\n")
	r.write("src/a.go", "package a\n\nfunc A() {}\n")
	a := &Adapter{}
	if _, _, err := a.Open(context.Background(), r.dir); err != nil {
		t.Fatal(err)
	}
	d, err := a.Diff(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	want := []diff.Change{{Path: "README.md", Added: 1}, {Path: "src/a.go", Added: 3}}
	if !slices.Equal(d.Changes, want) {
		t.Fatalf("Diff = %+v, want %+v", d.Changes, want)
	}
}

// In the hooks, changed is what is not committed yet, whatever base_branch says.
func TestChangedSinceHead(t *testing.T) {
	r := newRepo(t, "main")
	r.write("polix.yaml", "version: 1\nbase_branch: main\non:\n  stop:\n    - when: { changed: \"src/**\" }\n      action: { block: \"src cambiado\" }\n")
	r.git("add", "--all")
	r.git("commit", "--quiet", "-m", "workflow")
	r.git("checkout", "--quiet", "-b", "feature")
	r.write("src/a.go", "package a\n")
	code, _, stderr := r.stop()
	expect(t, "stop with uncommitted src", code, 2, stderr)
	r.git("add", "--all")
	r.git("commit", "--quiet", "-m", "src")
	code, _, stderr = r.stop()
	expect(t, "stop after the commit", code, 0, stderr)
}

// The example settings give the hooks more time than the run budget of a call, so the agent never kills a
// hook in the middle of a gate.
func TestExampleHookTimeouts(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "..", "..", "examples", "claude-settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	var settings struct {
		Hooks map[string][]struct {
			Hooks []struct {
				Command string  `json:"command"`
				Timeout float64 `json:"timeout"`
			} `json:"hooks"`
		} `json:"hooks"`
	}
	if err := json.Unmarshal(data, &settings); err != nil {
		t.Fatal(err)
	}
	for event, groups := range settings.Hooks {
		for _, g := range groups {
			for _, h := range g.Hooks {
				if strings.Contains(h.Command, "polix hook claude") && h.Timeout <= hook.RunBudget.Seconds() {
					t.Errorf("%s: timeout %vs, want more than the run budget (%v)", event, h.Timeout, hook.RunBudget)
				}
			}
		}
	}
}

// A decision does not compute the content hash just to log it: a plain block costs no git tree.
func TestDecisionsSkipTheHash(t *testing.T) {
	r := newRepo(t, "main")
	r.write("polix.yaml", "version: 1\non:\n  pre_edit:\n    - action: { block: x }\n")
	r.edit("src/a.go", "")
	data, _ := os.ReadFile(filepath.Join(r.dir, ".git", "polix", "log.jsonl"))
	if !strings.Contains(string(data), `"action":"block","line":4`) || strings.Contains(string(data), `"hash"`) {
		t.Fatalf("log: %s", data)
	}
}

// A line of the log longer than the read limit is skipped: the loop guard still counts and lets the agent stop,
// and polix log prints the other entries and says how many lines it skipped.
func TestStopLoopGuardHugeLogLine(t *testing.T) {
	r := newRepo(t, "main")
	r.write("polix.yaml", "version: 1\nmax_stop_retries: 2\n"+strings.TrimPrefix(reviewOnStop, "version: 1\n"))
	r.write("src/app.go", "package app\n")
	r.write(".git/polix/log.jsonl", `{"kind":"decision","reason":"`+strings.Repeat("x", 5*1024*1024)+"\"}\n")
	r.stops("", 2, 2, 0)
	// polix log on such a log is tested in internal/app/workflow/log (TestCommand).
	data, err := os.ReadFile(filepath.Join(r.dir, ".git", "polix", "log.jsonl"))
	if err != nil || !strings.Contains(string(data), "loop_guard") {
		t.Fatalf("the log records the loop guard: %v", err)
	}
}

// When the stop block cannot be written, the loop guard cannot count: a blocking stop lets the agent stop with
// a warning instead of trapping it.
func TestStopLoopGuardReadOnlyState(t *testing.T) {
	skipFileModes(t)
	r := newRepo(t, "main")
	r.write("polix.yaml", reviewOnStop)
	r.write("src/app.go", "package app\n")
	dir := filepath.Join(r.dir, ".git", "polix")
	r.write(".git/polix/log.jsonl", "")
	if err := os.Chmod(filepath.Join(dir, "log.jsonl"), 0o444); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dir, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o755) })
	code, _, stderr := r.stop()
	expect(t, "stop with a read only state", code, 0, stderr)
	if !strings.Contains(stderr, "no puede usar su log") {
		t.Fatalf("stderr: %q", stderr)
	}
}

// A failing run that prints a huge line records a capped reason, which polix log prints.
func TestRunHugeOutputCapped(t *testing.T) {
	r := newRepo(t, "main")
	r.write("polix.yaml", `version: 1
on:
  stop:
    - action: { run: [git, diff, --no-index, empty.txt, big.txt] }
`)
	r.write("empty.txt", "")
	r.write("big.txt", strings.Repeat("y", 4*1000*1000)+"\n")
	code, _, stderr := r.stop()
	expect(t, "stop with a failing run", code, 2, stderr)
	data, err := os.ReadFile(filepath.Join(r.dir, ".git", "polix", "log.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for line := range strings.SplitSeq(strings.TrimSpace(string(data)), "\n") {
		if len(line) > 16*1024 {
			t.Fatalf("a log line has %d bytes", len(line))
		}
		found = found || strings.Contains(line, "Falla git diff")
	}
	if !found {
		t.Fatalf("log: %.2000s", data)
	}
}

// Without a content hash, the end of a subagent is not observed, finished does not hold and the hook says why.
func TestUnknownHashObservesNothing(t *testing.T) {
	skipFileModes(t)
	r := newRepo(t, "main")
	r.write("polix.yaml", `version: 1
on:
  stop:
    - when: { changed: "**" }
      unless: { finished: reviewer }
      action: { block: "Falta la revisión" }
`)
	r.write("secret.txt", "x\n")
	p := filepath.Join(r.dir, "secret.txt")
	if err := os.Chmod(p, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(p, 0o644) })
	code, _, stderr := r.subagentStop("reviewer")
	expect(t, "the reviewer stops without a hash", code, 0, stderr)
	if !strings.Contains(stderr, "hash del contenido") {
		t.Fatalf("stderr should warn about the hash: %q", stderr)
	}
	if strings.Contains(r.log(), `"kind":"observed"`) {
		t.Fatalf("log: %s", r.log())
	}
	r.write("src/a.go", "package a\n")
	code, _, stderr = r.stop()
	expect(t, "stop after an edit without a hash", code, 2, stderr)
}

// A run may rewrite files: the later entries of the call use the content after it.
func TestRunRefreshesHash(t *testing.T) {
	r := newRepo(t, "main")
	r.write("polix.yaml", `version: 1
on:
  stop:
    - action: { run: [git, checkout, --, README.md] }
    - unless: { finished: reviewer }
      action: { block: "Falta la revisión" }
`)
	r.git("add", "polix.yaml")
	r.git("commit", "--quiet", "-m", "workflow")
	r.subagentStop("reviewer")
	r.write("README.md", "changed\n")
	code, _, stderr := r.stop()
	expect(t, "stop with a run that restores the reviewed content", code, 0, stderr)
}

// A failed command reaches post_command with exit 1, and a failed edit runs no post_edit.
func TestPostToolUseFailure(t *testing.T) {
	r := newRepo(t, "main")
	r.write("polix.yaml", "version: 1\non:\n  post_command:\n    - when: { exit: 1 }\n      action: { inject: \"el comando falló\" }\n"+
		"  post_edit:\n    - action: { inject: \"post edit\" }\n")
	_, stdout, stderr := r.hook("claude", map[string]any{"hook_event_name": "PostToolUseFailure", "tool_name": "Bash",
		"tool_input": map[string]any{"command": "go test ./..."}, "error": "exit status 1"})
	if !strings.Contains(stdout, "el comando falló") {
		t.Fatalf("a failed command matches exit 1: %q %q", stdout, stderr)
	}
	_, stdout, stderr = r.hook("claude", map[string]any{"hook_event_name": "PostToolUseFailure", "tool_name": "Edit",
		"tool_input": map[string]any{"file_path": filepath.Join(r.dir, "a.go")}})
	if strings.Contains(stdout, "post edit") || strings.Contains(stderr, "post edit") {
		t.Fatalf("a failed edit runs no post_edit: %q %q", stdout, stderr)
	}
	data, _ := os.ReadFile(filepath.Join(r.dir, ".git", "polix", "log.jsonl"))
	if strings.Contains(string(data), "post_edit") {
		t.Fatalf("no post_edit in the log: %s", data)
	}
}

// run: { rules: changed } runs each check through the runner of rules.run whose glob matches its file, with the
// placeholders expanded.
func TestRuleChecksThroughRunner(t *testing.T) {
	r := newRepo(t, "main")
	r.write("polix.yaml", `version: 1
rules:
  run:
    "*.ref": [git, rev-parse, --verify, --quiet, "{name}"]
on:
  stop:
    - when: { changed: "src/**" }
      action: { run: { rules: changed } }
`)
	r.write("ai/rules/style.yaml", `rules:
  - id: STYLE-1
    description: The ref of the rule exists.
    scope: ["src/**"]
    deterministic: true
    check: checks/style.ref
`)
	r.write("checks/style.ref", "")
	r.write("src/a.txt", "x\n")
	code, _, stderr := r.stop()
	expect(t, "stop with the ref missing", code, 2, stderr)
	if !strings.Contains(stderr, "- STYLE-1 (checks/style.ref): The ref of the rule exists.") {
		t.Fatalf("the reason names the rule: %q", stderr)
	}
	r.git("branch", "STYLE1")
	code, _, stderr = r.stop()
	expect(t, "stop with the ref of {name}", code, 0, stderr)
}

// A log that cannot be read is an error of LoadState, so the loop guard can let the agent stop.
func TestAdapterLoadStateUnreadable(t *testing.T) {
	skipFileModes(t)
	r := newRepo(t, "main")
	r.write(".git/polix/log.jsonl", `{"kind":"stop_block"}`+"\n")
	p := filepath.Join(r.dir, ".git", "polix", "log.jsonl")
	if err := os.Chmod(p, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(p, 0o644) })
	a := &Adapter{}
	if _, _, err := a.Open(context.Background(), r.dir); err != nil {
		t.Fatal(err)
	}
	if _, err := a.LoadState(context.Background()); err == nil {
		t.Fatal("want the error of the unreadable log")
	}
}

// The adapter reads the facts of the repository and appends to its log, stamped with the time.
func TestAdapter(t *testing.T) {
	r := newRepo(t, "feature")
	r.write(".gitignore", "build/\n")
	r.write("src/a.go", "package a\n")
	ctx := context.Background()
	a := &Adapter{}
	root, gitDir, err := a.Open(ctx, r.dir)
	if err != nil || root == "" || gitDir == "" || a.Deadline.IsZero() {
		t.Fatalf("Open: %q %q %v", root, gitDir, err)
	}
	if _, present, err := a.LoadWorkflow(ctx, "dev"); present || err != nil {
		t.Fatalf("no polix.yaml: %v %v", present, err)
	}
	r.write("polix.yaml", "version: 1\nfoo: 1\n")
	if _, present, err := a.LoadWorkflow(ctx, "dev"); !present || err == nil {
		t.Fatalf("a broken polix.yaml: %v %v", present, err)
	}
	if b := a.Branch(ctx); b != "feature" {
		t.Fatalf("Branch = %q", b)
	}
	if rel, inside := a.Locate(ctx, filepath.Join(r.dir, "src", "a.go"), ""); rel != "src/a.go" || !inside {
		t.Fatalf("Locate = %q %v", rel, inside)
	}
	if !a.Ignored(ctx, "build/x") || a.Ignored(ctx, "src/a.go") {
		t.Fatal("Ignored")
	}
	if !slices.Contains(a.Changed(ctx), "src/a.go") || !slices.Contains(a.Files(ctx), "README.md") {
		t.Fatalf("Changed %q Files %q", a.Changed(ctx), a.Files(ctx))
	}
	before, err := a.Hash(ctx)
	if err != nil || before == "" {
		t.Fatalf("Hash = %q %v", before, err)
	}
	r.write("docs/guide.md", "x\n")
	after, err := a.Hash(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if paths, err := a.TreeDiff(ctx, before, after); err != nil || !slices.Equal(paths, []string{"docs/guide.md"}) {
		t.Fatalf("TreeDiff = %q %v", paths, err)
	}
	if _, err := a.TreeDiff(ctx, strings.Repeat("1", len(before)), after); err == nil {
		t.Fatal("TreeDiff of a missing tree")
	}
	if data, err := a.Read(ctx, "src/a.go"); err != nil || data != "package a\n" {
		t.Fatalf("Read = %q %v", data, err)
	}
	if _, err := a.Read(ctx, "missing.txt"); err == nil {
		t.Fatal("Read of a missing file")
	}
	if out, err := a.Run(ctx, []string{"git", "rev-parse", "--abbrev-ref", "HEAD"}); err != nil || out != "feature\n" {
		t.Fatalf("Run = %q %v", out, err)
	}
	if err := a.Append(ctx, state.Entry{Kind: state.KindObserved, Outcome: state.OutcomeFinished,
		Key: "polix:reviewer"}); err != nil {
		t.Fatal(err)
	}
	if err := a.Append(ctx, state.Entry{Kind: state.KindObserved, Outcome: state.OutcomePassed}); err == nil {
		t.Fatal("an observed passed without hash is appended")
	}
	s, err := a.LoadState(ctx)
	if err != nil || len(s.Entries) != 1 || s.Entries[0].At == "" ||
		!strings.Contains(r.log(), `"key":"polix:reviewer","outcome":"finished"`) {
		t.Fatalf("LoadState = %+v %v", s, err)
	}
	if reals := a.Real(ctx, []string{r.dir}); len(reals) != 1 || reals[0] == "" {
		t.Fatalf("Real = %q", reals)
	}
	if rules, err := a.LoadRules(ctx, "ai/rules"); err != nil || rules != nil {
		t.Fatalf("no ai/rules: %v %v", rules, err)
	}
}

// With rules.path the rules come from that folder, not from ai/rules/, and the context names it.
func TestRulesOfRulesPath(t *testing.T) {
	r := newRepo(t, "main")
	r.write("polix.yaml", "version: 1\nrules: { path: governance/rules }\non:\n  pre_edit:\n    - action: { inject: { rules: \"{path}\" } }\n")
	r.write("ai/rules/old.yaml", "rules:\n  - id: OLD-1\n    description: Old rule.\n")
	r.write("governance/rules/style.yaml", "rules:\n  - id: NEW-1\n    description: New rule.\n")
	r.write("src/a.txt", "x\n")
	_, stdout, _ := r.edit("src/a.txt", "")
	if !strings.Contains(stdout, "NEW-1: New rule.") || !strings.Contains(stdout, "(governance/rules/)") ||
		strings.Contains(stdout, "OLD-1") {
		t.Fatalf("the rules of rules.path: %q", stdout)
	}
}

// A rule that is not valid does not hide the others: its check still runs and every rule is injected, as before.
func TestInvalidRuleNextToValid(t *testing.T) {
	r := newRepo(t, "main")
	r.write("polix.yaml", `version: 1
on:
  pre_edit:
    - action: { inject: { rules: "{path}" } }
  stop:
    - action: { run: { rules: changed } }
`)
	r.write("ai/rules/style.yaml", `rules:
  - id: BAD-1
    scope: ["src/**"]
    deterministic: true
    check: checks/bad.ref
  - id: GOOD-1
    description: Good rule.
    scope: ["src/**"]
`)
	r.write("src/a.txt", "x\n")
	_, stdout, _ := r.edit("src/a.txt", "")
	if !strings.Contains(stdout, "BAD-1") || !strings.Contains(stdout, "GOOD-1: Good rule.") {
		t.Fatalf("every rule is injected: %q", stdout)
	}
	code, _, stderr := r.stop()
	expect(t, "stop with the check of the invalid rule failing", code, 2, stderr)
	if !strings.Contains(stderr, "- BAD-1 (checks/bad.ref)") {
		t.Fatalf("the check of the invalid rule runs: %q", stderr)
	}
}
