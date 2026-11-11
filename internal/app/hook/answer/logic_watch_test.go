package answer

import (
	"errors"
	"path/filepath"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/mlophez/polix-ai/internal/app/agent"
	"github.com/mlophez/polix-ai/internal/app/hook"
	"github.com/mlophez/polix-ai/internal/app/rule"
	"github.com/mlophez/polix-ai/internal/app/state"
	"github.com/mlophez/polix-ai/internal/app/workflow"
)

// watchYAML is a workflow that watches the edits: AGENTS.md is protected, and post_command injects a context.
const watchYAML = "version: 1\nguardrails:\n  protected_files: [AGENTS.md]\n" +
	"on:\n  post_command:\n    - action: { inject: { text: after, always: true } }\n"

// command returns the evaluation of event name of the main session with the workflow of yaml, for the command line
// and the id of the call of the tool.
func command(t *testing.T, yaml, name, line, toolUse string) hook.Evaluation {
	t.Helper()
	ev := eval(t, yaml, name)
	ev.Event.Command, ev.Event.ToolUse = line, toolUse
	if err := ev.Valid(); err != nil {
		t.Fatal(err)
	}
	return ev
}

// invalid returns ev with a workflow that cannot be used.
func invalid(ev hook.Evaluation) hook.Evaluation {
	ev.Invalid = errors.New("guardrails: unknown key")
	return ev
}

// snapshotLine is the observed snapshot line of the main session for key, on the tree hash and the commit c1.
func snapshotLine(key, hash string) state.Entry {
	return state.Entry{Kind: state.KindObserved, Outcome: state.OutcomeSnapshot, Agent: "claude", Session: "s1",
		Event: workflow.PreCommand, Actor: "session", Branch: "main", Key: key, Hash: hash, Head: "c1"}
}

// asked returns the steps of steps that match.
func asked(steps []hook.Step, match func(hook.Step) bool) []hook.Step {
	var out []hook.Step
	for _, s := range steps {
		if match(s) {
			out = append(out, s)
		}
	}
	return out
}

// AC1, AC9, AC10: a pre_command that no protection denies writes one snapshot line, with the hash, the HEAD commit
// and the key, the id of the call or else the command line, when the workflow watches; without a watching
// workflow, with an invalid one, on a denied command or on a pre_commit nothing is hashed nor written.
func TestWatchSnapshot(t *testing.T) {
	f := facts{branch: "main", head: "c1", hashes: []string{"h0"}}
	for name, c := range map[string]struct {
		yaml, line, toolUse, key string
	}{
		"protected": {watchYAML, "python3 script.py", "toolu_1", "toolu_1"},
		"pre_edit":  {"version: 1\non:\n  pre_edit:\n    - action: { inject: x }\n", "make", "", "make"},
		"format":    {"version: 1\nguardrails:\n  format_on_edit:\n    - run: \"gofmt -w {path}\"\n", "ls", "t", "t"},
	} {
		out, steps := walk(t, command(t, c.yaml, workflow.PreCommand, c.line, c.toolUse), f)
		obs := observed(steps)
		want := state.Entry{Kind: state.KindObserved, Outcome: state.OutcomeSnapshot, Agent: "claude", Session: "s1",
			Event: workflow.PreCommand, Actor: "session", Branch: "main", Key: c.key, Hash: "h0", Head: "c1"}
		if out.Answer.Block || len(obs) != 1 || obs[0] != want {
			t.Errorf("%s: answer %+v, observed %+v", name, out.Answer, obs)
		}
	}

	for name, ev := range map[string]hook.Evaluation{
		"not watching": command(t, "version: 1\non:\n  pre_command:\n    - action: { inject: { text: x, always: true } }\n",
			workflow.PreCommand, "ls", "toolu_1"),
		"invalid":   invalid(command(t, watchYAML, workflow.PreCommand, "ls", "toolu_1")),
		"denied":    command(t, watchYAML, workflow.PreCommand, "rm AGENTS.md", "toolu_1"),
		"precommit": command(t, watchYAML, workflow.PreCommit, "git commit -m x", "toolu_1"),
	} {
		out, steps := walk(t, ev, f)
		if hashed := asked(steps, func(s hook.Step) bool { return s.Hash }); len(hashed) > 0 || len(observed(steps)) > 0 {
			t.Errorf("%s: hashed %d times, observed %+v", name, len(hashed), observed(steps))
		}
		if name == "denied" && (!out.Answer.Block ||
			!slices.Equal(out.Answer.Reasons, []string{"AGENTS.md is protected in polix.yaml: only a person changes it."})) {
			t.Errorf("rm AGENTS.md is not denied before it runs: %+v", out.Answer)
		}
	}
}

// AC2: a post_command whose command changed a protected path asks for the restore of that path from the tree of its
// snapshot, starts a new epoch, refuses naming the path and logs the denial of protected_files; the other changed
// paths are not restored, and the entries of post_command still run.
func TestWatchRestore(t *testing.T) {
	ev := command(t, watchYAML, workflow.PostCommand, "python3 script.py", "toolu_1")
	f := facts{branch: "main", head: "c1", hashes: []string{"new"}, trees: []string{"AGENTS.md", "main.go"},
		state: state.State{Entries: []state.Entry{snapshotLine("toolu_1", "old")}}}
	out, steps := walk(t, ev, f)

	compares := asked(steps, func(s hook.Step) bool { return s.Compare != "" })
	if len(compares) != 1 || compares[0].Compare != "old" || compares[0].Tree != "new" {
		t.Fatalf("comparisons %+v", compares)
	}
	restores := asked(steps, func(s hook.Step) bool { return s.Restore != nil })
	if len(restores) != 1 || !slices.Equal(restores[0].Restore, []string{"AGENTS.md"}) || restores[0].Source != "old" {
		t.Fatalf("restores %+v", restores)
	}
	reason := "AGENTS.md is protected in polix.yaml: a command changed it and polix put it back."
	if !out.Answer.Block || !slices.Equal(out.Answer.Reasons, []string{reason}) ||
		!slices.Equal(out.Answer.Contexts, []string{"after"}) || len(out.Answer.Warnings) > 0 {
		t.Fatalf("answer %+v", out.Answer)
	}
	if out.Facts.Epoch != 1 {
		t.Fatalf("the restore did not start a new epoch: %d", out.Facts.Epoch)
	}
	var denials []state.Entry
	for _, e := range appended(steps) {
		if e.Kind == state.KindDecision && e.Outcome == "deny" {
			denials = append(denials, e)
		}
	}
	if len(denials) != 1 || denials[0].Action != protectedFilesShortcut || denials[0].Reason != reason {
		t.Fatalf("denials %+v", denials)
	}
}

// AC8: a restore that fails refuses with the path and the error, logs the denial with it and warns the user to put
// the file back by hand.
func TestWatchRestoreFails(t *testing.T) {
	ev := command(t, watchYAML, workflow.PostCommand, "python3 script.py", "toolu_1")
	f := facts{branch: "main", head: "c1", hashes: []string{"new"}, trees: []string{"AGENTS.md"},
		state:      state.State{Entries: []state.Entry{snapshotLine("toolu_1", "old")}},
		restoreErr: errors.New("AGENTS.md: permission denied")}
	out, steps := walk(t, ev, f)
	reason := "AGENTS.md is protected in polix.yaml: a command changed it and polix could not put it back " +
		"(AGENTS.md: permission denied)."
	if !out.Answer.Block || !slices.Equal(out.Answer.Reasons, []string{reason}) || len(out.Answer.Warnings) != 1 ||
		!strings.Contains(out.Answer.Warnings[0], "put it back by hand") {
		t.Fatalf("answer %+v", out.Answer)
	}
	for _, e := range appended(steps) {
		if e.Kind == state.KindDecision && e.Outcome == "deny" && e.Reason != reason {
			t.Fatalf("denial %+v", e)
		}
	}
}

// AC5: when HEAD moved between the snapshot and the post_command nothing is compared nor restored, and a skip of
// builtin is logged.
func TestWatchHeadMoved(t *testing.T) {
	ev := command(t, watchYAML, workflow.PostCommand, "git pull", "toolu_1")
	f := facts{branch: "main", head: "c2", hashes: []string{"new"}, trees: []string{"AGENTS.md"},
		state: state.State{Entries: []state.Entry{snapshotLine("toolu_1", "old")}}}
	out, steps := walk(t, ev, f)
	if acts := asked(steps, func(s hook.Step) bool { return s.Compare != "" || s.Restore != nil || s.Hash }); len(acts) > 0 {
		t.Fatalf("steps %+v", acts)
	}
	if out.Answer.Block {
		t.Fatalf("answer %+v", out.Answer)
	}
	var skips []state.Entry
	for _, e := range appended(steps) {
		if e.Kind == state.KindDecision {
			skips = append(skips, e)
		}
	}
	if len(skips) != 1 || skips[0].Action != builtinProtection || skips[0].Outcome != "skip" {
		t.Fatalf("decisions %+v", skips)
	}
}

// AC7: without a snapshot nothing is compared, with an unknown hash nothing is compared nor refused, and two trees
// that cannot be compared refuse nothing and warn the user.
func TestWatchNothingToCompare(t *testing.T) {
	snap := state.State{Entries: []state.Entry{snapshotLine("toolu_1", "old")}}
	for name, c := range map[string]struct {
		f       facts
		compare bool
		warning string
	}{
		"no snapshot":  {facts{branch: "main", head: "c1", hashes: []string{"new"}, trees: []string{"AGENTS.md"}}, false, ""},
		"other key":    {facts{branch: "main", head: "c1", hashes: []string{"new"}, trees: []string{"AGENTS.md"}, state: state.State{Entries: []state.Entry{snapshotLine("toolu_9", "old")}}}, false, ""},
		"unknown hash": {facts{branch: "main", head: "c1", trees: []string{"AGENTS.md"}, state: snap}, false, ""},
		"unreadable":   {facts{branch: "main", head: "c1", hashes: []string{"new"}, stateErr: errors.New("broken"), state: snap}, false, ""},
		"compare fails": {facts{branch: "main", head: "c1", hashes: []string{"new"}, treeErr: errors.New("bad object"),
			state: snap}, true, "polix cannot compare the content before and after the command (bad object)"},
	} {
		out, steps := walk(t, command(t, watchYAML, workflow.PostCommand, "python3 script.py", "toolu_1"), c.f)
		compares := asked(steps, func(s hook.Step) bool { return s.Compare != "" })
		restores := asked(steps, func(s hook.Step) bool { return s.Restore != nil })
		if (len(compares) > 0) != c.compare || len(restores) > 0 || out.Answer.Block {
			t.Errorf("%s: compares %+v, restores %+v, answer %+v", name, compares, restores, out.Answer)
		}
		warned := slices.ContainsFunc(out.Answer.Warnings, func(w string) bool { return strings.Contains(w, "compare") })
		if c.warning == "" && warned || c.warning != "" && !strings.HasPrefix(strings.Join(out.Answer.Warnings, "\n"), c.warning) {
			t.Errorf("%s: warnings %q", name, out.Answer.Warnings)
		}
	}
}

// AC3: a post_command follows each path its command changed that is not protected with a pre_edit and then a
// post_edit of that path made by the command, in the order of the comparison, in the session, by the actor and the
// agent of the command, located inside the root; the protected path is restored instead.
func TestWatchFollows(t *testing.T) {
	ev := command(t, watchYAML, workflow.PostCommand, "python3 gen.py", "toolu_1")
	f := facts{branch: "main", head: "c1", hashes: []string{"new"}, trees: []string{"main.go", "AGENTS.md", "docs/a.md"},
		state: state.State{Entries: []state.Entry{snapshotLine("toolu_1", "old")}}}
	out, steps := walk(t, ev, f)
	var want []agent.Event
	for _, p := range []string{"main.go", "docs/a.md"} {
		for _, name := range []string{workflow.PreEdit, workflow.PostEdit} {
			want = append(want, agent.Event{Name: name, Agent: agent.Claude, Session: "s1", Actor: agent.ActorSession,
				Cwd: ev.Root, Path: p, AbsPath: filepath.Join(ev.Root, filepath.FromSlash(p)), ByCommand: true})
		}
	}
	if !reflect.DeepEqual(out.Follow, want) {
		t.Fatalf("follow %+v, want %+v", out.Follow, want)
	}
	if restores := asked(steps, func(s hook.Step) bool { return s.Restore != nil }); len(restores) != 1 ||
		!slices.Equal(restores[0].Restore, []string{"AGENTS.md"}) {
		t.Fatalf("restores %+v", restores)
	}

	// The other events never follow.
	if out, _ := walk(t, command(t, watchYAML, workflow.PreCommand, "python3 gen.py", "toolu_1"), f); len(out.Follow) > 0 {
		t.Fatalf("a pre_command follows %+v", out.Follow)
	}
}

// AC6: past 25 changed paths that are not protected none follows: the agent gets one context with the count, logged
// as an inform of builtin, and the protected path among them is still restored; 25 paths still follow.
func TestWatchTooManyEdits(t *testing.T) {
	paths := func(n int) []string {
		out := []string{"AGENTS.md"}
		for i := range n {
			out = append(out, "gen/f"+strconv.Itoa(i)+".go")
		}
		return out
	}
	snap := state.State{Entries: []state.Entry{snapshotLine("toolu_1", "old")}}
	ev := command(t, watchYAML, workflow.PostCommand, "make gen", "toolu_1")

	out, steps := walk(t, ev, facts{branch: "main", head: "c1", hashes: []string{"new"}, trees: paths(26), state: snap})
	count := "A command changed 26 files, more than 25: polix did not evaluate them as edits"
	counted := slices.ContainsFunc(out.Answer.Contexts, func(c string) bool { return strings.HasPrefix(c, count) })
	if len(out.Follow) > 0 || !counted || len(out.Answer.Contexts) != 2 {
		t.Fatalf("follow %d, answer %+v", len(out.Follow), out.Answer)
	}
	if informs := decided(steps, "inform"); len(informs) != 1 || informs[0].Action != builtinProtection ||
		!strings.HasPrefix(informs[0].Reason, count) {
		t.Fatalf("informs %+v", informs)
	}
	if restores := asked(steps, func(s hook.Step) bool { return s.Restore != nil }); len(restores) != 1 ||
		!slices.Equal(restores[0].Restore, []string{"AGENTS.md"}) || !out.Answer.Block {
		t.Fatalf("restores %+v, answer %+v", restores, out.Answer)
	}

	out, steps = walk(t, ev, facts{branch: "main", head: "c1", hashes: []string{"new"}, trees: paths(25), state: snap})
	if len(out.Follow) != 50 || len(decided(steps, "inform")) > 0 {
		t.Fatalf("25 paths: follow %d, answer %+v", len(out.Follow), out.Answer)
	}
}

// derivedYAML protects src/a.go, formats every edit, and before an edit injects the rules of its path and blocks.
const derivedYAML = `version: 1
guardrails:
  protected_files: [src/a.go]
  format_on_edit:
    - run: "gofmt -w {path}"
on:
  pre_edit:
    - action: { inject: { rules: "{path}" } }
    - action: { block: "No hay plan" }
`

// followed returns the evaluation of the edit name of path that a command made, as a post_command of the workflow of
// yaml follows it.
func followed(t *testing.T, yaml, name, path string) hook.Evaluation {
	t.Helper()
	ev := command(t, yaml, workflow.PostCommand, "python3 gen.py", "toolu_1")
	out, _ := walk(t, ev, facts{branch: "main", head: "c1", hashes: []string{"new"}, trees: []string{path},
		state: state.State{Entries: []state.Entry{snapshotLine("toolu_1", "old")}}})
	for _, e := range out.Follow {
		if e.Name == name {
			return with(t, ev, func(ev *agent.Event) { *ev = e })
		}
	}
	t.Fatalf("no %s of %s follows: %+v", name, path, out.Follow)
	return hook.Evaluation{}
}

// AC3, AC4: a pre_edit a command made gets the rules of its path as context, is not denied for a protected path,
// and its block reaches the agent as context, logged as inform, while the same entry still refuses an edit of the
// edit tool.
func TestWatchDerivedPreEdit(t *testing.T) {
	f := facts{branch: "main", hashes: []string{"h"}, rules: []rule.Rule{{ID: "GO-1", Description: "uno",
		Scope: []string{"src/**"}}}}
	pre := followed(t, derivedYAML, workflow.PreEdit, "src/c.go")
	protected := with(t, pre, func(e *agent.Event) {
		e.Path, e.AbsPath = "src/a.go", filepath.Join(pre.Root, "src", "a.go")
	})
	for _, ev := range []hook.Evaluation{pre, protected} {
		out, steps := walk(t, ev, f)
		if out.Answer.Block || len(out.Answer.Contexts) != 2 ||
			!strings.Contains(out.Answer.Contexts[0], "- GO-1: uno") || out.Answer.Contexts[1] != "No hay plan" ||
			len(out.Follow) > 0 {
			t.Fatalf("%s: answer %+v", ev.Event.Path, out.Answer)
		}
		if informs := decided(steps, "inform"); len(informs) != 1 || informs[0].Action != workflow.VerbBlock ||
			informs[0].Reason != "No hay plan" || len(decided(steps, "deny")) > 0 || len(decided(steps, "block")) > 0 {
			t.Fatalf("%s: decisions %+v", ev.Event.Path, appended(steps))
		}
	}

	tool := edit(t, eval(t, derivedYAML, workflow.PreEdit), workflow.PreEdit, "src/b.go", agent.ActorSession)
	out, steps := walk(t, tool, f)
	if !out.Answer.Block || !slices.Equal(out.Answer.Reasons, []string{"No hay plan"}) ||
		len(decided(steps, "block")) != 1 {
		t.Fatalf("the edit tool: answer %+v", out.Answer)
	}
}

// AC3, AC4: a post_edit a command made runs format_on_edit on its path, and a failure reaches the agent as context;
// a failing run of a pre_edit a command made informs too.
func TestWatchDerivedRuns(t *testing.T) {
	f := facts{branch: "main", hashes: []string{"h", "h"}, runErr: errors.New("exit status 2")}
	post := followed(t, derivedYAML, workflow.PostEdit, "src/c.go")
	out, steps := walk(t, post, f)
	if got := runs(steps); len(got) != 1 || !slices.Equal(got[0], []string{"gofmt", "-w", "src/c.go"}) {
		t.Fatalf("runs %q", got)
	}
	if out.Answer.Block || len(out.Answer.Contexts) != 1 ||
		!strings.HasPrefix(out.Answer.Contexts[0], "gofmt -w src/c.go failed: exit status 2") {
		t.Fatalf("answer %+v", out.Answer)
	}

	yaml := "version: 1\non:\n  pre_edit:\n    - action: { run: [lint, \"{path}\"] }\n    - action: { inject: { text: x, always: true } }\n"
	pre := followed(t, yaml, workflow.PreEdit, "src/c.go")
	out, steps = walk(t, pre, f)
	if out.Answer.Block || len(out.Answer.Contexts) != 2 ||
		!strings.HasPrefix(out.Answer.Contexts[0], "lint src/c.go failed: exit status 2") ||
		out.Answer.Contexts[1] != "x" || len(decided(steps, "inform")) != 1 {
		t.Fatalf("pre_edit: answer %+v, decisions %+v", out.Answer, appended(steps))
	}
}

// AC9: two snapshots of different keys pair with their own post_command, and without an id the command line pairs
// them.
func TestWatchPairs(t *testing.T) {
	st := state.State{Entries: []state.Entry{snapshotLine("toolu_1", "a"), snapshotLine("toolu_2", "b"),
		snapshotLine("make gen", "c")}}
	f := facts{branch: "main", head: "c1", hashes: []string{"new"}, state: st}
	for key, want := range map[string]string{"toolu_1": "a", "toolu_2": "b", "": "c"} {
		_, steps := walk(t, command(t, watchYAML, workflow.PostCommand, "make gen", key), f)
		compares := asked(steps, func(s hook.Step) bool { return s.Compare != "" })
		if len(compares) != 1 || compares[0].Compare != want {
			t.Errorf("%q: comparisons %+v, want from %s", key, compares, want)
		}
	}
}

// A path the command removed is not followed as an edit, so format_on_edit never runs on a file that no longer
// exists; a protected path it removed is still put back.
func TestWatchRemoved(t *testing.T) {
	ev := command(t, watchYAML, workflow.PostCommand, "git mv a.go b.go", "toolu_1")
	f := facts{branch: "main", head: "c1", hashes: []string{"new"}, trees: []string{"AGENTS.md", "a.go", "b.go"},
		removed: []string{"AGENTS.md", "a.go"}, state: state.State{Entries: []state.Entry{snapshotLine("toolu_1", "old")}}}
	out, steps := walk(t, ev, f)
	var paths []string
	for _, e := range out.Follow {
		paths = append(paths, e.Path)
	}
	if !slices.Equal(paths, []string{"b.go", "b.go"}) {
		t.Fatalf("follow %+v", out.Follow)
	}
	if restores := asked(steps, func(s hook.Step) bool { return s.Restore != nil }); len(restores) != 1 ||
		!slices.Equal(restores[0].Restore, []string{"AGENTS.md"}) {
		t.Fatalf("restores %+v", restores)
	}
}

// A post_command that finds its snapshot writes a watched line with its key, and a snapshot followed by a watched
// line of its key is not compared again: a post_command of the same command line without a snapshot of its own
// compares nothing; a newer snapshot of the key is compared.
func TestWatchSnapshotUsedOnce(t *testing.T) {
	watched := state.Entry{Kind: state.KindObserved, Outcome: state.OutcomeWatched, Agent: "claude", Session: "s1",
		Event: workflow.PostCommand, Actor: "session", Branch: "main", Key: "make gen"}
	ev := command(t, watchYAML, workflow.PostCommand, "make gen", "")
	f := facts{branch: "main", head: "c1", hashes: []string{"new"}, trees: []string{"AGENTS.md"},
		state: state.State{Entries: []state.Entry{snapshotLine("make gen", "old")}}}
	_, steps := walk(t, ev, f)
	if obs := observed(steps); len(obs) != 1 || obs[0] != watched {
		t.Fatalf("observed %+v, want %+v", obs, watched)
	}

	for name, c := range map[string]struct {
		entries []state.Entry
		compare bool
	}{
		"used":              {[]state.Entry{snapshotLine("make gen", "old"), watched}, false},
		"newer snapshot":    {[]state.Entry{snapshotLine("make gen", "a"), watched, snapshotLine("make gen", "b")}, true},
		"watched other key": {[]state.Entry{snapshotLine("make gen", "old"), func() state.Entry { w := watched; w.Key = "ls"; return w }()}, true},
	} {
		f.state = state.State{Entries: c.entries}
		out, steps := walk(t, ev, f)
		compares := asked(steps, func(s hook.Step) bool { return s.Compare != "" })
		if (len(compares) > 0) != c.compare || !c.compare && (out.Answer.Block || len(observed(steps)) > 0) {
			t.Errorf("%s: compares %+v, answer %+v, observed %+v", name, compares, out.Answer, observed(steps))
		}
	}
}

// AC5: a snapshot records the commit of refs/stash, and when refs/stash moved before the post_command (a git stash)
// nothing is compared nor restored, and a skip of builtin is logged.
func TestWatchStashMoved(t *testing.T) {
	_, steps := walk(t, command(t, watchYAML, workflow.PreCommand, "git stash", "toolu_1"),
		facts{branch: "main", head: "c1", hashes: []string{"h0"}, refs: "s1"})
	if obs := observed(steps); len(obs) != 1 || obs[0].Refs != "s1" {
		t.Fatalf("observed %+v", obs)
	}

	snap := snapshotLine("toolu_1", "old")
	snap.Refs = "s1"
	ev := command(t, watchYAML, workflow.PostCommand, "git stash", "toolu_1")
	out, steps := walk(t, ev, facts{branch: "main", head: "c1", refs: "s2", hashes: []string{"new"},
		trees: []string{"AGENTS.md"}, state: state.State{Entries: []state.Entry{snap}}})
	if acts := asked(steps, func(s hook.Step) bool { return s.Compare != "" || s.Restore != nil }); len(acts) > 0 ||
		out.Answer.Block {
		t.Fatalf("steps %+v, answer %+v", acts, out.Answer)
	}
	if skips := decided(steps, "skip"); len(skips) != 1 || skips[0].Action != builtinProtection ||
		!strings.HasPrefix(skips[0].Reason, `the stash or the operation of git in progress went from "s1" to "s2"`) {
		t.Fatalf("skips %+v", skips)
	}
}

// Each protected path is put back by a step of its own, those the command removed last, so a path renamed only in
// case is removed before its own name is written back; a path that cannot be put back is refused with its own error
// and named alone in the warning, while the other one is refused as put back.
func TestWatchRestoreEachPath(t *testing.T) {
	yaml := "version: 1\nguardrails:\n  protected_files: [AGENTS.md, CLAUDE.md, claude.md]\n"
	ev := command(t, yaml, workflow.PostCommand, "python3 x.py", "toolu_1")
	f := facts{branch: "main", head: "c1", hashes: []string{"new"},
		trees: []string{"AGENTS.md", "CLAUDE.md", "claude.md"}, removed: []string{"CLAUDE.md"},
		state:       state.State{Entries: []state.Entry{snapshotLine("toolu_1", "old")}},
		restoreErrs: map[string]error{"AGENTS.md": errors.New("permission denied")}}
	out, steps := walk(t, ev, f)
	var order []string
	for _, s := range asked(steps, func(s hook.Step) bool { return s.Restore != nil }) {
		order = append(order, s.Restore...)
	}
	if !slices.Equal(order, []string{"AGENTS.md", "claude.md", "CLAUDE.md"}) {
		t.Fatalf("restores %q", order)
	}
	want := []string{
		"AGENTS.md is protected in polix.yaml: a command changed it and polix could not put it back (permission denied).",
		"claude.md is protected in polix.yaml: a command changed it and polix put it back.",
		"CLAUDE.md is protected in polix.yaml: a command changed it and polix put it back.",
	}
	if !out.Answer.Block || !slices.Equal(out.Answer.Reasons, want) || len(out.Answer.Warnings) != 1 ||
		!strings.HasPrefix(out.Answer.Warnings[0], "polix could not put back AGENTS.md, protected") {
		t.Fatalf("answer %+v", out.Answer)
	}
	if out.Facts.Epoch != 1 {
		t.Fatalf("the restores started %d epochs, want 1", out.Facts.Epoch)
	}
}

// AC5: a merge that stopped on its conflicts, seen in the refs git moves without HEAD, leaves the files alone.
func TestWatchOperationInProgress(t *testing.T) {
	ev := command(t, watchYAML, workflow.PostCommand, "git merge other", "toolu_1")
	out, steps := walk(t, ev, facts{branch: "main", head: "c1", refs: "MERGE_HEAD c9", hashes: []string{"new"},
		trees: []string{"AGENTS.md", "main.go"}, state: state.State{Entries: []state.Entry{snapshotLine("toolu_1", "old")}}})
	if acts := asked(steps, func(s hook.Step) bool { return s.Compare != "" || s.Restore != nil }); len(acts) > 0 ||
		out.Answer.Block || len(out.Follow) > 0 {
		t.Fatalf("steps %+v, answer %+v, follow %+v", acts, out.Answer, out.Follow)
	}
}

// The snapshot comes after the entries of pre_command: a pre_command one of them blocks writes none and computes no
// hash for it, and one whose run may rewrite files records the content of the epoch the run left.
func TestWatchSnapshotAfterEntries(t *testing.T) {
	blocked := "version: 1\nguardrails:\n  protected_files: [AGENTS.md]\non:\n  pre_command:\n" +
		"    - action: { block: \"No hay plan\" }\n"
	out, steps := walk(t, command(t, blocked, workflow.PreCommand, "make", "toolu_1"),
		facts{branch: "main", head: "c1", hashes: []string{"h0"}})
	if !out.Answer.Block || len(observed(steps)) > 0 {
		t.Fatalf("answer %+v, observed %+v", out.Answer, observed(steps))
	}

	ran := "version: 1\nguardrails:\n  protected_files: [AGENTS.md]\non:\n  pre_command:\n" +
		"    - action: { run: { command: [gen], always: true } }\n"
	out, steps = walk(t, command(t, ran, workflow.PreCommand, "make", "toolu_1"),
		facts{branch: "main", head: "c1", hashes: []string{"h0", "h1"}})
	if obs := observed(steps); out.Answer.Block || len(obs) != 1 || obs[0].Outcome != state.OutcomeSnapshot ||
		obs[0].Hash != "h1" {
		t.Fatalf("answer %+v, observed %+v", out.Answer, obs)
	}
}
