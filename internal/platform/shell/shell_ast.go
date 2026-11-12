package shell

import (
	"strings"

	"mvdan.cc/sh/v3/syntax"
)

// reader turns the syntax tree of a command line into its commands: src is the line, windows whether the system is
// Windows, and out the commands read so far.
type reader struct {
	src     string
	windows bool
	out     []Command
}

// writing are the operators of the redirections that write their target.
var writing = map[syntax.RedirOperator]bool{syntax.RdrOut: true, syntax.AppOut: true, syntax.RdrClob: true,
	syntax.AppClob: true, syntax.RdrAll: true, syntax.RdrAllClob: true, syntax.AppAll: true,
	syntax.AppAllClob: true, syntax.RdrInOut: true}

// parseAST reads a command line with the bash parser of mvdan.cc/sh and returns its commands, or the error of the
// parser when the line is no valid bash.
func parseAST(line string, windows bool) ([]Command, error) {
	f, err := syntax.NewParser(syntax.Variant(syntax.LangBash)).Parse(strings.NewReader(line), "")
	if err != nil {
		return nil, err
	}
	r := &reader{src: line, windows: windows}
	r.list(f.Stmts, true, NotNested)
	return r.out, nil
}

// list reads a list of statements: the last one propagates when the list does, and any other is followed by ;, &
// or a newline, so its failure says nothing of the line.
func (r *reader) list(stmts []*syntax.Stmt, propagates bool, nest Nesting) {
	for i, s := range stmts {
		r.stmt(s, propagates && i == len(stmts)-1, nest)
	}
}

// stmt reads one statement. Of `x && y`, x propagates, as a failure stops the line there; of `x || y` and `x | y`,
// only y may. A subshell or a block propagates as its last command, and the redirections of a compound statement
// are given to every command inside it. The commands of the substitutions of its redirections follow.
func (r *reader) stmt(s *syntax.Stmt, propagates bool, nest Nesting) {
	first := len(r.out)
	switch c := s.Cmd.(type) {
	case nil:
		r.call(s, nil, propagates, nest)
	case *syntax.CallExpr:
		r.call(s, c, propagates, nest)
	case *syntax.BinaryCmd:
		r.stmt(c.X, c.Op == syntax.AndStmt, nest)
		at := len(r.out)
		r.stmt(c.Y, propagates, nest)
		if c.Op == syntax.Pipe || c.Op == syntax.PipeAll {
			r.piped(c.X, c.Y, at)
		}
	case *syntax.Subshell:
		r.list(c.Stmts, propagates, compound(nest))
	case *syntax.Block:
		r.list(c.Stmts, propagates, compound(nest))
	case *syntax.TimeClause:
		r.time(c, propagates, nest)
	case *syntax.CoprocClause:
		if c.Stmt != nil {
			r.stmt(c.Stmt, propagates, nest)
		}
	case *syntax.DeclClause:
		r.decl(s, c, propagates, nest)
	default:
		r.wordless(s, propagates, nest)
	}
	r.after(s, first)
}

// after ends the statement s, whose commands start at first: the redirections of a compound statement go to every
// command inside it, the commands of the substitutions of its redirections follow, and the commands of a negated
// statement (`! just test`) do not propagate, since the line succeeds when they fail.
func (r *reader) after(s *syntax.Stmt, first int) {
	if _, call := s.Cmd.(*syntax.CallExpr); !call && s.Cmd != nil {
		for i := first; i < len(r.out); i++ {
			r.redirs(&r.out[i], s.Redirs)
		}
	}
	for _, rd := range s.Redirs {
		r.substs(rd)
	}
	if s.Negated {
		r.quiet(first)
	}
}

// wordless reads a clause (see isClause) or a test ([[ ]], (( )), let): a command of its own, without words, so a
// reader that needs every word of the line (an allowlist) fails closed on it, followed by the commands of its
// substitutions and its lists. A clause never propagates.
func (r *reader) wordless(s *syntax.Stmt, propagates bool, nest Nesting) {
	clause := isClause(s.Cmd)
	r.out = append(r.out, Command{Text: r.stmtText(s), Propagates: propagates && !clause, Nest: nest})
	if clause {
		r.clause(s.Cmd, nest)
		return
	}
	r.substs(s.Cmd)
}

// isClause reports whether cmd is a compound command that runs lists of its own: `if`, `while`, `until`, `for`,
// `case` or a function.
func isClause(cmd syntax.Command) bool {
	switch cmd.(type) {
	case *syntax.IfClause, *syntax.WhileClause, *syntax.ForClause, *syntax.CaseClause, *syntax.FuncDecl:
		return true
	}
	return false
}

// clause reads the words and the lists of a compound command (see isClause): the substitutions of its words (the list
// of for, the word and the patterns of case) and the commands of its lists. Its commands are nested in it and none
// propagates: the exit code of the line says nothing of a command run in a condition, a loop or a branch.
func (r *reader) clause(cmd syntax.Command, nest Nesting) {
	first, in := len(r.out), compound(nest)
	switch c := cmd.(type) {
	case *syntax.IfClause:
		for ; c != nil; c = c.Else {
			r.list(c.Cond, false, in)
			r.list(c.Then, false, in)
		}
	case *syntax.WhileClause:
		r.list(c.Cond, false, in)
		r.list(c.Do, false, in)
	case *syntax.ForClause:
		r.substs(c.Loop)
		r.list(c.Do, false, in)
	case *syntax.CaseClause:
		r.substs(c.Word)
		for _, item := range c.Items {
			for _, p := range item.Patterns {
				r.substs(p)
			}
			r.list(item.Stmts, false, in)
		}
	case *syntax.FuncDecl:
		if c.Body != nil {
			r.stmt(c.Body, false, in)
		}
	}
	r.quiet(first)
}

// compound returns the nesting of a command inside a compound command whose own nesting is nest: a command of the
// line becomes Compound, and one already nested in a substitution, a script or eval keeps it.
func compound(nest Nesting) Nesting {
	if nest == NotNested {
		return Compound
	}
	return nest
}

// substs adds the commands of the command substitutions ($(...) and backticks) and process substitutions (<(...)
// and >(...)) found in node, none of which propagates. It does not enter the statements of node itself, only its
// words, so a caller gives it words, redirections, loops and the commands without statements.
func (r *reader) substs(node syntax.Node) {
	if node == nil {
		return
	}
	first := len(r.out)
	defer r.quiet(first)
	syntax.Walk(node, func(n syntax.Node) bool {
		switch n := n.(type) {
		case *syntax.CmdSubst:
			r.list(n.Stmts, false, Substitution)
			return false
		case *syntax.ProcSubst:
			r.list(n.Stmts, false, Substitution)
			return false
		}
		return true
	})
}

// quiet marks the commands read since first as not propagating: a command of a substitution, a clause or a negated
// statement says nothing through the exit code of the line, even after && inside it.
func (r *reader) quiet(first int) {
	for i := first; i < len(r.out); i++ {
		r.out[i].Propagates = false
	}
}

// call reads a simple command, c nil for a statement made only of redirections: its assignments and arguments are
// its words, and its redirections its writes. The commands of the script it runs (see nested) and of the
// substitutions of its words follow it.
func (r *reader) call(s *syntax.Stmt, c *syntax.CallExpr, propagates bool, nest Nesting) {
	cmd := Command{Text: r.stmtText(s), Propagates: propagates, Nest: nest}
	if c != nil {
		for _, a := range c.Assigns {
			text, literal := r.assign(a)
			addWord(&cmd, text, literal)
		}
		for _, w := range c.Args {
			text, literal := r.word(w)
			addWord(&cmd, text, literal)
		}
	}
	r.redirs(&cmd, s.Redirs)
	r.out = append(r.out, cmd)
	r.nested(cmd)
	r.fed(cmd, s.Redirs)
	if c != nil {
		for _, a := range c.Assigns {
			r.substs(a)
		}
		for _, w := range c.Args {
			r.substs(w)
		}
	}
}

// time reads `time [-p] command`: the first command it times gets the words `time` and `-p` before its own, and its
// text starts at time, as the wrapper it is.
func (r *reader) time(c *syntax.TimeClause, propagates bool, nest Nesting) {
	if c.Stmt == nil {
		return
	}
	first := len(r.out)
	r.stmt(c.Stmt, propagates, nest)
	if first == len(r.out) {
		return
	}
	cmd := &r.out[first]
	prefix := []string{"time"}
	if c.PosixFormat {
		prefix = append(prefix, "-p")
	}
	cmd.Text = r.src[c.Time.Offset():c.Stmt.Pos().Offset()] + cmd.Text
	cmd.Literal = append(literals(len(prefix)), cmd.Literal...)
	cmd.Words = append(prefix, cmd.Words...)
}

// decl reads a declaration (export, declare, local, readonly...): its keyword and its arguments are its words.
func (r *reader) decl(s *syntax.Stmt, c *syntax.DeclClause, propagates bool, nest Nesting) {
	cmd := Command{Text: r.stmtText(s), Propagates: propagates, Nest: nest}
	addWord(&cmd, c.Variant.Value, true)
	for _, a := range c.Args {
		text, literal := r.assign(a)
		addWord(&cmd, text, literal)
	}
	r.out = append(r.out, cmd)
	for _, a := range c.Args {
		r.substs(a)
	}
}

// redirs adds the redirections rs to the command: >, >>, >|, &>, &>> and <> write their target, and so does >& when
// its target is no descriptor (a digit or -). Inputs, here-documents and duplications write nothing.
func (r *reader) redirs(cmd *Command, rs []*syntax.Redirect) {
	for _, rd := range rs {
		cmd.Redirections = append(cmd.Redirections, r.source(rd))
		var target string
		if rd.Word != nil {
			target, _ = r.word(rd.Word)
		}
		switch {
		case writing[rd.Op]:
		case rd.Op == syntax.DplOut && strings.Trim(target, "0123456789") != "" && target != "-":
		default:
			continue
		}
		cmd.Writes = append(cmd.Writes, target)
		cmd.Redirects = true
	}
}

// addWord appends a word to the command c.
func addWord(c *Command, text string, literal bool) {
	c.Words = append(c.Words, text)
	c.Literal = append(c.Literal, literal)
}

// stmtText returns the text of a statement as written, with its redirections but without the ; or & after it.
func (r *reader) stmtText(s *syntax.Stmt) string {
	end := s.Pos().Offset()
	if s.Cmd != nil {
		end = max(end, s.Cmd.End().Offset())
	}
	for _, rd := range s.Redirs {
		end = max(end, rd.End().Offset())
	}
	return r.src[s.Pos().Offset():end]
}

// source returns the text of a node as written.
func (r *reader) source(n syntax.Node) string {
	return r.src[n.Pos().Offset():n.End().Offset()]
}
