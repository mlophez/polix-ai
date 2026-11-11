# Changelog

All notable changes to polix are recorded in this file.
The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and polix follows
[Semantic Versioning](https://semver.org/spec/v2.0.0.html).
A 0.x release may still change the format of `polix.yaml`; `requires:` lets a repository pin the oldest polix it needs.

## [Unreleased]

## [0.7.0] - 2026-11-12

### Added

- The section `githooks` of `polix.yaml` holds the gates git itself runs on every commit, whoever makes it (a person,
  an IDE or an agent), apart from the events of the AI: `pre_commit`, before the commit is made, and `commit_msg`,
  once its message is written. Its entries keep the shape of those of `on` with a vocabulary of their own: the verbs
  `block` and `run` (a command and a `message`, run on every commit with its output in the terminal: githooks keep
  no log), the conditions `branch`, `changed` (the files staged for the commit) and, in `commit_msg`, `message` (a
  regular expression, or a list, of the first line of the message, without the comment lines of git nor what
  follows the scissors line), and the placeholders `{branch}`, `{message}` and the variables of `vars`. So a team can
  demand a message format by branch, such as the key of the ticket, or by the files a commit holds, and run a command
  that blocks the commit. `polix config check` reports any other event, verb, key, condition or placeholder at the
  line of its entry, and `polix config fmt` writes `githooks` after `on`. `on.pre_commit` does not change and never
  evaluates `githooks`.
- `polix githook <pre-commit|commit-msg> [file]`, hidden, the command git runs: it exits with 1 to abort the commit,
  with each line of its reasons on stderr prefixed `polix: `. Without `polix.yaml`, without entries for the event or
  outside a repository it lets the commit go; a `polix.yaml` that cannot be used aborts every commit with its
  problems.
- `polix install` writes `hook.polix-pre-commit` and `hook.polix-commit-msg` (`polix githook pre-commit` and
  `polix githook commit-msg`) into the local git configuration of the repository, also without `githooks` in
  `polix.yaml`, and warns when git does not list them: git 2.54 or later runs the hooks of its configuration.
  `polix uninstall` removes them, and `polix doctor` warns, never fails, when git does not run them.
- `polix install --githooks` writes only those two hooks of git: no plugin, no hooks of the agents and no
  marketplace. It never asks for confirmation and prints only what changed, nothing when the hooks are already
  there; when git does not list them it keeps them, warns on stderr and exits with 1. Alone with `--agent`, or
  outside a repository, it is an error that writes nothing.
- A hook of git disabled with `hook.polix-<event>.enabled=false`, which git lists as disabled, is not taken for one
  git cannot run: `polix install` (also `--githooks`, which exits with 0) warns that it is disabled in the git
  configuration and leaves it so, and `polix doctor` says the same instead of asking to run `polix install`.
- The summary `polix install` and `polix uninstall` ask to confirm names the hooks of git they write or remove in
  the local git configuration of the repository.
- Declaring `githooks` is enough to get their hooks: a `polix.yaml` with at least one entry in `githooks` expands
  into one more entry first in `start`, from `githooks`, that runs `polix install --githooks` with `always: true`,
  so every session start of the AI puts the hooks of git back, and when git is too old to run them the message
  "The hooks of git of githooks are not installed, so the gates of githooks do not apply to the commits:" reaches
  the AI. `polix graph` draws it as `from githooks`, `polix config fmt` never writes it, and removing the entries of
  `githooks` turns it off.

### Changed

- The plugin spends its effort where a mistake multiplies or nothing else checks the work: `plan`, `reviewer`,
  `auditor` and `init` keep `high`; `builder`, `rules-reviewer`, `audit` and `risk` drop to `medium`, since tests,
  the checks of the rules or the measures of `polix risk` verify them; `build`, `review` and `doctor` drop to `low`,
  since they only launch agents, relay verdicts or follow `polix doctor`.
- `rules-reviewer`, `doctor`, `build`, `review` and `audit` run on `sonnet` instead of the model of the session,
  since they check a list or orchestrate; `plan`, `reviewer`, `auditor` and the rest still inherit it.
- `polix install` and `polix uninstall` take one flag per part, which combine, and every part without any:
  `--skills` (the plugin of the user, with its skills and agents), `--hooks` (the hooks of the agents of the
  repository, which could not be installed alone before) and `--githooks` (the hooks of git of the repository).
  `--skills` replaces `--user`: `polix uninstall --user` is gone, and `polix install --user` stays hidden as
  `--skills`, since `polix update` and the installers run it on the binary they install, which may be a release
  without `--skills`. `--hooks` or `--githooks` outside a git repository is an error; `polix uninstall
  --githooks` removes the hooks of git also when an agent stays.

## [0.6.1] - 2026-11-11

### Changed

- A failing `run` whose output is longer than the 15 lines its answer keeps says so in the line of the command, as
  `just test failed: exit status 1 (last 15 of 412 lines)`, so the AI knows it sees a part and can run the command
  to read the rest. The docs now name that line, `<command> failed: <error>`, in what a failure answers.

## [0.6.0] - 2026-11-11

### Changed

- `polix config fmt` writes the keys of the long form of `run` as `command` (or `rules`), `message` and `always`,
  so the command reads first and the flag last; `always` still goes first in `inject` and `notify`.
- `polix config fmt` separates two entries of an event of `on`, and two items of `forbidden_commands`,
  `format_on_edit` and `run_on_stop`, with one blank line, above the comments of the later one. Two items written on
  one line each, without comments above or below them (a comment at the end of the line is allowed), stay together,
  as a list of patterns does; the lists of texts and the lists
  inside an entry are not spaced, and the blank lines of the source are kept. A `polix.yaml` formatted with 0.5.0
  fails `polix config fmt --check` until it is formatted again.
- `polix rules fmt` keeps together two rules written in flow style on one line each, without comments above or
  below them, as the layout it shares with `polix config fmt` does; any other two rules keep one blank line between
  them.
- The skill `workflow` always keeps in `format_on_edit` the items that run `polix config fmt` on `polix.yaml` and
  `polix rules fmt` on the rules files, and adds them to a new `polix.yaml`; `examples/polix.yaml` carries them.

## [0.5.0] - 2026-11-11

### Added

- The skill `/polix:risk` calibrates `risk.weights` and the thresholds `changed_lines`, `changed_files` and
  `risk_score` of the entries of `polix.yaml` on the history of commits: it measures a sample with
  `polix risk --commit`, weighs the paths by their fix and revert commits, picks the thresholds that make each gate
  hold on the share of commits the user accepts to review (15% by default) and shows the commits that change side
  before writing. Only the user runs it, typing the command.
- `polix rules fmt [--check]` formats every `*.yaml` of the folder of rules (`rules.path`, `ai/rules` by default)
  in place, `retired.yaml` included, with the layout of `polix config fmt`: `rules` first, the keys of each rule in
  the order `id`, `description`, `scope`, `deterministic`, `check`, `reason` and then any other, the rules in their
  order, and exactly one blank line between two rules, above the comments of the later one. It is all or nothing:
  when a file cannot be formatted, an id is repeated (retired ones included) or a file does not decode as rules, so
  no repeat can be ruled out, it reports it and writes no file. With `--check` it writes nothing and exits with 1
  naming each file that is not formatted. A missing folder, or one without `*.yaml`, has nothing to format.
  A `polix.yaml` that cannot be read or parsed, requires a newer polix or has a `rules.path` that cannot be a folder
  stops it before any file is read.

### Changed

- The switches `edit` and `layout` of `guidance` are off by default: a `polix.yaml` that leaves them out no longer
  injects the instructions on edits nor the folders of the AI on `start`; `true` turns each one on. `commit` stays on
  by default.
- An id repeated among the rules is found by the loader of the rules, so `polix rules check` stops on it too, before
  running any check, with `<file> <id>: id repeated, also in <other file>`; an invalid rule of any file still wins.
  The id of a rule of `retired.yaml` counts as taken: an active rule with it is an id repeated, naming
  `retired.yaml`, in `polix rules check`, `polix config check` and `polix doctor`. Repeats among the retired rules
  are not reported, and a `retired.yaml` that is missing, cannot be read or is not YAML changes nothing. A repository
  that carries such an id today fails `polix rules check` after the update.

### Fixed

- The loop guard bounds the blocks of `stop` and `subagent_stop` after a `run` whose command writes files the tree
  does not ignore, as a coverage report or a build output, whether that `run` fails itself or a later entry blocks:
  every run left a new content hash, so the blocks never met on one and the agent was kept from stopping forever,
  above all with a `run_on_stop` item without `changed`, which runs on every stop. The `blocked` line of such a stop
  now carries `before`, the content hash the stop started from, and the guard chains the blocks through it while the
  agent changes nothing, letting the agent stop after `max_stop_retries` blocks; an edit of the agent breaks the
  chain. The warning of a stop let through names `max_stop_retries`.

## [0.4.0] - 2026-11-11

### Added

- The condition `agents_are_running: true|false` tells whether subagents of the session, of any type, are still
  running, the one that acts aside, on every event: a commit, an edit or the end of a turn can wait until every
  subagent has ended. polix writes to its log when each subagent starts and ends, with the `agent_id` of the
  subagent, so two of one type are told apart, and the `source` of a session start as the key of its `started`
  line. On each `stop` and `subagent_stop` of Claude Code that carries `background_tasks`, polix corrects the log
  with it: a subagent missing from the list, as an interrupted one, gets an `ended` line and no longer counts. A new
  session, a resume or a clear closes every subagent left open; a compaction does not. Codex sends no such list, so
  it counts by the log alone. `polix config check` reports a value that is not `true` or `false`.

### Changed

- The items of `guardrails.run_on_stop` wait for the subagents still running by default: while a subagent of the
  session runs, their checks are skipped, every stop that holds one back tells the user `run_on_stop waits for the
  subagents still running: its checks run on the next stop after they end.`, and they run on the next stop after
  the subagents end. `wait_agents: false` keeps an item running as before. A workflow with `run_on_stop` now writes
  the start and the end of every subagent to its log.

- The skill plan writes plans for one executor that runs every task in order, in one go: tasks have no
  `Depends on:` and may share files. A plan is written wholly in English, whatever the language of the session: it
  keeps the request of the user translated faithfully, never summarized, in `Request` and the limits no task may
  cross in `Constraints`, gives the executor a `Starting point` (files to read and a baseline command) and
  a `Progress` checklist, numbered acceptance criteria that each task `Covers:`, and each task a `High risk:` line.
  A header under the title holds the `Status` of the plan (`planned`, `building`, `built` or `partial`), its overall
  `Risk`, the `Reference` it comes from, the date it was `Created` and its estimated `Size`; an `Iterations`
  section at the end keeps the outcome of every build of the plan.
  Tasks are sliced by behaviour, about 400 changed lines as a guide, update the help and docs of what they change,
  and list their tests first, each `new` (fails before the change) or `kept` (holds the previous behaviour): every
  observable effect of each requirement, the behaviour of the code that calls what they change, and a differential
  check against the base version for a high-risk task. The planner makes no choice of its own: whatever the request,
  the docs, the code or a rule do not settle is asked, however minor, and an unanswered question is asked again;
  only a choice the user explicitly leaves to it is recorded, as `Delegated:` in `Decisions`. Before the interview,
  a brainstorm drafts two to four approaches, walks the change against the cases
  the request does not consider (inputs, failures, existing state, interactions, compatibility, operation,
  security), runs a pre-mortem and estimates the size; the interview proposes a split first when the plan is too
  big, then the approach, at most four questions per round. Adjusting a plan never rewrites a task already built:
  what changes it goes in a new task.
  A plan keeps its path from its first write: adjusting it, also within the same session or when its scope or title
  changes, edits the sections that change in place, never renames, copies or writes it whole again.
- The skills plan, review and audit name their files as the project names its plans and reports, by the convention
  of `AGENTS.md`, of `polix.yaml` or of the files already in the folder; their former names are only the fallback.
- The skill build launches one `polix:builder` for the whole plan instead of one per task in waves. It launches
  `polix:reviewer` itself, against the plan, instead of following the skill review, and has the builder fix the
  blocking and major findings up to twice, reviewing again after each fix. A plan is `built` only when every task is
  done, every acceptance criterion met (or a manual step left to the user) and the last review has no blocking nor
  major finding. A task left `[-]` is tried again by the next build. The reports of a run are kept next to the plan,
  `<plan>.iterNN.md` and `<plan>.revNN.md`; a run that did not end resumes from them at the first step not done, or
  is closed as `interrupted` when the plan was adjusted since, and at the end they are merged into the `Iterations`
  of the plan and deleted. Without subagents, as in Codex, the session plays the builder and the reviewer and says
  that the review is not independent.
- The agent `polix:builder` implements the plan from a task to the end without stopping nor asking: it runs the
  baseline, writes the tests before each change, records each departure from the plan, and marks each task of
  `Progress` `[x]` only when its tests and verification pass, `[-]` otherwise, removing or skipping the tests of a
  `[-]` task that still fail; it also fixes the findings of a review report, reading the reports of the earlier
  builders of the run first.
- The agent `polix:reviewer` has two modes: against a plan, from the skill build, where the decisions, the scope,
  the departures and the findings skipped in earlier runs are never findings again; and against an intent, from the
  skill review, which no longer takes a plan. It reports only what the lines the change adds or alters introduce,
  and keeps severities, categories and verdicts in English whatever the language of the report.

## [0.3.0] - 2026-11-11

### Added

- Every hook polix registers, in `.claude/settings.json` and `.codex/hooks.json`, the session start update included,
  carries `"statusMessage": "polix"`: both agents show `polix` next to their spinner while the hook runs, so the user
  knows who the wait is on. Run `polix install` again to add it to the hooks of an earlier release; every other hook
  is kept.
- The event `subagent_start`, a subagent that starts, with the subagent type as `actor`, from the `SubagentStart` of
  Claude Code and of Codex: its injects and the failures of its runs reach the subagent as context before its first
  tool use, so a subagent that only reads, such as a reviewer, gets the rules too. It takes `run`, `inject` and
  `notify`, never blocks, and every inject acts there every time, as on `start`; it gets neither the guidance nor the
  requirements of the main session. It makes the once-only injects due again for its subagent type only, while a
  `start` of the session still resets every actor. `polix install` registers `SubagentStart` for both agents;
  `polix doctor` warns when the settings of an earlier release lack it, and `polix install` adds it.
- The verb `notify`, a message for the user that never reaches the agent nor spends its context: `notify: <message>`,
  or `notify: { message: <message>, always: true }`, placeholders allowed, on every event. The agent shows it as
  `polix: <message>`, the `systemMessage` of the hook. It shows once per session for each entry and text it expands
  to, whatever the actor, and never again in that session, not even after a compaction or for a subagent;
  `always: true` shows it every time its entry applies. Each notice shown is a `notify` decision of the log, which
  `polix log` lists, and a notice that did not show because its hook call was blocked, by its own event or by
  another one of the call such as the commit of a command, stays due. `polix config fmt`
  writes `always` before `message`, and `polix workflow graph` draws it with its message.
- polix finds the files a command changed by its effect, not only by its command line, so a Python script, a `bash -c`
  or a `find -delete` no longer slips past the workflow. When `polix.yaml` has `guardrails.protected_files` or an entry
  on `pre_edit` or `post_edit` (`format_on_edit` included), a `pre_command` its entries let run (or, on a line that
  commits, its `pre_commit`) writes, after them, an observed `snapshot` line with the content hash, the HEAD commit,
  what git moves without HEAD and the `tool_use_id` of the call (the command line on Codex), and its `post_command`,
  also after a failed command, compares the content with it. A protected file the command changed is put back, or
  removed when it did not exist, each on its own, and the command is refused naming it, logged as a `deny` of
  `guardrails.protected_files`. Any other changed file is evaluated in the same call as a `pre_edit` and a `post_edit`:
  it is formatted and its rules reach the agent, while a `block` or a failing `run` there only informs, since the edit
  already happened; a file the command removed is not evaluated. A command that moves HEAD (a pull, a checkout, a
  commit), `refs/stash` (a `git stash`) or an operation stopped halfway (a merge, a cherry-pick, a revert, a rebase or
  `git am` stopped on its conflicts, a merge or a revert with `--no-commit`, a merge with `--squash`) is left alone,
  logged as a `skip` of `builtin`, and past 25 changed files the agent only gets the count. Each snapshot is compared
  once: the `post_command` writes an observed `watched` line with its key, so a later command of the same line never
  pairs with an old snapshot. It costs two content hashes per command, only in a repository whose workflow watches the
  edits; the check of the command line before it runs is unchanged, and files git ignores are still guarded only by it.

### Changed

- The shortcut `guardrails.run_before_done` is now `guardrails.run_on_stop`, named after the event `stop` its entries
  go to. A `polix.yaml` that still writes `run_before_done` is an invalid workflow: `polix config check` reports the
  unknown key and the hooks block edits and commands until it is renamed. Set `requires:` to this release in a file
  that uses `run_on_stop`, so an older polix asks to update instead of reporting the key as unknown.
- `requirements` also lists `git`, which polix itself runs: it is added at the end of a non-empty list that does
  not name it, so the OK line of `polix doctor` counts it. A missing git is still the `fail` of the area `git` of
  `polix doctor`, as the hook and the requirements look the programs up only once git runs.
- `polix config fmt` and `polix config check` take a workflow file as an argument, as `examples/polix.yaml`, to
  format or validate it in place of the `polix.yaml` of the repository: `--dir` names a repository, as `git -C`
  does, so it never reached such a file. `check` validates it with the rules and branches of the repository and
  names it in its problems; `fmt` needs no repository and refuses the file together with `--dir`.
- `polix config fmt` writes the flags `always` and `once` first in the long form of `run`, `inject` and `block`, and
  the message last; the unknown key problems of `polix config check` list the keys in the same order.
- `polix config fmt` writes the conditions `actor` and `finished` first in `when` and `unless`, before `branch` and
  `changed`, so who acts reads before what changed; the unknown condition problem of `polix config check` lists them
  first too.
- The verbs each event allows are one table, the same for Claude Code and Codex: `run`, `inject` and `notify` on
  `start`, `subagent_start`, `post_edit` and `post_command`; `run`, `inject`, `block` and `notify` on `prompt`,
  `pre_edit`, `pre_command` and `pre_commit`; `run`, `block` and `notify` on `stop` and `subagent_stop`. The problem
  of `polix config check` for any other verb names the event, the verb, why and the verbs the event allows.
- `polix config check` reports a notice a later gate could swallow: a `notify` followed in its event by an entry
  that may block it, a `block` or a `run` on an event other than `start`, `subagent_start`, `post_edit` and
  `post_command`, on every event, the two stops included. A blocked event shows the user no message, so the problem,
  at the line of the `notify`, names the line of that entry and asks to move the notice after it or into its
  message.
- `polix config check` reports an entry that never runs: one that follows, in the same event, a `block` without
  `when`, `unless` or `once`, as `never runs: the entry of line M always blocks the event`, at the line of the later
  entry. `stop` and `subagent_stop` are left out, since the loop guard lets the later entries run. A `polix.yaml`
  with such an entry is now an invalid workflow, and its hooks block edits and commands: move the entry before the
  block, give the block a `when`, `unless` or `once`, or remove the entry.
- The messages of polix for the user, such as a log that cannot be read, an unknown content hash or the loop guard
  letting the agent stop, are now the `systemMessage` of the JSON the hook writes on exit 0, each as
  `polix: <message>`, instead of lines on stderr that neither agent reliably showed. The hook writes each field of
  that JSON only when it has something and the native event accepts it, since Codex fails a hook on a field it does
  not accept. A blocked event shows them to nobody, as neither agent reads its output but the reasons: each one is
  logged as a `skip` decision of the action `message` that says it was not shown, also when another event of the
  same call, as the commit of a command, is the one that blocks. The warning of a workflow that requires a newer
  polix no longer repeats the prefix `polix:`.

## [0.2.0] - 2026-11-10

### Added

- The skill `/polix:build`, which implements a plan of the folder of plans with one `polix:builder` per task, several
  at once on tasks that do not depend on each other, updates the Markdown docs of the project (never `AGENTS.md`,
  `CLAUDE.md` nor a protected file), then reviews the whole change as `/polix:review` does and checks each acceptance
  criterion of the plan. It never commits nor fixes the findings.
- `requirements:` in `polix.yaml`, the programs a repository needs on the PATH, as `[go, golangci-lint]`. On the
  first prompt of a session the hook tells the AI which ones are missing and asks it to install them, or to ask the
  user to, before anything else; it tells it again after a start of the session or when the missing set changes.
  `polix doctor` warns once per missing program. Neither ever blocks. Names are written without extension: on
  Windows the lookup tries the extensions of `PATHEXT`. `polix config check` rejects an empty name, a name with
  spaces around it, a path and a repeated name.

### Fixed

- A review (`finished`), a `run` that passed and a `once` reminder are no longer asked again on a branch made from
  the commit they were recorded on, as `git checkout -b` after a review gives: they count on the current branch or on
  another one with the same HEAD commit, the files of their scope still compared. The observed lines `finished` with
  a hash, `passed` and `blocked` carry that commit as `head`, shown by `polix log --json`; older lines without it and
  repositories without commits still match by branch only.
- A commit in another repository, as `git -C ../other commit` run from a session in this one, no longer fires the
  entries of `pre_commit` of this repository; `protected_files` and `forbidden_commands` still apply to it.
  Only a line of plain `git [-C <folder>] add|commit` commands, with nothing the shell expands, whose commit folders
  are outside this repository's folder and inside another existing repository counts as such (on Windows, `-C`
  folders written with their drive); anything else, a linked worktree of this repository included, keeps the gates.

## [0.1.0] - 2026-11-08

The first public release: declarative guardrails for Claude Code, where one `polix.yaml` decides what the agent may
not touch, what must pass and what runs at every step.
Claude Code is the supported agent; Codex support is not implemented.

### Added

- The engine, `polix hook claude`, the entry point of the Claude Code hooks: it reads each hook call on stdin, blocks
  with the reasons for the agent, injects context and runs commands, in milliseconds per call.
- A built-in protection of the state of polix (`<git-dir>/polix/`), applied whatever the workflow says.
- `polix.yaml` `version: 1`, with `requires:` to pin the oldest polix a repository needs.
- `guardrails:`, the basics as five short lists: `protected_files`, `forbidden_commands`, `protected_branches`,
  `format_on_edit` and `run_before_done`.
- `on:`, the entries of each event (`start`, `prompt`, `pre_edit`, `post_edit`, `pre_command`, `post_command`,
  `pre_commit`, `subagent_stop` and `stop`), each with `when`, `unless` and one action.
- The conditions `branch`, `var`, `path`, `changed`, `file`, `contains`, `command`, `writes`, `exit`, `actor`,
  `finished`, `changed_lines`, `changed_files`, `risk_score`, `not` and `any`.
- The verbs `block` (with `once` for a reminder), `run` and `inject` (with `always`), the placeholders, `vars` taken
  from the branch name, the built-in `guidance` and a loop guard on stop (`max_stop_retries`).
- Rules in `ai/rules/*.yaml` (or `rules.path`): `inject: { rules }` injects only the rules whose scope matches the
  edited path, `run: { rules: changed }` runs the checks of the deterministic rules a change touches, and
  `polix rules check` (with `--changed` and `--base`) runs them in CI.
- `polix config check`, which validates `polix.yaml` and the rules, and `polix config fmt`, which formats
  `polix.yaml` in place keeping every comment (`--check` only reports).
- `polix graph`, which prints the workflow as a Mermaid diagram.
- `polix log`, which prints the last entries of the state log (`--tail`, `--json`).
- `polix risk`, which prints the lines, files and risk score of a change file by file (`--changed`, `--commit`),
  weighted by `risk.weights`.
- `polix doctor`, which checks that the machine and the repository are ready for polix (`--hook`, `--offline`).
- `polix install` and `polix uninstall`, which set up and remove the plugin of the user and the hooks of the
  repository in `.claude/settings.json`.
- Self update: `polix update` replaces the binary with a checked release and refreshes the plugin (`--check`,
  `--version`), and the session start hook runs it at most once an hour.
- Static binaries for Linux, macOS and Windows on amd64 and arm64, published with `checksums.txt`, and the
  installers `install.sh` and `install.ps1`, which check the download before installing it.
- The plugin `polix` for Claude Code, embedded in the binary: the skills `workflow`, `add-rule`, `doctor`, `init`,
  `plan`, `review` and `audit`, and the agents `workflow-author`, `rules-author`, `builder`, `reviewer`,
  `rules-reviewer` and `auditor`, each run only when the user or the workflow asks for it.

### Known limits

- Codex is not implemented: `polix hook codex` and `polix install --agent codex` exist, but were never tested
  against a running Codex, and `polix install` sets up only Claude Code unless `--agent codex` is passed.
- Releases are not signed: `checksums.txt` protects against a corrupt or swapped download, not against someone who
  can publish releases of the repository.
- The condition `pr`, on the state of the pull request, is not implemented.
- The rest of the known limits are in [docs/polix.md](docs/polix.md#known-limits).

[Unreleased]: https://github.com/mlophez/polix-ai/compare/v0.7.0...HEAD
[0.7.0]: https://github.com/mlophez/polix-ai/compare/v0.6.1...v0.7.0
[0.6.1]: https://github.com/mlophez/polix-ai/compare/v0.6.0...v0.6.1
[0.6.0]: https://github.com/mlophez/polix-ai/compare/v0.5.0...v0.6.0
[0.5.0]: https://github.com/mlophez/polix-ai/compare/v0.4.0...v0.5.0
[0.4.0]: https://github.com/mlophez/polix-ai/compare/v0.3.0...v0.4.0
[0.3.0]: https://github.com/mlophez/polix-ai/compare/v0.2.0...v0.3.0
[0.2.0]: https://github.com/mlophez/polix-ai/compare/v0.1.0...v0.2.0
[0.1.0]: https://github.com/mlophez/polix-ai/releases/tag/v0.1.0
