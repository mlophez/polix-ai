# Parse every bash command line with a real shell parser (mvdan.cc/sh) instead of the hand-written heuristics

- Status: built
- Risk: high: it changes how the protections (protected_files, forbidden_commands, the state) and pre_commit read a
  command line, adds a dependency, and changes the matching of the `command` condition of polix.yaml.
- Reference: none.
- Created: 2026-11-12.
- Size: 6 tasks, about 1650 changed lines.

## Overview

Every time the agent runs a shell command, polix reads the line to decide which rules apply: whether it commits,
whether it matches a `command` condition, whether it is a forbidden command, whether it writes a protected file.
Today it reads the line with home-made heuristics that split it on `&&`, `|` and `;` and guess the words.
They miss commands written in other ways, such as `git -C dir add .`, `sudo rm -rf /` or `bash -c 'rm AGENTS.md'`.

polix will now read every command line with a real bash parser, the one of the `shfmt` formatter, written in Go.
It sees each command with its exact words and redirections, including the commands nested in `bash -c '...'`,
`$(...)`, backticks, subshells, `if`, `for` and `eval`.

A `command` pattern now matches a command as written or in a normalized form, without assignments, wrappers such as
`sudo` or `env`, and the global options of git.
So `command: "git add*"` on `pre_command` recognises `git -C x add .` and `sudo env FOO=1 git add .`.
It does not recognise `git commit -m "add foo"`.
This is what lets slow formatters run right before a `git add` instead of at the end of every turn.

A commit is recognised only when a real `git commit` runs, so `echo "git commit"` no longer triggers `pre_commit`.
The two places where polix lets a line skip its rules also decide on the parsed line.
These are the commands allowed while polix.yaml is broken, and the commits in another repository.
A line that cannot be parsed, such as an incomplete one or a PowerShell one, is read exactly as today.
On Windows, a path with backslashes keeps them, as today.

## Request

actualmente el pre command se detecta bien, quiero detectar bien los git add porque quiero ejecutar algunos comandos
antes de ellos. sobre todo comandos que tardan que no ejecutar en run_on_stop como:

    # Move to pre git add (slow)
    #- run: mvn -s mvnw.xml -q spotless:apply
    #  changed: ["*.java", pom.xml]
    #  message: "spotless:apply could not format the Java code:"

    # ESLint fixes what it can; what it cannot is left to the check of the commit. (slow: move to git add)
    #- run: npm --prefix frontend run eslint -- --fix
    #  changed: ["frontend/**/*.ts", "frontend/**/*.tsx", "frontend/**/*.js", "frontend/**/*.mjs", "frontend/**/*.cjs", "frontend/**/*.html"]
    #  message: "ESLint left problems it cannot fix in the frontend: fix them before finishing."

Translated: currently pre command is detected well; I want to detect git add well, because I want to run some
commands before them, above all slow commands that should not run in run_on_stop, such as the ones above.

- Is there no option in golang to improve the parsing engine of the bash tool?
- Nobody talked about a new event pre_add: pre_command will be used.
- There is no git add event.
- Every evaluation of a bash command must go through the AST to get better results.

## Context

`internal/platform/shell` (`shell.Split`) splits a line into segments with a byte-by-byte heuristic.
`internal/app/hook/answer/logic_cmdline.go` (`parseCommand`, `simpleCommand`, `program`) parses each segment again into
words and redirection targets.
`internal/app/hook/answer/in_input.go` (`gitCommit`) recognises a commit with a regular expression.
The `command` condition and `forbidden_commands` (`matchingCommands`, `forbidden`) match each segment as written.
`writes` and `protected_files` (`lineWrites`, `writePaths`) use the words.
`onlyPolix` and `commitDirs` (with `placeable`) are text allowlists.
None of them sees commands nested in `bash -c`, `$(...)` or `eval`.
A pattern misses wrappers and global options of git, and `echo "git commit"` counts as a commit.

## Constraints

- Every evaluation of a command line goes through the AST of `mvdan.cc/sh/v3/syntax` (request).
- A line the parser rejects is evaluated exactly as today, by the hand-written parsers and the `gitCommit` regex,
  which stay as the fallback (decision of the interview).
- No new event: `git add` is gated with `pre_command` and the `command` condition (request).
- `mvdan.cc/sh/v3` is imported only by `internal/platform/shell`.
  The rest of polix sees the types of that package, never those of the library (GOARCH-19, GOARCH-33, GOARCH-41).
- `internal/platform/shell` stays a pure library without domain: it never imports `internal/app/` (GOARCH-42), and it
  stays importable by the domain and the logic (GOARCH-41 lists it).
- Each non-test Go file stays within 400 lines and each function within the complexity limits (GO-4).
- On Windows a word with a backslash before an ordinary character keeps its backslashes, as `parseCommand` does today
  (decision of the interview, XPLAT-1).
- The license of `mvdan.cc/sh` (BSD-3-Clause) is compatible with the Apache-2.0 license of polix.
- The line given to the placeholder `{command}` and written to the log stays the line as the agent sent it.

## Decisions

- Both hand-written parsers, `shell.Split` and `parseCommand`, are replaced by the AST (interview).
- Commands are recognised exactly from the AST, with recursion into nested commands.
  `echo "git commit"` stops triggering `pre_commit` (interview).
- A `command` pattern, in the `command` condition and in `forbidden_commands`, holds when it matches a command as
  written or its normalized form (interview).
- The normalized form drops leading assignments, the wrappers known today (`sudo`, `env`, `command`, `nohup`,
  `time`, `exec`, `xargs`, `nice`) with their options but not their values, and the global options of git (interview).
- The recursion into nested commands applies to everything: `command`, `forbidden_commands`, `writes`,
  `protected_files` and the recognition of commits (interview).
- A nested command never propagates its exit code to the line, except the last command of a `sh -c` or `bash -c`
  script that itself propagates (interview).
- A line the parser rejects falls back to the hand-written parsers and the `gitCommit` regex, which are kept
  (interview).
- On Windows a word with a backslash before an ordinary character keeps its literal text; on Linux and macOS bash
  quoting applies (interview).
- The dialect is bash (`syntax.LangBash`) (interview).
- `onlyPolix` and `commitDirs` decide on the AST too ("toda evaluacion de un comando de bash debe pasar por el AST").
  Every command, nested included, must still fit the allowlist, so a substitution such as `$(cat msg)` or
  `$(rm -rf ai)` brings a command that does not fit, and the line is refused as today.
- No `pre_add` event and no new key of polix.yaml (request).

## Approach

`internal/platform/shell` becomes the only reader of command lines.
It exposes a parse of a line, given whether the system is Windows, that returns the commands of the line in order.
It also reports whether the AST parsed the line or the fallback read it.
Each command carries:
- its text as written, the source slice of its statement with its redirections, as today's segment text;
- its words, quotes removed, and whether each word is literal, with no expansion, glob, brace or tilde;
- the targets of its redirections that write, and whether it has any redirection other than a duplication of a
  descriptor (`2>&1`) or an input;
- whether its failure propagates to the line;
- whether it is nested, and the kind of nesting (a substitution, a `-c` script, `eval`, a compound command).

The parse uses `syntax.NewParser` with `syntax.Variant(syntax.LangBash)` and walks the file.
Each `syntax.CallExpr` is a command.
`syntax.CmdSubst`, `syntax.ProcSubst`, subshells, blocks, `if`, `while`, `for`, `case` and function bodies are walked
for nested commands.
A `sh`, `bash` or `zsh` with `-c` and a literal script, and an `eval` whose arguments are literal, are parsed again
recursively; a script that is not literal is not entered.
Words are turned into text from their literal parts; on Windows a word whose source holds a backslash before an
ordinary character keeps its source text.
Propagation follows the rules of today's `shell.Split` for the top level.
The last command propagates, and so does one followed by `&&`.
One followed by `||`, `;`, `&` or a pipe does not.
A subshell or a block propagates as its last command, and nested commands follow the decision above.

The hand-written splitter and `parseCommand` move into `internal/platform/shell` as the fallback.
The parse uses them when the parser returns an error, and their output takes the same shape.

The library also gives each command its normalized words: leading assignments dropped, the known wrappers dropped
with their options, and for `git` the global options dropped (`-C <v>`, `-c <v>`, `--git-dir <v>`,
`--work-tree <v>`, `--namespace <v>`, `--config-env <v>` and any other option before the subcommand).
The normalized text is those words joined by single spaces.

In `internal/app/hook/answer`:
- `matchingCommands` matches a pattern against the text as written or the normalized text of every command, nested
  included.
- `program` takes the normalized words and keeps its mapping of `git rm` and `git mv` to `rm` and `mv`.
- `lineWrites` and `writePaths` read words and write targets from the parsed commands.
- `propagates` reads the propagation of the parsed commands.
- Commit recognition in `hookInput.commands` asks whether any command, nested included, is normalized to
  `git commit`; on a fallback parse the `gitCommit` regex decides as today.
- `onlyPolix` holds on a parsed line when every command, nested included, fits `onlyPolixCommands` as written and no
  command has a redirection other than the duplication `2>&1` or `1>&2`; on a fallback parse it decides as today.
- `commitDirs` places a parsed line when every command, nested included, is `git` with no assignment nor wrapper,
  only `-C` options with literal values, and `add` or `commit` as subcommand.
  No word may be non literal, no command may have a redirection, and a nested command fails the check.
  On a fallback parse it decides as today.

Error handling: a parse error is never an error of the hook; it selects the fallback.
Nothing new reaches polix.yaml, so `polix config check` and the schema do not change.

### Discarded alternatives

- A new event `pre_add` with its own regex: the user rejected a new event; `pre_command` with `command` is enough.
- Keep the text heuristics and only add patterns such as `git * add*`: they match `git commit -m "add foo"`.
- Drop the hand-written parsers and treat an unparseable line as one command: `writes` and `protected_files` would
  lose precision on those lines (interview).
- Keep `onlyPolix` and `commitDirs` as text allowlists: the user asked that every evaluation goes through the AST.
- Union of the AST and the regex for commits: keeps the false positives of quoted text (interview).

## Acceptance criteria

- AC1: Every command line polix evaluates on `pre_command`, `post_command` and `pre_commit` is parsed by
  `mvdan.cc/sh/v3/syntax` in bash mode, and the hand-written parsers run only when that parse fails.
- AC2: A line the parser rejects, such as `echo "unclosed` or a PowerShell line, gives the same commands, words,
  writes, commit recognition, `onlyPolix` and `commitDirs` results as before the change.
- AC3: Every existing test of `shell.Split`, `parseCommand`, `program`, `matchingCommands`, `onlyPolix`, `commitDirs`,
  the writes and `forbidden` keeps its result, except the cases another criterion changes on purpose.
  Accepted by the user after the build: four former refusals of `commitDirs` are placed as bash reads them (a
  comment followed by a second line, `c\ommit`, `\-C`, `"a\"b"`).
- AC4: On `pre_command`, an entry with `when: { command: ["git add*", "git stage*"] }` holds for `git add .`,
  `cd frontend && git add -A`, `git -C x add .`, `git -c core.x=y add .`, `git --no-pager add .`,
  `sudo env FOO=1 git add .` and `bash -c 'git add .'`.
  It does not hold for `git commit -m "add foo"`, `echo "git add"` nor `git status`.
- AC5: A command nested in `bash -c '...'`, `sh -c "..."`, `$(...)`, backticks, a subshell, `if`, `for` or `eval` with
  literal arguments is seen by `command`, `forbidden_commands`, `writes` and `protected_files`.
  With `rm -rf*` forbidden, `bash -c 'rm -rf /'` and `echo $(rm -rf /)` are denied.
  With `AGENTS.md` protected, `bash -c 'rm AGENTS.md'` is denied.
- AC6: With `rm -rf*` forbidden, `sudo rm -rf /` and `env X=1 rm -rf /` are denied through the normalized form.
- AC7: A `pre_commit` happens when a command of the line, nested included, is normalized to `git commit`.
  This holds for `git commit -m x`, `cd x && git commit -m x`, `git -C ../repo commit`,
  `git -c user.name=a commit -m x`, `git --git-dir .git --work-tree . commit`, `bash -c "git commit -m x"` and
  `sh -c 'git commit'`.
  It does not happen for `echo "git commit"`, `git commit-tree HEAD^{tree}`, `git log --grep commit` nor `git status`.
- AC8: The `exit` condition counts a nested command only when it is the last command of a `sh -c` or `bash -c` script
  that itself propagates; a command inside `$(...)` never propagates.
- AC9: `onlyPolix` decides on the AST: `polix config check 2>&1` and `cd x && polix doctor` pass.
  `polix doctor $(rm -rf ai)`, `polix config check > out.txt` and `polix config fmt` are refused.
  `polix doctor "x>y"` passes, since the `>` is quoted text and no redirection.
- AC10: `commitDirs` decides on the AST: `git -C ../other commit -m x` and `git -C ../other add . ; git -C ../other
  commit -m 'a; b'` are placed.
  `git -C ../other commit -m "$(cat msg)"`, `git -C "$OTHER" commit`, `git -C ~/x commit` and
  `git -C ../other commit > log` are not placed.
  `git -C ../other commit -m x # note` is placed, since a comment is no word of the command.
- AC11: On Windows, `rm C:\Users\x\AGENTS.md` keeps its backslashes in the word, as before the change.
  On Linux and macOS, `rm AGENT\S.md` is read as bash reads it.
- AC12: `$'...'` words are decoded (`rm $'AGENTS.md'` writes `AGENTS.md`) and a redirection target in quotes with a
  space (`echo x > 'a b'`) is one target.
- AC13: `docs/polix.md` describes the parse, the normalized form, the nested commands and the fallback, and its limits
  no longer list what the AST now sees.

## Out of scope

- A new event for `git add`, and any new key of polix.yaml (request).
- Widening the list of wrappers (`timeout`, `sudo -u X` with its value): the interview kept the current list.
- Resolving variables, aliases or scripts that are not literal (`$GIT add`, `bash -c "$CMD"`): no static parser
  can, and the watch of files a command changed still catches their effect.
- Parsing PowerShell: such lines take the fallback.
- Changing the `commit` switch of `guidance` or its template.

## Starting point

- Read first: `internal/platform/shell/shell.go`: the splitter and its propagation rules, kept for the top level.
- Read first: `internal/app/hook/answer/logic_cmdline.go`: `parseCommand`, `program`, `matchingCommands`,
  `onlyPolix`, and the wrappers and writers lists.
- Read first: `internal/app/hook/answer/logic_commit.go` and `logic_writes.go`: `commitDirs`, `placeable` and how
  the writes of a line are found.
- Read first: `internal/app/hook/answer/in_input.go`: `gitCommit` and `hookInput.commands`.
- Read first: `docs/polix.md`, sections Protections, Conditions and Known limits.
- Baseline: `just test`, expected: every package passes, no failures.

## Progress

- [x] Task 1: Move the hand-written parsers into internal/platform/shell behind a parse of commands. Evidence: new `TestParse` and `TestParseEmpty` in shell_test.go pass (they did not compile before, since `Parse` did not exist); `TestSplit` and `TestParseWords` moved to shell_fallback_test.go with their cases and pass; `TestProgram`, `TestMatchingCommands`, `TestOnlyPolix`, `TestCommitDirs`, `TestWrites`, `TestWritePaths` and `TestForbidden` stay green; `just test` and `golangci-lint run ./...` pass.
- [x] Task 2: Parse command lines with mvdan.cc/sh, falling back to the hand-written parsers. Evidence: new `TestParseSplitCases`, `TestParseWordCases`, `TestParseBackslash`, `TestParseQuotes`, `TestParseLiteral` and `TestParseShapes` in shell_ast_test.go failed with the fallback-only Parse and pass now; `TestParseFallback` passed before too (the fallback already existed); `TestWrites` gets `rm $'AGENTS.md'`; the verification run of TestParse, TestSplit, TestWrites, TestForbidden, TestOnlyPolix and TestCommitDirs passes; the differential check of 70 lines differs only on `rm AGENT\S.md` and `rm $'AGENTS.md'` (AC11, AC12).
- [x] Task 3: See the commands nested in -c scripts, substitutions, eval and compound commands. Evidence: new `TestParseNested` (shell_ast_test.go) failed with the top-level walk and passes now; new `TestScript` (shell_nested_test.go), `TestForbiddenNested`, the nested cases of `TestWrites` and `TestConditionExitNested` pass; `TestParseSplitCases` keeps the subshell propagation; `just test` passes; the differential check differs on lines1 only by AC11/AC12, and on the Task 3 lines only by AC5 and AC8 cases (plus one line whose inject had already acted in the same session, equal when run alone).
- [x] Task 4: Match command patterns as written or normalized. Evidence: new `TestNormal` (shell_normal_test.go) failed to build before (no `NormalText`) and passes; new `TestConditionCommandNormalized` (AC4) and `TestForbiddenNormalized` (AC6) failed on the wrappers and git options before the change and pass; `TestProgram` stays green with new cases of `git rm`/`git mv` after wrappers and assignments; the verification run passes, `just test` and `golangci-lint` pass; the differential check differs on the Task 4 lines only for the AC4 forms (`GOFLAGS=x go test`, `sudo env FOO=1 go test`, `bash -c 'go test'`), and on the earlier lines as before.
- [x] Task 5: Recognise commits from the parsed commands. Evidence: new `TestCommitRecognised` (in_test.go) failed on `echo "git commit"` and `git log -S "git commit"` with the regex and passes now, the fallback cases (`git commit -m "unclosed`) included; `TestGitCommit` and `TestParseClaude` stay green; `just test`, `golangci-lint` and TestGOARCH37 pass; the differential check differs on the AC7 lines only for `echo "git commit"`.
- [x] Task 6: Decide onlyPolix and commitDirs on the parsed commands. Evidence: new cases of `TestOnlyPolix` (`polix doctor "x>y"`) and `TestCommitDirs` (`git -C ../other commit -m x # note`) failed with the text checks and pass; the other AC9 and AC10 lines and the fallback lines keep their result; four former refusals of `TestCommitDirs` that bash reads exactly (a comment line, `c\ommit`, `\-C`, `"a\"b"`) are now placed where bash commits (see Departures of the report); the verification run passes; the differential check differs only on `polix doctor "x>y"` with a broken polix.yaml (AC9) and `git -C ../other commit -m x # note` (AC10).

## Tasks

### Task 1: Move the hand-written parsers into internal/platform/shell behind a parse of commands

- Covers: none.
- High risk: none; a refactor that keeps the behaviour, guarded by the existing tests of shell and of hook/answer.
- Files: `internal/platform/shell/shell.go` (modify: a parse returning commands, the command type),
  `internal/platform/shell/shell_fallback.go` (create: the splitter and the word parser moved from today's code),
  `internal/platform/shell/shell_test.go`, `internal/platform/shell/shell_fallback_test.go` (create),
  `internal/app/hook/answer/logic_cmdline.go` (remove `parseCommand`, `cmdParser`, `simpleCommand`; adapt `program`,
  `matchingCommands`, `propagates`, `onlyPolix`), `internal/app/hook/answer/logic_cmdline_test.go`,
  `internal/app/hook/answer/logic_writes.go`, `internal/app/hook/answer/logic_commit.go`.
- Rules: GOARCH-1, GOARCH-4, GOARCH-10, GOARCH-13, GOARCH-19, GOARCH-35, GOARCH-38, GOARCH-41, GOARCH-42, GO-2, GO-4.
- Reuse: `shell.Split`, `trimSubshell` from `internal/platform/shell/shell.go`; `parseCommand`, `cmdParser` from
  `internal/app/hook/answer/logic_cmdline.go`.
- Tests: `kept` `TestSplit` moves with the splitter and keeps its cases.
  `kept` `TestParseCommand` moves to `shell_fallback_test.go` and checks the words and write targets of each command.
  `kept` `TestProgram`, `TestMatchingCommands`, `TestOnlyPolix`, `TestCommitDirs`, `TestWrites`, `TestWritePaths`
  and `TestForbidden` keep their results unchanged.
  `new` a test of the parse in `shell_test.go` that each command carries its text, words, write targets and
  propagation, and that the parse reports it used the fallback; it fails before since the parse does not exist.
- Changes: `internal/platform/shell` gets a command type with the fields of the Approach and a parse of a line, given
  whether the system is Windows, that returns the commands and whether the AST parsed it.
  In this task the parse always uses the fallback: the splitter gives the segments, the moved word parser gives each
  segment its words and write targets, and the backslash handling of Windows moves with it.
  The callers in `internal/app/hook/answer` read the parsed commands instead of calling `shell.Split` and
  `parseCommand`; their results do not change.
  Each moved function keeps its comment; the package comment says the fallback is a heuristic.
- Verification: `just test -run 'TestSplit|TestParse|TestProgram|TestMatchingCommands|TestOnlyPolix|TestCommitDirs|
  TestWrites|TestWritePaths|TestForbidden'`, expected: all pass.

### Task 2: Parse command lines with mvdan.cc/sh, falling back to the hand-written parsers

- Covers: AC1, AC2, AC3, AC11, AC12.
- High risk: a new dependency (delivery); input from outside (the command line of the agent) read by the protections
  (security); behaviour of `protected_files`, `writes` and `forbidden_commands` that the tests only partly cover.
  Guarded by the kept tests, the new cases and the differential check.
- Files: `go.mod`, `go.sum` (add `mvdan.cc/sh/v3` at its latest release), `internal/platform/shell/shell.go`
  (the parse tries the AST first), `internal/platform/shell/shell_ast.go` (create: walk of the top level statements
  into commands, words and redirections), `internal/platform/shell/shell_ast_test.go` (create),
  `internal/app/hook/answer/logic_writes_test.go`, `internal/app/hook/answer/answer.md` (the Logic section),
  `docs/polix.md` (Protections: how a line is parsed and the fallback).
- Rules: GOARCH-1, GOARCH-13, GOARCH-19, GOARCH-21, GOARCH-35, GOARCH-41, GOARCH-42, GO-2, GO-4, XPLAT-1, DOC-1.
- Reuse: the fallback of Task 1.
- Tests: `new` in `shell_ast_test.go`: the cases of `TestSplit` and `TestParseCommand` give the same commands through
  the AST and the parse reports it parsed them; they fail before since no AST parse exists.
  `new` a line the parser rejects (`echo "unclosed`) and a PowerShell line (`Remove-Item -Path "x" -Force; if ($?)
  { ls }`) give exactly the fallback result.
  `new` with Windows set, `rm C:\Users\x\AGENTS.md` keeps its backslashes; with Windows unset, `rm AGENT\S.md` gives
  `AGENTS.md`.
  `new` `rm $'AGENTS.md'` gives the word `AGENTS.md`, and `echo x > 'a b'` gives the one target `a b`.
  `new` in `logic_writes_test.go`: with `AGENTS.md` protected, `rm $'AGENTS.md'` is found as a write of it.
  `kept` every test of Task 1.
- Changes: the parse first runs the bash parser on the line; on error it returns the fallback result.
  On success it walks the top level statements as `shell.Split` did, with its rules of propagation, and turns each
  call into a command with its text, its words from the literal parts, and its redirections.
  Redirections that write are `>`, `>>`, `>|`, `&>`, `&>>`, `<>` and `>&` to a file.
  A duplication to a digit or `-`, an input, a here-document and a here-string write nothing.
  A word with an expansion keeps its source text and is marked not literal.
  On Windows a word whose source has a backslash before an ordinary character keeps its source text.
  Nested commands are not walked yet.
  `docs/polix.md` and `answer.md` say that the line is parsed as bash, and that an unparseable line falls back to the
  heuristic.
- Verification: `just test -run 'TestParse|TestSplit|TestWrites|TestForbidden|TestOnlyPolix|TestCommitDirs'`,
  expected: all pass.
  Differential check: export `HEAD` with `git archive HEAD | tar -x -C <temporary directory>`.
  Build both trees with `go build -o <temporary directory>/polix-<base|new> ./cmd/polix`.
  In a temporary git repository with a polix.yaml that protects `AGENTS.md` and `ai/**`, forbids `rm -rf*` and has a
  `pre_command` entry with `command: "go test*"`, feed both binaries the same `PreToolUse` Bash inputs on stdin of
  `polix hook claude`.
  Use every command line of the tests of this task and of Task 1.
  Compare stdout and exit codes: only the cases of AC11 on Unix and AC12 may differ.

### Task 3: See the commands nested in -c scripts, substitutions, eval and compound commands

- Covers: AC5, AC8.
- High risk: security (what `forbidden_commands`, `protected_files` and `writes` deny) and existing behaviour of
  the `exit` condition; guarded by the new cases and the differential check.
- Files: `internal/platform/shell/shell_ast.go` (walk nested commands),
  `internal/platform/shell/shell_nested.go` (create: the recursion into `-c` scripts and `eval`),
  `internal/platform/shell/shell_ast_test.go`, `internal/platform/shell/shell_nested_test.go` (create),
  `internal/app/hook/answer/logic_protect_test.go`,
  `internal/app/hook/answer/logic_writes_test.go`, `internal/app/hook/answer/logic_condition_test.go`,
  `internal/app/hook/answer/answer.md`, `docs/polix.md` (Protections and its limits, the `command`, `writes` and
  `exit` conditions).
- Rules: GOARCH-1, GOARCH-13, GOARCH-21, GOARCH-35, GO-2, GO-4, DOC-1.
- Reuse: the AST walk of Task 2.
- Tests: `new` in `shell_ast_test.go`: `bash -c 'rm -rf /'`, `sh -c "git add . && git commit -m x"`,
  `echo $(rm -rf /)`, a backtick substitution, `(cd x && rm y)`, `if true; then rm y; fi`, `for f in a; do rm $f;
  done` and `eval rm AGENTS.md` each yield the nested commands, marked nested.
  `bash -c "$CMD"` yields no nested command.
  These fail before since the walk stops at the top level.
  `new` in `logic_protect_test.go`: with `rm -rf*` forbidden, `bash -c 'rm -rf /'` and `echo $(rm -rf /)` are denied.
  `new` in `logic_writes_test.go`: with `AGENTS.md` protected, `bash -c 'rm AGENTS.md'` writes it.
  `new` in `logic_condition_test.go`: `exit: 1` holds for a failing `bash -c 'true && just test'` with
  `command: just test*`, and not for `echo $(just test)`.
  `kept` the cases of `TestSplit` for subshells (`(cd h && go test) && just test`) keep their propagation.
- Changes: the walk enters substitutions, process substitutions, subshells, blocks, `if`, `while`, `for`, `case` and
  function bodies, and gives their calls as commands, nested where they are inside a substitution.
  For `sh`, `bash` and `zsh` with `-c` followed by a literal word, and for `eval` with literal words, the text is
  parsed again and its commands are added as nested.
  The last of a `-c` script propagates when the `-c` command propagates; any other nested command does not.
  The commands of a subshell, block or compound command at the top level keep today's propagation.
  The docs drop `bash -c`, `sh -c` and `$(...)` from the limits of `forbidden_commands` and `protected_files`, and
  say how nested commands propagate.
- Verification: `just test -run 'TestParse|TestForbidden|TestWrites|TestCondition'`, expected: all pass.
  Differential check: the one of Task 2 with the command lines of this task added; only the cases of AC5 and AC8 may
  differ.

### Task 4: Match command patterns as written or normalized

- Covers: AC4, AC6.
- High risk: a contract that others consume (the meaning of `command` and `forbidden_commands` in polix.yaml) and
  security (`forbidden_commands` denies more); guarded by the new cases and the differential check.
- Files: `internal/platform/shell/shell_normal.go` (create: the normalized words and text of a command, with the
  wrappers list and the assignment pattern moved from `logic_cmdline.go`),
  `internal/platform/shell/shell_normal_test.go` (create), `internal/app/hook/answer/logic_cmdline.go`
  (`matchingCommands`, `program`, `wrappers`, `assignment`, `gitProgram`),
  `internal/app/hook/answer/logic_cmdline_test.go`,
  `internal/app/hook/answer/logic_protect_test.go`, `internal/app/hook/answer/logic_condition_test.go`,
  `internal/app/hook/answer/answer.md`, `docs/polix.md` (the `command` condition and `forbidden_commands`),
  `docs/use-cases.md` (a recipe of a slow formatter before `git add`), `examples/polix.yaml` (the comment of
  `command`).
- Rules: GOARCH-1, GOARCH-13, GOARCH-19, GOARCH-21, GOARCH-35, GOARCH-41, GO-2, GO-4, DOC-1.
- Reuse: `program` and `gitProgram` from `internal/app/hook/answer/logic_cmdline.go`; `glob.Command` from
  `internal/platform/glob`.
- Tests: `new` in `shell_normal_test.go`: `sudo env FOO=1 git add .` normalizes to `git add .`.
  `git -C x -c k=v --no-pager add -A` normalizes to `git add -A`, and `LANG=C nice -n sort f` keeps the value as
  today's wrappers do.
  These fail before since no normalized form exists.
  `new` in `logic_condition_test.go`: the lines of AC4 make `command: ["git add*", "git stage*"]` hold or not as
  AC4 says.
  `new` in `logic_protect_test.go`: with `rm -rf*` forbidden, `sudo rm -rf /` and `env X=1 rm -rf /` are denied.
  `kept` `TestProgram`: `git rm` and `git mv` are still `rm` and `mv`, after wrappers and assignments.
- Changes: each command gets its normalized words and text as the Approach says; `program` starts from the normalized
  words and keeps its mapping of `git rm` and `git mv`.
  `matchingCommands` holds when a pattern matches the text as written or the normalized text of a command.
  The docs of `command` and `forbidden_commands` say both forms match, with the example of `git -C x add .`.
  `docs/use-cases.md` gets the recipe of a slow formatter on `pre_command` with
  `command: ["git add*", "git stage*"]`, modelled on the `spotless:apply` entry of the request.
- Verification: `just test -run 'TestNormal|TestProgram|TestMatchingCommands|TestForbidden|TestCondition'`,
  expected: all pass.
  Differential check: the one of Task 2 with the lines of AC4 and AC6; only those cases may differ.

### Task 5: Recognise commits from the parsed commands

- Covers: AC7.
- High risk: existing behaviour of `pre_commit`, the gate of every commit, changed on purpose for quoted text;
  guarded by the cases of `TestGitCommit` and the differential check.
- Files: `internal/app/hook/answer/in_input.go` (`hookInput.commands`, `gitCommit` kept for the fallback),
  `internal/app/hook/answer/in_test.go` (`TestGitCommit`), `internal/app/hook/answer/in_claude_test.go`,
  `internal/app/hook/answer/answer.md`, `docs/polix.md` (How a hook call is answered, Known limits).
- Rules: GOARCH-15, GOARCH-20, GOARCH-21, GOARCH-23, GOARCH-37, GO-2, GO-4, DOC-1.
- Reuse: the parse and the normalized words of Tasks 2 to 4; `gitCommit` for the fallback.
- Tests: `new` `TestCommitRecognised` in `in_test.go`: a table over `hookInput.commands` with the lines of AC7,
  which give `pre_commit` or not as AC7 says; `echo "git commit"` fails before since the regex matches it.
  `new` the fallback: `git commit -m "unclosed` still gives `pre_commit` through the regex.
  `kept` `TestParseClaude` in `in_claude_test.go`: a Bash `git commit -m x` gives `pre_command` and `pre_commit`.
- Changes: `hookInput.commands` adds `pre_commit` when a parsed command, nested included, has normalized words
  starting with `git` and `commit`; when the parse used the fallback, the `gitCommit` regex decides.
  The comment of `gitCommit` says it is the fallback.
  `docs/polix.md` replaces the known limit of the heuristic with the parse and the fallback, and says `echo "git
  commit"` no longer counts.
- Verification: `just test -run 'TestGitCommit|TestCommitRecognised|TestParseClaude'`, expected: all pass.
  Differential check: the one of Task 2 with the lines of AC7; only `echo "git commit"` may differ.

### Task 6: Decide onlyPolix and commitDirs on the parsed commands

- Covers: AC9, AC10.
- High risk: security, the two places where a line skips the rules (a broken polix.yaml, a commit of another
  repository); guarded by the cases of AC9 and AC10, the kept tests and the differential check.
- Files: `internal/app/hook/answer/logic_cmdline.go` (`onlyPolix`), `internal/app/hook/answer/logic_commit.go`
  (`commitDirs`, `gitPlace`, `placeable` kept for the fallback), `internal/app/hook/answer/logic_cmdline_test.go`,
  `internal/app/hook/answer/logic_commit_test.go`, `internal/app/hook/answer/answer.md`, `docs/polix.md` (Failure
  policy, How a hook call is answered).
- Rules: GOARCH-4, GOARCH-21, GO-2, GO-4, DOC-1.
- Reuse: the parsed commands, their literal flags and redirections from Tasks 2 and 3; `changeDir` and `driveless`
  from `internal/app/hook/answer/logic_commit.go`.
- Tests: `new` in `logic_cmdline_test.go`: `polix doctor "x>y"` passes, since the `>` is quoted text; it fails
  before, since the text check refuses any `>`.
  `kept` the other lines of AC9 keep today's result, `polix doctor $(rm -rf ai)` now refused by its nested `rm`.
  `new` in `logic_commit_test.go`: `git -C ../other commit -m x # note` is placed, since the comment is no word; it
  fails before, since the text check refuses `#`.
  `kept` the other lines of AC10 keep today's result.
  `kept` every case of `TestOnlyPolix` and `TestCommitDirs`, Windows folders included.
  `new` the fallback: a line the parser rejects keeps today's result in both.
- Changes: on a parsed line, `onlyPolix` holds when every command, nested included, matches `onlyPolixCommands` as
  written and none has a redirection other than `2>&1` or `1>&2`.
  On a parsed line, `commitDirs` places it when every command, nested included, is `git` with no assignment nor
  wrapper, has only `-C` options with literal values and `add` or `commit` as subcommand.
  Its words must all be literal and it must have no redirection.
  The folder is computed by `changeDir` as today.
  On a fallback parse both decide with today's text checks.
  The docs say both decide on the parsed line.
- Verification: `just test -run 'TestOnlyPolix|TestCommitDirs|TestCommand'`, expected: all pass.
  Differential check: the one of Task 2 with the lines of AC9 and AC10 and a broken polix.yaml for AC9; only the
  cases those criteria name may differ.

## Risks

- The parser accepts a line bash rejects or the reverse: the fallback keeps the old behaviour on errors, and the
  differential checks compare every known line.
- The binary grows with the new dependency: measure the size of `just build` before and after and report it.
- A deep or huge line slows the hook: the parser is linear, and recursion only enters literal scripts, bounded by the
  length of the line.
- `forbidden_commands` and `protected_files` deny more than before (nested commands, the normalized form): this is
  asked by AC5 and AC6; a repository that relied on `bash -c` to get around them is now stopped.
- `echo "git commit"` no longer triggers `pre_commit`: asked by AC7; the githooks of polix still gate the real commit.
- Windows paths: the source-text rule keeps backslashes; the CI matrix runs the Windows cases.

## Verification

- `just test`: every package passes.
- `golangci-lint run ./...`: no findings, depguard included.
- `just check`: polix config check, polix rules check and polix doctor --offline pass.
- End to end: in a temporary repository with the polix.yaml of the differential check plus a `pre_command` entry
  `when: { command: ["git add*", "git stage*"] }` with `run: touch formatted`, send `git -C . add .` through
  `polix hook claude`: the run happens; send `git commit -m "add foo"`: it does not.

## Iterations

### Iteration 01: 2026-11-12

- **Tasks**: Tasks 1 to 6 done (the builder stopped after Task 1 and was resumed for Tasks 2 to 6).
- **Departures**: logic_command.go, logic_condition.go and logic_protect.go changed only to pass `windows`;
  shell_word.go added for GO-4; `Command.Redirections` added; `((...))` takes the fallback; commands inside
  if/while/for/case/functions never propagate; emission order is command, `-c` script, substitutions;
  `shell.Program` turns `\` into `/` on every system; reference.yaml kept identical to examples/polix.yaml;
  `hookInput.commits` uses runtime.GOOS; four former refusals of `TestCommitDirs` are now placed (a comment then a
  second line, `c\ommit`, `\-C`, `"a\"b"`); a clause is a wordless command whose text may match a pattern; with a
  broken polix.yaml `if polix doctor; then polix version; fi` is refused; a here-document body is taken as written.
- **Docs**: CHANGELOG.md (Unreleased, Changed), docs/reference.md (`forbidden_commands`, `command`, `writes`,
  command patterns), docs/use-cases.md (forbidden commands, protected files), docs/polix.md (Known limits); the
  builder updated docs/polix.md, docs/use-cases.md, internal/app/hook/answer/answer.md and examples/polix.yaml.
- **Review rev01**: changes requested; fixed the blocking `case` patterns, the major wrappers before `-c` and the
  major stale comments of examples/polix.yaml; the minor on wrappers with a value fixed in the docs.
- **Review rev02**: approved with comments; fixed the major propagation inside `$(...)`, the major `! cmd`, the major
  heredocs fed to a shell (partly) and the minor of docs/reference.md.
- **Review rev03**: approved with comments; left open the major on here-documents with expansions, piped into a
  shell or fed to `bash -`, which the old splitter read line by line.
- **Acceptance**: AC1 met (`shell.Parse`, TestParse, TestFallback); AC2 met (TestFallback, TestParseFallback); AC3
  met with the four placed cases of `TestCommitDirs` as recorded; AC4 met (TestConditionCommandNormalized, end to
  end); AC5 met (TestParseNested, TestForbiddenNested, TestWrites, end to end); AC6 met (TestForbiddenNormalized,
  end to end); AC7 met (TestCommitRecognised); AC8 met (TestConditionExitNested); AC9 met (TestOnlyPolix); AC10 met
  (TestCommitDirs); AC11 met (TestParseBackslash; Windows CI not run); AC12 met (TestParse, TestWrites); AC13 met
  (docs/polix.md, docs/reference.md, CHANGELOG.md).
- **Status**: partial: the last review leaves a major finding open.

### Follow-up: 2026-11-12

- **Request**: "arregla lo de los heredocs"; "la 2 pasalo a ast yo creo" (the built-in protection of the state).
- **Heredocs**: the body of a here-document given to a shell is parsed as written, expansions included
  (`bash <<EOF\nrm -rf $HOME\nEOF`), `bash -` reads stdin, and a here-document or here-string piped into a shell
  (`cat <<'EOF' | bash`) is entered (`reader.piped`, `stdinShell`, `reader.hereDoc`); cases in TestScript,
  TestForbiddenNested and TestCommitRecognised. This closes the open finding of rev03.
- **State**: `mayWrite` decides on the parsed line, nested commands included, ignoring the null device and writer
  names that are only arguments; `writesFiles` stays for a line the parser rejects; `namesState` still reads the
  text of the line; cases in TestMayWrite.
- **Docs**: docs/polix.md (Protections, `command` condition), answer.md, CHANGELOG.md.
- **Review**: changes requested. Fixed: the blocking, `mayWrite` let through writers as arguments (`find -exec rm`,
  `xargs rm`, `sudo -u x rm`), shells fed from a pipe or a substitution and non-literal scripts, now any writer word
  and any shell, `eval`, `source` or `.` counts; the major heredocs, now a pipeline of any length, a compound
  feeder, a valued wrapper before the shell, the escapes of an unquoted delimiter, `--rcfile` and only fd 0; the
  major docs of the protection of the state; the two minor comments.
- **Review before commit**: approved with comments. Fixed: the major `shellOptions` with a cluster ending in o
  (`bash -euo pipefail -c`), which hid the script; the major interpreters of other languages (`awk`, `perl`,
  `python`, `ruby`, `node`...) now count for `mayWrite`; the minor wording of `interpreters` in the docs; the minor
  comment of `hereDoc` garbled by gofmt.
- **Second review before commit**: approved with comments. Fixed: the major `shellOptions`, where each o or O of a
  cluster takes a word in turn (`bash -oc pipefail 'x'`, `-oo a b -c`); the major `interpreters`, now with `ash`,
  `csh`, `tcsh`, `pwsh`, `powershell`, `pypy` and any version suffix (`python3.12`, `perl5.34`); the docs list the
  shells and interpreters polix knows.
- **Third review before commit**: approved with comments. Fixed: the major regression of `mayWrite`, which let
  through a shell in another case (`Bash -c`, `PWSH.EXE`) or whose name is not literal (`$SHELL -c`); names are now
  compared without case and a non-literal program counts (`unknownProgram`); the minor list of the docs is open.
- **Fourth review before commit**: approved with comments; two majors were regressions against HEAD (a non-literal
  program behind a wrapper or `find -exec`, `env -S` with a `>`), the third (case in `script` for the other
  protections) was not. Root fix: `mayWrite` is never weaker than HEAD, `writesFiles` on the text without its
  redirections into the null device decides first and the parsed line only adds to it; a `>` inside quotes is
  refused again. The case of names in `script` and `programStart` stays as it is (not a regression) and the docs
  scope "without case" to the protection of the state.
- **Fifth review before commit**: changes requested; `mayWrite` is never weaker than HEAD but for `> nul` on Windows.
  Fixed: the blocking untested Windows branch and the major `> nul` (Git Bash may create a file named nul) by
  dropping NUL from the text check, so only `/dev/null` is dropped on every system, with Windows cases in
  TestMayWrite; the minor redirect into `/dev/null` followed by an operator (`2>/dev/null|head`).
- **Verification**: `just test` passes and `golangci-lint run ./...` 0 issues after the fixes.
