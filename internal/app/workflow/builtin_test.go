package workflow

import (
	"reflect"
	"slices"
	"testing"

	"github.com/mlophez/polix-ai/internal/platform/glob"
)

// builtinOn is guardrails.builtin with the four built-in good practices on, at line 3.
var builtinOn = Builtin{ForbidCommitNoVerify: true, ForbidGitDestructiveCommands: true, FormatPolixFiles: true,
	CheckPolixFiles: true, Line: 3}

// summaries describes the entries for the tests, in order.
func summaries(entries []Entry) []string {
	out := make([]string, 0, len(entries))
	for _, e := range entries {
		out = append(out, shortcutsSummary(e))
	}
	return out
}

// CUJ-01, CUJ-02, AC-01: format_polix_files expands into two entries first on post_edit, before those of
// format_on_edit: polix config fmt on the polix.yaml of the root and polix rules fmt on the yaml files of the rules
// folder below path, each from guardrails.builtin.format_polix_files. An edit of examples/polix.yaml matches neither,
// and with path custom the rules of ai/rules no longer match.
func TestBuiltinFormat(t *testing.T) {
	g := Guardrails{Builtin: builtinOn, FormatOnEdit: []GuardrailCheck{
		{Run: []string{"gofmt", "-w", "{path}"}, Scope: "*.go", Line: 6}}}
	for path, rules := range map[string]string{"": "ai/rules", "custom": "custom/rules"} {
		w := Workflow{Version: 1, Path: path}.Guard(g)
		want := []string{
			`guardrails.builtin.format_polix_files line=3 when=map[path:/polix.yaml] run ["polix" "config" "fmt"] ""`,
			`guardrails.builtin.format_polix_files line=3 when=map[path:` + rules + `/*.yaml] run ["polix" "rules" "fmt"] ""`,
			`guardrails.format_on_edit line=6 when=map[path:*.go] run ["gofmt" "-w" "{path}"] ""`,
		}
		if got := summaries(w.On[PostEdit]); !slices.Equal(got, want) {
			t.Fatalf("path %q:\n%q\nwant:\n%q", path, got, want)
		}
		if err := w.Valid(); err != nil {
			t.Fatalf("path %q: %v", path, err)
		}
		fmtPath, _ := w.On[PostEdit][0].When["path"].(string)
		rulesPath, _ := w.On[PostEdit][1].When["path"].(string)
		for edited, want := range map[string][2]bool{
			"polix.yaml":           {true, false},
			"examples/polix.yaml":  {false, false},
			"ai/rules/go.yaml":     {false, path == ""},
			"custom/rules/go.yaml": {false, path == "custom"},
		} {
			if got := [2]bool{glob.Path(fmtPath, edited), glob.Path(rulesPath, edited)}; got != want {
				t.Errorf("path %q, edit of %s: runs %v, want %v", path, edited, got, want)
			}
		}
	}
}

// CUJ-03, AC-02: check_polix_files expands into the first entry of githooks.pre_commit, before those of
// check_on_save and those written in githooks: polix config check when a staged file is the polix.yaml of the root
// (and none for the rules folder, which polix rules check validates), from guardrails.builtin.check_polix_files; and, as any entry of githooks, the start
// installs the hooks of git.
func TestBuiltinCheckPolixFiles(t *testing.T) {
	g := Guardrails{Builtin: builtinOn, CheckOnSave: []GuardrailCheck{{Run: []string{"go", "test", "./..."}, Line: 8}}}
	written := map[string][]Entry{PreCommit: {{Action: Action{Block: &Block{Message: "no"}}, Line: 12}}}
	w := Workflow{Version: 1, Githooks: written}.Guard(g).ExpandGithooks()
	want := []string{
		`guardrails.builtin.check_polix_files line=3 when=map[changed:[/polix.yaml]] run ["polix" "config" "check"] ""`,
		`guardrails.check_on_save line=8 when=map[] run ["go" "test" "./..."] ""`,
		` line=12 when=map[] block "no"`,
	}
	if got := summaries(w.Githooks[PreCommit]); !slices.Equal(got, want) {
		t.Fatalf("githooks:\n%q\nwant:\n%q", got, want)
	}
	if start := w.On[Start]; len(start) != 1 || !slices.Equal(start[0].Action.Run.Command, githooksInstallCommand) {
		t.Fatalf("the start installs the hooks of git: %q", summaries(start))
	}
	custom := Workflow{Version: 1, Path: "custom"}.Guard(g)
	if c, _ := custom.Githooks[PreCommit][0].When["changed"].([]any); !slices.Equal(c, []any{"/polix.yaml"}) {
		t.Fatalf("the files that trigger the check: %v", c)
	}
}

// CUJ-08, AC-05: with format_polix_files or check_polix_files false its entries are not expanded, and without other
// entries of githooks neither is the install of the hooks of git on start; the switches reach the workflow.
func TestBuiltinOffEntries(t *testing.T) {
	off := Builtin{Line: 3}
	w := Workflow{Version: 1}.Guard(Guardrails{Builtin: off}).ExpandGithooks()
	if len(w.On) != 0 || len(w.Githooks) != 0 || !reflect.DeepEqual(w.Builtin, off) {
		t.Fatalf("all off: %+v", w)
	}
	noFormat := builtinOn
	noFormat.FormatPolixFiles = false
	w = Workflow{Version: 1}.Guard(Guardrails{Builtin: noFormat}).ExpandGithooks()
	if len(w.On[PostEdit]) != 0 || len(w.Githooks[PreCommit]) != 1 || len(w.On[Start]) != 1 {
		t.Fatalf("format off: %q %q", summaries(w.On[PostEdit]), summaries(w.Githooks[PreCommit]))
	}
	noCheck := builtinOn
	noCheck.CheckPolixFiles = false
	w = Workflow{Version: 1}.Guard(Guardrails{Builtin: noCheck}).ExpandGithooks()
	if len(w.On[PostEdit]) != 2 || len(w.Githooks) != 0 || len(w.On[Start]) != 0 {
		t.Fatalf("check off: %q %q", summaries(w.On[Start]), summaries(w.Githooks[PreCommit]))
	}
	saved := Guardrails{Builtin: noCheck, CheckOnSave: []GuardrailCheck{{Run: []string{"go", "test"}, Line: 5}}}
	if w := (Workflow{Version: 1}).Guard(saved).ExpandGithooks(); len(w.On[Start]) != 1 {
		t.Fatalf("check off with check_on_save: %q", summaries(w.On[Start]))
	}
}

// A problem of guardrails.builtin is valid with its text and its line, and the section is not valid while it has
// one.
func TestBuiltinValid(t *testing.T) {
	p := GuardrailProblem{Text: "guardrails.builtin: format_polix_files must be true or false", Line: 4}
	if err := p.Valid(); err != nil {
		t.Fatal(err)
	}
	if (GuardrailProblem{Line: 4}).Valid() == nil || (GuardrailProblem{Text: "x"}).Valid() == nil {
		t.Fatal("a problem needs its text and its line")
	}
	if err := builtinOn.Valid(); err != nil {
		t.Fatal(err)
	}
	bad := builtinOn
	bad.Problems = []GuardrailProblem{p}
	if bad.Valid() == nil || (Guardrails{Builtin: bad}).Valid() == nil {
		t.Fatal("a builtin with problems is not valid")
	}
}

// The reasons of the preset items of forbidden_commands, as the refusals give them before the line (LIT-01 to
// LIT-04).
const (
	lit01 = "Never skip the git hooks: fix what they block and commit again."
	lit02 = "Never force a push: ask the user to run it if it is needed."
	lit03 = "git reset --hard discards changes for good: ask the user to run it."
	lit04 = "git clean -f deletes untracked files for good: ask the user to run it."
)

// The items of forbidden_commands the presets of builtin expand into, in order, as pattern, reason and origin: those
// of forbid_commit_no_verify (LIT-12) and then those of forbid_git_destructive_commands (LIT-13).
var (
	presetNoVerify = []ForbiddenCommand{
		{Pattern: "git commit*--no-verify*", Reason: lit01, From: "guardrails.builtin.forbid_commit_no_verify"},
		{Pattern: "git commit* -n *", Reason: lit01, From: "guardrails.builtin.forbid_commit_no_verify"},
		{Pattern: "git commit* -n", Reason: lit01, From: "guardrails.builtin.forbid_commit_no_verify"},
	}
	presetDestructive = []ForbiddenCommand{
		{Pattern: "git push*--force*", Reason: lit02, From: "guardrails.builtin.forbid_git_destructive_commands"},
		{Pattern: "git push* -f*", Reason: lit02, From: "guardrails.builtin.forbid_git_destructive_commands"},
		{Pattern: "git reset*--hard*", Reason: lit03, From: "guardrails.builtin.forbid_git_destructive_commands"},
		{Pattern: "git clean* -f*", Reason: lit04, From: "guardrails.builtin.forbid_git_destructive_commands"},
		{Pattern: "git clean*--force*", Reason: lit04, From: "guardrails.builtin.forbid_git_destructive_commands"},
	}
)

// atBuiltin returns a copy of the items at the line of builtinOn, the line of builtin they come from.
func atBuiltin(items []ForbiddenCommand) []ForbiddenCommand {
	out := slices.Clone(items)
	for i := range out {
		out[i].Line = builtinOn.Line
	}
	return out
}

// CUJ-04, CUJ-06, AC-03, AC-04: the presets of builtin expand into items of forbidden_commands before the written
// ones: the patterns of LIT-12 with LIT-01, then those of LIT-13 with LIT-02 to LIT-04, each at the line of builtin
// and from guardrails.builtin and its key; those of forbid_commit_no_verify also without gates of git.
func TestBuiltinForbidPresets(t *testing.T) {
	written := []ForbiddenCommand{{Pattern: "kubectl* apply *", Reason: "Apply through the pipeline.", Line: 9}}
	w := Workflow{Version: 1, ForbiddenCommands: written}.Guard(Guardrails{Builtin: builtinOn})
	want := slices.Concat(atBuiltin(presetNoVerify), atBuiltin(presetDestructive), written)
	if !reflect.DeepEqual(w.ForbiddenCommands, want) {
		t.Fatalf("got:\n%+v\nwant:\n%+v", w.ForbiddenCommands, want)
	}
	if err := w.Valid(); err != nil {
		t.Fatal(err)
	}
	noGates := builtinOn
	noGates.CheckPolixFiles, noGates.Line = false, 0
	w = Workflow{Version: 1}.Guard(Guardrails{Builtin: noGates}).ExpandGithooks()
	if len(w.Githooks) != 0 {
		t.Fatalf("gates of git: %q", summaries(w.Githooks[PreCommit]))
	}
	if want := slices.Concat(presetNoVerify, presetDestructive); !reflect.DeepEqual(w.ForbiddenCommands, want) {
		t.Fatalf("without gates of git:\n%+v\nwant:\n%+v", w.ForbiddenCommands, want)
	}
}

// CUJ-08, AC-05: each forbid switch false leaves out only its items, and with both false only the written items
// stay.
func TestBuiltinOffBlocks(t *testing.T) {
	written := []ForbiddenCommand{{Pattern: "rm -rf /*", Line: 7}}
	noVerify, destructive, both := builtinOn, builtinOn, builtinOn
	noVerify.ForbidCommitNoVerify = false
	destructive.ForbidGitDestructiveCommands = false
	both.ForbidCommitNoVerify, both.ForbidGitDestructiveCommands = false, false
	for name, c := range map[string]struct {
		b    Builtin
		want []ForbiddenCommand
	}{
		"forbid_commit_no_verify false":         {noVerify, slices.Concat(atBuiltin(presetDestructive), written)},
		"forbid_git_destructive_commands false": {destructive, slices.Concat(atBuiltin(presetNoVerify), written)},
		"both false":                            {both, written},
	} {
		w := Workflow{Version: 1, ForbiddenCommands: written}.Guard(Guardrails{Builtin: c.b})
		if !reflect.DeepEqual(w.ForbiddenCommands, c.want) {
			t.Errorf("%s:\n%+v\nwant:\n%+v", name, w.ForbiddenCommands, c.want)
		}
	}
}

// FND-06: with a path that cannot be the folder of the AI, the built-in format and check of the polix files expand
// into nothing, so the only problem of the workflow is the one of path, never one of a rules folder built from it;
// the blocks of the built-in good practices stay on.
func TestBuiltinInvalidPath(t *testing.T) {
	for _, path := range []string{"a{b}", "/abs", "..", "*"} {
		w := Workflow{Version: 1, Path: path}.Guard(Guardrails{Builtin: builtinOn}).ExpandGithooks()
		if len(w.On[PostEdit]) != 0 || len(w.Githooks) != 0 || len(w.On[Start]) != 0 {
			t.Errorf("path %q: %q %q %q", path, summaries(w.On[PostEdit]), summaries(w.Githooks[PreCommit]),
				summaries(w.On[Start]))
		}
		if !w.Builtin.ForbidCommitNoVerify || !w.Builtin.ForbidGitDestructiveCommands {
			t.Errorf("path %q: the blocks stay on: %+v", path, w.Builtin)
		}
		want := w.ValidPath()
		if err := w.Valid(); want == nil || err == nil || err.Error() != want.Error() {
			t.Errorf("path %q: got %v, want only %v", path, err, want)
		}
	}
}
