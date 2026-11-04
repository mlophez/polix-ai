# Fix the findings of the audit of the engine, the setup, the installers and the docs

## Overview

An audit of the whole repository found bugs that let the workflow pass or block without warning, gaps in the
setup and the installers, and docs that no longer match the code.
This plan fixes all of them in one change, except the workflow of this repository itself, which is being rewritten.

- The workflow stays reliable when something goes wrong: a broken or huge log no longer traps the agent on stop,
  a content hash that cannot be computed no longer makes facts valid forever, and a hung command no longer
  outlives its time budget.
- `changed` means what is not committed yet, in the hooks: the flow is reviewed commit by commit.
  In CI, `polix rules check --changed` compares with the branch the change forked from, chosen from a list, and
  fails instead of passing when that branch cannot be found.
- polix stops protecting `AGENTS.md`, `CLAUDE.md`, `ai/rules/` and `polix.yaml` on its own.
  Instead, `polix.yaml` gets a list of protected paths that no agent may write, and a condition `writes` for finer
  entries; a new recipe of the use cases shows both.
  Only the state of polix stays protected by the engine itself, with a sturdier detection.
- `polix install` keeps the symlinks and permissions of the files it edits, cleans up its own hooks from mixed
  groups, registers the failure event of Claude Code and writes each group of hooks on one line.
- The installers no longer flatten the Windows `Path`, leave state in the PowerShell session or run half a script.
- The repository stops serving the plugin as a marketplace: the plugin only arrives with `polix install`.
- CI runs the deterministic rules, the release recipe checks its branch, and the docs match the code again.

## Context

Two audits (engine, and plugin, docs and distribution) reported 28 findings: 4 high, 12 medium and 12 low.
One is already fixed (the agent `polix:auditor` is listed in `polix install --help` and `examples/AGENTS-polix.md`).
The outcome is a polix whose gates fail in a known, visible direction, whose setup does not damage user files,
and whose docs, examples and CI say and do the same thing as the code.

## Decisions

- One plan for every finding, as the user asked, instead of five plans by function.
- polix is in development: no version is bumped, no `requires:` is added and no upgrade notes are written.
- Log that cannot be read whole or written: records are truncated when written, over-long lines are skipped on
  read with a warning, and on stop and subagent_stop a log error lets the agent stop with a warning (fail open).
- Content hash that cannot be computed: no fact is recorded, `done` does not hold, and a warning says why.
- In the hooks, `changed` always compares the working tree with HEAD: uncommitted changes plus untracked files.
  The key `base` of `polix.yaml` no longer affects the hooks.
- `polix rules check --changed` resolves its base from `--base` or from `base`, which accepts one branch or a list.
  Each candidate is tried locally and then as `origin/<branch>`; among those that exist, the one whose merge base
  is closest to HEAD wins.
  Without any, the command fails.
  `polix check` and `polix doctor` warn when no candidate exists, declared or the default `main`, and the
  repository has a deterministic rule with a check.
- The protection of `AGENTS.md`, `CLAUDE.md`, `ai/rules/` and `polix.yaml` against subagents leaves the engine.
  The protection of the state of polix stays built in.
- New top level key `protected` in `polix.yaml`: a list of path globs that no agent writes, the main session
  included; only a person changes them by hand.
  It blocks edits and commands that write files and name a matching path, never reads.
  It applies before the workflow, so it also covers files that git ignores, such as `.env`.
  Protecting `polix.yaml` or `ai/rules/` this way keeps the skills `workflow` and `add-rule` from writing them, and
  the docs say so.
- New condition `writes` (a glob or a list), on the events that carry a command: it holds when the command writes
  files, by the same heuristic as the protections, and names a path that matches.
  It lets an entry restrict writes by actor, for example only to subagents.
- `polix install` registers `PostToolUseFailure` for Claude Code with the same matcher of tools as `PostToolUse`;
  the adapter turns a failed command into `post_command` with a non-zero `exit` and ignores failed edits.
  Codex keeps its current events.
- Every group of hooks under an event is written on one line, as `examples/codex-hooks.json` does today, while the
  rest of the settings file keeps an indentation of two spaces; groups of other tools get the same shape.
- The marketplaces of the repository are removed instead of versioning `plugin.json` on each release.
- `just check` installs with `go install` and runs that binary by its explicit path, so a warning of `doctor` about
  another polix on the PATH is a real one.
- The workflow of this repository (`polix.yaml`, `polix.beta.yaml`) is not touched: the user is rewriting it.
- The use case of `protected` and `writes` is a recipe of `docs/use-cases.md`, which appeared while this plan was
  written, instead of a page of its own.
- Assumption: `--base` takes one branch and overrides the list of `base`.
- Assumption: the stored reason of a log entry is capped at 8 KiB and each line kept from a command output at
  500 characters, which keeps every log line far below the read limit.
- Assumption: when the hash is unavailable on stop, the loop guard still counts the blocks recorded without hash,
  so the agent can always stop after `max_stop_blocks`.
- Assumption: a run that times out waits at most 5 seconds for its pipes to close (`WaitDelay`); killing the whole
  process tree is not attempted.
- Assumption: on Windows, `.sh` checks run with the `bash` of Git for Windows found next to `git`, and `.py` checks
  with the `py` launcher, then `python3`, then `python`, skipping the Microsoft Store aliases and the WSL `bash`.
- Assumption: a command names a path when one of its words, stripped of quotes and of redirection operators and
  made relative to the root, matches the glob; this is the same heuristic level as the current protections.

## Approach

State (`internal/state`).
`Store.Entries` reads the log line by line with a reader that discards lines over the 4 MiB limit and keeps
reading; it records how many it skipped, which callers can report, and fails only when the file cannot be read.
`Store.Append` caps the reason of the entry before encoding it.
`Store.Done` and `Store.Injected` keep their signatures and treat a read error as "not found".
`Store.StopBlocks` returns the count and the read error, so the loop guard can decide.

Engine (`internal/engine`).
`Engine.Hash` reports whether the hash is known; an unknown hash is never a comparable value.
`Engine.recordFact`, the `done` condition and the `run` with `record` skip facts when the hash is unknown and add a
warning to the result.
The loop guard of `Engine.apply` lets the agent stop, with a warning, when the log cannot be read or the stop block
cannot be appended.
`Engine.run` drops the cached hash after every run, sets `WaitDelay` on the command and truncates each output line.
`Engine.Changed` compares with HEAD.
`Engine.protect` keeps the protection of the state and adds the protected paths of the workflow, for every actor:
`protectedPaths` and `protectedRef` go away.
`stateRef` also ends at `;`, `&`, `|`, `)` and `*`, and the comparisons of paths with the state folder ignore case
on darwin and windows.
A pure function extracts the paths a command names, relative to the root; `protect` and the condition `writes` use
it together with `writesFiles`.

Git (`internal/gitx`).
`Repo.Changed` takes the reference to compare with instead of a base name and never falls back silently.
A new `Repo.ResolveBase` turns a list of candidate branches into the merge base of the closest one, or an error that
names the candidates tried.

Workflow (`internal/workflow`).
`Workflow.Base` accepts a string or a list of strings; `BaseBranch` becomes `BaseBranches`, which returns the list,
`main` when empty.
`Workflow.Protected` is a new list of path globs.
`Conditions` gains `writes`, accepted on the events that carry a command, as `command` is.
Validation rejects an empty name in `base`, an empty glob in `protected` and `writes` on other events.

Rules (`internal/rules`).
`Load` lists the folder with `os.ReadDir`, so the root path is never read as a glob.
`runCheck` resolves the interpreter on Windows with a pure function that takes the candidates found on the PATH and
returns the first acceptable one, or an error that names what is missing.
It sets `WaitDelay` and `tail` truncates long lines.

Agent (`internal/agent`).
The adapter of Claude Code returns no event for a `PostToolUseFailure` of an edit tool.

Setup (`internal/setup`).
`writeFile` resolves a symlink and writes its target, and keeps the mode of an existing file; new files get 0644.
`mergeHooks` removes the handlers of polix from a mixed group and keeps the group with the others.
`Hooks` adds `PostToolUseFailure` for Claude Code, and `hookEvents` lists it after `PostToolUse`.
The settings are encoded with every group of hooks on one line.

CLI (`internal/cli`).
`rulesCheck` resolves the base with `Repo.ResolveBase` and fails with the candidates tried.
`polix check` and `doctor` warn about a base that resolves to nothing, and `doctor` reports a missing interpreter
for the checks of the rules on Windows.
`printLog` prints the entries it read and warns about the lines it skipped.
`polix update --check --version X` compares X with the running version.

Installers, tooling and docs change as described in their tasks.
Messages for the agent stay in Spanish, as today; code, comments and docs stay in English.

### Discarded alternatives

- Five plans by function: the user asked for one.
- Comparing `changed` with the merge base in the hooks: the flow reviews commit by commit, so HEAD is enough.
- Treating every file as changed when the base is missing: a repository without `main` would trigger every gate.
- An exception in the engine for the author agents: a named hole in a protection the user wants opt-in anyway.
- Protecting the context only with entries of `pre_edit` and `command` globs: a glob also denies reads.
- `protected` with allowed actors per path: a heavier schema; entries with `writes` and `actor` cover it.
- Blocking reads of protected paths: needs the read tools in the matcher of the hooks, a hook call per read.
- Registering `PostToolUseFailure` only for Bash: a second matcher for little gain over ignoring failed edits.
- Versioning `plugin.json` on each release: a commit per release only to serve a marketplace nobody needs.
- Killing the process group of a run: needs code per system for little gain over `WaitDelay`.
- Running `build/polix` in `just check`: doctor would always warn that the PATH runs another polix.

## Acceptance criteria

- AC1: A log with a line over 4 MiB still loads the entries around it, and the loop guard lets the agent stop
  after `max_stop_blocks` blocks of the same entry.
- AC2: When the stop block cannot be written (state folder read only), a blocking stop lets the agent stop with a
  warning instead of blocking forever.
- AC3: A run that prints a line of several megabytes records a reason that fits the limits, and `polix log` prints it.
- AC4: When the content hash cannot be computed, no fact is recorded, `done` does not hold, and the hook warns.
- AC5: A run without `record` that changes files makes the later entries of the same call use the new hash.
- AC6: In the hooks, `changed` lists uncommitted and untracked files only, on any branch and with any `base`.
- AC7: `polix rules check --changed` uses the closest existing candidate of `base` or `--base`, local or on
  `origin`, and on a branch forked from `dev` it picks `dev` over `main` when both are listed.
- AC8: `polix rules check --changed` fails with the candidates tried when none exists, and `polix check` and
  `polix doctor` warn about it when the repository has a deterministic rule with a check.
- AC9: A run or a check that times out returns within a few seconds even when a grandchild keeps its output open.
- AC10: Without `protected`, any agent may edit `AGENTS.md`, `CLAUDE.md`, `ai/rules/` and `polix.yaml`: the engine
  no longer denies them to subagents.
- AC11: Commands that write the state with `;`, `&&`, `|`, `)` or `*` right after its path are denied, and on
  darwin and windows a path to the state in another case is denied.
- AC12: Rules load from a repository whose path holds `[`, `]`, `*` or `?`.
- AC13: On Windows, checks never run through the WSL `bash` or a Microsoft Store alias, and a missing interpreter is
  reported by the check and by `polix doctor`.
- AC14: `polix install` writes through a symlinked settings file to its target and keeps the mode of the file.
- AC15: `polix install` on a group that mixes polix with another command leaves one handler of polix per event, and
  `polix uninstall` leaves the other command.
- AC16: `polix install` registers `PostToolUseFailure` for Claude Code, a failed command reaches `post_command`
  with a non-zero `exit`, and every group of hooks is written on one line.
- AC17: `examples/claude-settings.json`, `examples/codex-hooks.json` and `.claude/settings.json` are byte for byte
  what `polix install` writes, checked by tests for both agents.
- AC18: `polix update --check --version X` says whether X is newer, the same or older than the running version.
- AC19: `install.ps1` keeps the type and the unexpanded entries of the user `Path`, leaves no variable or preference
  in the session that ran it, and names the file that failed to download.
- AC20: `install.sh` runs nothing until it is downloaded whole.
- AC21: The repository serves no marketplace, and docs and code comments no longer offer it.
- AC22: CI runs the checks of the deterministic rules on the three systems, and the gofmt check covers every Go file.
- AC23: `just release` refuses a branch other than `main` or a HEAD that is not `origin/main`, and `just check`
  checks the binary that `go install` just wrote.
- AC24: The reference, the skill `workflow` and `docs/polix.md` say that agents of a plugin are named
  `plugin:agent` in `actor`, and a test validates `examples/polix.yaml`.
- AC25: README, `docs/polix.md` and `AGENTS.md` match the commands, flags, skills and protections of the code.
- AC26: `staticcheck ./...` reports no unused code or unused assignment in `internal/cli` and `internal/setup`.
- AC27: A path that matches `protected` cannot be edited nor written by a command by any actor, the main session
  included, also when git ignores it, and it can still be read.
- AC28: The condition `writes` holds for a command that writes files and names a matching path, not for one that
  only reads it, and `polix check` rejects it on events without a command.
- AC29: A failed edit reported by `PostToolUseFailure` runs no entry of `post_edit`.

## Out of scope

- The workflow of this repository, `polix.yaml` and `polix.beta.yaml`: the user is rewriting it.
  This leaves for that rewrite the entry that runs the rules of the edited file, the text that names a subagent
  `reviewer` (now `polix:reviewer`), the list of files of the stop gate, which misses `plugins/`, `examples/` and
  `.github/`, and whether to use the new key `protected`.
- Removing the moment without `polix.exe` during a self-update on Windows: it needs a different replace strategy;
  this plan documents it.
- A lock for concurrent appends to the log and a bound on its size: known limits already in `docs/polix.md`.
- Blocking reads of protected paths.
- Versioning, `requires:` and upgrade notes, while polix is in development.
- Listing `polix:auditor` in `polix install --help` and `examples/AGENTS-polix.md`: already done.

## Tasks

### Task 1: The state log survives huge lines and write errors, and the loop guard fails open

- Covers: AC1, AC2, AC3.
- Files: `internal/state/state.go` (`Store.Entries`, `Store.Append`, `Store.StopBlocks`, modify),
  `internal/state/state_test.go` (create), `internal/engine/engine.go` (`Engine.apply`, `Engine.run`, `lastLines`,
  modify), `internal/rules/rules.go` (`tail`, modify), `internal/cli/cli.go` (`printLog`, modify),
  `docs/polix.md` (loop guard and state sections, modify).
- Rules: GO-1, GO-2, XPLAT-1, DOC-1.
- Reuse: `lastLines` from `internal/engine/engine.go` and `tail` from `internal/rules/rules.go`, extended to cap
  each line.
- Changes: `Store.Entries` reads with a buffered reader that discards any line over the current 4 MiB limit and
  keeps reading; the store keeps the number of skipped lines for its callers, and `Entries` returns an error only
  when the file exists and cannot be read.
  `Store.Append` caps the reason at 8 KiB, marking the cut, before encoding.
  `lastLines` and `tail` cap each kept line at 500 characters, marking the cut.
  `Store.StopBlocks` returns the count and the read error.
  In `Engine.apply`, on stop and subagent_stop, an error reading the count or appending the stop block logs a
  `loop_guard` decision when possible, adds a warning in Spanish that names the error, and lets the agent stop.
  `printLog` prints the entries it read and, when lines were skipped, says how many on stderr.
  `docs/polix.md` says that the loop guard also lets the agent stop when the log fails, and that reasons are capped.
- Tests: in `internal/state/state_test.go`, a log with a 5 MiB line between two valid lines returns both valid
  entries and one skipped line; `Append` of a 1 MiB reason stores at most 8 KiB; `StopBlocks` on an unreadable log
  returns the error (skipped on Windows and as root, where the mode does not apply).
  In `internal/cli/cli_test.go`, a stop gate with a 5 MiB line in the log blocks `max_stop_blocks` times and then
  lets stop; with the state folder read only (same skips) the stop is let through with a warning; a failing run that
  prints a 4 MB line records a capped reason and `polix log` prints it.
- Verification: `just test`, expected: all packages pass.

### Task 2: Facts need a known content hash, and the hash is refreshed after every run

- Covers: AC4, AC5.
- Files: `internal/engine/engine.go` (`Engine.Hash`, `Engine.recordFact`, `Engine.condition` for `done`,
  `Engine.apply` for `run`, modify), `internal/cli/cli_test.go` (modify), `docs/polix.md` (state section, modify).
- Rules: GO-1, GO-2, XPLAT-1, DOC-1.
- Reuse: `Repo.ContentHash` from `internal/gitx/git.go`.
- Changes: `Engine.Hash` returns the hash and whether it is known, keeping the error of `Repo.ContentHash` for the
  warning; the string `unknown` disappears.
  `Engine.recordFact` records nothing without a hash and adds a warning in Spanish with the cause.
  The `done` condition does not hold without a hash.
  A `run` with `record` records nothing when the hash is unknown before or after the run.
  The loop guard keeps working without a hash: the stop block is stored with an empty hash and counted against the
  other stop blocks without hash of the same entry.
  The cached hash is dropped after every `run`, with or without `record`, since a run may rewrite files.
  `docs/polix.md` says that facts are not recorded nor satisfied when the content hash cannot be computed.
- Tests: in `internal/cli/cli_test.go`, with an unreadable untracked file (mode 000, skipped on Windows and as root)
  a `post_command` entry with `record` records nothing and warns, and a stop gate with `unless: { done: ... }`
  blocks after an edit; a stop gate whose first entry runs a command that rewrites a file and whose second entry
  records a fact ties the fact to the content after the rewrite.
- Verification: `just test`, expected: all packages pass.

### Task 3: `changed` compares with HEAD in the hooks, and `rules check --changed` resolves its base

- Covers: AC6, AC7, AC8.
- Files: `internal/gitx/git.go` (`Repo.Changed`, `Repo.ResolveBase` create, modify), `internal/gitx/git_test.go`
  (modify), `internal/workflow/workflow.go` (`Workflow.Base`, `BaseBranch` renamed `BaseBranches`, `Validate`,
  modify), `internal/workflow/workflow_test.go` (modify), `internal/engine/engine.go` (`Engine.Changed`, modify),
  `internal/cli/cli.go` (`rulesCheck`, the `check` command, flag help of `--base`, modify),
  `internal/cli/doctor.go` (`doctorRun.workflow`, modify), `internal/cli/cli_test.go`,
  `internal/cli/doctor_test.go` (modify), `examples/polix.yaml`, `plugins/polix/skills/workflow/reference.yaml`,
  `docs/polix.md`, `README.md` (modify).
- Rules: GO-1, GO-2, XPLAT-1, DOC-1.
- Reuse: `Repo.git` and `Repo.paths` from `internal/gitx/git.go`; `rules.Load` to know whether a deterministic rule
  with a check exists.
- Changes: `Repo.Changed` takes the reference to diff against; an empty reference means HEAD.
  `Repo.ResolveBase` takes candidate branch names, tries each as a local branch and then as `origin/<name>`,
  computes the merge base with HEAD of those that exist, and returns the one with the fewest commits from it to HEAD;
  ties keep the order of the list.
  Without any candidate it returns an error that lists the names tried.
  `Workflow.Base` decodes a string or a list of strings; `BaseBranches` returns it, `main` when empty;
  `Validate` rejects an empty name.
  `Engine.Changed` diffs against HEAD and no longer reads the base.
  `rulesCheck` builds the candidates from `--base` or `BaseBranches`, resolves them, diffs against the result and
  exits 1 with the error when nothing resolves.
  `polix check` and `doctor` warn, without failing, when no candidate exists and the repository has a deterministic
  rule with a check.
  The reference, `docs/polix.md` and `README.md` explain that `changed` in the hooks is what is not committed yet,
  that `base` only serves `polix rules check --changed`, and that in GitHub Actions the base is
  `--base ${{ github.base_ref }}` with a checkout of depth 0.
  The copy of the reference in the plugin stays identical to `examples/polix.yaml`.
- Tests: in `internal/gitx/git_test.go`, `ResolveBase` with `main`, `dev` forked from `main` and a feature forked from
  `dev` picks `dev` for the list `main, dev`; it falls back to `origin/main` in a clone of one branch; it fails
  naming the candidates when none exists; `Changed` with an empty reference lists only uncommitted and untracked
  files after a commit.
  In `internal/workflow/workflow_test.go`, `base` as a string and as a list decode, and an empty name is rejected.
  In `internal/cli/cli_test.go`, a stop gate with `changed` no longer fires after the change is committed, whatever
  `base` says; `rules check --changed` in a repository without the base exits 1 and names it.
  In `internal/cli/doctor_test.go`, a repository with a deterministic rule and no `main` gets a warning, and one
  without deterministic rules gets none.
- Verification: `just test`, expected: all packages pass.

### Task 4: A run or a check that times out returns even with grandchildren holding its output

- Covers: AC9.
- Files: `internal/engine/engine.go` (`Engine.run`, modify), `internal/rules/rules.go` (`runCheck`, modify),
  `internal/rules/rules_test.go` (modify), `docs/polix.md` (run budget, modify).
- Rules: GO-1, GO-2, XPLAT-1, DOC-1.
- Reuse: none.
- Changes: both commands set a `WaitDelay` of 5 seconds, so when the deadline kills the direct child the pipes are
  closed and the call returns even if a grandchild still holds them.
  `docs/polix.md` says that a grandchild may outlive the budget although polix no longer waits for it.
- Tests: in `internal/rules/rules_test.go`, a check script that starts a background `sleep` longer than the deadline
  and waits for it returns within the deadline plus the delay (skipped on Windows, which has no `sh`).
- Verification: `just test`, expected: all packages pass.

### Task 5: The engine protects only the state, with a sturdier detection

- Covers: AC10, AC11.
- Files: `internal/engine/engine.go` (`Engine.protect`, `protectedPaths`, `protectedRef`, `stateRef`,
  `Engine.namesState`, `within`, modify), `internal/cli/cli_test.go` (`TestProtections`,
  `TestSubagentCommandsOnProtectedFiles`, `TestStateProtectionByPath`, modify), `docs/polix.md` (built-in
  protections, failure policy, modify), `README.md` (built-in protections, modify), `examples/polix.yaml`,
  `plugins/polix/skills/workflow/reference.yaml` (header, modify).
- Rules: GO-1, GO-2, XPLAT-1, DOC-1.
- Reuse: `writesFiles` from `internal/engine/engine.go`.
- Changes: `Engine.protect` drops the branch that denies subagents the context files, and `protectedPaths` and
  `protectedRef` are removed.
  `stateRef` also accepts `;`, `&`, `|`, `)` and `*` right after the name of the state folder.
  On darwin and windows, `within` and `namesState` compare paths without case.
  `docs/polix.md`, `README.md` and the header of the reference describe only the protection of the state as built in
  and point to `protected` (Task 6); the stale mention of the skills `init` and `maintenance` goes away with that
  paragraph.
- Tests: in `internal/cli/cli_test.go`, a subagent may edit `polix.yaml` and `ai/rules/x.yaml` without `protected`;
  commands that remove the state followed by `;true`, `&&true`, `)` or with a `*` are denied; on darwin and windows,
  an edit of the state with another case is denied.
  The tests that expected subagents to be denied the context files are removed or moved to Task 6.
- Verification: `just test`, expected: all packages pass.

### Task 6: `protected` and the condition `writes` let a workflow keep agents away from paths

- Covers: AC27, AC28.
- Files: `internal/workflow/workflow.go` (`Workflow.Protected`, `Conditions`, `Validate`, modify),
  `internal/workflow/workflow_test.go` (modify), `internal/engine/engine.go` (`Engine.protect`, `Engine.condition`,
  new path extractor, modify), `internal/engine/engine_test.go` (create), `internal/cli/cli_test.go` (modify),
  `docs/use-cases.md` (recipes of the Safety section, modify),
  `docs/polix.md` (schema, conditions, protections, modify), `README.md` (conditions, modify),
  `examples/polix.yaml`, `plugins/polix/skills/workflow/reference.yaml` (modify).
- Rules: GO-1, GO-2, XPLAT-1, DOC-1.
- Reuse: `writesFiles` from `internal/engine/engine.go`, `shell.Split` from `internal/shell`, `glob.AnyPath` from
  `internal/glob`, `Repo.Rel` from `internal/gitx`.
- Changes: `Workflow` gains `Protected`, a list of path globs with the syntax of the other path globs;
  `Validate` rejects an empty glob.
  `Conditions` gains `writes`, a glob or a list, valid on the events that carry a command (`pre_command`,
  `post_command`, `pre_commit`), as `command` is; `Validate` rejects it elsewhere.
  A pure function returns the paths a command names: for each segment of `shell.Split`, each word stripped of
  quotes and of the redirection operators that may prefix it, made relative to the root when it lies inside it.
  `writes` holds when `writesFiles` matches the segment and one of its paths matches a glob.
  `Engine.protect` denies, for every actor, a `pre_edit` whose path matches `Protected` and a `pre_command` or
  `pre_commit` for which the same test as `writes` holds against `Protected`, with a message in Spanish that names the
  path and says that only a person changes it.
  It runs before the workflow and before the skip of edits outside the change, so ignored files are covered.
  With an invalid workflow, `Protected` cannot be read and the current fail closed policy applies unchanged.
  A new recipe in the Safety section of `docs/use-cases.md` explains why a repository protects files from the
  agents.
  It shows `protected` for `.env` and for the context files, warns that protecting `polix.yaml` or `ai/rules/` keeps the
  skills `workflow` and `add-rule` from writing them, and shows entries with `writes` and `not: { actor: session }`
  that keep only subagents away, with an exception for `polix:workflow-author` and `polix:rules-author`.
  It states the limit: commands are judged by a heuristic, so `python -c` that opens a file is not seen.
  The reference documents `protected` and `writes`, and its copy in the plugin stays identical.
- Tests: in `internal/engine/engine_test.go`, the path extractor on `echo x >AGENTS.md`, `tee -a "ai/rules/x.yaml"`,
  `cp a.txt ./polix.yaml` and an absolute path inside the root returns the expected relative paths, and a path
  outside the root returns nothing.
  In `internal/cli/cli_test.go`, with `protected: [".env", "AGENTS.md"]` the session and a subagent are denied an
  edit of `.env` (ignored by git) and `echo x >AGENTS.md`, and allowed `cat AGENTS.md`; a `pre_command` entry with
  `writes: "AGENTS.md"` and `not: { actor: session }` denies a subagent and lets the session through.
  In `internal/workflow/workflow_test.go`, `writes` on `pre_edit` and an empty glob in `protected` are rejected.
  A test extracts the YAML blocks of `docs/use-cases.md` and validates each as a workflow, so the recipes cannot
  drift from the schema.
  The recipe "The agent cannot switch off its own guardrails" stops claiming that polix protects the context files
  without any entry, and points to the new recipe.
- Verification: `just test`, expected: all packages pass.

### Task 7: Rules load from any path, and checks on Windows run with the right interpreter

- Covers: AC12, AC13.
- Files: `internal/rules/rules.go` (`Load`, `runCheck`, new interpreter resolver, modify),
  `internal/rules/rules_test.go` (modify), `internal/cli/doctor.go` (`doctorRun.rules`, modify),
  `internal/cli/doctor_test.go` (modify), `docs/polix.md` (rules section, modify).
- Rules: GO-1, GO-2, XPLAT-1, DOC-1.
- Reuse: `exec.LookPath`; the existing switch on `runtime.GOOS` of `runCheck`.
- Changes: `Load` lists the rules folder with `os.ReadDir`, keeps the `.yaml` files except the retired one, and
  sorts them as today; a missing folder is no rules.
  A new pure function chooses the interpreter of a check from its extension and the candidate paths found on the
  PATH: for `.sh`, the `bash` of Git for Windows next to `git` (its `bin` folder), then a `bash` on the PATH that is
  not in the system folder of Windows; for `.py`, `py` with `-3`, then `python3`, then `python`, skipping paths under
  `WindowsApps`.
  It returns an error that names the interpreter missing, which becomes the output of the failing check.
  `runCheck` uses it on Windows only.
  `doctor` reports, on Windows, a missing interpreter for the extensions the checks of the rules use, by calling the
  same resolver.
  `docs/polix.md` says which interpreters run the checks on Windows.
- Tests: in `internal/rules/rules_test.go`, `Load` from a root named `proj[1]` returns its rules; the resolver,
  fed with fake candidate paths, picks the Git `bash` over `System32\bash.exe`, skips the Store alias of `python`,
  and errors when nothing fits; these run on the three systems.
  In `internal/cli/doctor_test.go`, a resolver that finds nothing makes doctor report the missing interpreter, with
  the lookup injected as the tests already inject `lookPath`.
- Verification: `just test`, expected: all packages pass.

### Task 8: `polix install` keeps user files, cleans mixed groups, registers failures and writes groups inline

- Covers: AC14, AC15, AC16, AC17, AC29.
- Files: `internal/setup/setup.go` (`writeFile`, `mergeHooks`, `isPolixGroup`, `Hooks`, `hookEvents`, `pretty`
  or a new encoder of settings, modify), `internal/setup/setup_test.go` (modify), `internal/agent/claude.go`
  (`Claude.Parse`, modify), `internal/agent/agent_test.go` (modify), `internal/cli/doctor.go`
  (`defaultHookTimeout`, `doctorRun.hooks`, modify), `internal/cli/doctor_test.go`, `internal/cli/install_test.go`,
  `internal/cli/cli_test.go` (modify), `examples/claude-settings.json`, `examples/codex-hooks.json`,
  `.claude/settings.json` (modify), `docs/polix.md` (setup section and limits, modify).
- Rules: GO-1, GO-2, XPLAT-1, XPLAT-2, DOC-1.
- Reuse: `IsPolixCommand`, `decodeObject`, `encodeObject` and `encodeArray` from `internal/setup/setup.go`.
- Changes: `writeFile` resolves the path with `filepath.EvalSymlinks` when it exists and writes the target, keeps the
  mode of the existing file, and uses 0644 for a new one; `setCodexPlugin` benefits through it.
  `mergeHooks` handles a group whose handlers mix polix with other commands by removing the handlers of polix and
  keeping the group with the rest, then adding the group of polix once; uninstall keeps the remaining handlers.
  `Hooks` registers `PostToolUseFailure` for Claude Code with the same matcher of tools as `PostToolUse`, and
  `hookEvents` lists it right after `PostToolUse`; Codex stays as it is.
  `Claude.Parse` returns no event for a `PostToolUseFailure` of an edit tool, and keeps turning a failed Bash into
  `post_command` with a non-zero exit.
  The settings are encoded with the indentation of today except that each element of the array of an event under
  `hooks` is written on one line, the shape of `examples/codex-hooks.json`; the output is stable, so running install
  twice changes nothing.
  `doctor` knows the new event and expects one handler of polix per event.
  `examples/claude-settings.json` and `examples/codex-hooks.json` are regenerated from `Hooks`, and
  `.claude/settings.json` is rewritten by running the new `polix install` in the repository.
  `docs/polix.md` drops the limit that says `PostToolUseFailure` is not registered and describes the inline groups.
- Tests: in `internal/setup/setup_test.go`, install through a symlinked settings file with mode 0600 updates the target,
  keeps the link and the mode (the symlink case skipped when links cannot be created); a mixed group ends with one
  handler of polix for the event and uninstall leaves the other handler; the output has one line per group and a
  second install reports no change; both example files equal byte for byte the settings install writes for each
  agent, replacing the current comparison of parsed JSON.
  In `internal/agent/agent_test.go`, a `PostToolUseFailure` of `Edit` yields no event and one of `Bash` without an
  exit code yields `post_command` with exit 1.
  In `internal/cli/cli_test.go`, a failed Bash matches an entry with `exit: 1`, and a failed edit runs no `post_edit`.
- Verification: `just test`, expected: all packages pass; `polix install` run twice in the repository, expected:
  the second run reports nothing to change.

### Task 9: `polix update --check` compares the requested version, and the docs state the Windows window

- Covers: AC18.
- Files: `internal/cli/update.go` (the `--check` branch, modify), `internal/cli/update_test.go` (modify),
  `docs/polix.md` (self-update section, modify).
- Rules: GO-1, GO-2, XPLAT-1, DOC-1.
- Reuse: `version.Compare` from `internal/version`.
- Changes: with `--check` and `--version X`, the message says that X is newer and can be installed, that the running
  binary is already X, or that X is older and would be a downgrade.
  `docs/polix.md` replaces the sentence that a hook always finds the old or the new binary with the truth on Windows:
  between the two renames there is a short moment without `polix.exe`, and a hook that starts then fails without
  blocking.
- Tests: in `internal/cli/update_test.go`, `--check --version` with a newer, an equal and an older version prints the
  three messages and downloads nothing.
- Verification: `just test`, expected: all packages pass.

### Task 10: The installers keep the Windows `Path`, leave no state behind and never run half a script

- Covers: AC19, AC20.
- Files: `install.ps1`, `install.sh` (modify), `internal/cli/update_test.go` (the test that runs `install.ps1`,
  modify).
- Rules: GO-1, GO-2, XPLAT-1.
- Reuse: the existing flow of both scripts, unchanged in what they install.
- Changes: `install.ps1` wraps its whole body in a script block invoked with `&`, so `irm | iex` leaves no variable,
  preference or protocol change in the session; errors are still thrown, never `exit`.
  Each download has its own error handling and names its own URL.
  The user `Path` is read from `HKCU\Environment` without expanding its variables and written back as an expandable
  string, and a change of environment is broadcast so new terminals see it.
  `install.sh` moves its body into a `main` function called on the last line with the arguments of the script.
- Tests: the test that runs `install.ps1` on Windows checks that a user `Path` entry with `%USERPROFILE%` keeps its
  variable and its registry type after the install, restoring the original `Path` at the end, and that `$version` is
  not defined in the session after running the script through `iex`; `sh -n install.sh` passes.
- Verification: `just test` on the CI matrix, expected: the Windows job passes with the new checks.

### Task 11: The repository no longer serves the plugin as a marketplace

- Covers: AC21.
- Files: `.claude-plugin/marketplace.json`, `.agents/plugins/marketplace.json` (delete), `plugins/plugins.go`
  (package comment, modify), `plugins/plugins_test.go` (the test over the marketplaces of the repository, modify),
  `AGENTS.md`, `docs/polix.md`, `README.md` (modify).
- Rules: GO-1, GO-2.
- Reuse: none.
- Changes: both marketplace files of the repository root are removed; the local marketplace that `polix install`
  writes under `~/.polix/marketplace/` stays as it is.
  The comment of the package `plugins`, `AGENTS.md`, `docs/polix.md` and `README.md` stop offering
  `/plugin marketplace add mlophez/polix-ai` and say that the plugin arrives with `polix install`.
  The test that read the marketplaces of the repository is removed.
- Tests: `plugins/plugins_test.go` passes without the files; `claude plugin validate plugins/polix` still passes.
- Verification: `just test`, expected: all packages pass; `claude plugin validate plugins/polix`, expected: passed.

### Task 12: CI runs the deterministic rules, the gofmt check covers every Go file, and the recipes check their ground

- Covers: AC22, AC23.
- Files: `.github/workflows/ci.yml`, `ai/checks/gofmt.sh`, `Justfile` (modify).
- Rules: XPLAT-1.
- Reuse: the recipe `install` of the `Justfile`.
- Changes: CI adds a step after the tests on the three systems that runs `go run ./cmd/polix rules check`.
  `ai/checks/gofmt.sh` checks the Go files that git tracks, wherever they are, instead of `cmd` and `internal`.
  `just check` keeps depending on `install` and runs the binary that `go install` wrote, by its path under `GOBIN`,
  or `GOPATH/bin` when `GOBIN` is empty, for `check`, `rules check` and `doctor --offline`.
  `just release` also fails when the current branch is not `main` or, after fetching, HEAD is not `origin/main`.
- Tests: none beyond running the recipes and CI.
- Verification: `just check`, expected: config ok, rules checks pass, and doctor warns about the PATH only when
  another polix comes first; CI on the three systems, expected: the rules step passes; `just release` on another
  branch, expected: it refuses before tagging.

### Task 13: Docs, reference and skill match the code

- Covers: AC24, AC25.
- Files: `examples/polix.yaml`, `plugins/polix/skills/workflow/reference.yaml`,
  `plugins/polix/skills/workflow/SKILL.md`, `docs/polix.md`, `README.md`, `internal/workflow/workflow_test.go`
  (modify).
- Rules: GO-1, GO-2, XPLAT-2.
- Reuse: `workflow.Parse` and `workflow.Validate` from `internal/workflow/workflow.go`.
- Changes: the description of `actor` in the reference and in `docs/polix.md` says that agents of a plugin are named
  with their prefix, as `polix:rules-reviewer`, with one example that replaces the bare `builder` of the reference;
  the skill `workflow` says the same where it names reviewer subagents.
  `README.md` lists `-C` for `polix hook` and `--check` and `--hook` for `polix update`, and its section on
  dogfooding says that the gofmt rule runs in CI.
  `docs/polix.md` moves the sentence on `examples/polix.yaml` out of the loop guard section to where the examples are
  described, and the limit on `why` and `review_by` says that `why` is a note and expiry with `review_by` is pending.
  A new test in `internal/workflow/workflow_test.go` parses `examples/polix.yaml` and expects no problem from
  `Validate`.
- Tests: the new test of the reference; the existing test that the reference of the plugin equals the example.
- Verification: `just test`, expected: all packages pass.

### Task 14: Remove dead code

- Covers: AC26.
- Files: `internal/cli/ui.go` (`styleTitle`, `ui.title`, delete), `internal/setup/setup.go` (`isFile`, delete),
  `internal/cli/cli.go` (the unused first value of `ad.Parse` in the hook command, modify).
- Rules: GO-1, GO-2.
- Reuse: none.
- Changes: the unused style, method and helper are removed; the hook command ignores the native value it never
  reads at that point.
- Tests: none; the existing tests cover the touched paths.
- Verification: `staticcheck ./...`, expected: no U1000 nor SA4006 in `internal/cli` and `internal/setup`.

## Risks

- Removing the built-in protection of the context changes the behaviour of every governed repository on update:
  the recipe of `docs/use-cases.md` explains how to restore it with `protected`, and the README points to it.
- `protected` also binds the main session: a repository that protects `polix.yaml` can no longer change it with the
  skill `workflow`; the recipe and the reference say so.
- The path extractor is a heuristic: a command that writes through an interpreter is not seen, and a command that
  writes one file and reads a protected one is denied; the docs state both, as they do today.
- `changed` in the hooks no longer sees committed work: a workflow that gated on it after commits now relies on its
  `pre_commit` entries, which the docs must make clear.
- A list in `base` is unknown to older binaries, without `requires:` while polix is in development: acceptable
  because every machine updates itself on session start.
- Rewriting settings inline changes the diff of every `.claude/settings.json` on the next install: the change is
  one-off and stable afterwards.
- Failing open on log errors lets an agent stop with a gate unmet: the warning and the `loop_guard` entry, when it can
  be written, keep it visible.
- Whether Claude Code sends a failed Bash as `PostToolUseFailure` is not verified yet: the end to end check below
  confirms it before the change is done.
- Registry writes in `install.ps1` are only tested in CI on Windows: the test checks the type and the content.

## Verification

- `just test`: vet and every test pass.
- `just check`: the configuration is ok, the deterministic rules pass and doctor reports no problem beyond a real
  PATH warning.
- `claude plugin validate plugins/polix`: passed.
- `staticcheck ./...`: no unused code nor unused assignment.
- CI on Linux, macOS and Windows: tests, the install of `install.ps1` and the rules step pass.
- End to end in Claude Code, in a scratch repository with the new hooks and a `post_command` entry with `exit: 1`
  that injects context: run a command that fails and check the context arrives; make an edit fail and check no
  `post_edit` entry ran in `polix log`.
- End to end in a scratch repository with a stop gate on `changed` and `done`: edit a file and try to stop (blocked),
  run the tests that record the fact, commit, and try to stop again (allowed); then append a 5 MiB line to the log and
  check the loop guard still lets the agent stop after `max_stop_blocks`.
- End to end with `protected: [".env"]` and `.env` in `.gitignore`: the session cannot edit `.env` nor write it with
  `echo x > .env`, and can read it.
- End to end with `polix install` on a symlinked `.claude/settings.json` that holds another hook in a polix group:
  the link and its mode survive, the other hook stays, and every group is on one line.
