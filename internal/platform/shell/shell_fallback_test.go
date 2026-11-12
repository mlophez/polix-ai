package shell

import (
	"reflect"
	"slices"
	"testing"
)

// splitCases are lines and the commands the splitter reads in them; the parse through the syntax tree gives the same.
var splitCases = []struct {
	line string
	want []segment
}{
	{"just test", []segment{{"just test", true}}},
	{"cd /x && just test", []segment{{"cd /x", true}, {"just test", true}}},
	{"just test 2>&1 | tail -3", []segment{{"just test 2>&1", false}, {"tail -3", true}}},
	{"just test; echo done", []segment{{"just test", false}, {"echo done", true}}},
	{"(cd h && go test) && just test", []segment{{"cd h", true}, {"go test", true}, {"just test", true}}},
	{`echo "a && b | c" && git push`, []segment{{`echo "a && b | c"`, true}, {"git push", true}}},
	{"just test || true", []segment{{"just test", false}, {"true", true}}},
	{"echo x >|out.txt && ls", []segment{{"echo x >|out.txt", true}, {"ls", true}}},
	{"echo $(date)", []segment{{"echo $(date)", true}}},
	{"(echo $(date)) && ls", []segment{{"echo $(date)", true}, {"ls", true}}},
	{"((cd x && just test))", []segment{{"cd x", true}, {"just test", true}}},
}

// wordCases are simple commands with their words and the targets of their redirections; the parse through the
// syntax tree gives the same, on Windows for the path with backslashes.
var wordCases = []struct {
	command       string
	words, writes []string
}{
	{"echo x >AGENTS.md", []string{"echo", "x"}, []string{"AGENTS.md"}},
	{"echo x>AGENTS.md", []string{"echo", "x"}, []string{"AGENTS.md"}},
	{"cat a.txt>>AGENTS.md", []string{"cat", "a.txt"}, []string{"AGENTS.md"}},
	{"echo x 2>> logs/out.txt", []string{"echo", "x"}, []string{"logs/out.txt"}},
	{"echo x >|AGENTS.md 2>&1", []string{"echo", "x"}, []string{"AGENTS.md"}},
	{"echo x >&AGENTS.md", []string{"echo", "x"}, []string{"AGENTS.md"}},
	{"echo x >& AGENTS.md", []string{"echo", "x"}, []string{"AGENTS.md"}},
	{"echo x >& 2 2>&-", []string{"echo", "x"}, nil},
	{"go test &> out.txt", []string{"go", "test"}, []string{"out.txt"}},
	{"ls ai 2>/dev/null", []string{"ls", "ai"}, []string{"/dev/null"}},
	{"sort < in.txt <<< x", []string{"sort"}, nil},
	{`grep -n "x>y" AGENTS.md`, []string{"grep", "-n", "x>y", "AGENTS.md"}, nil},
	{`rg 'Vec<T>::new' AGENTS.md`, []string{"rg", "Vec<T>::new", "AGENTS.md"}, nil},
	{`rm -rf "a>b" ai`, []string{"rm", "-rf", "a>b", "ai"}, nil},
	{`echo "2">x`, []string{"echo", "2"}, []string{"x"}},
	{`tee -a "ai/rules/x.yaml"`, []string{"tee", "-a", "ai/rules/x.yaml"}, nil},
	{`echo 'it'\''s' > AGENTS.md`, []string{"echo", "it's"}, []string{"AGENTS.md"}},
	{`echo "a \"b" > AGENTS.md`, []string{"echo", `a "b`}, []string{"AGENTS.md"}},
	{`echo \"> AGENTS.md`, []string{"echo", `"`}, []string{"AGENTS.md"}},
	{`echo a\>b`, []string{"echo", "a>b"}, nil},
	{"sudo \\\n  rm -rf ai", []string{"sudo", "rm", "-rf", "ai"}, nil},
	{"echo x > \\\n  AGENTS.md", []string{"echo", "x"}, []string{"AGENTS.md"}},
	{"rm -rf \\\nai", []string{"rm", "-rf", "ai"}, nil},
	{`type C:\Users\x\a.txt > out.txt`, []string{"type", `C:\Users\x\a.txt`}, []string{"out.txt"}},
}

func TestSplit(t *testing.T) {
	for _, c := range splitCases {
		if got := split(c.line); !reflect.DeepEqual(got, c.want) {
			t.Errorf("split(%q) = %#v, want %#v", c.line, got, c.want)
		}
	}
}

// parseWords keeps the words of a command apart from the targets of its redirections, respecting quotes.
func TestParseWords(t *testing.T) {
	for _, c := range wordCases {
		words, writes := parseWords(c.command)
		if !slices.Equal(words, c.words) || !slices.Equal(writes, c.writes) {
			t.Errorf("parseWords(%q) = %q %q, want %q %q", c.command, words, writes, c.words, c.writes)
		}
	}
}
