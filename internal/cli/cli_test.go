package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/mlophez/polix-ai/internal/engine"
)

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
	cmd := exec.Command("git", append([]string{"-c", "user.name=test", "-c", "user.email=test@example.com"}, args...)...)
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
	var out, errb bytes.Buffer
	code = Run([]string{"hook", agent}, bytes.NewReader(data), &out, &errb)
	return code, out.String(), errb.String()
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

// run calls polix with args on the repository.
func (r *repo) run(args ...string) (code int, stdout, stderr string) {
	var out, errb bytes.Buffer
	code = Run(append(args, "--dir", r.dir), nil, &out, &errb)
	return code, out.String(), errb.String()
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
		expectCode(r.t, fmt.Sprintf("stop %d of %s", i+1, agentType), code, w, stderr)
	}
}

func expectCode(t *testing.T, what string, got, want int, stderr string) {
	t.Helper()
	if got != want {
		t.Fatalf("%s: exit %d, want %d (stderr: %s)", what, got, want, stderr)
	}
}

const reviewOnStop = `version: 1
on:
  subagent_stop:
    - if: { actor: reviewer }
      record: reviewed
  stop:
    - if: { changed: "**" }
      unless: { done: reviewed }
      require: "Lanza el subagente reviewer sobre el diff actual"
`

// Example 1 of the design: a review is required before stopping, and a new edit requires a new one.
func TestReviewOnStop(t *testing.T) {
	r := newRepo(t, "main")
	r.write("polix.yaml", reviewOnStop)
	r.git("add", "--all")
	r.git("commit", "--quiet", "-m", "workflow")

	code, _, stderr := r.stop()
	expectCode(t, "stop without changes", code, 0, stderr)

	r.write("src/app.go", "package app\n")
	code, _, stderr = r.stop()
	expectCode(t, "stop with unreviewed changes", code, 2, stderr)
	if !strings.Contains(stderr, "reviewer") {
		t.Fatalf("the reason should ask for the reviewer: %q", stderr)
	}

	code, _, stderr = r.subagentStop("reviewer")
	expectCode(t, "reviewer stops", code, 0, stderr)
	code, _, stderr = r.stop()
	expectCode(t, "stop after the review", code, 0, stderr)

	r.write("src/app.go", "package app\n\nfunc A() {}\n")
	code, _, stderr = r.stop()
	expectCode(t, "stop after a new edit", code, 2, stderr)
}

// The loop guard lets the agent stop after max_stop_blocks blocked stops on the same content.
func TestStopLoopGuard(t *testing.T) {
	r := newRepo(t, "main")
	r.write("polix.yaml", "version: 1\nmax_stop_blocks: 2\n"+strings.TrimPrefix(reviewOnStop, "version: 1\n"))
	r.write("src/app.go", "package app\n")
	for i, want := range []int{2, 2, 0} {
		code, _, stderr := r.stop()
		expectCode(t, "stop "+string(rune('1'+i)), code, want, stderr)
	}
}

const logaltyFlow = `version: 1
vars:
  key: { branch: "^([A-Z][A-Z0-9]+-[0-9]+)" }
on:
  pre_edit:
    - unless: { var: key }
      deny: "No hay ticket"
    - if: { path: "ai/plans/{key}-*.md" }
      allow: true
    - unless: { file: "ai/plans/{key}-*.md", contains: "Ticket: {key}" }
      deny: "No hay plan"
    - unless: { actor: builder }
      deny: "Solo el builder edita"
    - context: { rules: "{path}" }
  post_command:
    - if: { command: "just test", exit: 0 }
      record: tested
  subagent_stop:
    - if: { actor: reviewer }
      record: reviewed
  pre_commit:
    - unless: { done: tested }
      deny: "Faltan los tests"
    - unless: { done: reviewed }
      deny: "Falta la revisión"
`

const archRules = `rules:
  - id: ARCH-1
    description: When in src/domain/, never import infrastructure.
    scope: ["src/domain/**"]
    deterministic: false
`

// Example 2 of the design: ticket, plan, builder, rules as context, tests and review before committing.
func TestLogaltyFlow(t *testing.T) {
	r := newRepo(t, "main")
	r.write("polix.yaml", logaltyFlow)
	r.write("ai/rules/architecture.yaml", archRules)
	r.git("add", "--all")
	r.git("commit", "--quiet", "-m", "workflow")

	code, _, stderr := r.edit("src/domain/order.go", "builder")
	expectCode(t, "edit without a ticket branch", code, 2, stderr)
	if !strings.Contains(stderr, "No hay ticket") {
		t.Fatalf("reason: %q", stderr)
	}

	r.git("checkout", "--quiet", "-b", "PLAT-7-orders")
	code, _, stderr = r.edit("ai/plans/PLAT-7-orders.md", "")
	expectCode(t, "the session writes the plan", code, 0, stderr)
	code, _, stderr = r.edit("src/domain/order.go", "builder")
	expectCode(t, "edit without a plan", code, 2, stderr)

	r.write("ai/plans/PLAT-7-orders.md", "# Plan\n\nTicket: PLAT-7\n")
	code, _, stderr = r.edit("src/domain/order.go", "")
	expectCode(t, "the session edits code", code, 2, stderr)

	code, stdout, stderr := r.edit("src/domain/order.go", "builder")
	expectCode(t, "the builder edits code", code, 0, stderr)
	if !strings.Contains(stdout, "ARCH-1") || !strings.Contains(stdout, "additionalContext") {
		t.Fatalf("the rules of the path should be injected as context: %q", stdout)
	}
	_, stdout, _ = r.edit("src/domain/order.go", "builder")
	if strings.Contains(stdout, "ARCH-1") {
		t.Fatalf("a rule is injected once per session: %q", stdout)
	}

	r.write("src/domain/order.go", "package domain\n")
	code, _, stderr = r.bash("git commit -m 'PLAT-7 orders'", false)
	expectCode(t, "commit without tests", code, 2, stderr)

	code, _, stderr = r.bash("just test", true)
	expectCode(t, "tests pass", code, 0, stderr)
	code, _, stderr = r.bash("git commit -m 'PLAT-7 orders'", false)
	expectCode(t, "commit without review", code, 2, stderr)
	if !strings.Contains(stderr, "Falta la revisión") {
		t.Fatalf("reason: %q", stderr)
	}

	r.subagentStop("reviewer")
	code, _, stderr = r.bash("cd . && git commit -m 'PLAT-7 orders'", false)
	expectCode(t, "commit with tests and review", code, 0, stderr)
}

// A command chained with others still matches, but its exit code is only trusted when its failure propagates.
func TestChainedCommands(t *testing.T) {
	r := newRepo(t, "main")
	r.write("polix.yaml", `version: 1
on:
  post_command:
    - if: { command: "just test*", exit: 0 }
      record: tested
  pre_command:
    - if: { command: "git push*origin main*" }
      deny: "push a main"
  pre_commit:
    - unless: { done: tested }
      deny: "Faltan los tests"
`)
	r.write("src/a.go", "package a\n")

	r.bash("just test 2>&1 | tail -3", true)
	code, _, stderr := r.bash("git commit -m x", false)
	expectCode(t, "commit after piped tests", code, 2, stderr)

	r.bash("cd . && just test -k sync", true)
	code, _, stderr = r.bash("git commit -m x", false)
	expectCode(t, "commit after chained tests", code, 0, stderr)

	code, _, stderr = r.bash("git fetch && git push origin main", false)
	expectCode(t, "chained push to main", code, 2, stderr)
	code, _, stderr = r.bash(`echo "git push origin main"`, false)
	expectCode(t, "the text of a push inside quotes", code, 0, stderr)
}

// Built-in protections apply whatever the workflow says, even without one.
func TestProtections(t *testing.T) {
	r := newRepo(t, "main")

	code, _, stderr := r.edit("ai/rules/security.yaml", "builder")
	expectCode(t, "a subagent edits the rules", code, 2, stderr)
	code, _, stderr = r.edit("AGENTS.md", "reviewer")
	expectCode(t, "a subagent edits AGENTS.md", code, 2, stderr)
	code, _, stderr = r.edit("ai/rules/security.yaml", "")
	expectCode(t, "the session edits the rules (add-rule)", code, 0, stderr)
	code, _, stderr = r.edit(".git/polix/log.jsonl", "")
	expectCode(t, "anyone edits the state", code, 2, stderr)
	code, _, stderr = r.bash("rm .git/polix/log.jsonl", false)
	expectCode(t, "a command deletes the state", code, 2, stderr)
	code, _, stderr = r.bash("echo x > .git/polix/log.jsonl", false)
	expectCode(t, "a command overwrites the state", code, 2, stderr)
	code, _, stderr = r.bash("jq . .git/polix/log.jsonl", false)
	expectCode(t, "a command reads the state", code, 0, stderr)
}

// Edits outside the repository or of ignored files are not part of the change.
func TestOutsideAndIgnored(t *testing.T) {
	r := newRepo(t, "main")
	r.write("polix.yaml", "version: 1\non:\n  pre_edit:\n    - deny: \"bloqueado\"\n")
	r.write(".gitignore", "build/\n")

	code, _, stderr := r.edit("src/a.go", "")
	expectCode(t, "edit in the repository", code, 2, stderr)
	code, _, stderr = r.edit("build/out.txt", "")
	expectCode(t, "edit of an ignored file", code, 0, stderr)
	in := map[string]any{"hook_event_name": "PreToolUse", "tool_name": "Write",
		"tool_input": map[string]any{"file_path": filepath.Join(t.TempDir(), "notes.md")}}
	code, _, stderr = r.hook("claude", in)
	expectCode(t, "edit outside the repository", code, 0, stderr)
}

// warn turns a block into a warning in the context, audit only logs it.
func TestModes(t *testing.T) {
	r := newRepo(t, "main")
	r.write("polix.yaml", `version: 1
on:
  pre_edit:
    - if: { path: "a.txt" }
      mode: warn
      deny: "cuidado con a.txt"
    - if: { path: "b.txt" }
      mode: audit
      deny: "b.txt"
`)
	code, stdout, stderr := r.edit("a.txt", "")
	expectCode(t, "warn", code, 0, stderr)
	if !strings.Contains(stdout, "cuidado con a.txt") {
		t.Fatalf("warn should add context: %q", stdout)
	}
	code, stdout, stderr = r.edit("b.txt", "")
	expectCode(t, "audit", code, 0, stderr)
	if stdout != "" {
		t.Fatalf("audit should be silent: %q", stdout)
	}
	log, _ := os.ReadFile(filepath.Join(r.dir, ".git", "polix", "log.jsonl"))
	if !strings.Contains(string(log), `"outcome":"audit"`) {
		t.Fatalf("audit should be logged: %s", log)
	}
}

// run: { rules: changed } on stop runs the checks of the rules of the changed files and names the rule.
func TestRuleChecksOnStop(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the test check is a shell script")
	}
	r := newRepo(t, "main")
	r.write("polix.yaml", "version: 1\non:\n  stop:\n    - if: { changed: \"**\" }\n      run: { rules: changed }\n")
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
	expectCode(t, "stop with the rule passing", code, 0, stderr)
	r.write("src/a.txt", "TODO\n")
	code, _, stderr = r.stop()
	expectCode(t, "stop with the rule failing", code, 2, stderr)
	if !strings.Contains(stderr, "STYLE-1") || !strings.Contains(stderr, "TODO found") {
		t.Fatalf("the reason should name the rule and its output: %q", stderr)
	}

	var out, errb bytes.Buffer
	if code := Run([]string{"rules", "check", "--dir", r.dir}, nil, &out, &errb); code != 1 {
		t.Fatalf("rules check should fail: %d %s", code, errb.String())
	}
}

// An invalid workflow blocks edits and commands but never a stop.
func TestInvalidWorkflow(t *testing.T) {
	r := newRepo(t, "main")
	r.write("polix.yaml", "version: 1\non:\n  pre_edit:\n    - if: { pth: \"x\" }\n      deny: \"x\"\n")

	code, _, stderr := r.edit("src/a.go", "")
	expectCode(t, "edit with an invalid workflow", code, 2, stderr)
	if !strings.Contains(stderr, `unknown condition "pth"`) {
		t.Fatalf("reason: %q", stderr)
	}
	code, _, stderr = r.stop()
	expectCode(t, "stop with an invalid workflow", code, 0, stderr)

	var out, errb bytes.Buffer
	if code := Run([]string{"check", "--dir", r.dir}, nil, &out, &errb); code != 1 {
		t.Fatalf("check should fail: %d", code)
	}
}

// not holds unless every nested condition holds.
func TestNot(t *testing.T) {
	r := newRepo(t, "PLAT-1-x")
	r.write("polix.yaml", `version: 1
on:
  pre_command:
    - if: { command: "terraform apply*", not: { branch: "^main$" } }
      deny: "apply solo desde main"
    - if: { command: "deploy*", not: { branch: "^PLAT-", file: "ai/plans/*.md" } }
      deny: "deploy sin plan"
`)
	var out, errb bytes.Buffer
	if code := Run([]string{"check", "--dir", r.dir}, nil, &out, &errb); code != 0 {
		t.Fatalf("check: %d %s", code, errb.String())
	}
	code, _, stderr := r.bash("terraform apply", false)
	expectCode(t, "apply outside main", code, 2, stderr)
	code, _, stderr = r.bash("deploy", false)
	expectCode(t, "deploy without a plan: only one nested condition holds", code, 2, stderr)
	r.write("ai/plans/PLAT-1-x.md", "Ticket: PLAT-1\n")
	code, _, stderr = r.bash("deploy", false)
	expectCode(t, "deploy with a plan: every nested condition holds", code, 0, stderr)

	r.git("checkout", "--quiet", "main")
	code, _, stderr = r.bash("terraform apply", false)
	expectCode(t, "apply on main", code, 0, stderr)
}

// The example workflow of the repository is valid.
func TestExampleIsValid(t *testing.T) {
	r := newRepo(t, "main")
	data, err := os.ReadFile(filepath.Join("..", "..", "examples", "polix.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	r.write("polix.yaml", string(data))
	var out, errb bytes.Buffer
	if code := Run([]string{"check", "--dir", r.dir}, nil, &out, &errb); code != 0 {
		t.Fatalf("check: %d %s", code, errb.String())
	}
}

// Codex: apply_patch paths and commands given as a list of arguments.
func TestCodex(t *testing.T) {
	r := newRepo(t, "main")
	r.write("polix.yaml", `version: 1
on:
  pre_edit:
    - if: { path: "src/**" }
      deny: "src bloqueado"
  pre_command:
    - if: { command: "terraform apply*" }
      deny: "apply solo desde el pipeline"
`)
	patch := "*** Begin Patch\n*** Update File: src/a.go\n@@\n-a\n+b\n*** End Patch\n"
	code, _, stderr := r.hook("codex", map[string]any{"hook_event_name": "PreToolUse", "tool_name": "apply_patch",
		"tool_input": map[string]any{"command": []any{"apply_patch", patch}}})
	expectCode(t, "codex patches src", code, 2, stderr)
	code, _, stderr = r.hook("codex", map[string]any{"hook_event_name": "PreToolUse", "tool_name": "shell",
		"tool_input": map[string]any{"command": []any{"terraform", "apply", "-auto-approve"}}})
	expectCode(t, "codex runs terraform apply", code, 2, stderr)
	code, _, stderr = r.hook("codex", map[string]any{"hook_event_name": "PreToolUse", "tool_name": "shell",
		"tool_input": map[string]any{"command": []any{"terraform", "plan"}}})
	expectCode(t, "codex runs terraform plan", code, 0, stderr)
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
	} {
		code, _, stderr := r.bash(c, false)
		expectCode(t, c, code, 0, stderr)
	}
	for _, c := range []string{
		"rm -rf .git/polix",
		"cp /tmp/x ./.git/polix/log.jsonl",
		"echo x >> " + filepath.Join(r.dir, ".git", "polix", "log.jsonl"),
		"cd .. && rm repo/.git/worktrees/w1/polix/log.jsonl",
	} {
		code, _, stderr := r.bash(c, false)
		expectCode(t, c, code, 2, stderr)
	}
}

// A subagent cannot get around the protection of the context, the rules and the workflow through a command.
func TestSubagentCommandsOnProtectedFiles(t *testing.T) {
	r := newRepo(t, "main")
	for _, c := range []string{"sed -i s/a/b/ polix.yaml", "echo x >> ai/rules/a.yaml", "rm CLAUDE.md", "mv x AGENTS.md"} {
		code, _, stderr := r.bashAs(c, false, "builder")
		expectCode(t, "builder: "+c, code, 2, stderr)
		code, _, stderr = r.bashAs(c, false, "")
		expectCode(t, "session: "+c, code, 0, stderr)
	}
	code, _, stderr := r.bashAs("cat CLAUDE.md ai/rules/a.yaml", false, "builder")
	expectCode(t, "builder reads protected files", code, 0, stderr)
}

// The loop guard covers every block of a stop, not only require: a deny or a failing run cannot trap the agent.
func TestStopLoopGuardOnAnyBlock(t *testing.T) {
	r := newRepo(t, "main")
	r.write("polix.yaml", `version: 1
on:
  stop:
    - deny: "nunca paras"
  subagent_stop:
    - run: { command: [git, rev-parse, --verify, --quiet, no-such-ref] }
`)
	r.stops("", 2, 2, 2, 0)
	r.stops("builder", 2, 2, 2, 0)
}

// Each entry has its own guard, and subagents do not use up the guard of the main session.
func TestStopLoopGuardPerEntry(t *testing.T) {
	r := newRepo(t, "main")
	r.write("polix.yaml", `version: 1
on:
  stop:
    - require: "A"
    - require: "B"
  subagent_stop:
    - require: "C"
`)
	r.stops("builder", 2, 2, 2, 0)
	r.stops("", 2, 2, 2, 0)
	code, _, stderr := r.stop()
	if !strings.Contains(stderr, "A") || !strings.Contains(stderr, "B") {
		t.Fatalf("both requires should hit the guard together: %d %q", code, stderr)
	}
}

// An invalid workflow fails closed, but the main session can still fix it, and the protections still apply.
func TestInvalidWorkflowCanBeFixed(t *testing.T) {
	r := newRepo(t, "main")
	r.write("polix.yaml", "version: 1\non:\n  pre_edit:\n    - deny: x\n      foo: 1\n")
	r.write(".gitignore", "build/\n")

	code, _, stderr := r.edit("polix.yaml", "")
	expectCode(t, "the session edits the workflow", code, 0, stderr)
	code, _, stderr = r.edit("polix.yaml", "builder")
	expectCode(t, "a subagent edits the workflow", code, 2, stderr)
	code, _, stderr = r.edit("src/a.go", "")
	expectCode(t, "the session edits code", code, 2, stderr)
	code, _, stderr = r.edit("build/out.txt", "")
	expectCode(t, "edit of an ignored file", code, 0, stderr)
	code, _, stderr = r.hook("claude", map[string]any{"hook_event_name": "PreToolUse", "tool_name": "Write",
		"tool_input": map[string]any{"file_path": filepath.Join(t.TempDir(), "notes.md")}})
	expectCode(t, "edit outside the repository", code, 0, stderr)
	code, _, stderr = r.edit(".git/polix/log.jsonl", "")
	expectCode(t, "edit of the state", code, 2, stderr)
	if !strings.Contains(stderr, "estado de polix") {
		t.Fatalf("the protection should answer, not the invalid workflow: %q", stderr)
	}
	for c, want := range map[string]int{"polix check": 0, "cd . && polix check --dir .": 0, "ls": 2, "polix check && ls": 2} {
		code, _, stderr = r.bash(c, false)
		expectCode(t, c, code, want, stderr)
	}
	code, _, stderr = r.bash("git commit -m x", false)
	expectCode(t, "commit", code, 2, stderr)
	if n := strings.Count(stderr, "no es válido"); n != 1 {
		t.Fatalf("the command and its commit give one message, got %d: %q", n, stderr)
	}
}

// Codex gives relative paths in apply_patch: they are resolved from the cwd of the call, not of the process.
func TestCodexRelativeStatePath(t *testing.T) {
	r := newRepo(t, "main")
	patch := "*** Begin Patch\n*** Update File: .git/polix/log.jsonl\n@@\n-a\n+b\n*** End Patch\n"
	code, _, stderr := r.hook("codex", map[string]any{"hook_event_name": "PreToolUse", "tool_name": "apply_patch",
		"tool_input": map[string]any{"input": patch}})
	expectCode(t, "codex patches the state", code, 2, stderr)
}

// Paths that are not plain ASCII match globs.
func TestNonASCIIChangedFiles(t *testing.T) {
	r := newRepo(t, "main")
	r.write("polix.yaml", "version: 1\non:\n  stop:\n    - if: { changed: \"*.md\" }\n      require: \"md cambiado\"\n")
	r.git("add", "--all")
	r.git("commit", "--quiet", "-m", "workflow")
	r.write("diseño/año.md", "x\n")
	code, _, stderr := r.stop()
	expectCode(t, "stop with a changed año.md", code, 2, stderr)
}

// check without a workflow says so once, without claiming the file is valid.
func TestCheckWithoutWorkflow(t *testing.T) {
	r := newRepo(t, "main")
	code, stdout, stderr := r.run("check")
	expectCode(t, "check", code, 0, stderr)
	if want := "ok: no polix.yaml"; !strings.HasPrefix(stdout, want) || strings.Count(stdout, "\n") != 1 {
		t.Fatalf("want one line starting with %q, got %q", want, stdout)
	}
	code, _, stderr = r.run("graph")
	expectCode(t, "graph", code, 1, stderr)
	if !strings.Contains(stderr, "polix.yaml not found") || strings.Contains(stderr, "<nil>") {
		t.Fatalf("graph error: %q", stderr)
	}
}

// rules check --changed compares with the base of polix.yaml unless --base says otherwise.
func TestRulesCheckBase(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the test check is a shell script")
	}
	r := newRepo(t, "main")
	r.write("ai/rules/style.yaml", "rules:\n  - id: S-1\n    description: d\n    scope: [\"src/**\"]\n    deterministic: true\n    check: checks/fail.sh\n")
	r.write("checks/fail.sh", "#!/bin/sh\nexit 1\n")
	r.git("add", "--all")
	r.git("commit", "--quiet", "-m", "rules")
	r.git("checkout", "--quiet", "-b", "develop")
	r.write("src/a.txt", "x\n")
	r.git("add", "--all")
	r.git("commit", "--quiet", "-m", "develop")
	r.git("checkout", "--quiet", "-b", "feature")

	code, stdout, _ := r.run("rules", "check", "--changed")
	if code != 1 || !strings.Contains(stdout, "1 checks, 1 failed") {
		t.Fatalf("without a workflow the base is main: %d %q", code, stdout)
	}
	r.write("polix.yaml", "version: 1\nbase: develop\n")
	code, stdout, _ = r.run("rules", "check", "--changed")
	if code != 0 || !strings.Contains(stdout, "0 checks") {
		t.Fatalf("the base of polix.yaml is develop: %d %q", code, stdout)
	}
	code, stdout, _ = r.run("rules", "check", "--changed", "--base", "main")
	if code != 1 {
		t.Fatalf("--base overrides polix.yaml: %d %q", code, stdout)
	}
}

// The example settings give the hooks more time than the run budget of a call, so the agent never kills a
// hook in the middle of a gate.
func TestExampleHookTimeouts(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "examples", "claude-settings.json"))
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
				if strings.Contains(h.Command, "polix hook claude") && h.Timeout <= engine.RunBudget.Seconds() {
					t.Errorf("%s: timeout %vs, want more than the run budget (%v)", event, h.Timeout, engine.RunBudget)
				}
			}
		}
	}
}

// A decision does not compute the content hash just to log it: a plain deny costs no git tree.
func TestDecisionsSkipTheHash(t *testing.T) {
	r := newRepo(t, "main")
	r.write("polix.yaml", "version: 1\non:\n  pre_edit:\n    - deny: x\n")
	r.edit("src/a.go", "")
	data, _ := os.ReadFile(filepath.Join(r.dir, ".git", "polix", "log.jsonl"))
	if !strings.Contains(string(data), `"action":"deny","line":4`) || strings.Contains(string(data), `"hash"`) {
		t.Fatalf("log: %s", data)
	}
}
