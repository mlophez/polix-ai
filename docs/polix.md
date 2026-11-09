# polix

Declarative guardrails for Claude Code: one `polix.yaml` decides what the agent may not touch, what must pass and
what runs at every step.

`polix` is the engine that connects Claude Code to the workflow and the rules of a repository through its hooks.
It is a Go module of its own (`github.com/mlophez/polix-ai`), shipped as one static binary per operating system, so
it runs on every hook call in milliseconds. Claude Code is the supported agent; Codex support is not implemented
(see [Known limits](#known-limits)).

0.1.0 is the first public release. `polix.yaml` is `version: 1`, and a 0.x release may still change its format:
`requires:` lets a repository pin the oldest polix that knows every option it uses.

## Layers

- `AGENTS.md`: what the project is (facts).
- `ai/rules/*.yaml` (the folder `rules.path` of polix.yaml): how the code must be. Polix only reads `id`,
  `description`, `scope`, `deterministic` and `check` of each rule; the rest of the format belongs to the skills
  that write and use the rules. `retired.yaml` holds retired rules, which never apply.
- `polix.yaml`: what happens on each event of a session: what is blocked, what runs and what is injected. It has
  two layers of its own: `guardrails:`, the basics every project wants from the first minute (files no agent
  writes, commands no agent runs, branches nobody commits on, a formatter after each edit, the checks before the
  agent ends its turn), each a short list; and `on:`, where the team orchestrates its flow (the review subagents,
  the builder, the plan, the ticket) as entries per event. The files and the commands of `guardrails:` are
  protections of the engine, applied before the entries; its other shortcuts expand into entries of `on:` (see
  [Guardrails](#guardrails)).
- `polix`: the same binary in every repository; it reads the three and answers the hooks.

The human writes the three files; polix only enforces them.
It measures and never judges: conditions are facts of the repository, the event and what polix observed by itself,
and judging the code stays with the `reviewer` subagent.

## Commands

```bash
polix hook claude          # entry point of the Claude Code hooks (event on stdin)
polix hook codex           # entry point of the Codex hooks (not implemented)
polix config check         # validate polix.yaml and the rules (ai/rules/ or rules.path)
polix config fmt           # format polix.yaml in place: keys in the order of the reference, comments kept
                           # (--check only tells whether it is formatted, exiting with 1 when not)
polix rules check          # run the checks of every deterministic rule (for CI)
polix rules check --changed  # only the rules whose scope matches the files changed by the branch
                             # (--base BRANCH, default the closest of base_branch in polix.yaml, else main;
                             # it fails when no base exists, locally or on origin)
polix graph                # print the workflow as a Mermaid diagram
polix log --tail 20        # print the last entries of the state log (--json for the raw lines)
polix risk                 # print the lines, files and risk score of the uncommitted change, file by file
                           # (--changed GLOB, repeatable; --commit REV for the changes of one commit)
polix doctor               # check that this machine and repository are ready (--hook, --offline)
polix install              # plugin for the user, hooks for the repository (--user, --agent; default claude)
polix uninstall            # remove what install wrote (--user, --agent)
polix update               # replace the binary with the latest release and refresh the plugin
                           # (--check, --version V, --hook)
polix version
```

`polix rules check --changed` compares the branch with the branch it merges into, `--base`, else the candidates
of `base_branch` in `polix.yaml`, else `main`. Each candidate is tried as a local branch and then as `origin/<branch>`, and among
those that exist the one whose merge base is closest to HEAD wins, so a feature forked from `dev` compares with
`dev` when `base_branch: [main, dev]`. When none exists the command fails naming them, and `polix config check` and
`polix doctor` warn about it when a deterministic rule has a check. In GitHub Actions, run it with
`--base="${{ github.base_ref }}"` after a checkout with `fetch-depth: 0`: outside a pull request `base_ref` is
empty, and an empty `--base` falls back to `base_branch`. On a push to a branch of `base_branch` itself, such as
`main`, the branch is its own base and `--changed` checks nothing: run `polix rules check` without it there. The
hooks never use `base_branch`.

`polix rules check` reads `polix.yaml` and then the rules of its `rules.path` before running anything, and stops
without running a check when a file of the folder of rules cannot be read or parsed, when a rule is not valid (no id, no description, a `check` on
a rule that is not `deterministic`, or a `check` given as an absolute path), when `polix.yaml` cannot be read or
parsed or requires a newer polix (without the file it uses the defaults), when a runner of `rules.run` is not
valid, and when `--base` is given without `--changed`. The other problems of a `polix.yaml` that parses are left
to `polix config check`. The hint to set `base_branch` or pass
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

Both scripts then run `polix install --user --yes`, which installs the plugin of the user for Claude Code (see
below) and no hooks: they belong to each repository. `POLIX_NO_SETUP=1` skips it.

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
installed and enabled), the hooks and the plugin of Codex only when polix is registered for it (a Codex hooks file
of the project or the user that runs polix, or the plugin enabled in `~/.codex/config.toml`, as
`polix install --agent codex` leaves them; its OK line says Codex support is not implemented and untested, and a
Codex never set up for polix gets no line), and that the state can be written. It also
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

The use case `AnswerHook`, `internal/app/hook/answer` (layout in `docs/architecture.md`) answers it:

1. The agent (`internal/app/agent`, Claude Code; its Codex adapter is not implemented, see
   [Known limits](#known-limits)) turns the JSON of stdin into neutral events.
   An edit becomes one `pre_edit` or `post_edit` per path; a `git commit` in a command also produces
   `pre_commit`.
2. The logic of the hook (`internal/app/hook`) applies the [protections](#protections): the built-in protection of the
   state, then `guardrails.protected_files` and `guardrails.forbidden_commands`; then the entries of the event in
   order.
3. The agent answers: exit 2 with the reasons on stderr to block, or exit 0 with
   `hookSpecificOutput.additionalContext` on stdout to inject context.

Edits outside the repository (scratchpad, memory, another repository) and of files git ignores are not part of
the change: the entries do not apply to them. They govern the change, what a commit may hold, so a build output, a
local `.env` or a scratch file never fires a ticket, plan, format or docs entry. The built-in protection applies to
both, and `protected_files` to the edits of files git ignores.
Likewise a commit in another repository, as `git -C ../other commit`, skips the entries of `pre_commit`, while
`protected_files` and `forbidden_commands` still apply to it. polix takes a commit as one of another repository
only through a strict allowlist: every command of the line is `git [-C <folder>]... add|commit ...`, with no
other option, alias, wrapper or variable; outside quotes the line holds no `$`, backtick, glob, brace, `~`, parenthesis, redirection, `#` comment,
backslash or second line, and inside double quotes no `$`, backtick or backslash; and the folder of each commit,
the session folder after its `-C` options, is outside the folder of this repository (a repository nested in it
counts as this one) and inside another existing repository, as git itself resolves it, whose git directory,
objects and refs are not those of this repository (so a linked worktree of it is not another one). On Windows each
`-C` folder carries its drive (`C:/x`): a Git Bash `/c/x`, a `\x` or a `C:x` is not placed. Any other commit counts
as a commit of this repository.

### Protections

The only built-in protection applies whatever the workflow says, even without one: nobody edits the state
(`<git-dir>/polix/`), and no command that names it may write files (`rm`, `mv`, `cp`, `tee`, `sed -i`,
redirections other than `2>&1`...); reading it is allowed, and `polix log` prints it.
A command names the state by its absolute path or below a git directory (`.git/polix`,
`.git/worktrees/<name>/polix`), followed by a slash, a space, a quote, `;`, `&`, `|`, `)`, `*` or the end of the
line: the bare word `polix` is the binary, not the state. On macOS and Windows, whose file names ignore case,
the state is found in any case.

polix protects nothing else on its own: `AGENTS.md`, `CLAUDE.md`, `ai/rules/` and `polix.yaml` may be edited by
any agent unless the workflow protects them. With a valid `polix.yaml`, two protections of `guardrails` come right
after the protection of the state, before the skip of the edits of files git ignores and before every entry of
`on`, for every actor, the main session included, without exceptions; the first one that denies ends the
evaluation of the event:

- `guardrails.protected_files` lists path globs that no actor writes: it denies an edit of a matching path (never
  one outside the repository) and, on `pre_command` and `pre_commit`, a command that writes files and names one.
  Reading is allowed. The reason names the path, `<path> is protected in polix.yaml: only a person changes it.`, or
  the protected glob a folder or a wildcard holds, `ai holds ai/rules/**, protected in polix.yaml: only a person
  changes it.` for `rm -rf ai`, and the log records a `deny` decision of the action `guardrails.protected_files`.
  A glob without `/` matches the name in any folder, so `polix.yaml` also keeps `examples/polix.yaml`; write
  `/polix.yaml` to protect only the file of the root. It covers the files git ignores, so `.env` is covered too,
  and it binds the skills as well: protecting `polix.yaml` or `ai/rules/` keeps the skills `workflow` and
  `add-rule` from writing them. For finer control, such as keeping only subagents away, use the condition
  `writes` with `actor` in a `pre_command` entry and `path` in a `pre_edit` entry.
- `guardrails.forbidden_commands` lists command patterns no actor runs: on `pre_command` and `pre_commit`, the
  first item in file order whose pattern matches any command of the line denies it, wherever it comes (`cd tmp &&
  rm -rf /`), with its `reason` or the default `This command is forbidden by polix.yaml: a person must run it.`,
  followed by the line; the log records a `deny` decision of the action `guardrails.forbidden_commands` at the
  line of the item. Each command is matched as written: like `writes`, a heuristic aimed at accidents. A pattern
  misses another spelling (`rm -fr /`, `git push -f`), a wrapper (`sudo rm -rf /`, unless the pattern starts
  with `*`) and a command run through an interpreter or a substitution (`bash -c '...'`, `$(...)`).

Unlike the protection of the state, they need a valid `polix.yaml` (see [Failure policy](#failure-policy)).
`polix graph` draws both lists in a subgraph `guardrails` before the events.

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
  also count on the root and its parents (`rm -rf .`). The reason of `protected_files` names the protected glob a
  folder or a wildcard holds.

Reading is allowed: `cat CLAUDE.md > notes.txt`, `ls ai 2>/dev/null` or `rg install ai` write nothing protected.
On macOS and Windows the globs of `path` and `writes`, and those of `protected_files`, ignore case, so `.ENV` is
`.env` there. It is a
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

- Outside a git repository, or without `polix.yaml`, only the built-in protection applies.
- An invalid workflow blocks edits and commands (fail closed) with the problems found, so a broken file is
  noticed at once, except what fixing it needs: the main session may edit `polix.yaml`, any actor may run
  `polix config check`, `polix config fmt --check`, `polix update`, `polix doctor` and `polix version` (optionally
  after a `cd`, never with a command substitution or a redirection but `2>&1`), and edits outside the repository or
  of ignored files go through as always. The built-in protection still applies, but
  `protected_files` and `forbidden_commands` do not: an ignored `.env` of `protected_files` may be edited until the
  file is fixed. It never blocks a stop, which
  would trap the agent in a loop. When a problem names something unknown, the reason adds that it may be an
  option of a newer polix, to update instead of removing it; a name of the old vocabulary is reported with its
  new spelling instead (see [Names of the old vocabulary](#names-of-the-old-vocabulary)).
- A diff that cannot be measured makes `changed_lines`, `changed_files` and `risk_score` hold (fail closed), with
  one warning per call that names the cause.
- A workflow that `requires` a newer polix than the one running is not broken but ahead: it blocks the same way,
  asking to run `polix update`, and not even the main session may edit `polix.yaml`, so the agent does not take
  options out of it to please an old binary. A development build ignores `requires`, and `polix config check`
  rejects one that is not a version like `0.2.0`.

## polix.yaml

`examples/polix.yaml` is a complete team workflow (ticket, plan, builder, tests, reviewer, commit), annotated as a
reference of every option;
a test validates it. [reference.md](reference.md) describes every key, event, condition and verb one by one.

```yaml
version: 1
requires: 0.1.0       # oldest polix that knows every option of the file (optional, since 0.1.0)
base_branch: [main, dev]  # branch, or candidates, a change merges into, for rules check --changed (default main)
max_stop_retries: 3   # loop guard of every block on stop and subagent_stop, per entry (default 3)
guardrails:           # the basics every project wants (optional)
  protected_files: [".env"]                       # path globs no agent writes, the main session included
  forbidden_commands:                             # commands no agent runs, each command of the line as written
    - { pattern: "git push*--force*", reason: "No force pushes" }
  protected_branches: [main]                      # branches nobody commits on
  format_on_edit:                                 # runs on each edited file of its path; a failure informs
    - { run: "gofmt -w {path}", path: "*.go" }
  run_before_done:                                # must pass before the agent ends its turn
    - { run: "go test ./...", changed: "*.go" }
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
    - when: { <condition>: <value>, ... }    # optional: all must hold
      unless: { <condition>: <value>, ... }  # optional: skip the entry when all hold
      action:                                # required: exactly one verb, block, run or inject
        block: <message>                     # or { message: <message>, once: true }
    - action:
        run: <command>                       # or [<arg>, ...], or { command | rules, message, always: true }
    - action:
        inject: <text>                       # or { file | text | rules, always: true }
```

An entry has two parts: when it applies (`when` and `unless`) and what it does (`action`). A comment of YAML is
the place for its reason, for people; the message of a `block` or a `run` is the reason the AI reads.

### Guardrails

`guardrails` holds the basics as five shortcuts, each with exactly one form: `protected_files` and
`protected_branches` take a list of strings, and `forbidden_commands`, `format_on_edit` and `run_before_done` a
list of maps.

Two of them are protections of the engine, which the workflow holds as lists (`workflow.Workflow.ProtectedFiles`
and `ForbiddenCommands`) and the hook applies before every entry, also on the files git ignores, to every actor and
without exceptions (see [Protections](#protections)):

- `protected_files: [<glob>, ...]`: no actor edits a matching path nor runs a command that writes one.
- `forbidden_commands: [{ pattern, reason }, ...]`: no actor runs a line in which a command matches a pattern;
  the reason is the `reason` of the first matching item, or `This command is forbidden by polix.yaml: a person
  must run it.`, followed by the line.

The other three are only sugar: `workflow.Parse` expands each one into entries of `on`, so the hook,
`polix graph`, the repeat rules and the loop guard see them as any entry.

- `protected_branches: [<name>, ...]`: a regular entry on `pre_commit` with `branch`, the anchored regular
  expression of the names (`*` matches any text), and the message
  `{branch} is protected in polix.yaml: create a branch for this change and commit there.`
- `format_on_edit: [{ run, path, message }, ...]`: a regular entry on `post_edit` per item that runs its command,
  with the condition `path` when it has one (the edited file, not `changed`).
- `run_before_done: [{ run, changed, message }, ...]`: a regular entry on `stop` per item that runs its command,
  with the condition `changed` when it has one.

In each event the entries of `protected_branches`, `format_on_edit` and `run_before_done` come first, then the
entries written in `on`, each group in the order of the file. Each expanded entry takes the line of its item, so
the messages of `polix config check`, the keys of the repeat rules and the loop guard tell them apart, and the
path of its shortcut (`workflow.Entry.From`), which `polix graph` names; `polix graph` draws `protected_files` and
`forbidden_commands` in a subgraph `guardrails` before the events. `polix config check` reports a value in another
form than its own (a single string where a list goes, a string where a map goes) with the expected form, an
unknown key inside `guardrails` or inside an item, an empty glob, pattern or branch, an item without `pattern` or
`run`, and the problems of the entries they expand into, each at the line of its item. A polix older than the
shortcuts rejects `guardrails` as an unknown key, so a file that uses them sets `requires`.
[reference.md](reference.md#guardrails) describes each one with its expansion.

`rules.path`, `plans.path` and `reports.path` place the folders of the AI, relative to the root: the rules polix
reads, injects and checks, the plans the skill `plan` writes and the reports of audits and reviews. Each defaults
to its folder below `ai/`, and is cleaned (slashes, no `./` nor trailing slash). `polix config check` reports an
absolute path, the root itself, a path out of the root (`..`), a glob or an unknown key. polix reads only the folder
of the rules, its `*.yaml` but `retired.yaml` and not its subfolders; the other two reach the agent through
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
`polix config check` reports a runner with an empty argv, an empty glob, `{files}` or `{dirs}` inside a longer
argument, or an unknown placeholder. So a rule may be decided by any file, a Go test among them: with the runner above, the
rules of `tests/architecture/` run together as `go test ./tests/architecture -run ^Test(GOARCH1|GOARCH2|...)$`, and
when it fails, `GOARCH-4` (`check: tests/architecture/goarch04_test.go`) runs alone as
`go test ./tests/architecture -run ^Test(GOARCH4)$`.

`guidance` turns on or off the instructions polix injects into the agent by itself on `start`, so the agent
works in the forms its heuristics recognise. `commit` asks to run each commit as a `git commit` command of its own,
not chained with `&&`, `;` or `|` nor wrapped in `sh -c`, so `pre_commit` sees it. `edit` asks to create and change
the files of the repository with the edit tool of the agent instead of the shell (`sed -i`, redirections, `tee`,
heredocs), since `pre_edit`, `post_edit` and the rules they inject only see what goes through that tool. `layout`
tells the agent the folders of `rules.path`, `plans.path` and `reports.path`, so it and the skills use them instead
of assuming `ai/`. Every switch is on when the key, or the switch, is left out, and `false` turns one off. The text
is the template `templates/session_start.md.tmpl`, embedded in the binary (`text/template`, one block per switch),
rendered as one context. It is injected only with a valid `polix.yaml`, on every source of the event (startup,
resume, clear and compaction), before the contexts of the `start` entries, and in English. It asks the
agent to cooperate and blocks nothing. `polix config check` reports an unknown key, and a value that is not a
boolean, or a `guidance` that is not a map, stops the parse.

Events: `start`, `prompt`, `pre_edit`, `post_edit`, `pre_command`, `post_command`, `pre_commit`,
`subagent_stop` and `stop`.
`stop` is the main session about to end its turn, and `subagent_stop` a subagent about to end: they are
separate events, so the entries of `stop` never run when a subagent ends.

Conditions:

- `branch`: the current branch matches a regular expression.
- `var`: a variable of `vars` has a value.
- `path`: the edited path matches a glob (or a list); only on `pre_edit` and `post_edit`. On macOS and Windows,
  whose file names ignore case, it ignores case too, as `writes` and the protection of the state do.
- `changed`: a file not committed yet (changed since HEAD, or untracked and not ignored) matches a glob (or a
  list). The flow is reviewed commit by commit: once committed, a change no longer counts, whatever `base_branch` says,
  so gates that must hold before a commit belong on `pre_commit`.
- `file`: a file of the repository matches a glob; with `contains`, it also holds that text. `contains` is only
  valid next to `file`, and never inside `any`, where it would hold on its own and make the `any` always true.
- `command` and `exit`: a command of the line matches a pattern (`*` matches anything) and its exit code.
  `command` only on `pre_command`, `post_command` and `pre_commit`; `exit` only on `post_command`.
  The line is split into its commands (`cd x && just test 2>&1 | tail -3` runs `cd x`, `just test 2>&1` and
  `tail -3`, quotes respected), and each one is matched on its own.
  The exit code of the line only counts for a command whose failure propagates to it, alone, last or followed by
  `&&`: in `just test | tail`, `tail` decides it, so `exit: 1` does not hold. A command of the AI may trigger an
  `inject` this way, but it never counts as a check that passed (see
  [What polix observes](#what-polix-observes-and-finished)).
- `writes`: a command of the line writes files and names a path that matches a glob (or a list), with the same
  heuristic as `protected_files` (see [Protections](#protections)); reading does not count. Only on `pre_command`, `post_command` and `pre_commit`.
- `actor`: who acts, `session` or the subagent type. An agent of a plugin is named with its prefix, as
  `polix:rules-reviewer`; an agent of `.claude/agents/` by its bare name.
- `finished`: a subagent type, or a list of them (all must hold), ended on this branch and the files of the scope
  of the entry did not change since. See [What polix observes](#what-polix-observes-and-finished).
- `not`: a map of nested conditions; it holds unless every one of them holds (with one nested condition, it is its
  negation).
- `any`: a map of nested conditions; it holds when at least one of them holds. It may hold `not` and sit inside
  it; an empty `any`, or one that holds `contains`, is a problem.
- `changed_lines: N`, `changed_files: N` and `risk_score: N`: the measures of the change not committed yet reach
  `N`, a non-negative integer. See [Measures of the change](#measures-of-the-change).

An exception to an entry is a `not` in its `when`, or a condition in its `unless`. To let the agent write the plan
before any other edit, the gate leaves the plan itself out:

```yaml
vars:
  key: { branch: "^([A-Z][A-Z0-9]+-[0-9]+)" }
on:
  pre_edit:
    - when: { not: { path: "ai/plans/{key}-*.md" } }
      unless: { file: "ai/plans/{key}-*.md" }
      action: { block: "There is no plan for {key}: write it with /polix:plan" }
```

Globs are relative to the root and use `/`: `*` stays inside a segment, `**` crosses segments, a pattern
without `/` matches the base name anywhere (`*.md`), and a leading `/` anchors it to the root, as in `.gitignore`
(`/polix.yaml` is the file of the root, not `examples/polix.yaml`).
Strings may use `{path}`, `{command}`, `{branch}`, `{changed_lines}`, `{changed_files}`, `{risk_score}` and the
variables, which cannot take those built-in names.

### Measures of the change

The measure conditions look at the same files as `changed`: the diff against HEAD plus the untracked files not
ignored (every file, without a commit yet). An untracked file counts all its lines as added; a binary file counts as
a changed file with zero lines.

Each measure takes a non-negative integer and holds from that number up:

- `changed_lines: N` holds when the added plus deleted lines of the measured files are at least `N`.
- `changed_files: N` holds when at least `N` measured files changed, deleted and binary ones included.
- `risk_score: N` holds when the risk score of the measured files is at least `N`: the sum over the files of
  weight times log2(1 + added + deleted lines), rounded half away from zero. The weight of a file is the number of
  the first glob of `risk.weights` that matches it, in the order of the file, else 1; so a test of a critical
  folder can weigh less than the folder if its glob comes first.

A measure has no globs of its own: it measures the files of the nearest `changed`, the one in its own map, else
the one of the enclosing map (`any` and `not` nest maps), else the whole uncommitted diff, docs and plans included.
Write it next to a `changed`:

```yaml
on:
  pre_commit:
    - when:
        changed: ["*.go", go.mod, go.sum]   # scopes the measures below
        any:
          changed_lines: 150
          changed_files: 5
          risk_score: 25
      unless: { finished: polix:reviewer }
      action:
        block: "Cambio de {changed_lines} líneas en {changed_files} ficheros (riesgo {risk_score}): revísalo con polix:reviewer"
```

`{changed_lines}`, `{changed_files}` and `{risk_score}` expand in every string of an entry to the measures over the
`changed` of its `when`, else the whole diff, so a message shows the numbers its thresholds compare. The diff is
read once per call, and only when a condition or a string needs it. When it cannot be measured (git fails), the
three conditions hold and the call warns once with the cause, as `finished` does not hold with an unknown content
hash: a broken git never skips a review. The placeholders then show `?`. The measures are those of the start of
the call: a `run` that rewrites files on the same event does not change them, as for `changed`.

`polix config check` reports a measure that is not a non-negative integer (a map with `at_least`, the old spelling, with
the hint to write the number alone), an empty `any`, and a weight with an empty glob or a negative number. A
`risk` or `risk.weights` that is not a map, a key of `risk` other than `weights`, and a weight that is not a number
stop the parse. Calibrate the thresholds and the weights with `polix risk`.

### Verbs

The `action` of an entry is a map with exactly one verb. Each verb has a short form, its value alone, and a long
form, a map:

- `block: <message>` blocks the event with a message for the AI (placeholders allowed). The long form is
  `block: { message: <message>, once: true }`, and `message` is required. Valid on `prompt`, `pre_edit`,
  `pre_command`, `pre_commit`, `stop` and `subagent_stop`; `start`, `post_edit` and `post_command`
  already happened, so `polix config check` rejects it there.
- `run: <command>` runs a command from the root, without a shell (no pipes, redirections or globs). The command is
  a string, split on spaces into arguments and never given to a shell, or a list of arguments when one holds a
  space; placeholders are allowed in each argument, and `{path}` stays one argument. The long form is a map with
  exactly one of `command` (the same command) or `rules`, plus `message`, the text before the output of a failure
  (placeholders allowed), and `always`.
- `run: { rules: changed }` runs the checks of the deterministic rules whose scope matches the changed files
  (`rules: "{path}"` for one path); a failure names the rule ids, and a rules file that cannot be loaded fails it.
  Each check runs with the first runner of `rules.run` whose glob matches it, grouped and rerun on failure as
  `polix rules check` does, so the failure names only the rules that fail; a check no runner matches runs as a
  program. On Windows, where a shebang does nothing, a `.sh` check runs with the `bash` of Git for Windows, found
  next to `git` (else a `bash` of the `PATH` outside the Windows folder, never the one of WSL), and a `.py` check
  with `py -3`, then `python3`, then `python`, skipping the aliases of the Microsoft Store; a missing interpreter
  fails the check naming it, and `polix doctor` reports it.
- `inject: <text>` injects a text for the AI (placeholders allowed). The long form is a map with exactly one of
  `file` (a file of the repository; a missing file injects nothing), `text` or `rules` (the rules whose scope
  matches a path, `rules: "{path}"`, each one as `- <id>: <description>`), plus `always`. Not valid on
  `stop` or `subagent_stop`, where the agent cannot receive context.

`once` only goes in a `block` and `always` only in a `run` or an `inject`, both as booleans.

A failing `run` answers with its `message`, a blank line and the last 15 lines of the output (the failing rules and
their output for `rules`). What it does depends on the event:

- On `prompt`, `pre_edit`, `pre_command`, `pre_commit`, `stop` and `subagent_stop` it blocks, as a `block`
  does, and ends the evaluation of the event (up to the [loop guard](#loop-guard) on the two stops).
- On `start`, `post_edit` and `post_command`, which already happened, it never blocks: the message and the
  output reach the AI as context, as feedback it acts on at once, and the log records an `inform` decision. A lint
  or a formatter on `post_edit` is feedback, not a gate.

A warning is an `inject`: the AI is told and nothing is blocked. There is no mode to try an entry without
enforcing it: try a new gate in a branch, and read its decisions with `polix log` once it is enforced.

### How often each verb acts

- A `block` blocks every time its entry applies: a gate, which holds until its `unless` does. `once: true` makes it
  a reminder: it blocks the first attempt of an actor (the main session, or a subagent type) in a session on a
  content and lets the next attempt through. An edit inside the scope of the entry (see below) makes it block
  again, an edit outside does not, and another actor or a new session, which never saw it, is reminded on its own.
  A reminder is for a request the agent may rightly decline, such as checking whether the docs need an update;
  a gate that must hold has no `once`, since the agent would pass a reminder by trying again.
- A `run` is not run again once it passed on the same branch and content: the second `stop` on unchanged files
  runs nothing. The repeat is keyed by what it runs, the expanded arguments joined by spaces (`rules` and the
  expanded target for `run: { rules }`), whoever's entry it is, so two entries that run `just test` share it and
  `{path}` in the command makes one key per path. A failing `run` runs again on the next event, and any edit makes
  it run again. `always: true` runs it every time, for a command whose result depends on more than the files of the
  repository (the network, the clock, a service).
- An `inject` reaches each actor once per session for each text it expands to, and `inject: { rules }` each rule
  once per actor and session. It is due again after a `start` of the session (a new, resumed, cleared or
  compacted context) and, for a subagent, after that subagent type finished in the session, since its next run
  starts with a new context. `always: true` injects it on every event. A text with a placeholder that changes on
  every event (`{command}`, a measure such as `{changed_lines}`) expands to a new text each time, so it is injected
  every time its value changes; `{path}` gives one inject per path. On `start` every `inject` acts every
  time, so `always` has no effect there.

A verb that does not act because of these rules is logged as a `skip` decision of its entry and never blocks.
When the log cannot be read, a `run`, an `inject` and a `block` with `once` act every time, and the call warns once
with the cause. On `stop` and `subagent_stop` the [loop guard](#loop-guard) cannot count, so it lets the agent
stop.

The scope of an entry is the set of files the `changed` of its `when` names (the nearest `changed`, as the measures
take it), or the whole tree when it names none. `finished` and a `once` block compare only the files of that scope:
an edit of `README.md` or `docs/` keeps a review of `changed: ["*.go"]` and a reminder already given. Name in
`changed` every file the subagent reviews: `go.mod` is not `*.go`.

### What polix observes, and finished

polix does not keep facts declared by the workflow. It writes to its log an `observed` line only for what the
workflow, or the evidence, asks about, each with the session, the agent, the actor, the event, the branch, the
line of the entry, a key and one outcome:

- `finished`: a subagent ended and the answer to its `subagent_stop` did not refuse. It is written, with the
  content hash and the subagent type as key, when a `finished` of the workflow names that type; polix observes it
  by itself, so no entry on `subagent_stop` is needed. Without a content hash it is written for any subagent when
  the workflow has an `inject` that acts once, so the inject reaches its next run; a subagent the loop guard let
  through gets this line too, never one with the hash.
- `passed`: a `run` of polix passed, with or without `always`, on a content that did not change while it ran; the
  key is what it ran and the hash is the content it saw. A run that rewrote files (a formatter) or ran with an
  unknown hash writes none. These lines show which checks were green on each content.
- `blocked`: an entry with `once: true` blocked, or any entry blocked a `stop` or a `subagent_stop`, with the
  content hash: the memory of the reminders and the count of the loop guard. The key is what a failing `run` ran,
  empty for a `block`.
- `injected`: an `inject` that acts once reached the AI, keyed by its entry and the text, file or rule it injected,
  without hash. On `prompt`, an inject followed by a block that rejects the prompt writes none, since the AI never
  sees that prompt; on the other events an inject before a block reaches the AI with the block and counts.
- `started`: a session started, resumed, was cleared or was compacted. Written only when the workflow has an
  `inject` without `always` on an event other than `start` (one that acts once, `inject: { rules }`
  included).

`finished: <subagent>` (or a list, all of them) holds when the latest `finished` line with a hash of that type on
the current branch has a content equal to the current one on the scope of the entry: the files that differ between
its tree and the current tree, if any, are all outside the scope. So a review of the Go code survives an edit of the
docs, and any edit of a Go file asks for a new one. It never holds for another type, nor without a known content
hash, nor when the `subagent_stop` of the subagent was refused: a subagent the loop guard let through after its
blocks did not finish, and the `finished` line without hash it may get is never read by the condition. A tree git
no longer has (pruned by `git gc`) or a comparison that fails counts as changed, with a `warn` decision that names
the error: the gate asks again.
The branch is compared exactly: on a detached HEAD, whose branch is empty, nothing written on a branch counts, and
the lines written on a detached HEAD only match each other. The same holds for the repeat of `run` and for a `once`
block.

Only what polix sees by itself counts: its own runs and the end of a subagent. A command the AI runs is never
recorded and never counts as passed, green or not: its text does not prove what ran (an alias, another folder, a
wrapper), a command sent to the background reports success before it ends, and the command may change the files it
checked. A check that must hold is a `run` of polix; the repeat of `run` already spares the AI a second run on a
content polix saw green, and the order of the entries, with the single stop rule, keeps a later gate from running
before a failing check.

### What polix config check reports

`polix config check` rejects unknown events, conditions, verbs and keys (inside `action`, `run`, `inject`, `block` and
`vars` too), an entry without `action`, an `action` without a verb or with two, a verb on an event where it is not
valid, a `run` without exactly one of `command` and `rules`, an `inject` without exactly one of `file`, `text` and
`rules`, a `block` without `message`, a `once` or `always` that is not a boolean or sits in the wrong verb,
`contains` inside `any`, variables and placeholders that are not defined, and conditions on events where they never
hold, so a typo never turns into an entry that silently stops applying. Of `ai/rules/` it reports every problem,
not only the first: a rule without id or description, an id repeated, a check that does not exist, a `check` on a
rule that is not `deterministic: true`, and a `check` named by an absolute path instead of one relative to the root.
It also rejects the problems of the shortcuts of [Guardrails](#guardrails).

### Names of the old vocabulary

Earlier builds of polix used other names, and only the new ones are accepted. `polix config check` reports each old name
with its line and its new spelling: the events `session_start` (`start`) and `turn_stop` (`stop`), the keys of an
entry `if` (`when`), `why` (a comment of YAML), `mode` (`warn` is an `inject`, a failing `run` on `post_edit` or
`post_command` already informs, `audit` has no replacement), `deny` and `require` (`block`), `allow` (a `not` in
`when`, or `unless`), `run` and `context` at the level of the entry (`action: { run }` and `action: { inject }`) and
`record` (`finished`), the condition `done` (`finished: <subagent>`) and a measure written as `{ at_least: N }` (the
number alone), also inside an old `if`, all in the same pass. The entries of an old event are checked as those of
its new name. Three top-level keys were renamed too: `protected` is now `guardrails.protected_files`, `base` is now
`base_branch`, and the key of the loop guard is now `max_stop_retries`; each old one is reported as an unknown key
with its new spelling, without the hint of a newer polix. Lines of the kinds `fact`, `injected` and `stop_block` written by those
builds stay in the log, are ignored by every condition and are shown by `polix log` with their kind (and the name
of a `fact`): after the upgrade, the reminders, the injects and the loop guard start from zero.

### How the entries of an event are evaluated

One hook call may produce several events: an edit of several paths gives one `pre_edit` per path, and a command
with a `git commit` gives `pre_command` and `pre_commit`. Each event is answered on its own and the answers are
merged: the call blocks when any of them blocks, with the reasons of all of them.

For every event the [protections](#protections) come first, before any entry is read: the protection of the
state, then, with a valid `polix.yaml`, `protected_files` and `forbidden_commands`, also on the edits of files git
ignores; the first that denies ends the evaluation. Then, unless the edit or commit is outside the change, the
entries of the event run one after the other, in the order of the file, never in parallel. The shortcuts
`protected_branches`, `format_on_edit` and `run_before_done` are already entries by then, before the entries
written in `on` (see [Guardrails](#guardrails)). An entry whose `when` does not hold, or whose `unless` holds, is
skipped; one that applies carries out the verb of its action, unless the repeat rules of [How often each verb acts](#how-often-each-verb-acts)
skip it. A `run` waits for its command to finish before the next entry starts, so a `stop` takes the sum of
its runs, all of them bounded by the budget of 10 minutes of the hook call (see
[Known limits](#known-limits)).

One rule ends the evaluation, whatever the verb: the first entry that actually blocks, a `block`
or a failing `run` on an event where it blocks, and not let through by the [loop guard](#loop-guard). The entries
after it never run;
those before it already acted, so the texts of an `inject` before the block reach the agent with it and count as
given. On `prompt` the rejected prompt never reaches the AI, so an `inject` before the block is not counted as given
and acts again on the next prompt.
Everything else goes on to the next entry: an `inject`, a skipped verb, a passing `run`, a failing `run` on
`start`, `post_edit` or `post_command`, and a block the loop guard let through.

The trade-off is one failure per answer: when the lint and the tests of a `stop` both fail, the agent gets the
lint first and the tests on its next attempt. The order of the file is the priority, so put first the gate whose
failure makes the later ones pointless, and the cheaper one when it does not matter. The repeat of `run` keeps the
extra turns cheap: on the next attempt the checks that passed on the same content do not run again.

What a block does depends on the event:

- `pre_edit`, `pre_command`, `pre_commit`: the tool call is denied and the agent receives the reasons.
- `prompt`: the prompt is rejected before the agent reads it.
- `stop`, `subagent_stop`: the main session, or the subagent, may not end its turn and receives the reasons to
  act on them, up to the loop guard.
- `start`, `post_edit`, `post_command`: they already happened, so nothing blocks there: `block` is rejected
  by `polix config check`, and a failing `run` reaches the agent as context, logged as an `inform` decision.

The `stop` of this repository shows it. Its `guardrails.run_before_done` has two items, golangci-lint over the
module and `go test ./...`, which expand into two plain `run` entries of `stop`, each with a `when` on the files it
depends on. When the agent ends a turn after changing a Go file:

1. golangci-lint runs. When it fails, the turn is blocked with its output and the evaluation ends: the tests do not
   run on code that does not pass the lint.
2. The agent fixes the lint and ends its turn again. The content changed, so golangci-lint runs again; it passes,
   polix writes a `passed` line for it, and `go test ./...` runs. When the tests fail, the turn is blocked with
   only their output.
3. The agent fixes the tests and ends its turn. The fix changed the content, so both run again, pass and write their
   `passed` lines; the turn ends.
4. A later `stop` on the same content runs nothing: both runs are skipped, logged as `skip` decisions.
   If the agent ends its turn again without changing anything after a failure, the failing run runs again, since
   only a run that passed is spared, until the loop guard lets it stop after the third block.

### Loop guard

Every block of `stop` or `subagent_stop` (a `block`, with or without `once`, or a failing `run`) writes an
observed `blocked` line and counts for the loop guard: after `max_stop_retries` blocks of the same entry, on the
same event, in the same session, by the same actor and on the same content hash, the agent may stop and the log
records a `loop_guard` decision. A block let through by the guard does not end the evaluation, so the next entries
run, and a subagent let through does not count as `finished`: when the workflow has an `inject` that acts once it
gets a `finished` line without hash, so its next run gets those injects again, and the condition never reads it.
Counting per entry keeps several pending blocks from using up the guard of each other, and subagents never use
up the guard of the main session.
When the log cannot be read, or the block cannot be written to it (a read only state folder), the guard cannot
count: it fails open, lets the agent stop with a warning and records a `loop_guard` decision when it can.

## State

`<git-dir>/polix/log.jsonl`, one JSON object per line, only written by polix:

- `decision`: what it answered (`block`, `deny` for a protection, `inform`, `warn`, `skip` or `loop_guard`), the
  action (the verb of the entry, `builtin` for the protection of the state, `guardrails.protected_files` or
  `guardrails.forbidden_commands` for those protections,
  `invalid` for an invalid workflow and `finished` for
  the end of a subagent it could not observe or a tree it could not compare), the `line` of the entry and the
  reason, the evidence of the process each change followed. It carries the content hash only when the call already
  computed it, since computing it costs a pass over the whole tree.
- `observed`: what polix saw by itself and the workflow asks about, with its outcome (`finished`, `passed`,
  `blocked`, `injected` or `started`) and its key; see
  [What polix observes](#what-polix-observes-and-finished). Nothing else is recorded: an edit and a command of the
  AI leave no observed line, nor does the end of a subagent no `finished` names when no `inject` acts once.

Lines of the kinds `fact`, `injected` and `stop_block`, written by earlier builds, stay in the log, are ignored by
every query and are shown by `polix log` with their kind (and the name of a `fact`).

The reason of an entry is capped at 8 KiB and each line kept from the output of a `run` or a check at 500 bytes,
both marked where cut, so no line of the log nears the read limit of 4 MiB.
A line over that limit (written by hand, say) is skipped when the log is read, and `polix log` says how many
lines it skipped; the entries around it still count.

The content hash is the git tree the working tree would produce with everything staged (untracked files included,
ignored ones excluded), computed on a copy of the index with `git write-tree`, also in a repository with no
commit nor index yet.
The copy keeps the time of the real index, so git still compares the content of the files changed in the same tick
as it; a hash costs about 25 ms on this repository, mostly the start of two git processes.
The tree stays in the object database of git, so polix later lists the files that differ between the tree of an
observed line and the current one (`git diff-tree`) and matches them against the scope of the entry with the globs
of polix: an edit outside the scope keeps a `finished` or a reminder, any edit inside it makes them ask again,
without anyone invalidating anything.
It is computed only when something needs it. Each `run` computes it before it runs, `always` included, and again
after it passes when the first was known, so `passed` is written only when the run did not change the content;
anything that needs it after a run computes it again, since a run may rewrite files. A `once` block and the loop
guard compute it, and `finished` only when a `finished` line of that type exists. An `inject` and a `started` line
never compute it.
When it cannot be computed (an unreadable file, a broken index), nothing that needs it is observed nor holds:
`finished` does not hold, a `run` is not skipped, a `once` block blocks, and the hook warns with the cause. The loop
guard still counts the blocks of a stop made without a hash, as one content, so the agent can always stop.

## The plugin and the hooks: polix install

`polix install` installs what the agent needs, from the binary, so every release brings its plugin and hooks with
it. Running it again rewrites what changed and removes what the release dropped. Without `--agent` it sets up
Claude Code alone, even when Codex is on the `PATH` or configured: Codex is set up only with `--agent codex`, which
also prints that Codex support is not implemented in this release and the setup is untested. It has two parts:

- The plugin `polix`, for the user, since it helps in any repository. It is the folder `plugins/polix/` of this
  repository, embedded in the binary (package `plugins`): skills (`workflow` writes `polix.yaml`, `add-rule` writes
  the folder of rules, `doctor` fixes the setup, `plan` writes a plan in the folder of plans, `review` reviews the
  current change with the agent `reviewer` and saves the report in the folder of reports, `audit` audits the repository with several `auditor` agents in
  parallel and saves the report in the folder of reports) and, for Claude Code, agents (`workflow-author`,
  `rules-author`, `builder`, which implements one task of a plan without committing, so several run at once on tasks
  that do not depend on each other, and the read-only `reviewer`, `rules-reviewer` and `auditor`). Install writes it,
  with the version of the binary in its manifests, to the local marketplace
  `~/.polix/marketplace/` and registers it. A development build, whose version is always `dev`, writes
  `0.0.0-dev+<hash of the plugin>` instead, so every change of the plugin is a new version.
  - Claude Code: `claude plugin marketplace add ~/.polix/marketplace --scope user` and
    `claude plugin install polix@polix --scope user`. Claude Code copies the plugin to its cache
    (`~/.claude/plugins/cache/polix/polix/<version>`), so after rewriting the folder install and update run
    `claude plugin update`, which copies it only when its version changed; it loads in the next session. Its
    skills are `/polix:workflow`, `/polix:add-rule`, `/polix:doctor`, `/polix:init`, `/polix:plan`,
    `/polix:review` and `/polix:audit`, its agents `polix:workflow-author`, `polix:rules-author`, `polix:builder`,
    `polix:reviewer`, `polix:rules-reviewer` and `polix:auditor`. The skills `review` and `audit` launch the
    agents, and the main session launches `builder`; Codex, which has no agents of plugins and whose support is not
    implemented, would follow the file of the agent in the session instead.
  - Conventions, which the tests of the package `plugins` check. Every skill and every agent runs only when the user
    or the workflow of polix asks for it, as its description says: the model does not reach for them by itself, and
    a message of polix such as "Run /polix:review" still runs the skill. The read-only agents (`reviewer`,
    `rules-reviewer`, `auditor`) fill the report template inside their own file. The skills `audit` and `review`
    save their report in the folder of reports from the `template.md` next to them, as `plan` saves its plan; a
    `polix:reviewer` launched without the skill answers in the chat and saves no file.
  - Codex, only with `--agent codex` (not implemented): `codex plugin marketplace add ~/.polix/marketplace` and
    `[plugins."polix@polix"] enabled = true` in `~/.codex/config.toml`, the rest of the file kept. Codex documents
    no command to install a plugin without its interface, and this part was never checked against a running
    Codex. Codex plugins carry no agents.

  When the CLI of an agent is not on the `PATH`, install writes the plugin and prints the commands to run by hand.
- The hooks, for the repository of the working directory (or `--dir`), since they belong to the project like its
  `polix.yaml` and are committed with it: merged into `.claude/settings.json`, and into `.codex/hooks.json` only with
  `--agent codex`. For Claude Code they cover `SessionStart`, `UserPromptSubmit`, `PreToolUse`, `PostToolUse`,
  `PostToolUseFailure` (a failed command reaches `post_command` with a non-zero `exit`; a failed edit runs no
  `post_edit`), `SubagentStop` and `Stop`; Codex gets the same events except `PostToolUseFailure`. Every
  other setting, event and hook of those files is kept in its order, and the hooks of polix take the place of the
  ones they replace, so the file only changes where polix changed. A group that mixes a hook of polix with other
  commands keeps the other commands, and the hook of polix goes to a group of its own. Each group of hooks of an
  event, of polix or not, is written on one line, and the rest of the file is indented with two spaces. A
  symlinked file is written through the link, to its target, and an existing file keeps its permissions. Codex skips new or changed hooks until the
  user trusts them with `/hooks`. The plugin brings no hooks.

`--user` installs only the plugin; outside a git repository that is all install does. `polix doctor` checks Codex
only when polix is registered for it, so a Codex user who never passed `--agent codex` gets no Codex finding.
`polix uninstall` removes the same parts, and without `--agent` those of both agents, so it also removes a Codex
setup: it unregisters the plugin from each agent and, for both, removes `~/.polix/marketplace/`; unless `--user`,
it removes the hooks of polix of the repository, and a hooks file when nothing else is left in it.

The plugin only arrives with `polix install`: the repository serves no marketplace of its own. The
reference of the skill `workflow` (`plugins/polix/skills/workflow/reference.yaml`) is a copy of
`examples/polix.yaml`, kept equal by a test; `claude plugin validate plugins/polix` checks the plugin.

Claude Code merges the hooks of the project, local and user settings, so polix registered in two of them runs
twice, which doubles every block and every observed line: `polix install` warns about it and `polix doctor` fails until
one of them goes. `examples/claude-settings.json` and `examples/codex-hooks.json` are, byte for byte, what install
writes in an empty repository for each agent (a test keeps them equal; the Codex one only with `--agent codex`);
they remain for a rollout that renders the settings itself.

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

## Known limits

- Codex is not implemented: an adapter (`polix hook codex`) and an opt-in install (`polix install --agent codex`)
  exist, but they were never tested against a running Codex. Its tool names (`shell`, `apply_patch`...), whether
  it has a subagent stop event and how its hooks set a timeout are still unknown.
- A `git commit` is recognised by a heuristic (global options and quoted commands included); the text of a
  commit inside quotes, as in `echo "git commit"`, also triggers `pre_commit`. The `commit` switch of `guidance`
  asks the agent to run each commit on its own, so it stays recognisable.
  A commit in another repository skips the entries of `pre_commit` only in the plain forms of the allowlist (see
  [How a hook call is answered](#how-a-hook-call-is-answered)); `cd <folder> && git commit`, `git -C ~/x commit`,
  `git -c user.name=x -C ../x commit` or a line with any other command still counts as a commit of this repository
  and meets its gates. The other repository is trusted as it is: its own hooks, or a setup prepared to write into
  this one by other means than linking its objects or refs, are not inspected, like the rest of the protections a
  guard against mistakes rather than against an agent set on getting around them.
- Every hook call reads the whole log, which grows without bound: one decision per answer, and one `passed` line
  per run of polix that passed.
- A subagent type launched twice in a session gets an `inject` that acts once again only because the first one
  finished; two of them running at once share it, so the second may not get it.
- A `run` whose result depends on more than the files (the network, the clock) is skipped after it passed once on
  a content: give it `always: true`.
- A scope too narrow lets a `finished` survive an edit it should have seen: `changed: "*.go"` ignores `go.mod`
  unless it is named too.
- Releases are not signed: `checksums.txt` protects against a corrupt or swapped download from a mirror, not
  against someone who can publish releases of the repository. Signing them (cosign, or GitHub artifact
  attestations) is pending.
- The actions of `.github/workflows/release.yml` are pinned by tag, not by commit SHA.
- The condition `pr` (state of the pull request) is not implemented.
- The log is appended without a lock; parallel subagents could in theory interleave lines.
- Injected rules are not bounded: a project with many rules without scope gets all of them on its first edit.
- No tests inside `polix.yaml` yet, and expiring entries with `review_by` is pending.
- `finished` only says that a subagent ended on the content, not what it concluded: requiring the verdict of the
  reviewer needs its output in the input of the hook.
- Measured on macOS: about 13 ms per hook call without a workflow and about 130 ms for a stop that computes the
  content hash of a 730 file repository.
  Not measured on Windows yet.
