# A guardrails section of polix.yaml for the common cases, expanded into entries of on, with friendlier names

## Overview

Today a team that adopts polix has to learn events, conditions and verbs before it can write its first guardrail.
The most common ones (files no agent touches, commands no agent runs, no commits on main, format each edited file,
lint and tests before the agent ends its turn) each take a whole entry under `on:`.
After this change each of them is a short list in a new section `guardrails:` at the top of `polix.yaml`, apart
from the flow of the team, which stays in `on:`:

- `protected_files`: paths no agent writes, the main session included (today `protected`).
- `forbidden_commands`: commands no agent runs, in any position of the line, each with an optional reason.
- `protected_branches`: branches nobody commits on.
- `format_on_edit`: the command that formats or lints each edited file, by glob.
- `run_before_done`: the commands that must pass before the agent ends its turn.

Each one is called a shortcut below, and each has exactly one form, so every file reads the same.
Protected files and forbidden commands are protections of the engine, as `protected` was: they deny before every
entry of `on:`, for every actor, the main session included, also on files git ignores, and without exceptions.
The other three are only sugar: polix expands each one into ordinary entries of `on:` when it reads the file, and
`polix graph` names the shortcut each entry comes from; the expansion is internal, with no command to print it.

As built, after Task 8: the entry key `guard` and `polix check --expand` of Tasks 2 and 5 no longer exist.
The sections below keep the decisions and tasks as they were taken, and the Decisions marked "Revised" and Task 8
say what replaced them.

Three top-level keys get clearer names: `protected` becomes `protected_files` inside `guardrails:`, `base` becomes
`base_branch` and `max_stop_blocks` becomes `max_stop_retries`.
The old names stop working: `polix check` rejects each one and tells its new spelling, and this repository and the
example move to the new names in the same change.

The README opens with a polix.yaml of about fifteen lines built from `guardrails:`, and the recipes of the docs
lead with the shortcut and show what it expands to.

## Context

The friendlier vocabulary of `on:` made each entry readable, but the first guardrails still need the whole
grammar, which slows the first minute of adoption that the launch depends on.
The outcome is a file whose common cases read as a list of intentions, without a second engine behind them.

## Decisions

- Shortcuts in this plan: `protected_files`, `forbidden_commands`, `protected_branches`, `format_on_edit` and
  `run_before_done`; a shortcut for the review before a commit is left out by choice of the user.
- The shortcuts live under one top-level key, `guardrails`, chosen by the user: it matches the positioning of polix
  as declarative guardrails, and keeps the common cases apart from the flow of `on:`.
  An unknown key inside `guardrails` is a problem that lists the five.
  The origin of an expanded entry is its path in the file, as `guardrails.protected_files`.
- `run_before_done` is the name chosen by the user for the checks of the end of the turn: it reads as "run this
  before the work is done".
- `protected_files` is a key of `guardrails`, not a top-level key; the old top-level `protected` is reported with
  the hint `use guardrails.protected_files`.
- Renames only of top-level keys: `protected` to `guardrails.protected_files`, `base` to `base_branch`,
  `max_stop_blocks` to `max_stop_retries`.
  Events, conditions and verbs keep their names, renamed one change ago.
- Hard break, by choice of the user, as in the previous vocabulary change: only the new names are accepted.
  An old name is an unknown key whose problem names its new spelling, through the hints of renamed names that
  `internal/workflow` already keeps, and it never suggests that a newer polix is needed.
- Every shortcut expands into entries of `on:` when the file is parsed; the domain `Workflow` holds only the
  expanded entries, so the hook, `polix graph`, the repeat rules and the loop guard see one model.
- Revised after Tasks 1-7, by choice of the user (Task 8): the entry key `guard` is dropped, since it adds a concept
  few people would write by hand.
  `protected_files` and `forbidden_commands` are protections of the engine, as `protected` was: applied before the
  entries of `on:` and before the skip of edits of ignored files, to every actor, without exceptions, and only with a
  valid `polix.yaml`.
  The expansion of the other three shortcuts into entries of `on:` stays internal, by choice of the user: the flag
  `--expand` of `polix check` and `Workflow.Render` are removed (Task 5 is undone), and `polix graph`, which names the
  shortcut of each entry, is the way to see what a file does.
  The bullets below on guards describe Tasks 2-7 as they were built, and Task 8 replaces them.
- The skip of edits of files git ignores stays: the entries of `on:` govern the change, what a commit may hold, so
  an ignored file (a build output, a local `.env`, a scratch file) never fires a ticket, plan, format or docs entry.
- Protected files and forbidden commands expand into guard entries, chosen by the user over keeping them as
  protections outside `on:`.
- A guard is an entry with `guard: true`, written by hand too:
  - valid only on `pre_edit`, `pre_command` and `pre_commit`, and only with the verb `block`;
  - evaluated after the built-in protection of the state of polix and before the skip of edits of ignored files
    and of the regular entries, so it covers `.env` when git ignores it, as `protected` does today;
  - `when` and `unless` work as in any entry; the expansions of the shortcuts have no `unless`, so no exception
    reaches them;
  - the first guard that blocks ends the evaluation, as any block does.
- The state of polix stays a built-in protection in the hook, not a guard: it applies even without `polix.yaml`
  and with an invalid one.
- Order of the expanded entries in each event: guards of `protected_files`, then of `forbidden_commands`, then the
  guards written in `on:`, then the regular entries of `protected_branches`, `format_on_edit` and
  `run_before_done`, then the regular entries written in `on:`, each group in file order.
- `protected_files` expands into two guards: one on `pre_edit` with the condition `path`, and one on `pre_command`
  with the condition `writes`.
  `pre_commit` is evaluated after the `pre_command` of the same line, so a block there already stops the commit.
- The condition `path` ignores case on macOS and Windows, as `protected` and `writes` already do, so the guard of
  `protected_files` keeps today's matching; the change applies to every entry that uses `path`.
- The messages of the expanded guards are fixed texts in English with placeholders:
  for an edit, that `{path}` is protected in polix.yaml and only a person changes it;
  for a command, that the line writes a path protected in polix.yaml, followed by `{command}`.
  The detail of which protected path a folder holds (`rm -rf ai` for `ai/rules/**`) is no longer named.
- Every shortcut has exactly one form, by choice of the user, so a file reads the same everywhere: a list of
  strings for `protected_files` and `protected_branches`, and a list of maps for `forbidden_commands`,
  `format_on_edit` and `run_before_done`.
  A single string where a list goes, or a string where a map goes, is a problem that shows the expected form.
  The entries of `on:` keep the short forms of their verbs and conditions.
- `protected_files` takes a list of path globs.
- `forbidden_commands` takes a list of maps with `pattern` (required) and `reason` (optional).
  The items without a reason share one guard with a default message that the line is forbidden by polix.yaml and a
  person must run it; each item with a reason gets its own guard whose message is the reason followed by the line.
- `protected_branches` takes a list of branch names; a name may hold `*`, which matches any text.
  It expands into one regular `pre_commit` entry with the condition `branch`, an anchored regular expression of the
  names, and a message that asks to create a branch for the change.
- `format_on_edit` has the shape of `run_before_done`, by choice of the user: a list of maps with `run`
  (required), `path` and `message`; each expands into one `post_edit` entry with `run`, and with the condition `path`
  when it is given.
  The filter is named `path` and not `changed`: it matches the edited file, while the condition `changed` holds for
  any uncommitted file and would format a file of another kind.
  The `run` of an item is a command as the verb `run` takes it, a string split on spaces or a list of arguments,
  placeholders allowed; without `path` it runs on every edited file.
- `run_before_done` takes a list of maps with `run` (required), `changed` and `message`; each expands into one
  `stop` entry with `run`, scoped by `changed` when it is given.
- Each expanded entry takes the line of the item it comes from, so messages, the keys of the repeat rules and the
  loop guard tell the entries apart, and carries the name of its shortcut as its origin.
- `polix check --expand` prints the expanded file: the same top-level keys with their new names, no shortcuts,
  every entry under `on:` with `guard: true` where it applies and a comment naming its shortcut.
  It does not translate old names: a file that still uses them fails with their hints first.
  It prints nothing else and fails with the problems when the file is not valid.
- `polix graph` marks the guards and names the shortcut each expanded entry comes from.
- This repository and `examples/polix.yaml` move to the new names and shortcuts in the same change, and the example
  raises `requires` to the next release, since an older polix rejects the new keys.

## Approach

The parse of `polix.yaml` in `internal/workflow` gains the top-level key `guardrails` with its five shortcuts, the
new names in place of the old ones and the entry key `guard`.
A new domain file of the package expands the shortcuts into `Entry` values, prepended to the events of `On` in the
order of `Decisions`, and reports the problems of their values (an empty glob, pattern, branch or command, an
unknown key of an item, a `reason` or `run` that is not a string).
`Workflow` loses `Protected` and renames `MaxStopBlocks` to `MaxStopRetries`.
The unknown top-level key `protected`, `base` or `max_stop_blocks` is reported with its new spelling.
`Entry` gains `Guard` and `From`, the path of the shortcut it was expanded from (`guardrails.format_on_edit`), empty
for an entry written in `on:`.
The validator rejects a guard on another event or with a verb other than `block`.

The hook in `internal/hook` keeps the protection of the state of polix in `protect`, drops the list of protected
paths from it, and evaluates the guards of the event right after it, with the same matcher and apply as any entry,
when the workflow is valid and present.
A guard never applies to an edit outside the repository; the skip of edits of ignored files and the regular entries
come after the guards, skipping the guards in their loop.

`polix check` gains the flag `--expand`; the domain gains a rendering of a `Workflow` as `polix.yaml`, and the use
case CheckSetup returns that text instead of the summary when the flag is set and the file is valid.

The docs, the example, the plugin reference and this repository's workflow move to the new names and lead with the
shortcuts.

Errors: every problem of a shortcut is reported by `polix check` with the line of its item, like any problem of an
entry, and fails closed in the hook like any invalid file.
A file that uses an old name fails to load with one problem naming the new spelling, as any unknown top-level key.

### Discarded alternatives

- Keeping protected files and forbidden commands as protections outside `on:`: two models in the engine and an
  expansion that could not show them.
- One grouped key `protect: { files, commands }`: one more level of nesting for two lists.
- Named presets (`use: [no-secrets, tests-before-stop]`): they hide what they do and need a catalogue to maintain.
- Renaming the events to `before_edit` and the like: they were renamed one change ago, and the cost of another
  migration buys little.
- Aliases for the old top-level names with a deprecation warning: the user chose the hard break, there is no
  launch yet, and two names per key would need a channel for warnings and a later removal.

## Acceptance criteria

- AC1: `protected_files` denies an edit and a command that writes a matching path for every actor, the main session
  included, also when git ignores the file, with the messages of `Decisions`; reading is allowed.
- AC2: `protected_files`, `base_branch` and `max_stop_retries` behave as `protected`, `base` and `max_stop_blocks`
  did.
- AC3: a file that uses `protected`, `base` or `max_stop_blocks` fails `polix check` with a problem that names the
  new spelling, and without the hint of a newer polix.
- AC4: `forbidden_commands` denies a line in which any command matches a pattern, wherever it appears in the line,
  for every actor, before every regular entry; with the default message for an item without `reason` and with its
  reason for an item with one.
- AC15: a shortcut written in another form than its own (a single string for `protected_files` or
  `protected_branches`, a string item in `forbidden_commands`, `format_on_edit` or `run_before_done`) fails
  `polix check` with a problem at its line that shows the expected form.
- AC16: no entry key `guard` exists: `polix check` rejects it as an unknown key of an entry; `protected_files` and
  `forbidden_commands` still deny for every actor, before every entry of `on:` and also on edits of ignored files;
  `polix graph` draws them apart from the events.
- AC17: `polix check` has no flag `--expand`, and nothing in the code, the docs, the example or the plugin mentions
  it.
- AC5: an entry with `guard: true` is valid only on `pre_edit`, `pre_command` and `pre_commit` and only with
  `block`; it runs before the regular entries and also on edits of files git ignores, and never on edits outside
  the repository.
- AC6: `protected_branches` denies a commit on a listed branch, also one matched through `*`, and lets a commit on
  any other branch through.
- AC7: `format_on_edit` runs the command of every item whose `path` matches the edited path, or that has no `path`,
  once per path and content, and a failure reaches the agent as context.
- AC8: `run_before_done` blocks the end of the turn while a check fails, runs a check only when its `changed`
  holds, and does not run again a check that passed on the same content.
- AC9: the expanded entries of each event come in the order of `Decisions`, before the entries written in `on:`.
- AC10: `polix check --expand` prints a file without shortcuts nor old names that passes `polix check` and parses
  into the same entries, conditions and actions as the original.
- AC11: `polix graph` marks every guard and names the shortcut of each expanded entry.
- AC12: the `path` condition ignores case on macOS and Windows.
- AC13: `polix.yaml` of this repository and `examples/polix.yaml` use the new names and the shortcuts, and
  `just check` passes.
- AC14: README.md opens its configuration with a polix.yaml of about fifteen lines built from `guardrails:`;
  docs/reference.md documents every shortcut, `guard` and the renamed keys; docs/use-cases.md leads the matching
  recipes with the shortcut and its expansion.

## Out of scope

- A shortcut for the review before a commit: not chosen by the user.
- Renaming events, conditions or verbs.
- A gallery of ready-to-copy workflows: part of the launch tasks.

## Tasks

### Task 1: New top-level names, with the old ones rejected with their new spelling

- Covers: AC2, AC3.
- Depends on: none.
- Files: `internal/workflow/workflow.go` (`Workflow`, `Problems`, `renamed`, `Renamed`, modify),
  `internal/workflow/workflow_parse.go` (`workflowYAML`, `topKeys`, `Parse`, `workflow`, modify),
  `internal/workflow/workflow_parse_test.go` (modify), `internal/workflow/workflow_test.go` (modify),
  `internal/workflow/workflow_base_test.go` (modify), `internal/hook/answer_hook_logic.go` (the loop guard, modify),
  `internal/setup/check_setup_logic_test.go` (modify), and only to move the old names to the new ones:
  `internal/hook/answer_hook_logic_test.go`, `internal/hook/answer_hook_in_test.go`,
  `internal/hook/answer_hook_out_test.go`, `internal/hook/answer_hook.md`, `internal/rule/check_rule_in.go`,
  `internal/rule/check_rule_out.go`, `internal/rule/check_rule_in_test.go`, `internal/rule/check_rule_out_test.go`,
  `internal/setup/setup_problems.go`, `internal/setup/setup_problems_test.go`, `internal/setup/check_setup_in_test.go`,
  `examples/polix.yaml`, `plugins/polix/skills/workflow/reference.yaml` (an exact copy of the example, as
  `TestWorkflowReference` checks),
  `docs/use-cases.md` and `polix.yaml` (modify).
- Rules: GOARCH-4, GOARCH-13, GOARCH-16, GOARCH-18, GOARCH-25, GOARCH-26, GOARCH-27, GOARCH-28, GOARCH-35,
  GOARCH-38, GOARCH-39, GOARCH-40, GO-2, GO-3, XPLAT-1, DOC-1.
- Reuse: the line tracking of the top-level keys in `Parse`; the map `renamed`, the regular expression
  `renamedProblem` and `Renamed` of `internal/workflow/workflow.go`.
- Changes: `workflowYAML` and `topKeys` replace `protected`, `base` and `max_stop_blocks` by `guardrails`,
  `base_branch` and `max_stop_retries`; in this task `guardrails` holds only `protected_files`, a list of path
  globs, and any other key inside it is a problem until Task 3 adds the rest.
  The unknown top-level key of an old name is reported by `Parse` with the hint of its new spelling, in the shape
  `renamedProblem` matches, and `renamed` gains a place for the top-level names so `Renamed` recognises them and
  `polix check` does not suggest a newer polix.
  `Workflow.MaxStopBlocks` becomes `MaxStopRetries`, and `DefaultMaxStopBlocks` becomes `DefaultMaxStopRetries`;
  every reader is updated.
  `Workflow.Protected` keeps its name in this task, read from `guardrails.protected_files`; Task 3 removes it.
  The problems that named `protected`, `base` or `max_stop_blocks` name the new keys.
  Every polix.yaml written in a test, the example, the blocks of docs/use-cases.md, this repository's `polix.yaml`
  and the texts for the user that name those keys move to the new names, with no other change: Tasks 6 and 7 bring
  the shortcuts to them.
- Tests: in `workflow_parse_test.go`, each new name parses to the value the old one gave; each old name fails with
  its line and its new spelling; `Renamed` holds for those problems.
  In `check_setup_logic_test.go`, a file with an old name gets no hint of a newer polix.
- Verification: `just test`, expected: pass.

### Task 2: Guard entries, evaluated before the regular entries and the skip of ignored files

- Covers: AC5, AC12.
- Depends on: Task 1.
- Files: `internal/workflow/workflow_entry.go` (`Entry`, `Entry.Valid`, modify), `internal/workflow/workflow.go`
  (`validator`, modify), `internal/workflow/workflow_parse.go` (`entryYAML`, `entry`, modify),
  `internal/hook/answer_hook_logic.go` (`handle`, `protect`, the condition `path`, modify),
  `internal/hook/answer_hook.md` (modify), `internal/workflow/workflow_entry_test.go` (modify),
  `internal/hook/answer_hook_logic_test.go` (modify).
- Rules: GOARCH-4, GOARCH-13, GOARCH-18, GOARCH-21, GOARCH-23, GOARCH-25, GOARCH-26, GOARCH-27, GOARCH-28,
  GOARCH-35, GOARCH-36, GOARCH-38, GOARCH-40, GO-2, GO-3, XPLAT-1, DOC-1.
- Reuse: `AnyPathFold` from `internal/hook/hook_protect.go`; `matches` and `apply` of `answerHookRun`.
- Changes: `Entry` gains `Guard`, read from the entry key `guard`, and `From`, the name of the shortcut it comes
  from, set only by the expansion of Task 3.
  The validator rejects a guard on an event other than `pre_edit`, `pre_command` and `pre_commit`, and a guard whose
  verb is not `block`, each with the line of the entry.
  In the hook, `handle` evaluates, after `protect` and before the check of an edit outside the change, the guards of
  the event in order with the same matching and apply as any entry, ending the evaluation at the first that blocks;
  a guard never applies when the edit is outside the repository.
  The loop over the regular entries skips the guards.
  `protect` keeps the protection of the state of polix and the list of `Workflow.Protected` until Task 3 replaces
  it by guards.
  The condition `path` matches with `AnyPathFold`, so it ignores case where the system does.
  `answer_hook.md` describes the guards in its Logic and Flow.
- Tests: in `workflow_entry_test.go`, a guard on `post_edit`, `stop` or with `run` or `inject` is a problem; a guard
  on the three events with `block` is valid.
  In `answer_hook_logic_test.go`, a guard blocks an edit of a file git ignores while a regular entry on the same
  path does not apply; a guard blocks before an earlier regular entry that would block with another message; a guard
  with an `unless` that holds lets the event through; a guard never applies to an edit outside the repository;
  `path` matches `.ENV` for `.env` only when the system folds case.
- Verification: `just test -run 'Entry|AnswerHook'`, expected: pass.

### Task 3: Expansion of the shortcuts into entries of on

- Covers: AC1, AC4, AC6, AC7, AC8, AC9, AC15.
- Depends on: Task 2.
- Files: `internal/workflow/workflow_shortcuts.go` (create), `internal/workflow/workflow_shortcuts_test.go` (create),
  `internal/workflow/workflow_parse.go` (`workflowYAML`, `topKeys`, `workflow`, modify),
  `internal/workflow/workflow.go` (`Workflow`, `Problems`, modify), `internal/hook/answer_hook_logic.go` (`protect`,
  modify), `internal/hook/hook_protect.go` (modify), `internal/hook/answer_hook_logic_test.go` (modify),
  `internal/hook/answer_hook_in_test.go` (modify), `internal/hook/answer_hook_out_test.go` (modify),
  `internal/hook/hook_evaluation_test.go` (modify), `internal/hook/answer_hook.md` (modify).
- Rules: GOARCH-1, GOARCH-4, GOARCH-9, GOARCH-13, GOARCH-16, GOARCH-18, GOARCH-21, GOARCH-25, GOARCH-26,
  GOARCH-27, GOARCH-28, GOARCH-29, GOARCH-35, GOARCH-38, GOARCH-40, GO-2, GO-3, XPLAT-1, DOC-1.
- Reuse: `Entry`, `Cond`, `Action`, `Block` and `Run` of `internal/workflow`; `argsYAML` for the commands of
  `format_on_edit` and `run_before_done`, which share one decoding of their items; `regexp.QuoteMeta` for the
  names of `protected_branches`.
- Changes: the decoding of `guardrails` adds `forbidden_commands`, `protected_branches`, `format_on_edit` and
  `run_before_done` to `protected_files`, rejects any other key inside it with the list of the five, and keeps the
  line of each item, each in its one form: the decoders check
  the kind of each node before decoding it, so a string where a list or a map goes becomes a problem that shows the
  expected form instead of an error of yaml.
  The new domain file expands them into entries as `Decisions` sets: the guards of `protected_files` on `pre_edit`
  with `path` and on `pre_command` with `writes`; the guards of `forbidden_commands`, one for the items without
  reason and one per item with a reason; one `pre_commit` entry for `protected_branches`; one `post_edit` entry
  per item of `format_on_edit`; one `stop` entry per item of `run_before_done`.
  Each entry carries the line of its item and the path of its shortcut in `From`, and the parse prepends them to the
  events of `On` in the order of `Decisions`.
  The problems of the values (empty glob, pattern, branch or command, unknown key of an item, missing `run`) are
  reported with the line of the item.
  The key `guard` joins `entryKeys` of `internal/workflow/workflow_entry.go`, so the problems of an unknown entry key
  and of an entry that is not a map list it; the exact texts expected in `internal/workflow/workflow_test.go`,
  `internal/workflow/workflow_parse_test.go` and `internal/hook/hook_evaluation_test.go` follow.
  `Workflow.Protected` is removed; `protect` in the hook keeps only the state of polix, and the helpers of
  `hook_protect.go` used only for the list of protected paths go, while those `Writes` needs stay.
  `answer_hook.md` says the protected paths are guards.
- Tests: in `workflow_shortcuts_test.go`, each shortcut expands into the entries, events, conditions, messages,
  lines and origin of `Decisions`; the order of the expanded and written entries of one event; each problem of a
  value; a single string for `protected_files` and `protected_branches` and a string item in each list of maps
  fail with the expected form.
  In the tests of the hook, the cases of `guardrails.protected_files` keep their results as guards, also for a
  folder or a wildcard that holds a protected path; a forbidden command chained after `cd x &&` is denied; a commit
  on a protected branch is denied and on another branch is not; a format command runs once per edited path; a check
  before stop blocks while it fails and is skipped when its `changed` does not hold.
- Verification: `just test`, expected: pass.

### Task 4: polix graph shows guards and the origin of each entry

- Covers: AC11.
- Depends on: Task 3.
- Files: `internal/workflow/graph_workflow_logic.go` (modify), `internal/workflow/graph_workflow_logic_test.go`
  (modify), `internal/workflow/graph_workflow_out_test.go` (modify), `internal/workflow/graph_workflow.md` (modify).
- Rules: GOARCH-4, GOARCH-14, GOARCH-18, GOARCH-21, GOARCH-23, GOARCH-34, GOARCH-35, GOARCH-36, GOARCH-38, GO-2,
  GO-3, DOC-1.
- Reuse: the current drawing of an entry in `GraphWorkflow`.
- Changes: the graph labels a guard as such and appends to an expanded entry the name of its shortcut, so the
  drawing of a file with shortcuts shows where each entry comes from; `graph_workflow.md` describes both.
- Tests: in `graph_workflow_logic_test.go`, a workflow with every shortcut and a hand-written guard draws the guard
  label and the origin of each expanded entry, and an entry written in `on:` has no origin.
- Verification: `just test -run GraphWorkflow`, expected: pass.

### Task 5: polix check --expand

- Covers: AC10.
- Depends on: Task 3.
- Files: `internal/workflow/workflow_render.go` (create), `internal/workflow/workflow_render_test.go` (create),
  `internal/setup/check_setup_in.go` (`CheckSetupRequest`, `CheckSetupResponse`, `CheckSetupPort`,
  `CheckSetupCommand`, `checkSetup`, modify), `internal/setup/check_setup_logic.go` (`CheckSetup`,
  `CheckSetupOutput`, modify), `internal/setup/check_setup_out.go` (modify), `internal/setup/check_setup.md`
  (modify), `internal/setup/check_setup_logic_test.go` (modify), `internal/setup/check_setup_in_test.go` (modify).
- Rules: GOARCH-4, GOARCH-11, GOARCH-14, GOARCH-15, GOARCH-16, GOARCH-18, GOARCH-20, GOARCH-21, GOARCH-22,
  GOARCH-23, GOARCH-24, GOARCH-31, GOARCH-32, GOARCH-33, GOARCH-34, GOARCH-35, GOARCH-36, GOARCH-37, GOARCH-38,
  GOARCH-40, GO-2, GO-3, DOC-1.
- Reuse: `yaml.v3` nodes as `workflow_parse.go` uses them.
- Changes: the domain gains a method of `Workflow` that renders it as `polix.yaml`: the top-level keys with their new
  names and their values, no shortcuts, and every event of `On` with its entries in order, each with `guard: true`
  when it is a guard and a head comment naming its shortcut when it has one.
  `CheckSetupRequest` gains `Expand`, set by the flag `--expand`; the port gives the loaded workflow to the logic;
  with a valid file and `Expand`, the output and the response carry the rendered text and the command prints only
  it to stdout; with problems it prints them and fails as today.
  `check_setup.md` names `Expand` and `Expanded` in its sections.
- Tests: in `workflow_render_test.go`, the rendering of a workflow with every shortcut and hand-written entries,
  guards among them, parses back into a workflow with the same entries, conditions, actions, guards and top-level
  values, and has no shortcut.
  In the tests of CheckSetup, `Expand` with a valid file returns the text and no summary; with an invalid file
  returns the problems and no text.
- Verification: `just test -run 'Render|CheckSetup'`, expected: pass.

### Task 6: This repository, the example and the plugin use the new names and the shortcuts

- Covers: AC13.
- Depends on: Task 5.
- Files: `polix.yaml` (modify), `examples/polix.yaml` (modify), `plugins/polix/skills/workflow/reference.yaml`
  (modify), `plugins/polix/skills/workflow/SKILL.md` (modify, when it names the old keys or the entries a shortcut
  replaces).
- Rules: GO-2, DOC-1.
- Reuse: none.
- Changes: `polix.yaml` replaces `protected`, `base` and `max_stop_blocks` by their new names, moves the format of
  the edited Go file to `format_on_edit` and the lint and the tests of `stop` to `run_before_done` with their
  `changed`, and keeps the two entries of `pre_commit` in `on:`; its comments explain the shortcuts.
  `examples/polix.yaml` documents every shortcut and every key of its items, the entry key `guard` and the new
  names, and raises `requires` to the next release.
  The plugin reference and skill teach the shortcuts first and the entries of `on:` for what they do not cover.
- Tests: none beyond the checks below.
- Verification: `just check`, expected: pass; `polix check --expand` on this repository,
  expected: the same entries as before the change for `post_edit`, `pre_commit` and `stop`;
  `claude plugin validate plugins/polix`, expected: valid.

### Task 7: Docs and README lead with the shortcuts

- Covers: AC14.
- Depends on: Task 5.
- Files: `README.md` (modify), `docs/reference.md` (modify), `docs/use-cases.md` (modify), `docs/polix.md` (modify).
- Rules: GO-2, DOC-1.
- Reuse: none.
- Changes: README.md gains, before the details of `on:`, a polix.yaml of about fifteen lines with a `guardrails:`
  section that uses the five shortcuts on a typical project, and one sentence that they expand into `on:` and that
  `polix check --expand` shows it.
  README.md, docs/polix.md and docs/use-cases.md present polix as two layers: `guardrails:`, the basics every
  project wants from the first minute, and `on:`, where the team orchestrates its flow (the review subagents, the
  builder, the plan, the ticket); the recipes are grouped by layer, those of `guardrails:` first.
  docs/reference.md adds a section Guardrails after the top-level keys, with the type, the forms, the expansion and
  the order of each shortcut; renames `protected`, `base` and `max_stop_blocks` everywhere; documents
  `guard` in Entries and in How an event is evaluated; updates the file at a glance; and says `path` ignores case on
  macOS and Windows.
  docs/use-cases.md leads the recipes Destructive commands are left to a person, Paths no agent writes, Secrets
  never land in a commit, No commits on main, Format every file the agent edits, Lint feedback right after each
  edit and Tests that run themselves with the shortcut, followed by its expansion for those who need more.
  docs/polix.md describes guards in its protections and the evaluation of an event, and the command
  `polix check --expand`.
- Tests: none.
- Verification: `rg -n -w 'protected:|max_stop_blocks|base:' README.md docs`, expected: no match.

### Task 8: Protected files and forbidden commands are protections of the engine, without the key guard

- Covers: AC16, AC17.
- Depends on: Task 7.
- Also files: `internal/setup/check_setup_in.go`, `internal/setup/check_setup_logic.go`, `internal/setup/check_setup.md`
  and their tests (modify), `internal/workflow/workflow_render.go` and `internal/workflow/workflow_render_test.go`
  (delete).
- Files: `internal/workflow/workflow.go` (`Workflow`, modify), `internal/workflow/workflow_entry.go` (`Entry`,
  `entryKeys`, `keyGuard`, `guardEvents`, `validator.guard`, modify), `internal/workflow/workflow_parse.go` (modify),
  `internal/workflow/workflow_shortcuts.go` (modify), `internal/workflow/workflow_render.go` (modify),
  `internal/workflow/graph_workflow_logic.go` (modify), `internal/workflow/graph_workflow.md` (modify),
  `internal/hook/answer_hook_logic.go` (`handle`, `entries`, `protect`, modify), `internal/hook/hook_protect.go`
  (modify), `internal/hook/answer_hook.md` (modify), the tests of those files (modify), `examples/polix.yaml`,
  `plugins/polix/skills/workflow/reference.yaml`, `plugins/polix/skills/workflow/SKILL.md`, `README.md`,
  `docs/reference.md`, `docs/polix.md`, `docs/use-cases.md` (modify).
- Rules: GOARCH-4, GOARCH-13, GOARCH-14, GOARCH-18, GOARCH-21, GOARCH-23, GOARCH-25, GOARCH-26, GOARCH-27,
  GOARCH-28, GOARCH-29, GOARCH-34, GOARCH-35, GOARCH-36, GOARCH-38, GOARCH-40, GO-2, GO-3, XPLAT-1, DOC-1.
- Reuse: `Writes`, `AnyPathFold` and the helpers of `internal/hook/hook_protect.go`; the matching of command patterns
  of the condition `command`; the decoding and form checks of `workflow_shortcuts.go`.
- Changes: `Entry` loses `Guard`, and `guard` leaves `entryKeys` and the validator, so it is an unknown key.
  `Workflow` gains `ProtectedFiles`, the globs of `guardrails.protected_files`, and `ForbiddenCommands`, a list of a
  new domain type with `Pattern`, `Reason` and the line of its item, with `Valid`; the expansion produces entries
  only for `protected_branches`, `format_on_edit` and `run_before_done`.
  In the hook, `handle` has one pass over the entries again, and `protect`, after the state of polix and only with a
  valid and present workflow, denies an edit of a protected path (not outside the repository, also when git ignores
  it), a command line that writes one (with the message that names the protected path a folder or a wildcard holds,
  as before Task 3), and a command line in which any command matches a forbidden pattern, with its reason or the
  default message, followed by the line; each denial is logged with the shortcut it comes from.
  `internal/workflow/workflow_render.go` and its test are deleted; `CheckSetupRequest.Expand`,
  `CheckSetupResponse.Expanded`, `CheckSetupOutput.Expanded`, the parameter of `CheckSetup` and the flag `--expand`
  go, with their tests and the sections of `internal/setup/check_setup.md` that name them.
  `polix graph` draws the two lists in a subgraph `guardrails` before the events.
  The docs, the example, its copy in the plugin and the skill drop `guard`, present `protected_files` and
  `forbidden_commands` as protections applied before `on:` and also to ignored files, and explain why the entries
  of `on:` skip ignored files.
- Tests: an entry with `guard` fails as an unknown key; the hook denies an edit of a protected path that git ignores
  and a forbidden command after `cd x &&`, for the main session and a subagent, before an earlier entry of `on:` that
  would block; a folder that holds a protected path is denied with the message that names it; the graph draws the
  subgraph; `polix check --expand` fails as an unknown flag.
- Verification: `just test`, `golangci-lint run ./...`, `claude plugin validate plugins/polix`, expected: pass;
  `rg -n 'guard: true|guard:' README.md docs examples plugins internal`, expected: no match of the entry key.

## Risks

- A guard that runs before the skip of ignored files makes a hand-written guard on a broad glob block edits that
  polix ignored before: the docs say a guard covers ignored files, and the shortcuts are the recommended way.
- The `path` condition ignoring case on macOS and Windows can make an existing entry match more paths there: it is
  the behaviour `protected` and `writes` already had, and it is listed in the docs.
- Removing `Workflow.Protected` touches many tests of the hook: Task 3 moves each case to `protected_files` and keeps
  its expected result, so a change of behaviour shows as a failing case.
- An older polix rejects the new keys as unknown: the example raises `requires`, and the docs ask repositories that
  use the shortcuts to set it.
- The rendering of `--expand` drifts from the parse: the round trip test of Task 5 parses the output back.
- The messages of a protected path no longer name the protected path a folder holds: the line of the command is
  in the message, and the docs show it.

## Verification

- `just test`: every test passes.
- `golangci-lint run ./...`: no findings.
- `just check`: `polix check`, `polix rules check` and `polix doctor --offline` pass on this repository.
- `claude plugin validate plugins/polix`: valid.
- End to end, in a scratch repository with a polix.yaml that uses the five shortcuts and the hooks installed:
  1. Ask the agent to edit `.env`, ignored by git: denied with the message of `protected_files`.
  2. Ask it to run `cd tmp && rm -rf /`, with `rm -rf /*` forbidden: denied, with the line in the message.
  3. Commit on `main`: denied; on a new branch: allowed.
  4. Edit a Go file: the formatter of `format_on_edit` runs once.
  5. End a turn with a failing test: the agent keeps working with the output; once green, it may stop.
  6. Run `polix graph`: the protected files and the forbidden commands are drawn in the subgraph `guardrails`, and
     the entries of steps 3 to 5 name the shortcut they come from.
  7. Move `guardrails.protected_files` to a top-level `protected`: `polix check` fails with a problem naming
     `guardrails.protected_files`, and
     the hook denies edits and commands with that problem until it is fixed.
