package answer

import (
	"errors"
	"slices"
	"testing"

	"github.com/mlophez/polix-ai/internal/app/githook"
	"github.com/mlophez/polix-ai/internal/app/workflow"
)

// The logic is tested with domain values: a call of git and the workflow of polix.yaml in, the output out, without
// fakes.

// gates is a valid workflow whose githooks pre_commit holds entries, with the variable key of the branch.
func gates(entries ...workflow.Entry) workflow.Workflow {
	return workflow.Workflow{Version: 1, MaxStopRetries: workflow.DefaultMaxStopRetries,
		Vars:     map[string]workflow.Var{"key": {Branch: "^([A-Z][A-Z0-9]+-[0-9]+)"}},
		Githooks: map[string][]workflow.Entry{workflow.PreCommit: entries}}
}

// blocks is the action of a block with message, and runs that of a run of argv with message.
func blocks(message string) workflow.Action {
	return workflow.Action{Block: &workflow.Block{Message: message}}
}

func runs(message string, argv ...string) workflow.Action {
	return workflow.Action{Run: &workflow.Run{Command: argv, Message: message}}
}

// commit is a pre_commit call of git on branch with the staged files.
func commit(branch string, staged ...string) githook.Githook {
	return githook.Githook{Event: workflow.PreCommit, Branch: branch, Staged: staged}
}

// AC4: a block whose branch holds blocks with its message; on another branch the commit goes on.
func TestDecideBlockOnBranch(t *testing.T) {
	wf := gates(workflow.Entry{When: workflow.Cond{"branch": "^main$"}, Action: blocks("No se commitea en main")})
	out := Decide(commit("main"), wf, true, nil)
	if !out.Block || out.Run != nil || !slices.Equal(out.Reasons, []string{"No se commitea en main"}) {
		t.Fatalf("on main: %+v", out)
	}
	if out := Decide(commit("feature"), wf, true, nil); out.Block || out.Run != nil || out.Reasons != nil {
		t.Fatalf("on feature: %+v", out)
	}
	if out := Decide(commit(""), wf, true, nil); out.Block {
		t.Fatalf("detached: %+v", out)
	}
}

// unless skips an entry whose conditions hold; an empty when always holds.
func TestDecideUnless(t *testing.T) {
	wf := gates(workflow.Entry{Unless: workflow.Cond{"branch": "^release/"}, Action: blocks("no")})
	if out := Decide(commit("release/1.0"), wf, true, nil); out.Block {
		t.Fatalf("unless holds: %+v", out)
	}
	if out := Decide(commit("main"), wf, true, nil); !out.Block {
		t.Fatalf("unless does not hold: %+v", out)
	}
}

// AC5: changed holds for a staged file matching its glob, or one of the globs of a list, and not without a match.
func TestDecideChanged(t *testing.T) {
	cases := []struct {
		name    string
		changed any
		staged  []string
		block   bool
	}{
		{"a staged file matches", "src/**", []string{"README.md", "src/a/b.go"}, true},
		{"one glob of a list matches", []any{"docs/**", "*.go"}, []string{"cmd/main.go"}, true},
		{"no staged file matches", []any{"docs/**", "*.go"}, []string{"README.md"}, false},
		{"nothing staged", "**", nil, false},
	}
	for _, c := range cases {
		wf := gates(workflow.Entry{When: workflow.Cond{"changed": c.changed}, Action: blocks("x")})
		if out := Decide(commit("main", c.staged...), wf, true, nil); out.Block != c.block {
			t.Errorf("%s: %+v", c.name, out)
		}
	}
}

// AC6: a run asks for its run; when it failed the commit blocks with its message and the line of the failure, and
// when it passed the next entries are evaluated, with their own runs.
func TestDecideRun(t *testing.T) {
	wf := gates(
		workflow.Entry{Action: runs("Arregla los tests", "just", "test")},
		workflow.Entry{When: workflow.Cond{"branch": "^main$"}, Action: runs("", "just", "lint")},
		workflow.Entry{Action: blocks("last")},
	)
	g := commit("main")
	if out := Decide(g, wf, true, nil); out.Block || !slices.Equal(out.Run, []string{"just", "test"}) {
		t.Fatalf("first run: %+v", out)
	}
	failed := Decide(g.Ran(errors.New("exit status 1")), wf, true, nil)
	if !failed.Block || failed.Run != nil ||
		!slices.Equal(failed.Reasons, []string{"Arregla los tests", "just test failed: exit status 1"}) {
		t.Fatalf("failed run: %+v", failed)
	}
	g = g.Ran(nil)
	if out := Decide(g, wf, true, nil); !slices.Equal(out.Run, []string{"just", "lint"}) {
		t.Fatalf("second run: %+v", out)
	}
	out := Decide(g.Ran(errors.New("exit status 2")), wf, true, nil)
	if !out.Block || !slices.Equal(out.Reasons, []string{"just lint failed: exit status 2"}) {
		t.Fatalf("a failed run without message: %+v", out)
	}
	if out := Decide(g.Ran(nil), wf, true, nil); !out.Block || !slices.Equal(out.Reasons, []string{"last"}) {
		t.Fatalf("both passed: %+v", out)
	}
	if out := Decide(commit("dev").Ran(nil), wf, true, nil); !slices.Equal(out.Reasons, []string{"last"}) {
		t.Fatalf("an entry that does not hold runs nothing: %+v", out)
	}
}

// The first block ends the evaluation: a later entry is never evaluated, nor its run asked.
func TestDecideFirstBlockEnds(t *testing.T) {
	wf := gates(workflow.Entry{Action: blocks("first")}, workflow.Entry{Action: runs("", "ls")},
		workflow.Entry{Action: blocks("second")})
	if out := Decide(commit("main"), wf, true, nil); out.Run != nil || !slices.Equal(out.Reasons, []string{"first"}) {
		t.Fatalf("got %+v", out)
	}
}

// AC8: {branch} and the variables of vars expand in the messages, the command and the patterns of changed; a
// variable whose expression does not match the branch stays as written.
func TestDecidePlaceholders(t *testing.T) {
	wf := gates(
		workflow.Entry{When: workflow.Cond{"changed": "docs/{key}/**"}, Action: runs("Revisa {key} en {branch}",
			"check", "{key}", "--branch={branch}")},
	)
	g := commit("PROJ-12-login", "docs/PROJ-12/a.md")
	if out := Decide(g, wf, true, nil); !slices.Equal(out.Run, []string{"check", "PROJ-12", "--branch=PROJ-12-login"}) {
		t.Fatalf("run: %+v", out)
	}
	out := Decide(g.Ran(errors.New("exit status 1")), wf, true, nil)
	if !slices.Equal(out.Reasons, []string{"Revisa PROJ-12 en PROJ-12-login",
		"check PROJ-12 --branch=PROJ-12-login failed: exit status 1"}) {
		t.Fatalf("failed: %+v", out)
	}
	if out := Decide(commit("PROJ-13-x", "docs/PROJ-12/a.md"), wf, true, nil); out.Run != nil {
		t.Fatalf("another key: %+v", out)
	}
	plain := gates(workflow.Entry{Action: blocks("{key} de {branch}")})
	if out := Decide(commit("main"), plain, true, nil); !slices.Equal(out.Reasons, []string{"{key} de main"}) {
		t.Fatalf("no key: %+v", out)
	}
}

// AC9: without polix.yaml, or without entries for the event, the commit goes on; a polix.yaml that cannot be used
// blocks it with its problems.
func TestDecideWorkflow(t *testing.T) {
	if out := Decide(commit("main"), workflow.Workflow{}, false, nil); out.Block || out.Run != nil {
		t.Fatalf("no polix.yaml: %+v", out)
	}
	other := workflow.Workflow{Version: 1, Githooks: map[string][]workflow.Entry{
		workflow.CommitMsg: {{Action: blocks("x")}}}}
	if out := Decide(commit("main"), other, true, nil); out.Block || out.Run != nil {
		t.Fatalf("no entries for the event: %+v", out)
	}
	invalid := workflow.InvalidError{"polix.yaml:3: version must be 1, got 2", "polix.yaml:5: unknown key \"x\""}
	out := Decide(commit("main"), workflow.Workflow{Version: 2}, true, invalid)
	if !out.Block || out.Run != nil || !slices.Equal(out.Reasons, []string(invalid)) {
		t.Fatalf("problems: %+v", out)
	}
	old := workflow.TooOldError{Requires: "0.9.0", Running: "0.6.1"}
	out = Decide(commit("main"), workflow.Workflow{}, true, old)
	if !out.Block || !slices.Equal(out.Reasons, []string{old.Error()}) {
		t.Fatalf("too old: %+v", out)
	}
}

// message is a commit_msg call of git on branch with the raw message, as git wrote it, and the comment string #.
func message(branch, raw string) githook.Githook {
	return githook.Githook{Event: workflow.CommitMsg, Branch: branch, Message: raw, Comment: "#"}
}

// msgGates is a valid workflow whose githooks commit_msg holds entries, with the variable key of the branch.
func msgGates(entries ...workflow.Entry) workflow.Workflow {
	wf := gates()
	wf.Githooks = map[string][]workflow.Entry{workflow.CommitMsg: entries}
	return wf
}

// AC7: message matches the first line of the message, or one pattern of a list does, never its body; the line is
// taken after removing the comment lines, everything from the scissors line on and the leading blank lines.
func TestDecideMessage(t *testing.T) {
	cases := []struct {
		name    string
		pattern any
		raw     string
		holds   bool
	}{
		{"the first line matches", "^fix: ", "fix: el login\n\ncuerpo\n", true},
		{"one pattern of a list matches", []any{"^feat", "^fix"}, "fix: x\n", true},
		{"no pattern matches", []any{"^feat", "^docs"}, "fix: x\n", false},
		{"the body is not looked at", "^fix: ", "arregla\n\nfix: en el cuerpo\n", false},
		{"comment lines go", "^fix: ", "# Please enter the commit message\n#\nfix: x\n", true},
		{"leading blank lines go", "^fix: ", "\n  \n\r\nfix: x\r\n", true},
		{"trailing blanks go", "x$", "fix: x  \t\n", true},
		{"from the scissors line on", "^fix: ", "\n# ------------------------ >8 ------------------------\nfix: x\n",
			false},
		{"an empty message", "^$", "# only comments\n", true},
	}
	for _, c := range cases {
		wf := msgGates(workflow.Entry{When: workflow.Cond{"message": c.pattern}, Action: blocks("x")})
		if out := Decide(message("main", c.raw), wf, true, nil); out.Block != c.holds {
			t.Errorf("%s: %+v", c.name, out)
		}
	}
	other := githook.Githook{Event: workflow.CommitMsg, Message: "; nota\nfix: x\n", Comment: ";"}
	wf := msgGates(workflow.Entry{When: workflow.Cond{"message": "^fix"}, Action: blocks("x")})
	if out := Decide(other, wf, true, nil); !out.Block {
		t.Fatalf("the comment string of git: %+v", out)
	}
	scissors := githook.Githook{Event: workflow.CommitMsg, Message: "fix: x\n// ------------------------ >8 " +
		"------------------------\n", Comment: "//"}
	if out := Decide(scissors, wf, true, nil); !out.Block {
		t.Fatalf("the scissors of a comment string: %+v", out)
	}
}

// AC8: {message} expands to the first line, and with the variable key of the branch an entry demands the key at the
// start of the message on a ticket branch only.
func TestDecideMessagePlaceholders(t *testing.T) {
	wf := msgGates(workflow.Entry{When: workflow.Cond{"branch": "^[A-Z]+-[0-9]+"},
		Unless: workflow.Cond{"message": "^{key}: "}, Action: blocks("«{message}» necesita la clave {key}")})
	out := Decide(message("PROJ-12-login", "arregla el login\n\n# comentario\n"), wf, true, nil)
	if !out.Block || !slices.Equal(out.Reasons, []string{"«arregla el login» necesita la clave PROJ-12"}) {
		t.Fatalf("without the key: %+v", out)
	}
	if out := Decide(message("PROJ-12-login", "PROJ-12: arregla el login\n"), wf, true, nil); out.Block {
		t.Fatalf("with the key: %+v", out)
	}
	if out := Decide(message("main", "arregla el login\n"), wf, true, nil); out.Block {
		t.Fatalf("not a ticket branch: %+v", out)
	}
	run := msgGates(workflow.Entry{Action: runs("", "check", "{message}")})
	if out := Decide(message("main", "fix: x\n"), run, true, nil); !slices.Equal(out.Run, []string{"check", "fix: x"}) {
		t.Fatalf("run: %+v", out)
	}
}

// The values of the placeholders are matched literally in the patterns of message: a branch or a variable with
// metacharacters of a regular expression neither breaks the pattern nor widens it, and the texts of the verbs keep
// them as they are.
func TestDecideMessageLiteralValues(t *testing.T) {
	branch := msgGates(workflow.Entry{Unless: workflow.Cond{"message": "^{branch}: "}, Action: blocks("{branch}: …")})
	cases := []struct {
		name, branch, raw string
		block             bool
	}{
		{"a branch with + passes its own prefix", "fix/c++", "fix/c++: add a\n", false},
		{"a branch with + blocks another prefix", "fix/c++", "fix/c: add a\n", true},
		{"a branch with . passes its own prefix", "feat/a.b", "feat/a.b: add a\n", false},
		{"the . of a branch is no wildcard", "feat/a.b", "feat/aXb: add a\n", true},
		{"a branch with ( and [ passes its own prefix", "fix/(a[b", "fix/(a[b: add a\n", false},
	}
	for _, c := range cases {
		if out := Decide(message(c.branch, c.raw), branch, true, nil); out.Block != c.block {
			t.Errorf("%s: %+v", c.name, out)
		}
	}
	vars := msgGates(workflow.Entry{Unless: workflow.Cond{"message": "^{scope}: "}, Action: blocks("usa {scope}")})
	vars.Vars = map[string]workflow.Var{"scope": {Branch: "^[^/]+/(.+)$"}}
	if out := Decide(message("feat/a.b", "aXb: add\n"), vars, true, nil); !out.Block ||
		!slices.Equal(out.Reasons, []string{"usa a.b"}) {
		t.Fatalf("a variable with a dot, another prefix: %+v", out)
	}
	if out := Decide(message("feat/a.b", "a.b: add\n"), vars, true, nil); out.Block {
		t.Fatalf("a variable with a dot, its own prefix: %+v", out)
	}
	whole := msgGates(workflow.Entry{When: workflow.Cond{"message": "^{message}$"}, Action: blocks("x")})
	if out := Decide(message("main", "fix(a)+ [b]\n"), whole, true, nil); !out.Block {
		t.Fatalf("{message} with metacharacters matches itself: %+v", out)
	}
}
