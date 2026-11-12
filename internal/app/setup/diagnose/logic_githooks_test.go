package diagnose

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"
)

// githookLines is the part of the report about the hooks of git.
func githookLines(out Output) string {
	var b strings.Builder
	for _, f := range out.Findings {
		if f.Area == "githooks" {
			b.WriteString(f.Level + " " + f.Message + "\n")
		}
	}
	return b.String()
}

// AC15: git listing both hooks of polix is one ok line, right after the hooks of Claude Code; one missing is a
// warning naming its event and polix install; one git lists as disabled is a warning that it is disabled in the git
// configuration, without polix install, which does not enable it; a git that cannot list hooks is a warning that it
// does not run hooks from its configuration; none of them fails the report.
func TestDecideGitHooks(t *testing.T) {
	i := healthy(t)
	i.ListsHooks = true
	i.GitHooks = map[string][]string{"pre-commit": {"lint", "polix-pre-commit"}, "commit-msg": {"polix-commit-msg"}}
	out := Decide(i, false)
	if got := githookLines(out); got != "ok git runs polix githook on pre-commit, commit-msg (githooks need git 2.54 or "+
		"later)\n" || out.Failed {
		t.Fatalf("both listed:\n%s", text(out))
	}
	if got := text(out); !strings.Contains(got, "ok hooks registered in "+filepath.FromSlash("/r/.claude/settings.json")+"\nok githooks ") {
		t.Fatalf("after the hooks of Claude Code:\n%s", got)
	}

	i.GitHooks = map[string][]string{"pre-commit": {"lint"}}
	out = Decide(i, false)
	if got := githookLines(out); got != "warn git does not run polix githook on pre-commit: run polix install\n"+
		"warn git does not run polix githook on commit-msg: run polix install\n" || out.Failed {
		t.Fatalf("missing:\n%s", text(out))
	}

	i.GitHooks = map[string][]string{"pre-commit": {"lint"}, "commit-msg": {"polix-commit-msg"}}
	i.DisabledGitHooks = map[string][]string{"pre-commit": {"polix-pre-commit"}}
	out = Decide(i, false)
	if got := githookLines(out); got != "warn git does not run polix githook on pre-commit: hook.polix-pre-commit "+
		"is disabled in the git configuration (hook.polix-pre-commit.enabled false)\n" || out.Failed {
		t.Fatalf("disabled:\n%s", text(out))
	}

	i.ListsHooks, i.GitHooks, i.DisabledGitHooks = false, map[string][]string{}, nil
	out = Decide(i, false)
	if got := githookLines(out); !strings.HasPrefix(got, "warn git does not run hooks from its configuration") ||
		!strings.Contains(got, "git 2.54 or later") || strings.Count(got, "\n") != 1 || out.Failed {
		t.Fatalf("a git that cannot list hooks:\n%s", text(out))
	}
}

// Outside a repository, or without git, there is no finding about the hooks of git, nor when git was not asked.
func TestDecideGitHooksOutside(t *testing.T) {
	i := healthy(t)
	i.ListsHooks, i.GitHooks = false, map[string][]string{}
	i.RepoErr = errors.New("not inside a git repository")
	if got := githookLines(Decide(i, false)); got != "" {
		t.Fatalf("outside: %s", got)
	}
	i = healthy(t)
	if got := githookLines(Decide(i, false)); got != "" {
		t.Fatalf("not asked: %s", got)
	}
}
