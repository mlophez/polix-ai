# The demo of the README: a real Claude Code session in a scratch Go module governed by polix.yaml. Claude is asked
# to add a function and commit it; polix blocks the commit on main and the commit without a review, and Claude
# recovers by itself: a branch, the reviewer subagent, the tests polix runs, and the commit. The reviewer is reviewer.md,
# a read-only agent of the demo: without Bash it never stops the recording on a permission prompt.
#
# Run it with `just demo` from the root of the repository: it records docs/assets/demo.mp4 in real time, and the
# recipe turns it into docs/assets/demo.gif, sped up and without its idle frames. It needs vhs (with ttyd and
# ffmpeg), git, go, golangci-lint, just, and Claude Code logged in.
# The session is real, so each recording differs: watch the GIF before committing it.

Output docs/assets/demo.mp4

Require git
Require go
Require just
Require claude

Set Shell "bash"
Set Theme "Catppuccin Mocha"
Set FontSize 16
Set Width 1200
Set Height 760
Set Padding 20
Set TypingSpeed 40ms

# Build polix, put build/ first on the PATH, and make a scratch repository with the module, polix.yaml and the hooks,
# committed on main. The alias claude carries the flags of the demo: edits and the commands a change and its review
# need run without a prompt, and the answers stay short and in English. Claude Code starts once to trust the new
# folder (Down picks "Yes, I trust this folder"), exits with a double Ctrl+C, and the screen is cleared.
Hide
Type `just build && export PATH="$PWD/build:$PATH" POLIX_NO_UPDATE=1 && src="$PWD/docs/assets/demo" && demo=$(mktemp -d)/text && mkdir -p "$demo/.claude/agents" && cp "$src"/{polix.yaml,go.mod,text.go,text_test.go} "$demo" && cp "$src/claude-settings.json" "$demo/.claude/settings.json" && cp "$src/reviewer.md" "$demo/.claude/agents/reviewer.md" && cd "$demo" && git init -q -b main && git config user.name Demo && git config user.email demo@example.com && git add -A && git commit -qm "Count words" && clear && echo "built $((40+2))"`
Enter
Sleep 20s
Type `unset $(env | sed -n 's/^\(CLAUDE[A-Z_]*\)=.*/\1/p') && alias claude='command claude --setting-sources project --permission-mode acceptEdits --allowedTools "Bash(git *)" "Bash(go *)" "Bash(gofmt *)" "Bash(golangci-lint *)" "Bash(polix *)" "Bash(ls *)" "Bash(cat *)" "Bash(echo *)" "Bash(head *)" "Bash(tail *)" "Bash(diff *)" "Bash(grep *)" "Bash(wc *)" --append-system-prompt "This is a recorded demo: answer in English, in one or two short sentences, and never mention polix doctor."'`
Enter
Type "claude"
Enter
Sleep 10s
Down
Enter
Sleep 10s
Ctrl+C
Sleep 300ms
Ctrl+C
Sleep 3s
Type "clear"
Enter
Sleep 1s
Show

Type "claude"
Enter
Sleep 6s
Type "Add a Reverse function to text.go with a test, then commit it."
Sleep 500ms
Enter

# Claude works on its own: the edits, the blocked commits, the review and the tests, about 30 seconds. The tape waits
# with fixed pauses, since Wait+Screen of vhs fails now and then on the screen of Claude Code; the recipe drops the idle
# frames. At the end of the turn Claude Code may offer the LSP plugin of Go: Escape dismisses it, out of the recording.
Sleep 120s
Hide
Sleep 2s
Escape
Sleep 1s
Show
Sleep 6s
