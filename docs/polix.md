# polix (proof of concept)

This repository holds a proof of concept of `polix`, the engine that connects Claude Code and Codex to the
workflow and the rules of a repository through their hooks.
It is a Go module of its own (`github.com/mlophez/polix-ai`), separate from the Python engine of Logalty Control, so it can be
shipped as one static binary per operating system and run on every hook call in milliseconds.

The design, its alternatives and the open decisions live outside this repository, in the artifact "Dispatcher de
workflows" (proposal 2).

## Layers

- `AGENTS.md`: what the project is (facts).
- `ai/rules/*.yaml`: how the code must be. Polix only reads `id`, `description`, `scope`, `deterministic` and
  `check` of each rule; the rest of the format belongs to the skills that write and use the rules.
  `retired.yaml` holds retired rules, which never apply.
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
polix check                # validate polix.yaml and ai/rules/
polix rules check          # run the checks of every deterministic rule (for CI)
polix rules check --changed  # only the rules whose scope matches the files changed by the branch
                             # (--base BRANCH, default the base of polix.yaml, else main)
polix graph                # print the workflow as a Mermaid diagram
polix log --tail 20        # print the last entries of the state log (--json for the raw lines)
polix doctor               # check that this machine and repository are ready (--hook, --offline)
polix install              # plugin for the user, hooks for the repository (--user, --agent)
polix uninstall            # remove what install wrote (--user, --agent)
polix update               # replace the binary with the latest release and refresh the plugin
                           # (--check, --version V, --hook)
polix version
```

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
`checksums.txt` and replaces the running binary atomically: a hook that starts meanwhile runs the old binary or
the new one, never half of it. A development build (`dev`) is only replaced with `--version`, and
`POLIX_NO_UPDATE=1` disables updates where the version is pinned by other means.
`--hook` is the mode of the session start hook: at most one check a day (cached in the user cache folder), five
seconds to reach GitHub, never fails, and prints one line only when it updated.
After the binary, `update` refreshes the plugin: when it replaced the binary, the new one does it, since the plugin
is embedded in it. A person who runs `polix update` also gets the plugin of the user installed when it is missing,
and the hooks of polix rewritten in the repository of the working directory when it registers them. The session
start hook only refreshes the plugin already installed: it never installs anything new nor touches the files of
the repository.

`polix doctor` checks the version against the latest release, that the `PATH` the hooks use runs this binary,
git, `polix.yaml`, the rules and that their checks exist and are executable, the hooks of Claude Code (project,
local and user settings: every event, every governed tool in the matchers, a timeout above the run budget, and
never twice), the plugin of the user (its version against the binary and, except in `--hook`, that Claude Code has
it installed and enabled), the hooks and the plugin of Codex when it is used and that the state can be written. It exits 1 on any failure.
`--hook` prints nothing when everything is fine and never fails; otherwise it prints the problems, which Claude
Code adds to the context of the session, so the agent tells the user and offers to fix them. It does not use
the network: it compares with the latest release the last `polix update` saw.

To publish a release, from a clean `main`: `just release 0.2.0`. It runs the tests, tags `v0.2.0` and pushes
the tag. `.github/workflows/release.yml` then runs the tests on Linux, macOS and Windows (`ci.yml`, which also
runs on every push and pull request), builds the assets with `scripts/dist.sh`, creates the GitHub release, and
finally installs it on the three systems with the documented commands (`install.sh`; on Windows `install.ps1`
under Windows PowerShell 5.1 and `install.sh` through Git Bash), checking the installed version.
The tests of `install.ps1` run with every PowerShell found, so on the Windows runners under 5.1 and 7.
The Justfile recipes use `sh`: on Windows, run them from Git Bash.

## How a hook call is answered

1. The adapter of the agent (`internal/agent`) turns the JSON of stdin into neutral events.
   An edit becomes one `pre_edit` or `post_edit` per path; a `git commit` in a command also produces
   `pre_commit`.
2. The engine (`internal/engine`) applies the built-in protections, then the entries of the event in order.
3. The adapter answers: exit 2 with the reasons on stderr to block, or exit 0 with
   `hookSpecificOutput.additionalContext` on stdout to inject context.

Edits outside the repository (scratchpad, memory, another repository) and of files git ignores are not part of
the change: the workflow does not apply to them, only the protections.

### Built-in protections

They apply whatever the workflow says, even without one:

- Nobody edits the state (`<git-dir>/polix/`), and no command that names it may write files (`rm`, `mv`,
  `cp`, `tee`, `sed -i`, redirections other than `2>&1`...); reading it is allowed, and `polix log` prints it.
  A command names the state by its absolute path or below a git directory (`.git/polix`,
  `.git/worktrees/<name>/polix`): the bare word `polix` is the binary, not the state.
- Subagents never edit `AGENTS.md`, `CLAUDE.md`, `ai/rules/` or `polix.yaml`, with the edit tools or with a
  command that names them and writes files.
  The main session may, because `init`, `add-rule` and `maintenance` write them with the user.

Commands are judged with a heuristic, aimed at accidents rather than at an agent determined to get around it:
`cat CLAUDE.md > notes.txt` is denied to a subagent, and `python -c "open(...)"` is not seen.

### Failure policy

- Outside a git repository, or without `polix.yaml`, only the protections apply.
- An invalid workflow blocks edits and commands (fail closed) with the problems found, so a broken file is
  noticed at once, except what fixing it needs: the main session may edit `polix.yaml` and run `polix check`
  (optionally after a `cd`), and edits outside the repository or of ignored files go through as always.
  The built-in protections still apply. It never blocks a stop, which would trap the agent in a loop.

## polix.yaml

```yaml
version: 1
base: main            # branch the change merges into, for `changed` (default main)
max_stop_blocks: 3    # loop guard of every block on stop and subagent_stop, per entry (default 3)
vars:
  key: { branch: "^([A-Z][A-Z0-9]+-[0-9]+)" }   # first group of the branch name, used as {key}
on:
  <event>:
    - if: { <condition>: <value>, ... }      # optional: all must hold
      unless: { <condition>: <value>, ... }  # optional: skip when all hold
      mode: enforce | warn | audit           # optional, enforce by default
      why: <reason of the entry>             # optional, for people
      <action>: <value>                      # exactly one
```

Events: `session_start`, `prompt`, `pre_edit`, `post_edit`, `pre_command`, `post_command`, `pre_commit`,
`subagent_stop` and `stop`.

Conditions:

- `branch`: the current branch matches a regular expression.
- `var`: a variable of `vars` has a value.
- `path`: the edited path matches a glob (or a list); only on `pre_edit` and `post_edit`.
- `changed`: a file changed by the branch matches a glob (or a list).
- `file`: a file of the repository matches a glob; with `contains`, it also holds that text.
- `command` and `exit`: a command of the line matches a pattern (`*` matches anything) and its exit code.
  `command` only on `pre_command`, `post_command` and `pre_commit`; `exit` only on `post_command`.
  The line is split into its commands (`cd x && just test 2>&1 | tail -3` runs `cd x`, `just test 2>&1` and
  `tail -3`, quotes respected), and each one is matched on its own.
  The exit code of the line only counts for a command whose failure propagates to it, alone, last or followed by
  `&&`: in `just test | tail`, `tail` decides it, so `exit: 0` does not hold and a fact like `tested` is not
  recorded from it.
- `actor`: who acts, `session` or the subagent type (`builder`, `reviewer`...).
- `done`: a fact was recorded on this branch for exactly the current content.
- `not`: a map of nested conditions; it holds unless every one of them holds.

Globs are relative to the root and use `/`: `*` stays inside a segment, `**` crosses segments, and a pattern
without `/` matches the base name anywhere (`*.md`).
Strings may use `{path}`, `{command}`, `{branch}` and the variables, which cannot take those three names.

Actions:

- `deny: <reason>` blocks the event and stops the evaluation; not valid on `session_start`, `post_edit` or
  `post_command`, which already happened.
- `allow: true` stops the evaluation without blocking (`false` is rejected, since it would stop it too).
- `require: <what to do>` blocks `stop` or `subagent_stop` until it is done; only valid there.
- `run: { command: [..] }` runs a command from the root, without a shell; a failure blocks with its output.
- `run: { rules: changed }` runs the checks of the deterministic rules whose scope matches the changed files
  (`rules: "{path}"` for one path); a failure blocks naming the rule ids.
- `context: { file | text | rules }` injects a file, a text, or the rules whose scope matches a path
  (`rules: "{path}"`), each rule once per session. Not valid on `stop` or `subagent_stop`, which cannot
  receive context.
- `record: <fact>` records a fact on the current content.
- `run: { ..., record: <fact> }` also records the fact when the run passes, in any mode, on the content the run
  saw: a failing run records nothing, and neither does one during which the content changed (a formatter that
  rewrites files, another agent editing). A gate on `stop` with `unless: { done: <fact> }` then runs until it
  passes, and once per content after that. A rules file that cannot be loaded fails `run: { rules }`.

Modes: `enforce` applies the entry; `warn` turns a block into a warning in the context; `audit` only logs what it
would have blocked, to try a new entry before enforcing it.

`polix check` rejects unknown events, conditions, actions and keys (inside `run`, `context` and `vars` too),
variables and placeholders that are not defined, and conditions on events where they never hold, so a typo
never turns into an entry that silently stops applying.

### Loop guard

Every block of `stop` or `subagent_stop` in enforce mode (`require`, `deny` or a failing `run`) counts for the
loop guard: after `max_stop_blocks` blocks of the same entry, in the same session, by the same actor and on the
same content, the agent may stop and the log records a `loop_guard` decision.
Counting per entry keeps several pending blocks from using up the guard of each other, and subagents never use
up the guard of the main session.
`examples/polix.yaml` is the current Logalty flow written as a workflow.

## State

`<git-dir>/polix/log.jsonl`, one JSON object per line, only written by polix:

- `fact`: what it observed (`record`), with the branch and the content hash.
- `decision`: what it answered (block, warn, audit, allow, loop guard), the `action` and `line` of the entry and
  why: the evidence of the process each change followed. It carries the content hash only when the call already
  computed it, since computing it costs a pass over the whole tree.
- `injected`: the rules already injected in a session.
- `stop_block`: the blocks of a stop, per entry, session, actor and content, for the loop guard.

The content hash is the git tree the working tree would produce with everything staged (untracked files included,
ignored ones excluded), computed on a copy of the index.
Any edit changes it, so a fact recorded before the edit stops counting without anyone invalidating it.

## The plugin and the hooks: polix install

`polix install` installs what each agent needs, from the binary, so every release brings its plugin and hooks with
it. Running it again rewrites what changed and removes what the release dropped. It has two parts:

- The plugin `polix`, for the user, since it helps in any repository. It is the folder `plugins/polix/` of this
  repository, embedded in the binary (package `plugins`): skills (`workflow` writes `polix.yaml`, `add-rule` writes
  `ai/rules/`, `doctor` fixes the setup) and, for Claude Code, agents (`workflow-author`, `rules-author` and the
  read-only `rules-reviewer`). Install writes it, with the version of the binary in its manifests, to the local
  marketplace `~/.polix/marketplace/` and registers it:
  - Claude Code: `claude plugin marketplace add ~/.polix/marketplace --scope user` and
    `claude plugin install polix@polix --scope user`. Claude Code loads a plugin of a local marketplace in place,
    so a rewrite of the folder applies in the next session; install and update then run `claude plugin update` to
    record the new version. Its skills are `/polix:workflow`, `/polix:add-rule` and `/polix:doctor`, its agents
    `polix:workflow-author`, `polix:rules-author` and `polix:rules-reviewer`.
  - Codex, when it is installed (or with `--agent codex`): `codex plugin marketplace add ~/.polix/marketplace` and
    `[plugins."polix@polix"] enabled = true` in `~/.codex/config.toml`, the rest of the file kept. Codex documents
    no command to install a plugin without its interface, so this part is experimental until it is checked against
    a running Codex. Codex plugins carry no agents.

  When the CLI of an agent is not on the `PATH`, install writes the plugin and prints the commands to run by hand.
- The hooks, for the repository of the working directory (or `--dir`), since they belong to the project like its
  `polix.yaml` and are committed with it: merged into `.claude/settings.json` and `.codex/hooks.json`. Every
  other setting, event and hook of those files is kept in its order, and the hooks of polix take the place of the
  ones they replace, so the file only changes where polix changed. Codex skips new or changed hooks until the
  user trusts them with `/hooks`. The plugin brings no hooks.

`--user` installs only the plugin; outside a git repository that is all install does. `polix uninstall` removes
the same parts: it unregisters the plugin from each agent and, for both, removes `~/.polix/marketplace/`; unless
`--user`, it removes the hooks of polix of the repository, and a hooks file when nothing else is left in it.

The repository is a marketplace too (`.claude-plugin/marketplace.json` and `.agents/plugins/marketplace.json`, named
`polix-ai`), for whoever wants the plugin without the binary: `/plugin marketplace add mlophez/polix-ai`. The
reference of the skill `workflow` (`plugins/polix/skills/workflow/reference.yaml`) is a copy of
`examples/polix.yaml`, kept equal by a test; `claude plugin validate plugins/polix` checks the plugin.

Claude Code merges the hooks of the project, local and user settings, so polix registered in two of them runs
twice, which doubles every block and every record: `polix install` warns about it and `polix doctor` fails until
one of them goes. `examples/claude-settings.json` is what install registers for Claude Code (a test keeps them
equal); it and `examples/codex-hooks.json` remain for a rollout that renders the settings itself (Logalty Control).

On `SessionStart` with matcher `startup` (a new session, not a resume, a clear or a compaction) the hooks
also run `polix update --hook; polix doctor --hook`: it keeps polix up to date and reports any problem of the
machine or the repository. Update and doctor run in one command because Claude Code runs the hooks of an event
in parallel, and doctor must see the binary the update left.

Every hook command is a plain `polix ...` call, joined with `;` at most, because Claude Code runs hook commands
with `sh -c` on macOS and Linux, with Git Bash on Windows, or with PowerShell on Windows when Git Bash is not
installed (Git for Windows is optional). That syntax means the same in all of them; `command -v`, `||` or
`{ ...; }` would break under PowerShell. For the same reason a hook cannot detect that polix is missing: until
it is installed, the hooks fail with "command not found" (or "is not recognized" in PowerShell), which Claude
Code treats as a non blocking error, and the workflow does not apply on that machine. The fragment of
`examples/AGENTS-polix.md` covers that case.

Every `run` of one hook call shares a budget of 10 minutes (commands and rule checks together), and the hooks
set the `timeout` of each `polix hook` to 900 seconds, above it. The default of Claude Code is 600
seconds (30 on `UserPromptSubmit`), not above the budget, so a long gate could be killed and the action let
through; `polix doctor` warns about it.
In the real rollout Logalty Control would render them and pin the version of the binary.

## Limits of the proof of concept

- Codex is experimental: its tool names (`shell`, `apply_patch`...), whether it has a subagent stop event and how
  its hooks set a timeout are still to be checked against a running Codex.
- A `git commit` is recognised by a heuristic (global options and quoted commands included); the text of a
  commit inside quotes, as in `echo "git commit"`, also triggers `pre_commit`.
- Every hook call reads the whole log, which grows without bound.
- Releases are not signed: `checksums.txt` protects against a corrupt or swapped download from a mirror, not
  against someone who can publish releases of the repository. Signing them (cosign, or GitHub artifact
  attestations) is pending.
- The actions of `.github/workflows/release.yml` are pinned by tag, not by commit SHA.
- The condition `pr` (state of the pull request) is not implemented.
- The log is appended without a lock; parallel subagents could in theory interleave lines.
- `PostToolUseFailure` of Claude Code is understood (exit code 1) but not registered in the example, until it is
  checked against the installed version.
- Injected rules are not bounded: a project with many rules without scope gets all of them on its first edit.
- No tests inside `polix.yaml` yet, nor `why` and `review_by` to expire entries.
- Measured on macOS: about 13 ms per hook call without a workflow and about 130 ms for a stop that computes the
  content hash of a 730 file repository.
  Not measured on Windows yet; the code builds for `windows/amd64` and runs rule checks through `python` or
  `bash` there, since a shebang does nothing.
