package shell

import (
	"slices"
	"testing"
)

// script finds the literal script of `sh -c`, `bash -c` and `zsh -c`, and the literal words of eval.
func TestScript(t *testing.T) {
	cases := []struct {
		line, text string
		kind       Nesting
	}{
		{"bash -c 'rm -rf /'", "rm -rf /", Script},
		{"sh -c \"git add . && git commit -m x\"", "git add . && git commit -m x", Script},
		{"/bin/zsh -lc 'rm a' name x", "rm a", Script},
		{"bash -e -c 'rm a'", "rm a", Script},
		{"bash.exe -c 'rm a'", "rm a", Script},
		{"bash -o pipefail -c 'rm a'", "rm a", Script},
		{"bash -euo pipefail -c 'rm a'", "rm a", Script},
		{"bash -eo pipefail -uc 'rm a'", "rm a", Script},
		{"bash -oc pipefail 'rm a'", "rm a", Script},
		{"sh -eoc pipefail 'rm a'", "rm a", Script},
		{"bash -Oc extglob 'rm a'", "rm a", Script},
		{"bash -oo pipefail errexit -c 'rm a'", "rm a", Script},
		{"sudo bash -c 'rm a'", "rm a", Script},
		{"sudo -n env FOO=1 bash -c 'rm a'", "rm a", Script},
		{"nohup sh -c 'rm a'", "rm a", Script},
		{"exec bash -c 'rm a'", "rm a", Script},
		{"xargs -0 bash -c 'rm a'", "rm a", Script},
		{"time bash -c 'rm a'", "rm a", Script},
		{"sudo eval rm a", "rm a", Eval},
		{"bash -c -- 'rm a'", "rm a", Script},
		{"LANG=C bash -c 'rm a'", "rm a", Script},
		{"eval rm AGENTS.md", "rm AGENTS.md", Eval},
		{"eval 'rm a;' rm b", "rm a; rm b", Eval},
		{`bash -c "$CMD"`, "", NotNested},
		{"bash -c", "", NotNested},
		{"bash script.sh -c 'rm a'", "", NotNested},
		{"bash -x 'rm a'", "", NotNested},
		{"eval $CMD", "", NotNested},
		{"eval", "", NotNested},
		{"echo -c 'rm a'", "", NotNested},
	}
	// A here-document, or a literal here-string, fed to a shell reading stdin, directly or through a pipe, is a
	// script too.
	for line, want := range map[string][]string{
		"bash <<'EOF'\nrm -rf /\nEOF": {"bash <<'EOF'\nrm -rf /\nEOF|0|true", "rm -rf /|2|true"},
		"sh -s <<EOF\nrm a\ngit commit\nEOF": {"sh -s <<EOF\nrm a\ngit commit\nEOF|0|true", "rm a|2|false",
			"git commit|2|true"},
		"sudo bash <<< 'rm a'":   {"sudo bash <<< 'rm a'|0|true", "rm a|2|true"},
		"bash <<EOF\nrm $x\nEOF": {"bash <<EOF\nrm $x\nEOF|0|true", "rm $x|2|true"},
		"bash <<EOF\nrm -rf $HOME\ngit commit -m \"$MSG\"\nEOF": {
			"bash <<EOF\nrm -rf $HOME\ngit commit -m \"$MSG\"\nEOF|0|true", "rm -rf $HOME|2|false",
			"git commit -m \"$MSG\"|2|true"},
		"bash - <<'EOF'\nrm a\nEOF": {"bash - <<'EOF'\nrm a\nEOF|0|true", "rm a|2|true"},
		// The text of a command with a here-document ends where its body ends, past the pipe.
		"cat <<'EOF' | bash\nrm -rf /\nEOF": {"cat <<'EOF' | bash\nrm -rf /\nEOF|0|false", "bash|0|true",
			"rm -rf /|2|true"},
		"cat <<'EOF' | sudo sh -s\nrm a\nEOF": {"cat <<'EOF' | sudo sh -s\nrm a\nEOF|0|false", "sudo sh -s|0|true",
			"rm a|2|true"},
		"cat <<'EOF' | grep x\nrm a\nEOF":   {"cat <<'EOF' | grep x\nrm a\nEOF|0|false", "grep x|0|true"},
		"echo hi | bash <<< 'rm a'":         {"echo hi|0|false", "bash <<< 'rm a'|0|true", "rm a|2|true"},
		"bash script.sh <<'EOF'\nrm a\nEOF": {"bash script.sh <<'EOF'\nrm a\nEOF|0|true"},
		"bash -c 'ls' <<'EOF'\nrm a\nEOF":   {"bash -c 'ls' <<'EOF'\nrm a\nEOF|0|true", "ls|2|true"},
		"cat <<'EOF'\nrm a\nEOF":            {"cat <<'EOF'\nrm a\nEOF|0|true"},
	} {
		if got := summary(line); !slices.Equal(got, want) {
			t.Errorf("Parse(%q) = %q, want %q", line, got, want)
		}
	}
	for _, c := range cases {
		cmds, _ := Parse(c.line, false)
		text, kind, ok := script(cmds[0])
		if text != c.text || kind != c.kind || ok != (c.kind != NotNested) {
			t.Errorf("script(%q) = %q %v %v, want %q %v", c.line, text, kind, ok, c.text, c.kind)
		}
	}
}
