# The demo of the README: real calls of `polix hook claude` fed with recorded Claude Code hook inputs (PreToolUse,
# without cwd, so the hook takes the working directory) in a scratch git repository governed by polix.yaml.
# Regenerate docs/assets/demo.gif with `just demo` from the root of the repository; it needs vhs (with ttyd and
# ffmpeg), git and just.

Output docs/assets/demo.gif

Require git
Require just

Set Shell "bash"
Set Theme "Catppuccin Mocha"
Set FontSize 18
Set Width 1000
Set Height 600
Set Padding 24
Set TypingSpeed 45ms

# Build polix, put build/ first on the PATH and enter a new git repository holding polix.yaml and the hook inputs.
Hide
Type `just build && export PATH="$PWD/build:$PATH" && demo=$(mktemp -d) && cp docs/assets/demo/polix.yaml docs/assets/demo/*.json "$demo" && cd "$demo" && git init -q -b main && clear`
Enter
Wait@60s
Show

Type "cat polix.yaml"
Enter
Sleep 4s

Type "polix config check"
Enter
Sleep 1.5s

# Claude Code asks to force push: forbidden, exit 2 with the reason.
Type `polix hook claude < push-force.json; echo "exit $?"`
Enter
Sleep 3s

# Claude Code asks to write .env: protected, exit 2.
Type `polix hook claude < edit-env.json; echo "exit $?"`
Enter
Sleep 3s

# Claude Code asks to run the tests: allowed, exit 0.
Type `polix hook claude < run-tests.json; echo "exit $?"`
Enter
Sleep 5s
