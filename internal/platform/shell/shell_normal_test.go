package shell

import "testing"

// The normalized form drops assignments, wrappers with their options and the global options of git.
func TestNormal(t *testing.T) {
	for line, want := range map[string]string{
		"sudo env FOO=1 git add .":                     "git add .",
		"git -C x -c k=v --no-pager add -A":            "git add -A",
		"LANG=C nice -n sort f":                        "sort f",
		"nice -n 10 sort f":                            "10 sort f",
		"git --git-dir .git --work-tree . commit":      "git commit",
		"git --git-dir=.git --namespace n commit -m x": "git commit -m x",
		"git --config-env k=V -p log --grep commit":    "git log --grep commit",
		"/usr/bin/git -C x status":                     "/usr/bin/git status",
		"git commit -m \"add foo\"":                    "git commit -m add foo",
		"echo \"git add\"":                             "echo git add",
		"xargs -0 rm -f":                               "rm -f",
		"FOO=1":                                        "",
		"sudo":                                         "",
		"git -C":                                       "git",
		"time -p go test":                              "go test",
	} {
		cmds, _ := Parse(line, false)
		if got := cmds[0].NormalText(); got != want {
			t.Errorf("NormalText(%q) = %q, want %q", line, got, want)
		}
	}
}
