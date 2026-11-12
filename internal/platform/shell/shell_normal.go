package shell

import (
	"path"
	"regexp"
	"strings"
)

// wrappers run the command that follows them, after their own options.
var wrappers = map[string]bool{"sudo": true, "env": true, "command": true, "nohup": true, "time": true,
	"exec": true, "xargs": true, "nice": true}

// gitValued are the global options of git that take their value as the next word.
var gitValued = map[string]bool{"-C": true, "-c": true, "--git-dir": true, "--work-tree": true, "--namespace": true,
	"--config-env": true}

// assignment is a variable set for the command, as in `LANG=C sort`.
var assignment = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*=`)

// Normal returns the normalized words of the command: without its leading assignments, without the wrappers it runs
// through (sudo, env, command, nohup, time, exec, xargs and nice) with their options but not their values, and, for
// git, without its global options before the subcommand (`git -C x --no-pager add .` gives `git add .`).
func (c Command) Normal() []string {
	ws := c.Words[programStart(c.Words):]
	if len(ws) > 0 && Program(ws[0]) == "git" {
		return append([]string{ws[0]}, gitArgs(ws[1:])...)
	}
	if len(ws) == 0 {
		return nil
	}
	return ws
}

// programStart returns the index of the word that names the program a command runs: past its leading assignments
// and the wrappers it runs through with their options; len(ws) when there is none.
func programStart(ws []string) int {
	i := 0
	for i < len(ws) {
		if assignment.MatchString(ws[i]) {
			i++
			continue
		}
		if !wrappers[Program(ws[i])] {
			return i
		}
		i++
		for i < len(ws) && strings.HasPrefix(ws[i], "-") {
			i++
		}
	}
	return i
}

// NormalText returns the normalized words of the command (see Normal) joined by single spaces.
func (c Command) NormalText() string {
	return strings.Join(c.Normal(), " ")
}

// Program returns the name of the program a word runs: the base name of its path, with slashes or backslashes,
// without `.exe`.
func Program(word string) string {
	return strings.TrimSuffix(path.Base(strings.ReplaceAll(word, `\`, "/")), ".exe")
}

// gitArgs returns the arguments of git from its subcommand on, past its global options: -C, -c, --git-dir,
// --work-tree, --namespace and --config-env with their value, and any other option.
func gitArgs(ws []string) []string {
	for len(ws) > 0 && strings.HasPrefix(ws[0], "-") {
		if gitValued[ws[0]] && len(ws) > 1 {
			ws = ws[1:]
		}
		ws = ws[1:]
	}
	return ws
}
