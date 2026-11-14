package workflow

import (
	"strings"
	"testing"
)

// cuj01 is the start context of CUJ-01: the root /work/repo and the folders below docs/ai.
const cuj01 = `This project is managed by polix.
Follow these rules:
- Run each git commit as a command of its own: never chain it with &&, ; or |, nor wrap it in sh -c or bash -c.
- Pass the commit message with -m, -F or a heredoc inside -m.
- Otherwise polix does not recognise the commit and skips its pre_commit gates.
- The root of the project is /work/repo.
- Use these folders, below the root, and no other paths:
  - docs/ai/rules/: the rules on how to write the code, one yaml file per topic.
  - docs/ai/plans/: the implementation plans, one per file.
  - docs/ai/reports/: the reports of audits and reviews, one per file.
  - docs/ai/tasks/: the backlog of tasks, one per file.`

// CUJ-01, CUJ-02, AC-07, AC-08: the start context is always the commit lines, the root and the four folders below
// path, cleaned, or below ai by default; a root with backslashes is shown with slashes, and no line asks to change
// files only with the edit tool.
func TestWorkflowSessionContext(t *testing.T) {
	if got := (Workflow{Version: 1, Path: "docs/ai"}).SessionContext("/work/repo"); got != cuj01 {
		t.Fatalf("CUJ-01:\n%s\nwant:\n%s", got, cuj01)
	}
	defaults := strings.ReplaceAll(cuj01, "  - docs/ai/", "  - ai/")
	if got := (Workflow{Version: 1}).SessionContext("/work/repo"); got != defaults {
		t.Fatalf("CUJ-02:\n%s\nwant:\n%s", got, defaults)
	}
	if got := (Workflow{Version: 1, Path: `.\docs\ai\`}).SessionContext("/work/repo"); got != cuj01 {
		t.Fatalf("a cleaned path:\n%s", got)
	}
	got := (Workflow{Version: 1, Path: "docs/ai"}).SessionContext(`C:\work\repo`)
	if want := strings.Replace(cuj01, "/work/repo", "C:/work/repo", 1); got != want {
		t.Fatalf("a root of Windows:\n%s", got)
	}
	for line := range strings.SplitSeq(defaults, "\n") {
		if strings.HasPrefix(line, "- Create and change files") || strings.Contains(line, "edit tool") {
			t.Fatalf("an edit line: %q", line)
		}
	}
}

// AC-07: every line of the start context is of at most 120 characters with the default folders and a short root,
// and after the header lines every line is an item.
func TestWorkflowSessionContextLines(t *testing.T) {
	got := Workflow{Version: 1}.SessionContext("/work/repo")
	items := false
	for i, line := range strings.Split(got, "\n") {
		if n := len([]rune(line)); n > 120 {
			t.Errorf("line %d has %d characters: %q", i, n, line)
		}
		item := strings.HasPrefix(strings.TrimLeft(line, " "), "- ")
		if items && !item {
			t.Errorf("line %d is not an item: %q", i, line)
		}
		items = items || item
	}
	if !items {
		t.Fatalf("no items: %q", got)
	}
}
