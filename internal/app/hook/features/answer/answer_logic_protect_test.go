package answer

import (
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/mlophez/polix-ai/internal/app/agent"
	"github.com/mlophez/polix-ai/internal/app/hook"
	"github.com/mlophez/polix-ai/internal/app/workflow"
)

// anyPathFold matches without case only when asked.
func TestAnyPathFold(t *testing.T) {
	if !anyPathFold([]string{".env"}, ".ENV", true) || anyPathFold([]string{".env"}, ".ENV", false) {
		t.Fatal("anyPathFold")
	}
}

// The state is named by its path below a git directory or by its folder, not by the name of the binary.
func TestNamesState(t *testing.T) {
	dir := filepath.FromSlash("/repo/.git/polix")
	for command, want := range map[string]bool{
		"rm -rf .git/polix":                                       true,
		"cat .git/worktrees/w1/polix/log.jsonl":                   true,
		"rm " + dir + "/log.jsonl":                                true,
		"rm .git/polix/sessions/s1.jsonl":                         true,
		"echo x >> " + filepath.Join(dir, "sessions", "s1.jsonl"): true,
		"go install ./cmd/polix":                                  false,
		"cp build/polix ~/bin/":                                   false,
	} {
		if got := namesState(command, dir, dir, false); got != want {
			t.Errorf("namesState(%q) = %v", command, got)
		}
	}
	if !namesState("rm -rf .GIT/Polix", dir, dir, true) || namesState("rm -rf .GIT/Polix", dir, dir, false) {
		t.Error("namesState folds the case only when asked")
	}
	if !namesState("rm /real/polix/x", dir, "/real/polix", false) {
		t.Error("namesState sees the real path of the folder")
	}
}

// mayWrite sees the commands that can write, nested ones included, and not a descriptor duplicated, the null
// device nor the name of a writer that is only an argument.
func TestMayWrite(t *testing.T) {
	for command, want := range map[string]bool{"rm x": true, "echo x > y": true, "sed -i s/a/b/ x": true,
		"cat x 2>&1": false, "jq . x": false, "echo x >&y": true,
		"sudo rm x": true, "git rm x": true, "bash -c 'rm x'": true, "echo $(rm x)": true,
		"bash <<'EOF'\nrm x\nEOF": true, "cat x 2>/dev/null": false, "jq . x | head": false,
		// The heuristic of the text still counts a `>` inside quotes, on the side of the guard.
		`echo "a > b"`: true,
		// A writer named as an argument, a wrapper with a value, or a script polix does not enter, may write too.
		"find x -name y -exec rm {} +": true, "ls x | xargs -I {} rm x/{}": true, "xargs -n 1 rm < list": true,
		"sudo -u me rm -rf x": true, "nice -n 10 rm -rf x": true, "env -u X rm -rf x": true,
		"busybox rm -rf x": true, "find x -exec sed -i s/a/b/ {} +": true, "echo 'x; rm -rf x' | sh": true,
		"printf x | bash": true, "eval rm -rf x/$X": true, `bash -c "true; rm -rf x/$X"`: true,
		"dash -c 'rm -rf x'": true, "ksh <<'EOF'\nrm x\nEOF": true, "bash <(echo x)": true,
		"source <(echo x)": true, ". ./script": true, "cat x | grep rm": true,
		`awk '{print > "x"}' f`: true, `perl -e 'open(my $f, ">", "x")'`: true, `python3 -c 'open("x", "w")'`: true,
		`node -e 'require("fs").rmSync("x")'`: true, "ruby -e 'File.delete(\"x\")'": true,
		"tcsh -c 'rm x'": true, "ash -c 'rm x'": true, "pwsh -c 'Remove-Item x'": true,
		"powershell -Command 'Remove-Item x'": true, `python3.12 -c 'import os; os.remove("x")'`: true,
		"python2 -c x": true, "pypy3 -c x": true, "/usr/bin/perl5.34 -e x": true, "cat 2.txt": false,
		// A program in another case (macOS and Windows run it) or whose name is not literal may be a shell too.
		"Bash -c 'echo x > f'": true, "PowerShell -Command 'Remove-Item x'": true, "pwsh.EXE -c x": true,
		"Python3 -c x": true, "RM x": true, "$SHELL -c 'echo x > f'": true, `"$X" -c 'echo x > f'`: true,
		"FOO=1 $SHELL -c x": true, "FOO=$X cat x": false, "echo $HOME": false, "Cat x": false,
		// Never weaker than the text heuristic: what it refused is still refused.
		"nohup $SHELL -c 'echo x > f'": true, "sudo $SHELL -c 'echo x > f'": true, "env -S 'echo x > f'": true,
		"find . -exec $SHELL -c 'echo x > f' \\;": true, "xargs $SHELL -c 'echo x > f'": true,
		"cat x >/dev/null 2>&1": false, "cat x &> /dev/null": false, "cat x 2> /dev/null | head": false,
		"cat x 2>/dev/null|head": false, "cat x 2>/dev/null;echo": false, "(cat x 2>/dev/null)": false,
		"cat x 2>/dev/null>y": true, "cat x 2> /dev/null 1> f": true,
		// A line the parser rejects keeps the heuristic of the text.
		`rm "x`: true, `cat "x`: false,
	} {
		if got := mayWrite(command, false); got != want {
			t.Errorf("mayWrite(%q) = %v", command, got)
		}
	}
	// On Windows too, `> nul` may write: under Git Bash it can create a file named nul.
	for command, want := range map[string]bool{"echo x > nul": true, "cat x 2>NUL": true, "cat x 2>/dev/null": false} {
		if got := mayWrite(command, true); got != want {
			t.Errorf("mayWrite(%q) on Windows = %v", command, got)
		}
	}
}

// within sees a path inside a folder, the folder itself, and nothing outside, without case when asked.
func TestWithin(t *testing.T) {
	dir := filepath.FromSlash("/repo/.git/polix")
	for p, want := range map[string]bool{dir: true, filepath.Join(dir, "log.jsonl"): true,
		filepath.Join(dir, "sessions", "s1.jsonl"): true,
		filepath.FromSlash("/repo/.git"):           false, filepath.FromSlash("/repo/.git/polixx"): false} {
		if got := within(dir, p, false); got != want {
			t.Errorf("within(%q) = %v", p, got)
		}
	}
	upper := filepath.FromSlash("/repo/.git/POLIX/x")
	if !within(dir, upper, true) || within(dir, upper, false) {
		t.Error("within folds the case only when asked")
	}
}

// forbidden finds the first forbidden command, in file order, that matches any command of the line.
func TestForbidden(t *testing.T) {
	list := []workflow.ForbiddenCommand{{Pattern: "rm -rf /*", Line: 4},
		{Pattern: "git push --force*", Reason: "no", Line: 5}, {Pattern: "git push*", Line: 7}}
	for line, want := range map[string]int{
		"rm -rf /":                       4,
		"cd tmp && rm -rf / ; ls":        4,
		"git fetch && git push --force":  5,
		"git push origin main":           7,
		"rm -rf build":                   0,
		"echo 'git push --force' > note": 0,
	} {
		f, ok := forbidden(list, line, false)
		if got := map[bool]int{true: f.Line}[ok]; got != want {
			t.Errorf("%q: line %d, want %d", line, got, want)
		}
	}
}

// AC5: a forbidden command nested in a -c script, a substitution, eval or a compound command is denied too.
func TestForbiddenNested(t *testing.T) {
	list := []workflow.ForbiddenCommand{{Pattern: "rm -rf*", Line: 4}}
	for line, want := range map[string]bool{
		"bash -c 'rm -rf /'": true, "echo $(rm -rf /)": true, "sh -c \"cd x && rm -rf y\"": true,
		"echo `rm -rf /`": true, "eval rm -rf /": true, "if true; then rm -rf x; fi": true,
		"sudo bash -c 'rm -rf /'": true, "env X=1 sh -c 'rm -rf /'": true, "nohup bash -c 'rm -rf /' &": true,
		"case x in $(rm -rf /)) ;; esac": true,
		"bash <<'EOF'\nrm -rf /\nEOF":    true, "sh -s <<< 'rm -rf /'": true, "cat <<'EOF'\nrm -rf /\nEOF": false,
		"bash <<EOF\nrm -rf $HOME\nEOF": true, "cat <<'EOF' | bash\nrm -rf /\nEOF": true,
		"bash - <<'EOF'\nrm -rf /\nEOF": true, "cat <<'EOF' | grep x\nrm -rf /\nEOF": false,
		"cat <<'EOF' | grep -v z | bash\nrm -rf /\nEOF": true, "{ cat <<'EOF'; } | bash\nrm -rf /\nEOF": true,
		"cat <<'EOF' | sudo -u x bash\nrm -rf /\nEOF": true, "bash <<EOF\necho \\$(rm -rf /)\nEOF": true,
		"bash --rcfile f <<'EOF'\nrm -rf /\nEOF": true, "bash 3<<'EOF'\nrm -rf /\nEOF": false,
		"bash -euo pipefail -c 'rm -rf /'": true, "bash -eo pipefail <<'EOF'\nrm -rf /\nEOF": true,
		"bash -oc pipefail 'rm -rf /'": true, "bash -oe pipefail <<'EOF'\nrm -rf /\nEOF": true,
		"bash -ou pipefail -s <<'EOF'\nrm -rf /\nEOF": true, "bash -c \"$CMD\"": false, "echo 'rm -rf /'": false,
	} {
		if _, ok := forbidden(list, line, false); ok != want {
			t.Errorf("%q: forbidden %v, want %v", line, ok, want)
		}
	}
}

// denial walks the pre_command of line by actor on the workflow of ev and returns the reasons of its answer and the
// lines it logged.
func denial(t *testing.T, ev hook.Evaluation, line, actor string) ([]string, []entry) {
	t.Helper()
	e := with(t, ev, func(e *agent.Event) { e.Actor, e.Command = actor, line })
	out, steps := walk(t, e, facts{branch: "main"})
	if !out.Answer.Block {
		return nil, appended(steps)
	}
	return out.Answer.Reasons, appended(steps)
}

// CUJ-04, CUJ-05, AC-03: with the default builtin the hook denies to every actor a commit that skips the hooks,
// after the global options of git, in the script of bash -c and with the option in the message, with LIT-01 and the
// line, logged as a denial from guardrails.builtin.forbid_commit_no_verify at the line of builtin, also in a
// repository without gates of git; the forms the patterns do not match pass.
func TestBuiltinNoVerifyHook(t *testing.T) {
	ev := eval(t, "version: 1\non:\n  pre_command:\n    - action: { inject: { text: x, always: true } }\n",
		workflow.PreCommand)
	noGates := eval(t, "version: 1\nguardrails:\n  builtin: { check_polix_files: false }\n", workflow.PreCommand)
	for _, line := range []string{
		"git commit --no-verify -m x", "git commit -m x --no-verify", "git commit -n -m x", "git commit -m x -n",
		"git -C . commit --no-verify -m x", `bash -c "git commit -n -m x"`, `git commit -m "skip --no-verify"`,
	} {
		for _, actor := range []string{agent.ActorSession, "builder"} {
			reasons, logged := denial(t, ev, line, actor)
			if want := "Never skip the git hooks: fix what they block and commit again.\n" + line; !slices.Equal(reasons,
				[]string{want}) {
				t.Errorf("%s by %s: reasons %q", line, actor, reasons)
			}
			if len(logged) != 1 || logged[0].Outcome != "deny" ||
				logged[0].Action != "guardrails.builtin.forbid_commit_no_verify" || logged[0].Line != 0 {
				t.Errorf("%s by %s: logged %+v", line, actor, logged)
			}
		}
		if reasons, _ := denial(t, noGates, line, agent.ActorSession); len(reasons) != 1 ||
			!strings.HasPrefix(reasons[0], "Never skip the git hooks") {
			t.Errorf("%s without gates of git: reasons %q", line, reasons)
		}
	}
	for _, line := range []string{
		"git commit -nm x", "git commit -mn", "git commit --no-verbose -m x", `git commit -m "fix -nothing"`,
	} {
		if reasons, _ := denial(t, ev, line, agent.ActorSession); reasons != nil {
			t.Errorf("%s: refused %q", line, reasons)
		}
	}
}

// CUJ-06, CUJ-07, AC-04: the hook denies a forced push, git reset --hard and a forced git clean, each with its reason
// and the line, logged from guardrails.builtin.forbid_git_destructive_commands at the line of builtin; the safe forms,
// git clean -xfd, a known limit, and a quoted mention pass, and an invalid workflow applies no preset.
func TestBuiltinDestructiveGitHook(t *testing.T) {
	ev := eval(t, "version: 1\nguardrails:\n  builtin: { check_polix_files: false }\n", workflow.PreCommand)
	const push, reset, clean = "Never force a push: ask the user to run it if it is needed.",
		"git reset --hard discards changes for good: ask the user to run it.",
		"git clean -f deletes untracked files for good: ask the user to run it."
	for line, reason := range map[string]string{
		"git push --force": push, "git push -f": push, "git push origin main --force": push,
		"git push origin main -f": push, "git push --force-with-lease=main origin": push,
		"git reset --hard": reset, "git reset HEAD~1 --hard": reset,
		"git clean -f": clean, "git clean -fd": clean, "git clean --force": clean,
	} {
		reasons, logged := denial(t, ev, line, agent.ActorSession)
		if !slices.Equal(reasons, []string{reason + "\n" + line}) {
			t.Errorf("%s: reasons %q", line, reasons)
		}
		if len(logged) != 1 || logged[0].Action != "guardrails.builtin.forbid_git_destructive_commands" ||
			logged[0].Line != 3 {
			t.Errorf("%s: logged %+v", line, logged)
		}
	}
	for _, line := range []string{
		"git push origin main", "git push --no-force origin", "git push --follow-tags", "git reset --soft HEAD~1",
		"git clean -n", "git clean -xfd", `echo "git push --force"`,
	} {
		if reasons, _ := denial(t, ev, line, agent.ActorSession); reasons != nil {
			t.Errorf("%s: refused %q", line, reasons)
		}
	}
	broken := invalid(with(t, ev, func(e *agent.Event) { e.Command = "git push -f" }))
	if out, steps := walk(t, broken, facts{branch: "main"}); !out.Answer.Block ||
		strings.Contains(strings.Join(out.Answer.Reasons, "\n"), "Never force a push") {
		t.Errorf("an invalid workflow applies a preset: %+v %+v", out.Answer, appended(steps))
	}
}

// CON-02: on Windows the hook reads the line the same way, so the presets deny a forced push, also in the script of
// bash -c, with LIT-02 and the line, and let a quoted mention pass.
func TestBuiltinDestructiveGitHookWindows(t *testing.T) {
	ev := eval(t, "version: 1\nguardrails:\n  builtin: { check_polix_files: false }\n", workflow.PreCommand)
	ev.System = "windows"
	for _, line := range []string{"git push -f", "git push origin main --force", `bash -c "git push -f"`} {
		reasons, logged := denial(t, ev, line, agent.ActorSession)
		if want := "Never force a push: ask the user to run it if it is needed.\n" + line; !slices.Equal(reasons,
			[]string{want}) {
			t.Errorf("%s: reasons %q", line, reasons)
		}
		if len(logged) != 1 || logged[0].Action != "guardrails.builtin.forbid_git_destructive_commands" {
			t.Errorf("%s: logged %+v", line, logged)
		}
	}
	if reasons, _ := denial(t, ev, `echo "git push -f"`, agent.ActorSession); reasons != nil {
		t.Errorf("a quoted mention: %q", reasons)
	}
}

// CUJ-08, AC-05: with the four keys of builtin false neither preset refuses, and with only
// forbid_git_destructive_commands false a forced push passes while a commit that skips the hooks is still refused.
func TestBuiltinOffHook(t *testing.T) {
	off := eval(t, "version: 1\n"+builtinOff, workflow.PreCommand)
	for _, line := range []string{"git commit --no-verify -m x", "git push --force"} {
		if reasons, _ := denial(t, off, line, agent.ActorSession); reasons != nil {
			t.Errorf("all off, %s: refused %q", line, reasons)
		}
	}
	one := eval(t, "version: 1\nguardrails:\n  builtin: { forbid_git_destructive_commands: false }\n",
		workflow.PreCommand)
	if reasons, _ := denial(t, one, "git push --force", agent.ActorSession); reasons != nil {
		t.Errorf("forbid_git_destructive_commands false: the push is refused %q", reasons)
	}
	if reasons, _ := denial(t, one, "git commit --no-verify -m x", agent.ActorSession); !slices.Equal(reasons,
		[]string{"Never skip the git hooks: fix what they block and commit again.\ngit commit --no-verify -m x"}) {
		t.Errorf("forbid_git_destructive_commands false: the commit %q", reasons)
	}
}

// CUJ-15, AC-10: a forbidden command the repository writes keeps refusing with its reason next to the built-in
// blocks, and a forced push is refused by the built-in block with LIT-02.
func TestBuiltinKeepsWrittenForbidden(t *testing.T) {
	ev := eval(t, `version: 1
guardrails:
  forbidden_commands:
    - pattern: "kubectl* apply *"
      reason: Apply through the pipeline.
`, workflow.PreCommand)
	if reasons, _ := denial(t, ev, "kubectl apply -f x.yaml", agent.ActorSession); !slices.Equal(reasons,
		[]string{"Apply through the pipeline.\nkubectl apply -f x.yaml"}) {
		t.Errorf("kubectl: %q", reasons)
	}
	if reasons, _ := denial(t, ev, "git push --force", agent.ActorSession); !slices.Equal(reasons,
		[]string{"Never force a push: ask the user to run it if it is needed.\ngit push --force"}) {
		t.Errorf("push: %q", reasons)
	}
}

// AC6: a forbidden command is denied through its normalized form too, past wrappers and assignments.
func TestForbiddenNormalized(t *testing.T) {
	list := []workflow.ForbiddenCommand{{Pattern: "rm -rf*", Line: 4}, {Pattern: "git push --force*", Line: 5}}
	for line, want := range map[string]bool{
		"sudo rm -rf /": true, "env X=1 rm -rf /": true, "X=1 rm -rf build": true, "git -C x push --force": true,
		"sudo rm -r /": false, "echo sudo rm -rf /": false,
	} {
		if _, ok := forbidden(list, line, false); ok != want {
			t.Errorf("%q: forbidden %v, want %v", line, ok, want)
		}
	}
}
