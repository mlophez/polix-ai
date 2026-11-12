package shell

import (
	"fmt"
	"reflect"
	"slices"
	"testing"
)

// The lines of the splitter give the same commands and propagation through the syntax tree, but `((`, which bash
// reads as arithmetic and the parser rejects, so the heuristic reads it; the commands of substitutions come on top.
func TestParseSplitCases(t *testing.T) {
	for _, c := range splitCases {
		cmds, ast := Parse(c.line, false)
		if want := c.line != "((cd x && just test))"; ast != want {
			t.Errorf("Parse(%q) through the syntax tree = %v, want %v", c.line, ast, want)
		}
		var got []segment
		for _, cmd := range cmds {
			if cmd.Nest == NotNested || cmd.Nest == Compound { // the splitter does not see the substitutions
				got = append(got, segment{cmd.Text, cmd.Propagates})
			}
		}
		if !reflect.DeepEqual(got, c.want) {
			t.Errorf("Parse(%q) = %#v, want %#v", c.line, got, c.want)
		}
	}
}

// The commands of the word parser give the same words and writes through the syntax tree, on Windows where the path
// with backslashes keeps them.
func TestParseWordCases(t *testing.T) {
	for _, c := range wordCases {
		cmds, ast := Parse(c.command, true)
		if !ast || len(cmds) != 1 {
			t.Errorf("Parse(%q) = %#v, %v", c.command, cmds, ast)
			continue
		}
		if !slices.Equal(cmds[0].Words, c.words) || !slices.Equal(cmds[0].Writes, c.writes) {
			t.Errorf("Parse(%q) = %q %q, want %q %q", c.command, cmds[0].Words, cmds[0].Writes, c.words, c.writes)
		}
		if cmds[0].Redirects != (len(c.writes) > 0) {
			t.Errorf("Parse(%q) redirects = %v", c.command, cmds[0].Redirects)
		}
	}
}

// A line the parser rejects gives exactly what the heuristic reads.
func TestParseFallback(t *testing.T) {
	for _, line := range []string{`echo "unclosed`, `Remove-Item -Path "x" -Force; if ($?) { ls }`} {
		cmds, ast := Parse(line, false)
		if ast || !reflect.DeepEqual(cmds, fallback(line)) {
			t.Errorf("Parse(%q) = %#v, %v, want the fallback", line, cmds, ast)
		}
	}
}

// On Windows a backslash before an ordinary character stays; elsewhere bash quoting applies.
func TestParseBackslash(t *testing.T) {
	if cmds, _ := Parse(`rm C:\Users\x\AGENTS.md`, true); !slices.Equal(cmds[0].Words, []string{"rm",
		`C:\Users\x\AGENTS.md`}) {
		t.Errorf("Windows: %q", cmds[0].Words)
	}
	if cmds, _ := Parse(`rm AGENT\S.md`, false); !slices.Equal(cmds[0].Words, []string{"rm", "AGENTS.md"}) {
		t.Errorf("Unix: %q", cmds[0].Words)
	}
}

// $'...' words are decoded and a quoted target with a space is one target.
func TestParseQuotes(t *testing.T) {
	if cmds, _ := Parse(`rm $'AGENTS.md' $'a\x41\n\101\'' $'\u00e9'`, false); !slices.Equal(cmds[0].Words,
		[]string{"rm", "AGENTS.md", "aA\nA'", "é"}) {
		t.Errorf("$'': %q", cmds[0].Words)
	}
	if cmds, _ := Parse(`echo x > 'a b'`, false); !slices.Equal(cmds[0].Writes, []string{"a b"}) {
		t.Errorf("quoted target: %q", cmds[0].Writes)
	}
}

// A word is literal unless it holds an expansion, which keeps its text as written, or a glob, brace or tilde.
func TestParseLiteral(t *testing.T) {
	cmds, _ := Parse(`git -C "$OTHER" add ~/x a* 'b*' "c" $(cat msg) {a,b}`, false)
	want := []string{"git", "-C", `"$OTHER"`, "add", "~/x", "a*", "b*", "c", "$(cat msg)", "{a,b}"}
	literal := []bool{true, true, false, true, false, false, true, true, false, false}
	if !slices.Equal(cmds[0].Words, want) || !slices.Equal(cmds[0].Literal, literal) {
		t.Errorf("Parse = %q %v", cmds[0].Words, cmds[0].Literal)
	}
}

// Assignments are words, a statement made of redirections is a command, the redirections are kept as written, and
// time and declarations are commands with their words.
func TestParseShapes(t *testing.T) {
	cmds, _ := Parse("LANG=C sort f 2>&1 < in; > out; time -p go test | tail; export A=1 B", false)
	want := []Command{
		{Text: "LANG=C sort f 2>&1 < in", Words: []string{"LANG=C", "sort", "f"}, Literal: literals(3),
			Redirections: []string{"2>&1", "< in"}},
		{Text: "> out", Writes: []string{"out"}, Redirects: true, Redirections: []string{"> out"}},
		{Text: "time -p go test", Words: []string{"time", "-p", "go", "test"}, Literal: literals(4)},
		{Text: "tail", Words: []string{"tail"}, Literal: literals(1)},
		{Text: "export A=1 B", Words: []string{"export", "A=1", "B"}, Literal: literals(3), Propagates: true},
	}
	if !reflect.DeepEqual(cmds, want) {
		t.Errorf("Parse = %#v", cmds)
	}
}

// summary gives each command of a line as its text, its nesting (0 the line, 1 a substitution, 2 a -c script, 3 eval,
// 4 a compound command) and whether it propagates.
func summary(line string) []string {
	cmds, _ := Parse(line, false)
	out := make([]string, len(cmds))
	for i, c := range cmds {
		out[i] = fmt.Sprintf("%s|%d|%v", c.Text, c.Nest, c.Propagates)
	}
	return out
}

// The commands nested in -c scripts, substitutions, eval and compound commands are commands of the line too. A
// clause (if, while, for, case, a function) is a command of its own, without words, before the commands of its
// words and of its lists, so a reader that needs every word fails closed on it; the patterns of case are walked.
func TestParseNested(t *testing.T) {
	cases := map[string][]string{
		"bash -c 'rm -rf /'":                   {"bash -c 'rm -rf /'|0|true", "rm -rf /|2|true"},
		`sh -c "git add . && git commit -m x"`: {`sh -c "git add . && git commit -m x"|0|true`, "git add .|2|false", "git commit -m x|2|true"},
		"bash -c 'true && just test' | tail":   {"bash -c 'true && just test'|0|false", "true|2|false", "just test|2|false", "tail|0|true"},
		"echo $(rm -rf /)":                     {"echo $(rm -rf /)|0|true", "rm -rf /|1|false"},
		"echo `rm x` > out":                    {"echo `rm x` > out|0|true", "rm x|1|false"},
		"(cd x && rm y)":                       {"cd x|4|true", "rm y|4|true"},
		"if true; then rm y; fi":               {"if true; then rm y; fi|0|false", "true|4|false", "rm y|4|false"},
		"for f in a; do rm $f; done":           {"for f in a; do rm $f; done|0|false", "rm $f|4|false"},
		"while read l; do rm $l; done < x":     {"while read l; do rm $l; done < x|0|false", "read l|4|false", "rm $l|4|false"},
		"case $x in a) rm a;; esac":            {"case $x in a) rm a;; esac|0|false", "rm a|4|false"},
		"case x in $(rm a)) ;; esac":           {"case x in $(rm a)) ;; esac|0|false", "rm a|1|false"},
		"case x in a|$(rm a)|`rm b`) :;; esac": {"case x in a|$(rm a)|`rm b`) :;; esac|0|false", "rm a|1|false", "rm b|1|false", ":|4|false"},
		"f() { rm a; }":                        {"f() { rm a; }|0|false", "rm a|4|false"},
		"eval rm AGENTS.md":                    {"eval rm AGENTS.md|0|true", "rm AGENTS.md|3|false"},
		`bash -c "$CMD"`:                       {`bash -c "$CMD"|0|true`},
		"eval $CMD":                            {"eval $CMD|0|true"},
		"bash -c 'echo $(rm a)'":               {"bash -c 'echo $(rm a)'|0|true", "echo $(rm a)|2|true", "rm a|1|false"},
		"x=$(rm a) cat <(rm b) > >(rm c)":      {"x=$(rm a) cat <(rm b) > >(rm c)|0|true", "rm a|1|false", "rm b|1|false", "rm c|1|false"},
		"(cd h && go test) && just test":       {"cd h|4|true", "go test|4|true", "just test|0|true"},
		"[[ -n $(rm a) ]]":                     {"[[ -n $(rm a) ]]|0|true", "rm a|1|false"},
		"for f in $(ls); do :; done":           {"for f in $(ls); do :; done|0|false", "ls|1|false", ":|4|false"},
		"echo $(just test && true)":            {"echo $(just test && true)|0|true", "just test|1|false", "true|1|false"},
		"cat <(just test && true)":             {"cat <(just test && true)|0|true", "just test|1|false", "true|1|false"},
		"! just test":                          {"! just test|0|false"},
		"! just test && ls":                    {"! just test|0|false", "ls|0|true"},
		"(if true; then rm a; fi)":             {"if true; then rm a; fi|4|false", "true|4|false", "rm a|4|false"},
		"bash -lc 'rm a' && sh -e -c 'rm b' x": {"bash -lc 'rm a'|0|true", "rm a|2|true", "sh -e -c 'rm b' x|0|true", "rm b|2|true"},
		"zsh script.sh -c 'rm a'":              {"zsh script.sh -c 'rm a'|0|true"},
		"/bin/bash -c 'rm a; rm b'":            {"/bin/bash -c 'rm a; rm b'|0|true", "rm a|2|false", "rm b|2|true"},
		"cat <<EOF\n$(rm a)\nEOF":              {"cat <<EOF\n$(rm a)\nEOF|0|true", "rm a|1|false"},
	}
	for line, want := range cases {
		if got := summary(line); !slices.Equal(got, want) {
			t.Errorf("Parse(%q) = %q, want %q", line, got, want)
		}
	}
}
