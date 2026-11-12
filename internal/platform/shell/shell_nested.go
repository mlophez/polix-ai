package shell

import (
	"slices"
	"strings"

	"mvdan.cc/sh/v3/syntax"
)

// shells are the programs whose -c runs a script given as a word.
var shells = map[string]bool{"sh": true, "bash": true, "zsh": true}

// nested adds the commands of the script that the command c runs, when it is a literal `sh -c`, `bash -c`, `zsh -c`
// or `eval` (see script): the script is parsed again, as a line of its own, and its commands follow c. The last
// direct command of a -c script propagates when c does and its failure ends the script; any other command of the
// script, and every command of eval, does not.
func (r *reader) nested(c Command) {
	if text, kind, ok := script(c); ok {
		r.addScript(c, text, kind)
	}
}

// fed adds the commands of a here-document or a literal here-string of the redirections rs that the command c gives
// to a shell reading its script from stdin (see stdinShell): `bash <<'EOF'`, `sh -s <<< 'rm a'`, and, through
// piped, `cat <<'EOF' | bash`. The body of a here-document is parsed as written, so a word with an expansion
// (`rm -rf $HOME`) is a command whose word is not literal, as at the top level.
func (r *reader) fed(c Command, rs []*syntax.Redirect) {
	if !stdinShell(c) {
		return
	}
	for _, rd := range rs {
		if rd.N != nil && rd.N.Value != "0" {
			continue // another descriptor than stdin feeds no script (`bash 3<<EOF`)
		}
		var text string
		var ok bool
		switch rd.Op {
		case syntax.Hdoc, syntax.DashHdoc:
			text, ok = r.hereDoc(rd)
		case syntax.WordHdoc:
			text, ok = r.word(rd.Word)
		default: // any other redirection feeds no script
		}
		if ok {
			r.addScript(c, text, Script)
		}
	}
}

// piped adds the commands of every here-document and here-string of the stages x of a pipe, compound statements
// included, that reach the last stage of y, read from index at, when it is a shell reading stdin (see fed):
// `cat <<'EOF' | bash`, `cat <<'EOF' | grep -v x | bash`, `{ cat <<'EOF'; } | bash`. A filter between them may
// change the script; polix reads it as written, on the side of the guard.
func (r *reader) piped(x, y *syntax.Stmt, at int) {
	for {
		pipe, ok := y.Cmd.(*syntax.BinaryCmd)
		if !ok || pipe.Op != syntax.Pipe && pipe.Op != syntax.PipeAll {
			break
		}
		y = pipe.Y
	}
	if _, call := y.Cmd.(*syntax.CallExpr); !call {
		return
	}
	var feeds []*syntax.Redirect
	syntax.Walk(x, func(n syntax.Node) bool {
		if rd, ok := n.(*syntax.Redirect); ok {
			feeds = append(feeds, rd)
		}
		return true
	})
	text := r.stmtText(y)
	for i := at; i < len(r.out); i++ {
		if r.out[i].Text == text && r.out[i].Nest == r.out[at].Nest {
			r.fed(r.out[i], feeds)
			return
		}
	}
}

// stdinShell reports whether the command c runs `sh`, `bash` or `zsh` reading its script from stdin: without -c,
// and either with -s, with `-` as its script, or without a script file. The shell is the first word named after
// one, past the assignments, the wrappers and their options with a value (`sudo -u x bash`).
func stdinShell(c Command) bool {
	ws := c.Words[programStart(c.Words):]
	i := slices.IndexFunc(ws, func(w string) bool { return shells[Program(w)] })
	if i < 0 {
		return false
	}
	ws = ws[i:]
	i, dashC := shellOptions(ws[1:])
	if dashC {
		return false
	}
	return i == len(ws)-1 || ws[i+1] == "-" || slices.ContainsFunc(ws[1:i+1], func(w string) bool {
		return len(w) > 1 && w[0] == '-' && w[1] != '-' && strings.Contains(w[1:], "s")
	})
}

// hereDoc returns the body of the here-document of rd, expansions included: the script the shell reads. The source
// of the body ends with the line of its closing delimiter, which is no part of the script. With an unquoted
// delimiter the outer shell removes the backslash before a dollar, a backtick or a backslash and joins a line
// continuation, so `\$(rm x)` runs rm: polix does the same before parsing it.
func (r *reader) hereDoc(rd *syntax.Redirect) (string, bool) {
	w := rd.Hdoc
	if w == nil || len(w.Parts) == 0 {
		return "", false
	}
	body := strings.TrimRight(r.source(w), "\n")
	if i := strings.LastIndexByte(body, '\n'); i >= 0 {
		body = body[:i+1]
	}
	if rd.Word != nil && !strings.ContainsAny(r.source(rd.Word), `'"\`) {
		body = hdocEscapes.Replace(body)
	}
	return body, true
}

// hdocEscapes undoes the escapes the shell decodes in the body of a here-document with an unquoted delimiter.
var hdocEscapes = strings.NewReplacer(`\$`, `$`, "\\`", "`", `\\`, `\`, "\\\n", "")

// addScript adds the commands of the script text that the command c runs, nested as kind, after c: the script is
// parsed again, as a line of its own. The last direct command of a script propagates when c does; any other command
// of the script, and every command of eval, does not.
func (r *reader) addScript(c Command, text string, kind Nesting) {
	inner, _ := Parse(text, r.windows)
	last := -1
	for i, cmd := range inner {
		if cmd.Nest != Substitution {
			last = i
		}
	}
	for i := range inner {
		cmd := &inner[i]
		cmd.Propagates = cmd.Propagates && kind == Script && i == last && c.Propagates
		if cmd.Nest != Substitution {
			cmd.Nest = kind
		}
	}
	r.out = append(r.out, inner...)
}

// script returns the script a command runs and how it is nested: the word after the options of `sh`, `bash` or
// `zsh` when one of them is -c (or a cluster with c, as -lc), and the words after `eval` joined by spaces, past the
// assignments and the wrappers of the command (`sudo bash -c`, `env X=1 sh -c`, see programStart). ok is false for
// any other command and when a word of the script is not literal, which no static reading resolves.
func script(c Command) (text string, kind Nesting, ok bool) {
	start := programStart(c.Words)
	ws, lit := c.Words[start:], c.Literal[start:]
	if len(ws) == 0 || !lit[0] {
		return "", NotNested, false
	}
	name := Program(ws[0])
	switch {
	case name == "eval":
		return evalScript(ws[1:], lit[1:])
	case shells[name]:
		return dashC(ws[1:], lit[1:])
	}
	return "", NotNested, false
}

// evalScript returns the words of eval joined by spaces, when there are some and all are literal.
func evalScript(ws []string, lit []bool) (string, Nesting, bool) {
	if len(ws) == 0 {
		return "", NotNested, false
	}
	for _, l := range lit {
		if !l {
			return "", NotNested, false
		}
	}
	return strings.Join(ws, " "), Eval, true
}

// dashC returns the script of the arguments of a shell: the first word after its options, when one of them is -c
// and the word is literal.
func dashC(ws []string, lit []bool) (string, Nesting, bool) {
	i, c := shellOptions(ws)
	if !c || i == len(ws) || !lit[i] {
		return "", NotNested, false
	}
	return ws[i], Script, true
}

// shellOptions returns the index of the first argument of a shell after its options, and whether one of them is -c.
// Each o or O of a cluster of letters takes the name of an option as value, in turn (`bash -o pipefail -c x`,
// `bash -euo pipefail -c x`, `bash -oc pipefail x`), --rcfile and --init-file a file, and -- ends the options.
func shellOptions(ws []string) (int, bool) {
	c := false
	for i := 0; i < len(ws); i++ {
		w := ws[i]
		switch {
		case w == "--":
			return i + 1, c
		case len(w) < 2 || w[0] != '-' && w[0] != '+':
			return i, c
		case w == "--rcfile" || w == "--init-file":
			i++
		case w[1] != '-':
			// A cluster of letters (-e, -euo, -oc, +o): -c in it, and each o or O, wherever it comes, takes the next
			// word in turn (`bash -oo pipefail errexit -c x`).
			c = c || w[0] == '-' && strings.Contains(w[1:], "c")
			i += strings.Count(w[1:], "o") + strings.Count(w[1:], "O")
		}
	}
	return len(ws), c
}
