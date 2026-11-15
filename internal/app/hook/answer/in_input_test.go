package answer

import (
	"encoding/json"
	"slices"
	"testing"

	"github.com/mlophez/polix-ai/internal/app/agent"
	"github.com/mlophez/polix-ai/internal/app/workflow"
)

// CUJ-13, AC-13: every form of staging and committing the start rules allow (templates/session_start.md.tmpl) fires
// the events they promise through the input of a Claude Code hook, and every form they warn about behaves as they
// say: the git add entry (a pre_command entry on git add* or git stage*) fires on the lines that stage with git add
// or git stage, pre_commit on the lines that hold a git commit, and post_commit, once the line ended with exit code
// 0, only when the git commit decides that code. The recognition of commands is not changed by the rules: these
// cases pin it to them.
func TestStartRulesForms(t *testing.T) {
	staging := []string{"git add*", "git stage*"}
	heredoc := "git commit -m \"$(cat <<'EOF'\nAdd the shortcuts\n\nWith a body.\nEOF\n)\""
	cases := []struct {
		line       string
		add        bool // the git add entry fires
		preCommit  bool
		postCommit bool
	}{
		// Allowed: staging with git add or git stage, by the name of git.
		{"git add a.go", true, false, false},
		{"git stage a.go", true, false, false},
		{"git -C . add a.go", true, false, false},
		{"git add . && git commit -m x", true, true, true},
		// Allowed: a commit by its name, last or followed by &&, with its message given.
		{"git commit -m x", false, true, true},
		{"git commit -F msg.txt", false, true, true},
		{heredoc, false, true, true},
		{"cd sub && git commit -m x", false, true, true},
		{"git commit -m x && git push", false, true, true},
		{`bash -c "git commit -m x"`, false, true, true},
		// Warned: after ; or | polix cannot tell whether the commit succeeded.
		{"git commit -m x; git push", false, true, false},
		{"git commit -m x | tail -1", false, true, false},
		// Warned: staging inside the commit, without git add.
		{"git commit -am x", false, true, true},
		{"git commit -m x a.go", false, true, true},
		// Warned: a commit not run by its name.
		{"timeout 60 git commit -m x", false, false, false},
		{"$GIT commit -m x", false, false, false},
		{"git ci -m x", false, false, false},
	}
	for _, c := range cases {
		input, err := json.Marshal(map[string]any{"hook_event_name": "PreToolUse", "tool_name": "Bash",
			"tool_input": map[string]any{"command": c.line}})
		if err != nil {
			t.Fatal(err)
		}
		_, pre, err := claude{}.parse(input)
		if err != nil || len(pre) == 0 || pre[0].Name != workflow.PreCommand {
			t.Fatalf("%q: %+v %v", c.line, pre, err)
		}
		if add := len(matchingCommands(staging, pre[0].Command, false)) > 0; add != c.add {
			t.Errorf("%q: the git add entry fires %v, want %v", c.line, add, c.add)
		}
		if got := names(pre); slices.Contains(got, workflow.PreCommit) != c.preCommit {
			t.Errorf("%q: before it ran %v, pre_commit %v", c.line, got, c.preCommit)
		}
		input, err = json.Marshal(map[string]any{"hook_event_name": "PostToolUse", "tool_name": "Bash",
			"tool_input": map[string]any{"command": c.line}, "tool_response": map[string]any{"exit_code": 0}})
		if err != nil {
			t.Fatal(err)
		}
		_, post, err := claude{}.parse(input)
		if err != nil || len(post) == 0 || post[0].Name != workflow.PostCommand || post[0].ExitCode != 0 {
			t.Fatalf("%q: %+v %v", c.line, post, err)
		}
		if got := names(post); slices.Contains(got, workflow.PostCommit) != c.postCommit {
			t.Errorf("%q: after it ran %v, post_commit %v", c.line, got, c.postCommit)
		}
	}
}

// names returns the names of the events, in order.
func names(evs []agent.Event) []string {
	out := make([]string, 0, len(evs))
	for _, ev := range evs {
		out = append(out, ev.Name)
	}
	return out
}
