package hook

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

	"github.com/mlophez/polix-ai/internal/state"
	"github.com/mlophez/polix-ai/pkg/build"
	"github.com/mlophez/polix-ai/pkg/cli"
)

// These tests run the command polix hook end to end, adapter included, against a temporary repository.

// answerHookRepo is a throwaway git repository for one test.
type answerHookRepo struct {
	t   *testing.T
	dir string
}

// answerHookNewRepo creates a repository with one commit on main and switches to branch.
func answerHookNewRepo(t *testing.T, branch string) *answerHookRepo {
	t.Helper()
	r := &answerHookRepo{t: t, dir: t.TempDir()}
	r.git("init", "--quiet", "--initial-branch=main")
	r.write("README.md", "test\n")
	r.git("add", "--all")
	r.git("commit", "--quiet", "-m", "init")
	if branch != "main" {
		r.git("checkout", "--quiet", "-b", branch)
	}
	return r
}

func (r *answerHookRepo) git(args ...string) {
	r.t.Helper()
	cmd := exec.Command("git", append([]string{"-c", "user.name=test", "-c", "user.email=test@example.com",
		"-c", "commit.gpgsign=false"}, args...)...)
	cmd.Dir = r.dir
	cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL="+os.DevNull, "GIT_CONFIG_SYSTEM="+os.DevNull)
	if out, err := cmd.CombinedOutput(); err != nil {
		r.t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func (r *answerHookRepo) write(rel, content string) {
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
func (r *answerHookRepo) log() string {
	data, _ := os.ReadFile(filepath.Join(r.dir, ".git", "polix", "log.jsonl"))
	return string(data)
}

// answerHookCall runs `polix hook` with args and stdin through the command of the use case, as cmd/polix does.
func answerHookCall(args []string, stdin []byte) (code int, stdout, stderr string) {
	root := cli.Root("polix", "", "", "")
	root.AddCommand(AnswerHookCommand())
	var out, errb bytes.Buffer
	code = cli.Run(root, append([]string{"hook"}, args...), bytes.NewReader(stdin), &out, &errb)
	return code, out.String(), errb.String()
}

// hook calls `polix hook <agent>` with input, as the agent would.
func (r *answerHookRepo) hook(agent string, input map[string]any) (code int, stdout, stderr string) {
	r.t.Helper()
	if _, ok := input["cwd"]; !ok {
		input["cwd"] = r.dir
	}
	if _, ok := input["session_id"]; !ok {
		input["session_id"] = "s1"
	}
	data, _ := json.Marshal(input)
	return answerHookCall([]string{agent}, data)
}

func (r *answerHookRepo) edit(path, agentType string) (int, string, string) {
	in := map[string]any{"hook_event_name": "PreToolUse", "tool_name": "Edit",
		"tool_input": map[string]any{"file_path": filepath.Join(r.dir, filepath.FromSlash(path))}}
	if agentType != "" {
		in["agent_type"] = agentType
	}
	return r.hook("claude", in)
}

func (r *answerHookRepo) stop() (int, string, string) {
	return r.hook("claude", map[string]any{"hook_event_name": "Stop"})
}

func (r *answerHookRepo) subagentStop(agentType string) (int, string, string) {
	return r.hook("claude", map[string]any{"hook_event_name": "SubagentStop", "agent_type": agentType})
}

func (r *answerHookRepo) bash(command string, post bool) (int, string, string) {
	return r.bashAs(command, post, "")
}

func (r *answerHookRepo) bashAs(command string, post bool, agentType string) (int, string, string) {
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
func (r *answerHookRepo) stops(agentType string, want ...int) {
	r.t.Helper()
	for i, w := range want {
		var code int
		var stderr string
		if agentType == "" {
			code, _, stderr = r.stop()
		} else {
			code, _, stderr = r.subagentStop(agentType)
		}
		answerHookExpect(r.t, fmt.Sprintf("stop %d of %s", i+1, agentType), code, w, stderr)
	}
}

func answerHookExpect(t *testing.T, what string, got, want int, stderr string) {
	t.Helper()
	if got != want {
		t.Fatalf("%s: exit %d, want %d (stderr: %s)", what, got, want, stderr)
	}
}

// answerHookSkipFileModes skips a test that relies on file modes denying access: they do not on Windows nor as
// root.
func answerHookSkipFileModes(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("file modes do not deny access here")
	}
}

const answerHookReviewOnStop = `version: 1
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
func TestAnswerHookReviewOnStop(t *testing.T) {
	r := answerHookNewRepo(t, "main")
	r.write("polix.yaml", answerHookReviewOnStop)
	r.git("add", "--all")
	r.git("commit", "--quiet", "-m", "workflow")

	code, _, stderr := r.stop()
	answerHookExpect(t, "stop without changes", code, 0, stderr)

	r.write("src/app.go", "package app\n")
	code, _, stderr = r.stop()
	answerHookExpect(t, "stop with unreviewed changes", code, 2, stderr)
	if !strings.Contains(stderr, "reviewer") {
		t.Fatalf("the reason should ask for the reviewer: %q", stderr)
	}

	code, _, stderr = r.subagentStop("reviewer")
	answerHookExpect(t, "reviewer stops", code, 0, stderr)
	code, _, stderr = r.stop()
	answerHookExpect(t, "stop after the review", code, 0, stderr)

	r.write("src/app.go", "package app\n\nfunc A() {}\n")
	code, _, stderr = r.stop()
	answerHookExpect(t, "stop after a new edit", code, 2, stderr)
}

// The loop guard lets the agent stop after max_stop_blocks blocked stops on the same content.
func TestAnswerHookStopLoopGuard(t *testing.T) {
	r := answerHookNewRepo(t, "main")
	r.write("polix.yaml", "version: 1\nmax_stop_blocks: 2\n"+strings.TrimPrefix(answerHookReviewOnStop, "version: 1\n"))
	r.write("src/app.go", "package app\n")
	for i, want := range []int{2, 2, 0} {
		code, _, stderr := r.stop()
		answerHookExpect(t, "stop "+string(rune('1'+i)), code, want, stderr)
	}
}

const answerHookLogaltyFlow = `version: 1
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

const answerHookArchRules = `rules:
  - id: ARCH-1
    description: When in src/domain/, never import infrastructure.
    scope: ["src/domain/**"]
    deterministic: false
`

// Example 2 of the design: ticket, plan, builder, rules as context, tests and review before committing.
func TestAnswerHookLogaltyFlow(t *testing.T) {
	r := answerHookNewRepo(t, "main")
	r.write("polix.yaml", answerHookLogaltyFlow)
	r.write("ai/rules/architecture.yaml", answerHookArchRules)
	r.git("add", "--all")
	r.git("commit", "--quiet", "-m", "workflow")

	code, _, stderr := r.edit("src/domain/order.go", "builder")
	answerHookExpect(t, "edit without a ticket branch", code, 2, stderr)
	if !strings.Contains(stderr, "No hay ticket") {
		t.Fatalf("reason: %q", stderr)
	}

	r.git("checkout", "--quiet", "-b", "PLAT-7-orders")
	code, _, stderr = r.edit("ai/plans/PLAT-7-orders.md", "")
	answerHookExpect(t, "the session writes the plan", code, 0, stderr)
	code, _, stderr = r.edit("src/domain/order.go", "builder")
	answerHookExpect(t, "edit without a plan", code, 2, stderr)

	r.write("ai/plans/PLAT-7-orders.md", "# Plan\n\nTicket: PLAT-7\n")
	code, _, stderr = r.edit("src/domain/order.go", "")
	answerHookExpect(t, "the session edits code", code, 2, stderr)

	code, stdout, stderr := r.edit("src/domain/order.go", "builder")
	answerHookExpect(t, "the builder edits code", code, 0, stderr)
	if !strings.Contains(stdout, "ARCH-1") || !strings.Contains(stdout, "additionalContext") {
		t.Fatalf("the rules of the path should be injected as context: %q", stdout)
	}
	_, stdout, _ = r.edit("src/domain/order.go", "builder")
	if strings.Contains(stdout, "ARCH-1") {
		t.Fatalf("a rule is injected once per session: %q", stdout)
	}

	r.write("src/domain/order.go", "package domain\n")
	code, _, stderr = r.bash("git commit -m 'PLAT-7 orders'", false)
	answerHookExpect(t, "commit without tests", code, 2, stderr)

	code, _, stderr = r.bash("just test", true)
	answerHookExpect(t, "tests pass", code, 0, stderr)
	code, _, stderr = r.bash("git commit -m 'PLAT-7 orders'", false)
	answerHookExpect(t, "commit without review", code, 2, stderr)
	if !strings.Contains(stderr, "Falta la revisión") {
		t.Fatalf("reason: %q", stderr)
	}

	r.subagentStop("reviewer")
	code, _, stderr = r.bash("cd . && git commit -m 'PLAT-7 orders'", false)
	answerHookExpect(t, "commit with tests and review", code, 0, stderr)
}

// A command chained with others still matches, but its exit code is only trusted when its failure propagates.
func TestAnswerHookChainedCommands(t *testing.T) {
	r := answerHookNewRepo(t, "main")
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
	answerHookExpect(t, "commit after piped tests", code, 2, stderr)

	r.bash("cd . && just test -k sync", true)
	code, _, stderr = r.bash("git commit -m x", false)
	answerHookExpect(t, "commit after chained tests", code, 0, stderr)

	code, _, stderr = r.bash("git fetch && git push origin main", false)
	answerHookExpect(t, "chained push to main", code, 2, stderr)
	code, _, stderr = r.bash(`echo "git push origin main"`, false)
	answerHookExpect(t, "the text of a push inside quotes", code, 0, stderr)
}

// The protection of the state applies whatever the workflow says, even without one; the context, the rules and
// the workflow are not protected unless the workflow says so.
func TestAnswerHookProtections(t *testing.T) {
	r := answerHookNewRepo(t, "main")

	code, _, stderr := r.edit("ai/rules/security.yaml", "builder")
	answerHookExpect(t, "a subagent edits the rules", code, 0, stderr)
	code, _, stderr = r.edit("polix.yaml", "builder")
	answerHookExpect(t, "a subagent edits the workflow", code, 0, stderr)
	code, _, stderr = r.edit("AGENTS.md", "reviewer")
	answerHookExpect(t, "a subagent edits AGENTS.md", code, 0, stderr)
	code, _, stderr = r.edit(".git/polix/log.jsonl", "")
	answerHookExpect(t, "anyone edits the state", code, 2, stderr)
	code, _, stderr = r.bash("rm .git/polix/log.jsonl", false)
	answerHookExpect(t, "a command deletes the state", code, 2, stderr)
	code, _, stderr = r.bash("echo x > .git/polix/log.jsonl", false)
	answerHookExpect(t, "a command overwrites the state", code, 2, stderr)
	code, _, stderr = r.bash("jq . .git/polix/log.jsonl", false)
	answerHookExpect(t, "a command reads the state", code, 0, stderr)
}

// Edits outside the repository or of ignored files are not part of the change.
func TestAnswerHookOutsideAndIgnored(t *testing.T) {
	r := answerHookNewRepo(t, "main")
	r.write("polix.yaml", "version: 1\non:\n  pre_edit:\n    - deny: \"bloqueado\"\n")
	r.write(".gitignore", "build/\n")

	code, _, stderr := r.edit("src/a.go", "")
	answerHookExpect(t, "edit in the repository", code, 2, stderr)
	code, _, stderr = r.edit("build/out.txt", "")
	answerHookExpect(t, "edit of an ignored file", code, 0, stderr)
	in := map[string]any{"hook_event_name": "PreToolUse", "tool_name": "Write",
		"tool_input": map[string]any{"file_path": filepath.Join(t.TempDir(), "notes.md")}}
	code, _, stderr = r.hook("claude", in)
	answerHookExpect(t, "edit outside the repository", code, 0, stderr)
}

// warn turns a block into a warning in the context, audit only logs it.
func TestAnswerHookModes(t *testing.T) {
	r := answerHookNewRepo(t, "main")
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
	answerHookExpect(t, "warn", code, 0, stderr)
	if !strings.Contains(stdout, "cuidado con a.txt") {
		t.Fatalf("warn should add context: %q", stdout)
	}
	code, stdout, stderr = r.edit("b.txt", "")
	answerHookExpect(t, "audit", code, 0, stderr)
	if stdout != "" {
		t.Fatalf("audit should be silent: %q", stdout)
	}
	log, _ := os.ReadFile(filepath.Join(r.dir, ".git", "polix", "log.jsonl"))
	if !strings.Contains(string(log), `"outcome":"audit"`) {
		t.Fatalf("audit should be logged: %s", log)
	}
}

// run: { rules: changed } on stop runs the checks of the rules of the changed files and names the rule.
func TestAnswerHookRuleChecksOnStop(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the test check is a shell script")
	}
	r := answerHookNewRepo(t, "main")
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
	answerHookExpect(t, "stop with the rule passing", code, 0, stderr)
	r.write("src/a.txt", "TODO\n")
	code, _, stderr = r.stop()
	answerHookExpect(t, "stop with the rule failing", code, 2, stderr)
	if !strings.Contains(stderr, "STYLE-1") || !strings.Contains(stderr, "TODO found") {
		t.Fatalf("the reason should name the rule and its output: %q", stderr)
	}
}

// An invalid workflow blocks edits and commands but never a stop.
func TestAnswerHookInvalidWorkflow(t *testing.T) {
	r := answerHookNewRepo(t, "main")
	r.write("polix.yaml", "version: 1\non:\n  pre_edit:\n    - if: { pth: \"x\" }\n      deny: \"x\"\n")

	code, _, stderr := r.edit("src/a.go", "")
	answerHookExpect(t, "edit with an invalid workflow", code, 2, stderr)
	if !strings.Contains(stderr, `unknown condition "pth"`) {
		t.Fatalf("reason: %q", stderr)
	}
	code, _, stderr = r.stop()
	answerHookExpect(t, "stop with an invalid workflow", code, 0, stderr)

}

// not holds unless every nested condition holds.
func TestAnswerHookNot(t *testing.T) {
	r := answerHookNewRepo(t, "PLAT-1-x")
	r.write("polix.yaml", `version: 1
on:
  pre_command:
    - if: { command: "terraform apply*", not: { branch: "^main$" } }
      deny: "apply solo desde main"
    - if: { command: "deploy*", not: { branch: "^PLAT-", file: "ai/plans/*.md" } }
      deny: "deploy sin plan"
`)
	code, _, stderr := r.bash("terraform apply", false)
	answerHookExpect(t, "apply outside main", code, 2, stderr)
	code, _, stderr = r.bash("deploy", false)
	answerHookExpect(t, "deploy without a plan: only one nested condition holds", code, 2, stderr)
	r.write("ai/plans/PLAT-1-x.md", "Ticket: PLAT-1\n")
	code, _, stderr = r.bash("deploy", false)
	answerHookExpect(t, "deploy with a plan: every nested condition holds", code, 0, stderr)

	r.git("checkout", "--quiet", "main")
	code, _, stderr = r.bash("terraform apply", false)
	answerHookExpect(t, "apply on main", code, 0, stderr)
}

// Codex: apply_patch paths and commands given as a list of arguments.
func TestAnswerHookCodex(t *testing.T) {
	r := answerHookNewRepo(t, "main")
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
	answerHookExpect(t, "codex patches src", code, 2, stderr)
	code, _, stderr = r.hook("codex", map[string]any{"hook_event_name": "PreToolUse", "tool_name": "shell",
		"tool_input": map[string]any{"command": []any{"terraform", "apply", "-auto-approve"}}})
	answerHookExpect(t, "codex runs terraform apply", code, 2, stderr)
	code, _, stderr = r.hook("codex", map[string]any{"hook_event_name": "PreToolUse", "tool_name": "shell",
		"tool_input": map[string]any{"command": []any{"terraform", "plan"}}})
	answerHookExpect(t, "codex runs terraform plan", code, 0, stderr)
}

// The state is found by its path, not by the name alone, which is also the name of the binary: commands that
// only mention polix are not taken for writes of the state.
func TestAnswerHookStateProtectionByPath(t *testing.T) {
	r := answerHookNewRepo(t, "main")
	for _, c := range []string{
		"go install ./cmd/polix",
		"cd ~/Code/polix-ai && just test 2>&1 | tail -3",
		"polix log > /tmp/out.txt",
		"cp build/polix ~/bin/",
		"cat .git/polix/log.jsonl >&2",
	} {
		code, _, stderr := r.bash(c, false)
		answerHookExpect(t, c, code, 0, stderr)
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
		answerHookExpect(t, c, code, 2, stderr)
	}
}

// On the systems whose file names ignore case, the state is found in any case.
func TestAnswerHookStateProtectionFoldsCase(t *testing.T) {
	if runtime.GOOS != "darwin" && runtime.GOOS != "windows" {
		t.Skip("file names keep their case on " + runtime.GOOS)
	}
	r := answerHookNewRepo(t, "main")
	code, _, stderr := r.edit(".git/POLIX/log.jsonl", "")
	answerHookExpect(t, "edit of the state in another case", code, 2, stderr)
	code, _, stderr = r.bash("rm -rf .GIT/Polix", false)
	answerHookExpect(t, "a command on the state in another case", code, 2, stderr)
}

// protected keeps every actor, the main session included, from writing a path, also an ignored one; reads pass.
func TestAnswerHookProtectedPaths(t *testing.T) {
	r := answerHookNewRepo(t, "main")
	r.write(".gitignore", ".env\n")
	r.write("polix.yaml", "version: 1\nprotected: [\".env\", \"AGENTS.md\", \"ai/rules/**\"]\n")
	for _, actor := range []string{"", "builder"} {
		code, _, stderr := r.edit(".env", actor)
		answerHookExpect(t, actor+" edits .env", code, 2, stderr)
		if !strings.Contains(stderr, ".env is protected in polix.yaml: only a person changes it.") {
			t.Fatalf("the reason says who changes it: %q", stderr)
		}
		code, _, stderr = r.bashAs("echo x >AGENTS.md", false, actor)
		answerHookExpect(t, actor+" writes AGENTS.md", code, 2, stderr)
		code, _, stderr = r.bashAs("cat a.txt>>AGENTS.md", false, actor)
		answerHookExpect(t, actor+" appends to AGENTS.md without spaces", code, 2, stderr)
		code, _, stderr = r.bashAs("cat AGENTS.md", false, actor)
		answerHookExpect(t, actor+" reads AGENTS.md", code, 0, stderr)
		for _, cmd := range []string{"rm -rf ai/rules", "rm -rf ai/rules/", "rm -rf ai", "mv ai/rules x", "rm -rf .",
			"rm -rf ./", "rm -rf *", "rm -rf ai/*", "rm -rf ai/r*", "rm -f ai/rules/*", "cat a.txt > ai/rules/x.yaml",
			"echo x >&AGENTS.md", `rm -rf "a>b" ai`} {
			code, _, stderr = r.bashAs(cmd, false, actor)
			answerHookExpect(t, actor+" runs "+cmd, code, 2, stderr)
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
			answerHookExpect(t, actor+" runs "+cmd, code, 0, stderr)
		}
		code, _, stderr = r.edit("README.md", actor)
		answerHookExpect(t, actor+" edits README.md", code, 0, stderr)
	}
	// Where file names ignore case, so does protected: .ENV is .env there.
	if runtime.GOOS == "darwin" || runtime.GOOS == "windows" {
		code, _, stderr := r.edit(".ENV", "")
		answerHookExpect(t, "edits .ENV", code, 2, stderr)
		code, _, stderr = r.bash("echo x > ./agents.md", false)
		answerHookExpect(t, "writes ./agents.md", code, 2, stderr)
	}
}

// A protected glob with a leading slash is anchored to the root: /polix.yaml keeps polix.yaml, not
// examples/polix.yaml, which a glob without slash would keep too.
func TestAnswerHookProtectedRootedPath(t *testing.T) {
	r := answerHookNewRepo(t, "main")
	r.write("polix.yaml", "version: 1\nprotected: [\"/polix.yaml\", \"AGENTS.md\"]\n")
	r.write("examples/polix.yaml", "version: 1\n")
	r.write("docs/AGENTS.md", "x\n")
	code, _, stderr := r.edit("polix.yaml", "")
	answerHookExpect(t, "edits polix.yaml", code, 2, stderr)
	if !strings.Contains(stderr, "polix.yaml is protected in polix.yaml") {
		t.Fatalf("the reason: %q", stderr)
	}
	for _, cmd := range []string{"echo x > polix.yaml", "rm -f polix.yaml", "rm -f ./polix.yaml", "rm -f *.yaml"} {
		code, _, stderr = r.bash(cmd, false)
		answerHookExpect(t, "runs "+cmd, code, 2, stderr)
	}
	code, _, stderr = r.edit("examples/polix.yaml", "")
	answerHookExpect(t, "edits examples/polix.yaml", code, 0, stderr)
	for _, cmd := range []string{"echo x > examples/polix.yaml", "rm -f examples/polix.yaml", "rm -rf examples"} {
		code, _, stderr = r.bash(cmd, false)
		answerHookExpect(t, "runs "+cmd, code, 0, stderr)
	}
	// Without the slash the glob still matches the name in any folder.
	code, _, stderr = r.edit("docs/AGENTS.md", "")
	answerHookExpect(t, "edits docs/AGENTS.md", code, 2, stderr)
}

// writes lets an entry keep only subagents away from a path.
func TestAnswerHookWritesCondition(t *testing.T) {
	r := answerHookNewRepo(t, "main")
	r.write("polix.yaml", `version: 1
on:
  pre_command:
    - if: { writes: ["AGENTS.md", "ai/rules/**"], not: { actor: session } }
      deny: "Solo la sesión principal cambia AGENTS.md"
`)
	code, _, stderr := r.bashAs("rm -rf ai/rules", false, "builder")
	answerHookExpect(t, "builder removes ai/rules", code, 2, stderr)
	code, _, stderr = r.bashAs("ls ai 2>/dev/null", false, "builder")
	answerHookExpect(t, "builder lists ai", code, 0, stderr)
	code, _, stderr = r.bashAs("sed -i s/a/b/ AGENTS.md", false, "builder")
	answerHookExpect(t, "builder writes AGENTS.md", code, 2, stderr)
	code, _, stderr = r.bashAs("cat AGENTS.md", false, "builder")
	answerHookExpect(t, "builder reads AGENTS.md", code, 0, stderr)
	code, _, stderr = r.bashAs("sed -i s/a/b/ AGENTS.md", false, "")
	answerHookExpect(t, "the session writes AGENTS.md", code, 0, stderr)
}

// run with record records the fact only when the run passes, so a passing gate on stop counts as done.
func TestAnswerHookRunRecordsOnSuccess(t *testing.T) {
	const gate = `  pre_commit:
    - unless: { done: tested }
      deny: "Faltan los tests"
`
	r := answerHookNewRepo(t, "main")
	r.write("polix.yaml", `version: 1
on:
  stop:
    - unless: { done: tested }
      mode: warn
      run: { command: [git, rev-parse, --verify, --quiet, no-such-ref], record: tested }
`+gate)
	r.write("src/a.go", "package a\n")
	code, _, stderr := r.stop()
	answerHookExpect(t, "stop with the run failing in warn mode", code, 0, stderr)
	code, _, stderr = r.bash("git commit -m x", false)
	answerHookExpect(t, "commit after a failing run", code, 2, stderr)

	r.write("polix.yaml", `version: 1
on:
  stop:
    - unless: { done: tested }
      run: { command: [git, rev-parse, --verify, --quiet, HEAD], record: tested }
`+gate)
	code, _, stderr = r.stop()
	answerHookExpect(t, "stop with the run passing", code, 0, stderr)
	code, _, stderr = r.bash("git commit -m x", false)
	answerHookExpect(t, "commit after a passing run", code, 0, stderr)

	r.write("src/a.go", "package a\n\nvar x = 1\n")
	code, _, stderr = r.bash("git commit -m x", false)
	answerHookExpect(t, "commit after an edit", code, 2, stderr)

	// A run that passes in warn mode records too.
	r.write("polix.yaml", `version: 1
on:
  stop:
    - mode: warn
      run: { command: [git, rev-parse, --verify, --quiet, HEAD], record: tested }
`+gate)
	r.stop()
	code, _, stderr = r.bash("git commit -m x", false)
	answerHookExpect(t, "commit after a passing run in warn mode", code, 0, stderr)
}

// Rules that cannot be loaded fail run: { rules }, instead of passing with no checks and recording a fact.
func TestAnswerHookRunRulesFailsOnBadRules(t *testing.T) {
	r := answerHookNewRepo(t, "main")
	r.write("polix.yaml", "version: 1\non:\n  stop:\n    - run: { rules: changed, record: checked }\n")
	r.write("ai/rules/bad.yaml", "rules: [\n")
	code, _, stderr := r.stop()
	answerHookExpect(t, "stop with rules that do not load", code, 2, stderr)
	if !strings.Contains(stderr, "No se pueden cargar las rules") {
		t.Fatalf("reason: %q", stderr)
	}
}

// A run that changes the content proves nothing about the result: its fact is not recorded.
func TestAnswerHookRunRecordSkipsChangedContent(t *testing.T) {
	r := answerHookNewRepo(t, "main")
	r.write("polix.yaml", `version: 1
on:
  stop:
    - run: { command: [git, checkout, --, README.md], record: tested }
  pre_commit:
    - unless: { done: tested }
      deny: "Faltan los tests"
`)
	r.write("README.md", "changed\n")
	code, _, stderr := r.stop()
	answerHookExpect(t, "stop with a run that rewrites a file", code, 0, stderr)
	code, _, stderr = r.bash("git commit -m x", false)
	answerHookExpect(t, "commit after a run that changed the content", code, 2, stderr)

	// Same run, nothing left to rewrite: the content holds and the fact is recorded.
	r.stop()
	code, _, stderr = r.bash("git commit -m x", false)
	answerHookExpect(t, "commit after a run on stable content", code, 0, stderr)
}

// The loop guard covers every block of a stop, not only require: a deny or a failing run cannot trap the agent.
func TestAnswerHookStopLoopGuardOnAnyBlock(t *testing.T) {
	r := answerHookNewRepo(t, "main")
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
func TestAnswerHookStopLoopGuardPerEntry(t *testing.T) {
	r := answerHookNewRepo(t, "main")
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
func TestAnswerHookInvalidWorkflowCanBeFixed(t *testing.T) {
	r := answerHookNewRepo(t, "main")
	r.write("polix.yaml", "version: 1\non:\n  pre_edit:\n    - deny: x\n      foo: 1\n")
	r.write(".gitignore", "build/\n")

	code, _, stderr := r.edit("polix.yaml", "")
	answerHookExpect(t, "the session edits the workflow", code, 0, stderr)
	code, _, stderr = r.edit("polix.yaml", "builder")
	answerHookExpect(t, "a subagent edits the workflow", code, 2, stderr)
	code, _, stderr = r.edit("src/a.go", "")
	answerHookExpect(t, "the session edits code", code, 2, stderr)
	code, _, stderr = r.edit("build/out.txt", "")
	answerHookExpect(t, "edit of an ignored file", code, 0, stderr)
	code, _, stderr = r.hook("claude", map[string]any{"hook_event_name": "PreToolUse", "tool_name": "Write",
		"tool_input": map[string]any{"file_path": filepath.Join(t.TempDir(), "notes.md")}})
	answerHookExpect(t, "edit outside the repository", code, 0, stderr)
	code, _, stderr = r.edit(".git/polix/log.jsonl", "")
	answerHookExpect(t, "edit of the state", code, 2, stderr)
	if !strings.Contains(stderr, "estado de polix") {
		t.Fatalf("the protection should answer, not the invalid workflow: %q", stderr)
	}
	for c, want := range map[string]int{"polix check": 0, "cd . && polix check --dir .": 0, "ls": 2, "polix check && ls": 2} {
		code, _, stderr = r.bash(c, false)
		answerHookExpect(t, c, code, want, stderr)
	}
	code, _, stderr = r.bash("git commit -m x", false)
	answerHookExpect(t, "commit", code, 2, stderr)
	if n := strings.Count(stderr, "no es válido"); n != 1 {
		t.Fatalf("the command and its commit give one message, got %d: %q", n, stderr)
	}
}

// A workflow that requires a newer polix asks to update polix, not to fix the file: not even the main session may
// edit it, lest it take out options of the newer polix, and the commands that update polix stay open.
func TestAnswerHookWorkflowAheadOfPolix(t *testing.T) {
	old := build.Version
	build.Version = "0.1.0"
	t.Cleanup(func() { build.Version = old })
	r := answerHookNewRepo(t, "main")
	r.write("polix.yaml", "version: 1\nrequires: 0.2.0\nan_option_of_0_2: true\n")

	code, _, stderr := r.edit("polix.yaml", "")
	answerHookExpect(t, "the session edits a workflow ahead", code, 2, stderr)
	if !strings.Contains(stderr, "necesita polix 0.2.0") || !strings.Contains(stderr, "polix update") ||
		strings.Contains(stderr, "corrígelo") {
		t.Fatalf("the reason asks to update polix: %q", stderr)
	}
	code, _, stderr = r.edit("src/a.go", "")
	answerHookExpect(t, "the session edits code", code, 2, stderr)
	for c, want := range map[string]int{"polix update": 0, "polix update --version 0.2.0": 0, "polix version": 0,
		"polix doctor": 0, "ls": 2, "polix update && ls": 2} {
		code, _, stderr = r.bash(c, false)
		answerHookExpect(t, c, code, want, stderr)
	}
	code, _, stderr = r.stop()
	answerHookExpect(t, "stop", code, 0, stderr)

}

// Unknown options may belong to a newer polix: the reason says so, so the agent updates instead of removing them.
func TestAnswerHookUnknownOptionsHintAnUpdate(t *testing.T) {
	r := answerHookNewRepo(t, "main")
	r.write("polix.yaml", "version: 1\non:\n  pre_edit:\n    - deny: x\n      an_option_of_0_2: 1\n")
	code, _, stderr := r.edit("src/a.go", "")
	answerHookExpect(t, "edit", code, 2, stderr)
	if !strings.Contains(stderr, "no las quites: ejecuta polix update") {
		t.Fatalf("the reason hints an update: %q", stderr)
	}
	r.write("polix.yaml", "version: 2\n")
	if _, _, stderr = r.edit("src/a.go", ""); !strings.Contains(stderr, "polix update") {
		t.Fatalf("a later version of the schema hints an update: %q", stderr)
	}
	r.write("polix.yaml", "version: 1\nmax_stop_blocks: -1\n")
	if _, _, stderr = r.edit("src/a.go", ""); strings.Contains(stderr, "polix update") {
		t.Fatalf("no hint for a plain mistake: %q", stderr)
	}
}

// Codex gives relative paths in apply_patch: they are resolved from the cwd of the call, not of the process.
func TestAnswerHookCodexRelativeStatePath(t *testing.T) {
	r := answerHookNewRepo(t, "main")
	patch := "*** Begin Patch\n*** Update File: .git/polix/log.jsonl\n@@\n-a\n+b\n*** End Patch\n"
	code, _, stderr := r.hook("codex", map[string]any{"hook_event_name": "PreToolUse", "tool_name": "apply_patch",
		"tool_input": map[string]any{"input": patch}})
	answerHookExpect(t, "codex patches the state", code, 2, stderr)
}

// Paths that are not plain ASCII match globs.
func TestAnswerHookNonASCIIChangedFiles(t *testing.T) {
	r := answerHookNewRepo(t, "main")
	r.write("polix.yaml", "version: 1\non:\n  stop:\n    - if: { changed: \"*.md\" }\n      require: \"md cambiado\"\n")
	r.git("add", "--all")
	r.git("commit", "--quiet", "-m", "workflow")
	r.write("diseño/año.md", "x\n")
	code, _, stderr := r.stop()
	answerHookExpect(t, "stop with a changed año.md", code, 2, stderr)
}

// In the hooks, changed is what is not committed yet, whatever base says.
func TestAnswerHookChangedSinceHead(t *testing.T) {
	r := answerHookNewRepo(t, "main")
	r.write("polix.yaml", "version: 1\nbase: main\non:\n  stop:\n    - if: { changed: \"src/**\" }\n      require: \"src cambiado\"\n")
	r.git("add", "--all")
	r.git("commit", "--quiet", "-m", "workflow")
	r.git("checkout", "--quiet", "-b", "feature")
	r.write("src/a.go", "package a\n")
	code, _, stderr := r.stop()
	answerHookExpect(t, "stop with uncommitted src", code, 2, stderr)
	r.git("add", "--all")
	r.git("commit", "--quiet", "-m", "src")
	code, _, stderr = r.stop()
	answerHookExpect(t, "stop after the commit", code, 0, stderr)
}

// The example settings give the hooks more time than the run budget of a call, so the agent never kills a
// hook in the middle of a gate.
func TestAnswerHookExampleHookTimeouts(t *testing.T) {
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
				if strings.Contains(h.Command, "polix hook claude") && h.Timeout <= RunBudget.Seconds() {
					t.Errorf("%s: timeout %vs, want more than the run budget (%v)", event, h.Timeout, RunBudget)
				}
			}
		}
	}
}

// A decision does not compute the content hash just to log it: a plain deny costs no git tree.
func TestAnswerHookDecisionsSkipTheHash(t *testing.T) {
	r := answerHookNewRepo(t, "main")
	r.write("polix.yaml", "version: 1\non:\n  pre_edit:\n    - deny: x\n")
	r.edit("src/a.go", "")
	data, _ := os.ReadFile(filepath.Join(r.dir, ".git", "polix", "log.jsonl"))
	if !strings.Contains(string(data), `"action":"deny","line":4`) || strings.Contains(string(data), `"hash"`) {
		t.Fatalf("log: %s", data)
	}
}

// A line of the log longer than the read limit is skipped: the loop guard still counts and lets the agent stop,
// and polix log prints the other entries and says how many lines it skipped.
func TestAnswerHookStopLoopGuardHugeLogLine(t *testing.T) {
	r := answerHookNewRepo(t, "main")
	r.write("polix.yaml", "version: 1\nmax_stop_blocks: 2\n"+strings.TrimPrefix(answerHookReviewOnStop, "version: 1\n"))
	r.write("src/app.go", "package app\n")
	r.write(".git/polix/log.jsonl", `{"kind":"decision","reason":"`+strings.Repeat("x", 5*1024*1024)+"\"}\n")
	r.stops("", 2, 2, 0)
	// polix log on such a log is tested in internal/workflow (TestLogWorkflowCommand).
	data, err := os.ReadFile(filepath.Join(r.dir, ".git", "polix", "log.jsonl"))
	if err != nil || !strings.Contains(string(data), "loop_guard") {
		t.Fatalf("the log records the loop guard: %v", err)
	}
}

// When the stop block cannot be written, the loop guard cannot count: a blocking stop lets the agent stop with
// a warning instead of trapping it.
func TestAnswerHookStopLoopGuardReadOnlyState(t *testing.T) {
	answerHookSkipFileModes(t)
	r := answerHookNewRepo(t, "main")
	r.write("polix.yaml", answerHookReviewOnStop)
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
	answerHookExpect(t, "stop with a read only state", code, 0, stderr)
	if !strings.Contains(stderr, "no puede usar su log") {
		t.Fatalf("stderr: %q", stderr)
	}
}

// A failing run that prints a huge line records a capped reason, which polix log prints.
func TestAnswerHookRunHugeOutputCapped(t *testing.T) {
	r := answerHookNewRepo(t, "main")
	r.write("polix.yaml", `version: 1
on:
  stop:
    - run: { command: [git, diff, --no-index, empty.txt, big.txt] }
`)
	r.write("empty.txt", "")
	r.write("big.txt", strings.Repeat("y", 4*1000*1000)+"\n")
	code, _, stderr := r.stop()
	answerHookExpect(t, "stop with a failing run", code, 2, stderr)
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

// Without a content hash, no fact is recorded, done does not hold and the hook says why.
func TestAnswerHookUnknownHashRecordsNothing(t *testing.T) {
	answerHookSkipFileModes(t)
	r := answerHookNewRepo(t, "main")
	r.write("polix.yaml", `version: 1
on:
  post_command:
    - if: { command: "just test", exit: 0 }
      record: tested
  stop:
    - if: { changed: "**" }
      unless: { done: tested }
      require: "Faltan los tests"
`)
	r.write("secret.txt", "x\n")
	p := filepath.Join(r.dir, "secret.txt")
	if err := os.Chmod(p, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(p, 0o644) })
	code, _, stderr := r.bash("just test", true)
	answerHookExpect(t, "tests pass without a hash", code, 0, stderr)
	if !strings.Contains(stderr, "hash del contenido") {
		t.Fatalf("stderr should warn about the hash: %q", stderr)
	}
	data, _ := os.ReadFile(filepath.Join(r.dir, ".git", "polix", "log.jsonl"))
	if strings.Contains(string(data), `"kind":"fact"`) || strings.Contains(string(data), "unknown") {
		t.Fatalf("log: %s", data)
	}
	r.write("src/a.go", "package a\n")
	code, _, stderr = r.stop()
	answerHookExpect(t, "stop after an edit without a hash", code, 2, stderr)
}

// A run without record may rewrite files: the later entries of the call use the content after it.
func TestAnswerHookRunRefreshesHash(t *testing.T) {
	r := answerHookNewRepo(t, "main")
	r.write("polix.yaml", `version: 1
on:
  stop:
    - unless: { done: formatted }
      run: { command: [git, checkout, --, README.md] }
    - record: tested
  pre_commit:
    - unless: { done: tested }
      deny: "Faltan los tests"
`)
	r.git("add", "polix.yaml")
	r.git("commit", "--quiet", "-m", "workflow")
	r.write("README.md", "changed\n")
	code, _, stderr := r.stop()
	answerHookExpect(t, "stop with a run that rewrites a file", code, 0, stderr)
	code, _, stderr = r.bash("git commit -m x", false)
	answerHookExpect(t, "commit on the content after the rewrite", code, 0, stderr)
}

// A failed command reaches post_command with exit 1, and a failed edit runs no post_edit.
func TestAnswerHookPostToolUseFailure(t *testing.T) {
	r := answerHookNewRepo(t, "main")
	r.write("polix.yaml", "version: 1\non:\n  post_command:\n    - if: { exit: 1 }\n      context: { text: \"el comando falló\" }\n"+
		"  post_edit:\n    - context: { text: \"post edit\" }\n")
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
func TestAnswerHookRuleChecksThroughRunner(t *testing.T) {
	r := answerHookNewRepo(t, "main")
	r.write("polix.yaml", `version: 1
rules:
  run:
    "*.ref": [git, rev-parse, --verify, --quiet, "{name}"]
on:
  stop:
    - if: { changed: "src/**" }
      run: { rules: changed }
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
	answerHookExpect(t, "stop with the ref missing", code, 2, stderr)
	if !strings.Contains(stderr, "- STYLE-1 (checks/style.ref): The ref of the rule exists.") {
		t.Fatalf("the reason names the rule: %q", stderr)
	}
	r.git("branch", "STYLE1")
	code, _, stderr = r.stop()
	answerHookExpect(t, "stop with the ref of {name}", code, 0, stderr)
}

// A log that cannot be read is an error of LoadState, so the loop guard can let the agent stop.
func TestAnswerHookAdapterLoadStateUnreadable(t *testing.T) {
	answerHookSkipFileModes(t)
	r := answerHookNewRepo(t, "main")
	r.write(".git/polix/log.jsonl", `{"kind":"stop_block"}`+"\n")
	p := filepath.Join(r.dir, ".git", "polix", "log.jsonl")
	if err := os.Chmod(p, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(p, 0o644) })
	a := &AnswerHookAdapter{}
	if _, _, err := a.Open(context.Background(), r.dir); err != nil {
		t.Fatal(err)
	}
	if _, err := a.LoadState(context.Background()); err == nil {
		t.Fatal("want the error of the unreadable log")
	}
}

// The adapter reads the facts of the repository and appends to its log, stamped with the time.
func TestAnswerHookAdapter(t *testing.T) {
	r := answerHookNewRepo(t, "feature")
	r.write(".gitignore", "build/\n")
	r.write("src/a.go", "package a\n")
	ctx := context.Background()
	a := &AnswerHookAdapter{}
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
	if h, err := a.Hash(ctx); err != nil || h == "" {
		t.Fatalf("Hash = %q %v", h, err)
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
	if err := a.Append(ctx, state.Entry{Kind: state.KindFact, Fact: "tested"}); err != nil {
		t.Fatal(err)
	}
	s, err := a.LoadState(ctx)
	if err != nil || len(s.Entries) != 1 || s.Entries[0].At == "" || !strings.Contains(r.log(), `"fact":"tested"`) {
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
func TestAnswerHookRulesOfRulesPath(t *testing.T) {
	r := answerHookNewRepo(t, "main")
	r.write("polix.yaml", "version: 1\nrules: { path: governance/rules }\non:\n  pre_edit:\n    - context: { rules: \"{path}\" }\n")
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
func TestAnswerHookInvalidRuleNextToValid(t *testing.T) {
	r := answerHookNewRepo(t, "main")
	r.write("polix.yaml", `version: 1
on:
  pre_edit:
    - context: { rules: "{path}" }
  stop:
    - run: { rules: changed }
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
	answerHookExpect(t, "stop with the check of the invalid rule failing", code, 2, stderr)
	if !strings.Contains(stderr, "- BAD-1 (checks/bad.ref)") {
		t.Fatalf("the check of the invalid rule runs: %q", stderr)
	}
}
