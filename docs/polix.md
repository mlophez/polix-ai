# polix (proof of concept)

This repository holds a proof of concept of `polix`, the engine that connects Claude Code and Codex to the
workflow and the rules of a repository through their hooks.
It is a Go module of its own (`github.com/mlophez/polix-ai`), separate from the Python engine of Logalty Control, so it can be
shipped as one static binary per operating system and run on every hook call in milliseconds.

The design, its alternatives and the open decisions live outside this repository, in the artifact "Dispatcher de
workflows" (proposal 2).

## Layers

- `AGENTS.md`: what the project is (facts).
- `ai/rules/*.yaml` (the folder `rules.path` of polix.yaml): how the code must be. Polix only reads `id`,
  `description`, `scope`, `deterministic` and `check` of each rule; the rest of the format belongs to the skills
  that write and use the rules. `retired.yaml` holds retired rules, which never apply.
- `polix.yaml`: what happens on each event of a session: what is blocked, required, run, injected and
  recorded.
- `polix`: the same binary in every repository; it reads the three and answers the hooks.

The human writes the three files; polix only enforces them.
It measures and never judges: conditions are facts of the repository, the event and the state, and judging the
code stays with the `reviewer` subagent.

## Commands

```bash
polix hook claude          # entry point of the Claude Code hooks (event on stdin)
polix hook codex           # entry point of the Codex hooks (experimental)
polix check                # validate polix.yaml and the rules (ai/rules/ or rules.path)
polix rules check          # run the checks of every deterministic rule (for CI)
polix rules check --changed  # only the rules whose scope matches the files changed by the branch
                             # (--base BRANCH, default the closest of base in polix.yaml, else main;
                             # it fails when no base exists, locally or on origin)
polix graph                # print the workflow as a Mermaid diagram
polix log --tail 20        # print the last entries of the state log (--json for the raw lines)
polix risk                 # print the lines, files and risk score of the uncommitted change, file by file
                           # (--changed GLOB, repeatable; --commit REV for the changes of one commit)
polix doctor               # check that this machine and repository are ready (--hook, --offline)
polix install              # plugin for the user, hooks for the repository (--user, --agent)
polix uninstall            # remove what install wrote (--user, --agent)
polix update               # replace the binary with the latest release and refresh the plugin
                           # (--check, --version V, --hook)
polix version
```

`polix rules check --changed` compares the branch with its base: `--base`, else the candidates of `base` in
`polix.yaml`, else `main`. Each candidate is tried as a local branch and then as `origin/<branch>`, and among those
that exist the one whose merge base is closest to HEAD wins, so a feature forked from `dev` compares with `dev`
when `base: [main, dev]`. When none exists the command fails naming them, and `polix check` and `polix doctor`
warn about it when a deterministic rule has a check. In GitHub Actions, run it with
`--base="${{ github.base_ref }}"` after a checkout with `fetch-depth: 0`: outside a pull request `base_ref` is
empty, and an empty `--base` falls back to `base`. On a push to a branch of `base` itself, such as `main`, the
branch is its own base and `--changed` checks nothing: run `polix rules check` without it there. The hooks never
use `base`.

`polix rules check` reads `polix.yaml` and then the rules of its `rules.path` before running anything, and stops
without running a check when a file of the folder of rules cannot be read or parsed, when a rule is not valid (no id, no description, a `check` on
a rule that is not `deterministic`, or a `check` given as an absolute path), when `polix.yaml` cannot be read or
parsed or requires a newer polix (without the file it uses the defaults), when a runner of `rules.run` is not
valid, and when `--base` is given without `--changed`. The other problems of a `polix.yaml` that parses are left
to `polix check`. The hint to set `base` or pass
`--base` only comes when no base branch exists. Each check runs from the root, with 5 minutes to finish, through
the first runner of `rules.run` whose glob matches its file, or else as a program (see `rules.run` below); it
prints each failure with the last 15 lines of its output, then `<n> checks, <m> failed`, and exits with 1 when a
check failed.

`polix risk` prints what the conditions `changed_lines`, `changed_files` and `risk_score` would compare (see
[Measures of the change](#measures-of-the-change)): what was measured, the three totals, and one line per file with
its added and deleted lines, its weight and its share of the risk score, the highest first. `--changed` narrows the
measure as the condition `changed` does, so `polix risk --changed '*.go'` shows the numbers of a gate scoped to the
Go files. `--commit <rev>` measures the changes a commit introduced instead (against its first parent, a root commit
against the empty tree), with the weights of the current `polix.yaml`, to calibrate thresholds and weights on the
history:

```bash
for c in $(git log --format=%h -20); do polix risk --commit "$c" --changed '*.go' | head -4; done
```

It fails, exiting with 1, outside a repository, with a `polix.yaml` that does not parse or is not valid, with a
revision that is not a commit, or when git fails.

The repository commands, `doctor`, `install` and `uninstall` take `--dir <repo>` (`-C`; default: the working
directory, or the `cwd` of the hook input).
The CLI is built with cobra and fang: `polix --help` and `polix <command> --help` give styled help, and
`polix completion <bash|zsh|fish|powershell>` the completions of the shell.
The output for people is styled with lipgloss on a terminal and plain text in a pipe, CI or with `NO_COLOR`; the
output of `polix hook` is never styled, since the agents read it.

Recipes: `just test`, `just build` (to `build/polix`), `just install` (`go install`, so the hooks find
`polix` on the `PATH`), `just dist <version>` (the assets of a release, in `dist/`) and `just release <version>`.

## Install, update and release

Polix runs on Linux, macOS and Windows, on amd64 and arm64. It is published as GitHub Releases of
`mlophez/polix-ai`: each release holds one raw binary per platform (`polix_<os>_<arch>`, `.exe` on Windows),
`install.sh`, `install.ps1` and `checksums.txt` with the SHA-256 of all of them.

macOS and Linux:

```bash
curl -fsSL https://github.com/mlophez/polix-ai/releases/latest/download/install.sh | sh
```

Windows, from PowerShell (Windows PowerShell 5.1 or PowerShell 7) or from cmd:

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -Command "irm https://github.com/mlophez/polix-ai/releases/latest/download/install.ps1 | iex"
```

On Windows, `install.sh` from Git Bash also works: it checks and runs the `install.ps1` of the same release, so a
machine gets the same install wherever it starts from.

Both scripts check the binary against `checksums.txt` before installing it and accept `POLIX_VERSION=0.2.0` to
pin a version and `POLIX_INSTALL_DIR` to change the folder:

- macOS and Linux: `~/.local/bin`; the script warns when it is not on the `PATH`.
- Windows: `%LOCALAPPDATA%\Programs\polix`, added to the user `PATH` (`POLIX_NO_MODIFY_PATH=1` leaves it alone),
  which every shell and Claude Code see once restarted. Installing over a running `polix.exe` moves it aside
  first, since Windows cannot overwrite a running binary.

With Go, `go install github.com/mlophez/polix-ai/cmd/polix@latest` also works on every system and reports its
version.

`examples/AGENTS-polix.md` is the fragment to paste into the `AGENTS.md` or `CLAUDE.md` of a governed
repository: it tells the agent to propose the install command of the system of the user, and to run it only
with their approval, when polix is missing.

Both scripts then run `polix install --user --yes`, which installs the plugin of the user (see below) and no hooks:
they belong to each repository. `POLIX_NO_SETUP=1` skips it.

`polix update` downloads the latest release (or `--version V`, older ones included), checks it against
`checksums.txt` and replaces the running binary atomically: on macOS and Linux a hook that starts meanwhile runs
the old binary or the new one, never half of it. On Windows the running `polix.exe` is moved aside and the new one
renamed in its place: between the two renames there is a short moment without `polix.exe`, and a hook that starts
then fails as a non blocking error, so that one call is not governed.
`--check` only reports whether a newer release exists; with `--version V` it says whether `V` is newer than the
running binary, the same or older (a downgrade). Neither downloads anything. A development build (`dev`) is only replaced with `--version`, and
`POLIX_NO_UPDATE=1` disables updates where the version is pinned by other means.
`--hook` is the mode of the session start hook: at most one check an hour (cached in the user cache folder), five
seconds to reach GitHub, never fails, prints one line only when it updated and ends with `polix doctor --hook` run
by the binary it leaves. The time of a check is saved only once the binary is the latest. The hook checks at once,
without waiting for the hour, when the cache already knows a newer release (a `polix doctor` or `--check` saw it)
or when this binary rejects the `polix.yaml` of the repository in a way a newer polix may not (it `requires` one,
names something this one does not know or has a later `version`). An update that fails (no network, a download
cut, a folder it cannot write, which it tests before downloading) is retried 15 minutes later, not at every
session start. `POLIX_NO_UPDATE=1` skips the update and the refresh of the plugin in the hook, not the doctor; it
is also what keeps an older version installed with `--version`, which the hook would otherwise update again.
Every release must keep what earlier releases rely on to update into it, since their update code is frozen in
the binaries of the users: the assets `polix_<os>_<arch>` (`.exe` on Windows) and `checksums.txt` in the format of
`sha256sum`, the redirect of `releases/latest` to `releases/tag/v<version>`, the cache `update.json`, and the
commands an old binary runs with the new one after replacing it: `__refresh --quiet`, `__refresh --hooks`,
`doctor --hook` and `install --user --yes --quiet` (`TestCommandsTheUpdateRuns` keeps them). A tag with a
prerelease (`v0.2.0-rc1`) is published as a GitHub prerelease, which `releases/latest` never points to.
Only releases update themselves: a development build (`just install`, or `go install` of a commit) never does, so
the machines that try polix install it with `install.sh` or `install.ps1`.
After the binary, `update` refreshes the plugin: when it replaced the binary, the new one does it, since the plugin
is embedded in it. A person who runs `polix update` also gets the plugin of the user installed when it is missing,
and the hooks of polix rewritten in the repository of the working directory when it registers them. The session
start hook only refreshes the plugin already installed: it never installs anything new nor touches the files of
the repository.

`polix doctor` checks the version against the latest release, that the `PATH` the hooks use runs this binary, git,
`polix.yaml`, the rules and that their checks exist and are executable (on Windows, that their interpreter is found;
a check that a runner of `rules.run` runs skips both, since its runner runs it), the hooks of Claude Code (project,
local and user settings: every event, every governed tool in the matchers, a timeout above the run budget, and never
twice), the plugin of the user (its version against the binary and, except in `--hook`, that Claude Code has it
installed and enabled), the hooks and the plugin of Codex when it is used and that the state can be written. It also
warns when `SessionStart` does not run `polix update --hook`, or runs the command of an earlier build, which `polix
install` replaces, and when `PostToolUseFailure` does not run polix, as in the settings of an earlier build: only
the feedback of failed commands is lost. It exits 1 on any failure. `--hook` prints nothing when everything is fine
and never fails; otherwise it prints the problems, which Claude Code adds to the context of the session, so the
agent tells the user and offers to fix them. It does not use the network: it compares with the latest release the
last `polix update` or `polix doctor` saw.

To publish a release, from a clean `main`: `just release 0.2.0`. It runs the tests, tags `v0.2.0` and pushes
the tag. `.github/workflows/release.yml` then runs the tests on Linux, macOS and Windows (`ci.yml`, which also
runs on every push and pull request), builds the assets with `scripts/dist.sh`, creates the GitHub release, and
finally installs it on the three systems with the documented commands (`install.sh`; on Windows `install.ps1`
under Windows PowerShell 5.1 and `install.sh` through Git Bash), checking the installed version.
The tests of `install.ps1` run with every PowerShell found, so on the Windows runners under 5.1 and 7.
The Justfile recipes use `sh`: on Windows, run them from Git Bash.

## How a hook call is answered

The use case `AnswerHook` of `internal/hook` (layout in `docs/architecture.md`) answers it:

1. The agent (`internal/agent`, Claude Code or Codex) turns the JSON of stdin into neutral events.
   An edit becomes one `pre_edit` or `post_edit` per path; a `git commit` in a command also produces
   `pre_commit`.
2. The logic of the hook (`internal/hook`) applies the protections, then the entries of the event in order.
3. The agent answers: exit 2 with the reasons on stderr to block, or exit 0 with
   `hookSpecificOutput.additionalContext` on stdout to inject context.

Edits outside the repository (scratchpad, memory, another repository) and of files git ignores are not part of
the change: the workflow does not apply to them, only the protections.

### Protections

The only built-in protection applies whatever the workflow says, even without one: nobody edits the state
(`<git-dir>/polix/`), and no command that names it may write files (`rm`, `mv`, `cp`, `tee`, `sed -i`,
redirections other than `2>&1`...); reading it is allowed, and `polix log` prints it.
A command names the state by its absolute path or below a git directory (`.git/polix`,
`.git/worktrees/<name>/polix`), followed by a slash, a space, a quote, `;`, `&`, `|`, `)`, `*` or the end of the
line: the bare word `polix` is the binary, not the state. On macOS and Windows, whose file names ignore case,
the state is found in any case.

polix protects nothing else on its own: `AGENTS.md`, `CLAUDE.md`, `ai/rules/` and `polix.yaml` may be edited by
any agent unless the workflow protects them. The key `protected` of `polix.yaml` lists path globs that no actor
writes, the main session included: an edit of a matching path is denied, and so is a command that writes files
and names one. Reading is allowed. A glob without `/` matches the name in any folder, so `polix.yaml` also keeps
`examples/polix.yaml`; write `/polix.yaml` to protect only the file of the root. It applies before the workflow, so files git ignores, such as `.env`, are
covered too, and it binds the skills as well: protecting `polix.yaml` or `ai/rules/` keeps the skills `workflow`
and `add-rule` from writing them. For finer control, such as keeping only subagents away, use the condition
`writes` with `actor` in a `pre_command` entry and `path` in a `pre_edit` entry.

Each command of the line is parsed with its quotes into the program it runs (past variable assignments, `sudo`,
`env`, `xargs`..., and `git rm` and `git mv` as `rm` and `mv`), its arguments and its redirections. It writes:

- the targets of its redirections inside the repository: `>`, `>>`, `>|`, `&>`, `>&file` and `x>AGENTS.md`
  without spaces. `2>&1`, an input, `/dev/null` and a path outside the repository write nothing, nor does `NUL`
  on Windows. A `>` inside quotes is text, not a redirection;
- every file it names, read or written, when the program writes by itself: `rm`, `mv`, `cp`, `tee`, `touch`,
  `ln`, `chmod`, `install`, `rsync`, `truncate`, `dd of=`, and `sed` or `perl` with `-i`. For these a folder that
  holds what a glob matches counts too: the literal folder a glob with a slash starts with, or a parent of it
  (`ai/rules` and `ai` for `ai/rules/**`, `docs` for `docs/secret.md`), and a word with wildcards that may reach
  it (`rm -rf ai/*`, and `rm *.md` for `AGENTS.md`, but not for `docs/secret.md`). `rm`, and `mv` for its sources,
  also count on the root and its parents (`rm -rf .`). The reason then names the glob the folder holds.

Reading is allowed: `cat CLAUDE.md > notes.txt`, `ls ai 2>/dev/null` or `rg install ai` write nothing protected.
On macOS and Windows the globs of `protected` and `writes` ignore case, so `.ENV` is `.env` there. It is a
heuristic, aimed at accidents rather than at an agent determined to get around it. It does not see:

- a program that writes through an interpreter or another command: `python -c "open(...)"`, `bash -c '...'`,
  `sh -c`, `find ai -delete` or `find ai -exec rm {} \;`;
- a wrapper with an option that takes a value (`sudo -u root rm`, `nice -n 10 rm`, `xargs -I {} rm`), or one it
  does not know (`timeout 10 rm`): the wrappers it skips are `sudo`, `env`, `command`, `nohup`, `time`, `exec`,
  `xargs` and `nice`, with their options but not their values;
- a wildcard against a pattern that has wildcards too (`rm -f .env*` for `.env.*`, `rm ai/rules/*` for
  `ai/rules/*.yaml`): wildcards are only compared with literal paths and folders, and with a whole folder of `/**`;
- a path reached through `cd` (`cd ai && rm -rf rules`) or built by the command itself;
- quoting other than that of bash: a backslash before an ordinary character stays (for the paths of Windows, so
  `rm AGENT\S.md` is not `AGENTS.md`), `$'...'` is not decoded, and under PowerShell a quoted path that ends in a
  backslash (`"ai\rules\"`) is read as an escaped quote.

### Failure policy

- Outside a git repository, or without `polix.yaml`, only the protections apply.
- An invalid workflow blocks edits and commands (fail closed) with the problems found, so a broken file is
  noticed at once, except what fixing it needs: the main session may edit `polix.yaml`, any actor may run
  `polix check`, `polix update`, `polix doctor` and `polix version` (optionally after a `cd`), and edits outside
  the repository or of ignored files go through as always. The built-in protections still apply. It never blocks a stop, which
  would trap the agent in a loop. When a problem names something unknown, the reason adds that it may be an
  option of a newer polix, to update instead of removing it.
- A diff that cannot be measured makes `changed_lines`, `changed_files` and `risk_score` hold (fail closed), with
  one warning per call that names the cause.
- A workflow that `requires` a newer polix than the one running is not broken but ahead: it blocks the same way,
  asking to run `polix update`, and not even the main session may edit `polix.yaml`, so the agent does not take
  options out of it to please an old binary. A development build ignores `requires`, and `polix check` rejects one
  that is not a version like `0.2.0`.

## polix.yaml

`examples/polix.yaml` is the current Logalty flow written as a workflow, annotated as a reference of every option;
a test validates it.

```yaml
version: 1
requires: 0.1.0       # oldest polix that knows every option of the file (optional, since 0.1.0)
base: [main, dev]     # branch, or candidates, a change merges into, for rules check --changed (default main)
protected: [".env"]   # path globs no agent writes, the main session included (optional)
max_stop_blocks: 3    # loop guard of every block on stop and subagent_stop, per entry (default 3)
guidance:             # built-in instructions injected at session start (optional, all true by default)
  commit: true        # run each commit as a git commit of its own
  edit: true          # change files with the edit tool of the agent, not the shell
  layout: true        # the folders of rules, plans and reports
rules:
  path: ai/rules      # the folder of the rules (optional, default ai/rules)
  run:                # how a check of a rule runs, by the glob of its file (optional)
    "*_test.go": [go, test, "{dir}", -run, "^Test({names})$"]
plans:
  path: ai/plans      # the folder of the plans (optional, default ai/plans)
reports:
  path: ai/reports    # the folder of the reports (optional, default ai/reports)
risk:
  weights:            # factor of each path glob in risk_score, first match wins (optional, default 1)
    "*_test.go": 0.5
    "internal/core/**": 3
vars:
  key: { branch: "^([A-Z][A-Z0-9]+-[0-9]+)" }   # first group of the branch name, used as {key}
on:
  <event>:
    - if: { <condition>: <value>, ... }      # optional: all must hold
      unless: { <condition>: <value>, ... }  # optional: skip when all hold
      mode: enforce | warn | audit           # optional, enforce by default
      why: <reason of the entry>             # optional, for people
      <action>: <value>                      # exactly one
      record: <fact>                         # optional next to another action: recorded once it applied
```

`rules.path`, `plans.path` and `reports.path` place the folders of the AI, relative to the root: the rules polix
reads, injects and checks, the plans the skill `plan` writes and the reports of audits and reviews. Each defaults
to its folder below `ai/`, and is cleaned (slashes, no `./` nor trailing slash). `polix check` reports an absolute
path, the root itself, a path out of the root (`..`), a glob or an unknown key. polix reads only the folder of the
rules, its `*.yaml` but `retired.yaml` and not its subfolders; the other two reach the agent through
`guidance.layout`, so the skills write where the repository says.

`rules.run` maps a glob, matched against the `check` of a rule as the globs of `scope` are (without `/` against its
base name, with `/` against its path relative to the root, `**` crossing folders), to the argv that runs it from the
root, without a shell; the first glob that matches wins, in the order of the file. So `tests/architecture/*_test.go`
and `tests/e2e/*_test.go` may have different runners. The argv may use the singular placeholders `{file}` (the
check, relative to the root, with `/`), `{dir}` (its folder as `./<folder>`, `./` at the root), `{id}` (the id of
the rule, `GOARCH-4`) and `{name}` (the id with only its letters and digits, `GOARCH4`), and the plural ones
`{names}` (the names of a group joined with `|`, a regex alternation), `{files}` and `{dirs}` (one argument per
distinct file or folder of the group, in the order of the rules; each must be a whole argument).

The checks of one runner whose argv, with the singular placeholders expanded, is the same share one invocation,
where the plural placeholders cover them all: `[go, test, "{dir}", -run, "^Test({names})$"]` runs once per folder,
`[go, test, ./...]` once for all its rules, and a runner with `{name}` once per rule. A shared invocation that
passes makes every rule of it pass. One that fails runs each of its rules again alone, with the plural placeholders
reduced to that rule, and those runs decide them; when the argv of a rule alone is the shared one (no placeholder
tells the rules apart), every rule of the group fails with the shared output. The runs are sequential, and the
timeout of a check (5 minutes) applies to each invocation. A check that no
glob matches runs as a program, as before: through its shebang, or on Windows through its interpreter by extension
(see `run: { rules }` below). On Windows a runner whose program is `bash`, `sh`, `python`, `python3` or `py` runs
the program of that name the PATH finds (an active virtualenv wins), unless it is an alias of the Store or, for
`bash` and `sh`, the bash of WSL in the Windows folder; then the interpreter that lookup finds (the bash of Git for
Windows, the py launcher, python3 or python), with the arguments of the runner kept. There are no default runners.
`polix check` reports a runner with an empty argv, an empty glob, `{files}` or `{dirs}` inside a longer argument,
or an unknown placeholder. So a rule may be decided by any file, a Go test among them: with the runner above, the
rules of `tests/architecture/` run together as `go test ./tests/architecture -run ^Test(GOARCH1|GOARCH2|...)$`, and
when it fails, `GOARCH-4` (`check: tests/architecture/goarch04_test.go`) runs alone as
`go test ./tests/architecture -run ^Test(GOARCH4)$`.

`guidance` turns on or off the instructions polix injects into the agent by itself on `session_start`, so the agent
works in the forms its heuristics recognise. `commit` asks to run each commit as a `git commit` command of its own,
not chained with `&&`, `;` or `|` nor wrapped in `sh -c`, so `pre_commit` sees it. `edit` asks to create and change
the files of the repository with the edit tool of the agent instead of the shell (`sed -i`, redirections, `tee`,
heredocs), since `pre_edit`, `post_edit` and the rules they inject only see what goes through that tool. `layout`
tells the agent the folders of `rules.path`, `plans.path` and `reports.path`, so it and the skills use them instead
of assuming `ai/`. Every switch is on when the key, or the switch, is left out, and `false` turns one off. The text
is the template `templates/session_start.md.tmpl`, embedded in the binary (`text/template`, one block per switch),
rendered as one context. It is injected only with a valid `polix.yaml`, on every source of the event (startup,
resume, clear and compaction), before the contexts of the `session_start` entries, and in English. It asks the
agent to cooperate and blocks nothing. `polix check` reports an unknown key, and a value that is not a boolean, or a
`guidance` that is not a map, stops the parse.

Events: `session_start`, `prompt`, `pre_edit`, `post_edit`, `pre_command`, `post_command`, `pre_commit`,
`subagent_stop` and `stop`.

Conditions:

- `branch`: the current branch matches a regular expression.
- `var`: a variable of `vars` has a value.
- `path`: the edited path matches a glob (or a list); only on `pre_edit` and `post_edit`.
- `changed`: a file not committed yet (changed since HEAD, or untracked and not ignored) matches a glob (or a
  list). The flow is reviewed commit by commit: once committed, a change no longer counts, whatever `base` says,
  so gates that must hold before a commit belong on `pre_commit`.
- `file`: a file of the repository matches a glob; with `contains`, it also holds that text.
- `command` and `exit`: a command of the line matches a pattern (`*` matches anything) and its exit code.
  `command` only on `pre_command`, `post_command` and `pre_commit`; `exit` only on `post_command`.
  The line is split into its commands (`cd x && just test 2>&1 | tail -3` runs `cd x`, `just test 2>&1` and
  `tail -3`, quotes respected), and each one is matched on its own.
  The exit code of the line only counts for a command whose failure propagates to it, alone, last or followed by
  `&&`: in `just test | tail`, `tail` decides it, so `exit: 0` does not hold and a fact like `tested` is not
  recorded from it.
- `writes`: a command of the line writes files and names a path that matches a glob (or a list), with the same
  heuristic as `protected`; reading does not count. Only on `pre_command`, `post_command` and `pre_commit`.
- `actor`: who acts, `session` or the subagent type. An agent of a plugin is named with its prefix, as
  `polix:rules-reviewer`; an agent of `.claude/agents/` by its bare name.
- `done`: a fact was recorded on this branch for exactly the current content.
- `not`: a map of nested conditions; it holds unless every one of them holds.
- `any`: a map of nested conditions; it holds when at least one of them holds. It may hold `not` and sit inside
  it, and an empty `any` is a problem.
- `changed_lines: { at_least: N }`, `changed_files: { at_least: N }` and `risk_score: { at_least: N }`: the
  measures of the change not committed yet reach `N`, a non-negative integer. See
  [Measures of the change](#measures-of-the-change).

Globs are relative to the root and use `/`: `*` stays inside a segment, `**` crosses segments, a pattern
without `/` matches the base name anywhere (`*.md`), and a leading `/` anchors it to the root, as in `.gitignore`
(`/polix.yaml` is the file of the root, not `examples/polix.yaml`).
Strings may use `{path}`, `{command}`, `{branch}`, `{changed_lines}`, `{changed_files}`, `{risk_score}` and the
variables, which cannot take those built-in names.

### Measures of the change

The measure conditions look at the same files as `changed`: the diff against HEAD plus the untracked files not
ignored (every file, without a commit yet). An untracked file counts all its lines as added; a binary file counts as
a changed file with zero lines.

- `changed_lines` holds when the added plus deleted lines of the measured files are at least `at_least`.
- `changed_files` holds when at least `at_least` measured files changed, deleted and binary ones included.
- `risk_score` holds when the risk score of the measured files is at least `at_least`: the sum over the files of
  weight times log2(1 + added + deleted lines), rounded half away from zero. The weight of a file is the number of
  the first glob of `risk.weights` that matches it, in the order of the file, else 1; so a test of a critical
  folder can weigh less than the folder if its glob comes first.

Each value is a map with one key, `at_least`; there is no short form, so `changed_lines: 150` is a problem.
A measure has no globs of its own: it measures the files of the nearest `changed`, the one in its own map, else
the one of the enclosing map (`any` and `not` nest maps), else the whole uncommitted diff, docs and plans included.
Write it next to a `changed`:

```yaml
on:
  pre_commit:
    - if:
        changed: ["*.go", go.mod, go.sum]   # scopes the measures below
        any:
          changed_lines: { at_least: 150 }
          changed_files: { at_least: 5 }
          risk_score: { at_least: 25 }
      unless: { done: reviewed }
      deny: "Cambio de {changed_lines} líneas en {changed_files} ficheros (riesgo {risk_score}): revísalo"
```

`{changed_lines}`, `{changed_files}` and `{risk_score}` expand in every string of an entry to the measures over the
`changed` of its `if`, else the whole diff, so a message shows the numbers its thresholds compare. The diff is read
once per call, and only when a condition or a string needs it. When it cannot be measured (git fails), the three
conditions hold and the call warns once with the cause, as `done` with an unknown content hash: a broken git never
skips a review. The placeholders then show `?`. The measures are those of the start of the call: a `run` that
rewrites files on the same event does not change them, as for `changed`.

`polix check` reports a measure that is not a map, without `at_least`, with an `at_least` that is not a
non-negative integer or with another key, an empty `any`, and a weight with an empty glob or a negative number. A
`risk` or `risk.weights` that is not a map, a key of `risk` other than `weights`, and a weight that is not a number
stop the parse. Calibrate the thresholds and the weights with `polix risk`.

Actions:

- `deny: <reason>` blocks the event and stops the evaluation; not valid on `session_start`, `post_edit` or
  `post_command`, which already happened.
- `allow: true` stops the evaluation without blocking (`false` is rejected, since it would stop it too).
- `require: <what to do>` blocks `stop` or `subagent_stop` until it is done; only valid there.
- `run: { command: [..] }` runs a command from the root, without a shell; a failure blocks with its output.
- `run: { rules: changed }` runs the checks of the deterministic rules whose scope matches the changed files
  (`rules: "{path}"` for one path); a failure blocks naming the rule ids. Each check runs with the first runner
  of `rules.run` whose glob matches it, grouped and rerun on failure as `polix rules check` does, so the block
  names only the rules that fail; a check no runner matches runs as a
  program. On Windows, where a shebang does
  nothing, a `.sh` check runs with the `bash` of Git for Windows, found next to `git` (else a `bash` of the `PATH`
  outside the Windows folder, never the one of WSL), and a `.py` check with `py -3`, then `python3`, then
  `python`, skipping the aliases of the Microsoft Store; a missing interpreter fails the check naming it, and
  `polix doctor` reports it.
- `context: { file | text | rules }` injects a file, a text, or the rules whose scope matches a path
  (`rules: "{path}"`), each rule once per session. Not valid on `stop` or `subagent_stop`, which cannot
  receive context.
- `record: <fact>` alone records a fact on the current content. A rules file that cannot be loaded fails
  `run: { rules }`.

`record` may also go next to any other action of the entry, as the fact that action records once it applied:

- `deny` and `require` record when they block, when they warn and when the loop guard lets a stop through; a
  `deny` that blocks records before it stops the evaluation.
- `allow` and `context` record when the entry applies, `context` even when every rule it would inject was
  already injected in the session.
- `run` records only when it passes, on the content it saw: a failing run records nothing, and neither does one
  during which the content changed (a formatter that rewrites files, another agent editing). A gate on `stop`
  with `unless: { done: <fact> }` then runs until it passes, and once per content after that.
- In `audit` mode no entry records anything, whatever its action, so trying an entry never changes what the
  others see.

`record` goes next to `run`, never inside it: `polix check` reports `run: { ..., record }` and names the fix.
A `deny` or `require` that records the fact its own `unless` (or a `not: { done }` of its `if`) checks asks once
per content instead of enforcing: the next stop or commit on the same content passes even if nothing was done.
That suits a request the agent answers in the background, such as a review; keep a `deny` on `pre_commit` without
`record` when the gate must hold.

Modes: `enforce` applies the entry; `warn` turns a block into a warning for the agent; `audit` only logs what it
would have blocked, to try a new entry before enforcing it. What each one does on each event is in
[How the entries of an event are evaluated](#how-the-entries-of-an-event-are-evaluated).

`polix check` rejects unknown events, conditions, actions and keys (inside `run`, `context` and `vars` too),
variables and placeholders that are not defined, and conditions on events where they never hold, so a typo
never turns into an entry that silently stops applying. Of `ai/rules/` it reports every problem, not only the
first: a rule without id or description, an id repeated, a check that does not exist, a `check` on a rule that is
not `deterministic: true`, and a `check` named by an absolute path instead of one relative to the root.

### How the entries of an event are evaluated

One hook call may produce several events: an edit of several paths gives one `pre_edit` per path, and a command
with a `git commit` gives `pre_command` and `pre_commit`. Each event is answered on its own and the answers are
merged: the call blocks when any of them blocks, with the reasons of all of them.

For every event the [protections](#protections) come first: the state of polix and the `protected` paths are
denied before any entry is read. Then the entries of the event run one after the other, in the order of the file,
never in parallel. An entry whose `if` does not hold, or whose `unless` holds, is skipped. A `run` waits for its
command to finish before the next entry starts, so a `stop` takes the sum of its runs, all of them bounded by the
budget of 10 minutes of the hook call (see [Limits of the proof of concept](#limits-of-the-proof-of-concept)).

Only two things stop the evaluation and leave the later entries of the event unread:

- `allow`, which answers without blocking.
- `deny` when it actually blocks: in `enforce` mode and not let go by the [loop guard](#loop-guard).

Everything else goes on to the next entry. A failing `run` and a pending `require` block the event, but the later
entries still run, so one answer reports every failure at once. `context` and `record` never block; when the event
blocks, the texts of its `context` entries go with the reasons.

What a block does depends on the event, since some of them already happened:

- `pre_edit`, `pre_command`, `pre_commit`: the tool call is denied and the agent receives the reasons.
- `prompt`: the prompt is rejected before the agent reads it.
- `post_edit`, `post_command`: the edit or the command already happened. Only a failing `run` blocks here, since
  `deny` and `require` are not valid on them, and its output reaches the agent as feedback on that call.
- `stop`, `subagent_stop`: the agent, or the subagent, may not end its turn and receives the reasons to act on
  them, up to the loop guard.
- `session_start`: the session starts anyway; the agent only shows the reasons to the user.

The mode of the entry decides whether a block happens and who learns about it:

- `enforce`: the entry blocks as above.
- `warn`: nothing is blocked; the log records a `warn` decision and the reason reaches the agent as context, prefixed
  with "Aviso de polix:", on the events that take context (`session_start`, `prompt`, `pre_*` and `post_*`).
  `stop` and `subagent_stop` take no context: there a warning only reaches the agent inside the message of a block
  of another entry of the same stop. Otherwise polix writes it on stderr, which the agent does not read and Claude
  Code only shows in the transcript, so a gate on stop that the agent must notice is `enforce`.
- `audit`: nothing is blocked and the agent learns nothing; the log records an `audit` decision, read with
  `polix log`.

In `enforce` and `warn` mode an entry that applies records its fact, if it has one, and in `audit` none does; an
entry that does not block lets the evaluation go on.

The `stop` of this repository shows it. It has two entries, golangci-lint and `go test ./...`, both in `enforce`
mode, each with `record` and `unless: { done: <fact> }`. When the agent ends a turn after changing a Go file:

1. golangci-lint runs. When it fails, the stop blocks and nothing is recorded, but the evaluation goes on.
2. `go test ./...` runs anyway. When it passes, it records `tested` on the current content.
3. The agent receives the output of golangci-lint and has to go on. If it fixes the code, the content changes, so on
   the next stop both entries run again; if it stops again without changing anything, `tested` is done for that
   content and only golangci-lint runs, until the loop guard lets it stop after the third block.
4. Once both pass, `linted` and `tested` are recorded, and a later stop on the same content runs nothing.

### Loop guard

Every block of `stop` or `subagent_stop` in enforce mode (`require`, `deny` or a failing `run`) counts for the
loop guard: after `max_stop_blocks` blocks of the same entry, in the same session, by the same actor and on the
same content, the agent may stop and the log records a `loop_guard` decision.
Counting per entry keeps several pending blocks from using up the guard of each other, and subagents never use
up the guard of the main session.
When the log cannot be read, or the block cannot be written to it (a read only state folder), the guard cannot
count: it fails open, lets the agent stop with a warning and records a `loop_guard` decision when it can.

## State

`<git-dir>/polix/log.jsonl`, one JSON object per line, only written by polix:

- `fact`: what it observed (`record`), with the branch and the content hash.
- `decision`: what it answered (block, warn, audit, allow, loop guard), the `action` and `line` of the entry and
  why: the evidence of the process each change followed. It carries the content hash only when the call already
  computed it, since computing it costs a pass over the whole tree.
- `injected`: the rules already injected in a session.
- `stop_block`: the blocks of a stop, per entry, session, actor and content, for the loop guard.

The reason of an entry is capped at 8 KiB and each line kept from the output of a `run` or a check at 500 bytes,
both marked where cut, so no line of the log nears the read limit of 4 MiB.
A line over that limit (written by hand, say) is skipped when the log is read, and `polix log` says how many
lines it skipped; the entries around it still count.

The content hash is the git tree the working tree would produce with everything staged (untracked files included,
ignored ones excluded), computed on a copy of the index.
Any edit changes it, so a fact recorded before the edit stops counting without anyone invalidating it.
It is computed again after every `run`, with or without `record`, since a run may rewrite files.
When it cannot be computed (an unreadable file, a broken index), facts are neither recorded nor satisfied: `done`
does not hold, and the hook warns with the cause. The loop guard still counts the blocks of a stop made without a
hash, as one content, so the agent can always stop.

## The plugin and the hooks: polix install

`polix install` installs what each agent needs, from the binary, so every release brings its plugin and hooks with
it. Running it again rewrites what changed and removes what the release dropped. It has two parts:

- The plugin `polix`, for the user, since it helps in any repository. It is the folder `plugins/polix/` of this
  repository, embedded in the binary (package `plugins`): skills (`workflow` writes `polix.yaml`, `add-rule` writes
  the folder of rules, `doctor` fixes the setup, `plan` writes a plan in the folder of plans, `review` reviews the
  current change with the agent `reviewer`, `audit` audits the repository with several `auditor` agents in
  parallel and saves the report in the folder of reports) and, for Claude Code, agents (`workflow-author`,
  `rules-author` and the read-only `reviewer`, `rules-reviewer` and `auditor`). Install writes it, with the version of the binary in its manifests, to the local marketplace
  `~/.polix/marketplace/` and registers it. A development build, whose version is always `dev`, writes
  `0.0.0-dev+<hash of the plugin>` instead, so every change of the plugin is a new version.
  - Claude Code: `claude plugin marketplace add ~/.polix/marketplace --scope user` and
    `claude plugin install polix@polix --scope user`. Claude Code copies the plugin to its cache
    (`~/.claude/plugins/cache/polix/polix/<version>`), so after rewriting the folder install and update run
    `claude plugin update`, which copies it only when its version changed; it loads in the next session. Its
    skills are `/polix:workflow`, `/polix:add-rule`, `/polix:doctor`, `/polix:init`, `/polix:plan`,
    `/polix:review` and `/polix:audit`, its agents `polix:workflow-author`, `polix:rules-author`, `polix:reviewer`,
    `polix:rules-reviewer` and `polix:auditor`. The skills `review` and `audit` launch the agents; Codex, which
    has no agents of plugins, follows the file of the agent in the session instead.
  - Codex, when it is installed (or with `--agent codex`): `codex plugin marketplace add ~/.polix/marketplace` and
    `[plugins."polix@polix"] enabled = true` in `~/.codex/config.toml`, the rest of the file kept. Codex documents
    no command to install a plugin without its interface, so this part is experimental until it is checked against
    a running Codex. Codex plugins carry no agents.

  When the CLI of an agent is not on the `PATH`, install writes the plugin and prints the commands to run by hand.
- The hooks, for the repository of the working directory (or `--dir`), since they belong to the project like its
  `polix.yaml` and are committed with it: merged into `.claude/settings.json` and `.codex/hooks.json`. For Claude Code
  they cover `SessionStart`, `UserPromptSubmit`, `PreToolUse`, `PostToolUse`, `PostToolUseFailure` (a failed
  command reaches `post_command` with a non-zero `exit`; a failed edit runs no `post_edit`), `SubagentStop` and
  `Stop`; Codex gets the same events except `PostToolUseFailure`. Every
  other setting, event and hook of those files is kept in its order, and the hooks of polix take the place of the
  ones they replace, so the file only changes where polix changed. A group that mixes a hook of polix with other
  commands keeps the other commands, and the hook of polix goes to a group of its own. Each group of hooks of an
  event, of polix or not, is written on one line, and the rest of the file is indented with two spaces. A
  symlinked file is written through the link, to its target, and an existing file keeps its permissions. Codex skips new or changed hooks until the
  user trusts them with `/hooks`. The plugin brings no hooks.

`--user` installs only the plugin; outside a git repository that is all install does. `polix uninstall` removes
the same parts: it unregisters the plugin from each agent and, for both, removes `~/.polix/marketplace/`; unless
`--user`, it removes the hooks of polix of the repository, and a hooks file when nothing else is left in it.

The plugin only arrives with `polix install`: the repository serves no marketplace of its own. The
reference of the skill `workflow` (`plugins/polix/skills/workflow/reference.yaml`) is a copy of
`examples/polix.yaml`, kept equal by a test; `claude plugin validate plugins/polix` checks the plugin.

Claude Code merges the hooks of the project, local and user settings, so polix registered in two of them runs
twice, which doubles every block and every record: `polix install` warns about it and `polix doctor` fails until
one of them goes. `examples/claude-settings.json` and `examples/codex-hooks.json` are, byte for byte, what install
writes in an empty repository for each agent (a test keeps them equal); they remain for a rollout that renders the
settings itself (Logalty Control).

On `SessionStart` with matcher `startup` (a new session, not a resume, a clear or a compaction) the hooks
also run `polix update --hook`: it keeps polix up to date and then runs `polix doctor --hook`, which reports any
problem of the machine or the repository. Update and doctor are one command because Claude Code runs the hooks of
an event in parallel, and doctor must see the binary the update left. These commands are committed in the
repositories and must keep working with every later release, so what they do lives in the binary: an earlier
build registered `polix update --hook; polix doctor --hook`, which `polix install`, or a `polix update` run by a
person, replaces (the session start hook never touches the files of the repository); until then the doctor runs
twice at session start.

Every hook command is a plain `polix ...` call, with no shell syntax, because Claude Code runs hook commands
with `sh -c` on macOS and Linux, with Git Bash on Windows, or with PowerShell on Windows when Git Bash is not
installed (Git for Windows is optional). A plain command means the same in all of them; `command -v`, `||` or
`{ ...; }` would break under PowerShell. For the same reason a hook cannot detect that polix is missing: until
it is installed, the hooks fail with "command not found" (or "is not recognized" in PowerShell), which Claude
Code treats as a non blocking error, and the workflow does not apply on that machine. The fragment of
`examples/AGENTS-polix.md` covers that case.

Every `run` of one hook call shares a budget of 10 minutes (commands and rule checks together), and the hooks
set the `timeout` of each `polix hook` to 900 seconds, above it. The default of Claude Code is 600
seconds (30 on `UserPromptSubmit`), not above the budget, so a long gate could be killed and the action let
through; `polix doctor` warns about it.
When the budget runs out, polix kills the command or the check and waits at most 5 seconds more for its output: a
grandchild that keeps the output open (a server started in the background) no longer holds the hook, although it
may outlive the budget, since polix does not kill the whole process tree.
In the real rollout Logalty Control would render them and pin the version of the binary.

## Limits of the proof of concept

- Codex is experimental: its tool names (`shell`, `apply_patch`...), whether it has a subagent stop event and how
  its hooks set a timeout are still to be checked against a running Codex.
- A `git commit` is recognised by a heuristic (global options and quoted commands included); the text of a
  commit inside quotes, as in `echo "git commit"`, also triggers `pre_commit`. The `commit` switch of `guidance`
  asks the agent to run each commit on its own, so it stays recognisable.
- Every hook call reads the whole log, which grows without bound.
- Releases are not signed: `checksums.txt` protects against a corrupt or swapped download from a mirror, not
  against someone who can publish releases of the repository. Signing them (cosign, or GitHub artifact
  attestations) is pending.
- The actions of `.github/workflows/release.yml` are pinned by tag, not by commit SHA.
- The condition `pr` (state of the pull request) is not implemented.
- The log is appended without a lock; parallel subagents could in theory interleave lines.
- Injected rules are not bounded: a project with many rules without scope gets all of them on its first edit.
- No tests inside `polix.yaml` yet. The key `why` of an entry is only a note; expiring entries with `review_by` is
  pending.
- Measured on macOS: about 13 ms per hook call without a workflow and about 130 ms for a stop that computes the
  content hash of a 730 file repository.
  Not measured on Windows yet.
