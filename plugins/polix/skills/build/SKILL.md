---
name: build
description: >
  Implements a plan of the folder of plans (ai/plans/ by default) with polix:builder subagents in parallel, one per
  task, in the order its dependencies allow, updates the Markdown docs of the project, then reviews the whole change
  with polix:reviewer and checks the acceptance criteria of the plan.
  It runs only when the user or the workflow of polix asks for it.
  It never edits the code itself, never commits and never fixes the findings of the review.
effort: medium
model: inherit
user-invocable: true
disable-model-invocation: false
argument-hint: "[plan and optional tasks, e.g. ai/plans/add-retries.md 2 3; empty to take it from the conversation]"
---

# Building

Run a plan to its end: each task in its own `polix:builder` subagent, as many at once as its dependencies allow, and
then the docs brought up to date, one review of the whole change and a check of each acceptance criterion of the plan.
You orchestrate, document and verify; the builders write the code and the reviewer judges it.
The folder of plans is `plans.path` of `polix.yaml` and the folder of reports is `reports.path`.
Without them, `ai/plans/` and `ai/reports/`; polix names both at session start.

## Requirements

- A plan file of the folder of plans with `### Task N:` sections, as the skill plan writes it.
  Without one, stop and offer `/polix:plan`: never build without a plan nor write one yourself.
- The subagent `polix:builder`.
  Without it, as in Codex, read `../../agents/builder.md` next to this skill and follow it yourself, one task at a
  time in the order of the plan.
- The skill review, `../review/SKILL.md` next to this skill, for the review at the end.
  Without it, stop and tell the user.

## Input

- **Plan**: the path from the arguments, or the plan named, written or discussed in the conversation.
  Without either, list the files of the folder of plans and ask the user which one; never pick one yourself.
- **Tasks**: the task numbers from the arguments, to build only those, for example to resume a partial build.
  Without them, every task of the plan.

## Rules

- Never edit the code yourself: only the builders change it.
  You edit only the Markdown docs of the project (see `Docs`), and the review writes its report.
- Never commit, stage, stash, reset, checkout nor create branches: the builders share the working tree and the index.
- Launch each builder with the plan and one task only, never several tasks in one builder.
- Launch at the same time only tasks that are ready (see `Waves`), in one message with one subagent call per task.
- Never launch a task whose dependencies are not `done`.
- Relay what the builders and the reviewer report as it is: add no findings of your own and drop none.
- Never fix the findings of the review nor a failure a builder reported: ask the user what to do.
- Talk to the user in the language of the session.

## References

### Waves

Read every task of the plan: its number, its `Depends on:` and its `Files:`.

1. A task is ready when every task of its `Depends on:` is `done`, in this build or before it.
   A task outside `Tasks` that a selected task depends on counts as done only when the working tree holds its changes;
   otherwise report it and leave the selected task out.
2. A wave is every ready task not yet launched.
   When two ready tasks share a file of their `Files:`, keep the earlier one in the wave and the later one for the next.
3. Launch the wave and wait for every builder of it before the next.
4. A task reported `partial` or `blocked` is not done: its dependents, direct or not, are skipped and reported.
   Tasks that do not depend on it go on.
5. Stop when no task is ready.

A `Depends on:` that names a task missing from the plan, or a cycle, stops the build before the first wave: report it
and offer to adjust the plan with `/polix:plan`.

### Builder prompt

Send each builder this block, in the language of the session:

```text
Plan: <path of the plan file>
Task: <number and title of the task>
```

### Docs

Once no task is ready, bring the docs in line with what the builders changed, before the review checks them:

1. Read the diff of the change and list what a reader of the docs would see differently: behaviour, commands,
   flags, configuration, contracts, files and folders, install or setup steps.
2. Find the Markdown files that describe it with `rg`: `README.md`, the folder of docs (`docs/` or what `AGENTS.md`
   names), `CHANGELOG.md` and any other `.md` of the project, except the folders of plans, reports and rules.
3. Update each passage that the change made stale or incomplete, and add an entry to the unreleased section of the
   changelog when the project keeps one.
   Follow the style, language and line format of each file; write nothing the change does not support.
4. Never edit `AGENTS.md`, `CLAUDE.md` nor a file protected by `polix.yaml`: the instructions of the repository
   belong to a person.
   Write the change they need in `Pending`.

Leave the docs alone when the change alters nothing a reader sees, and say so.

### Review

After the docs, review the change by following `../review/SKILL.md` from its Step 1, with:

- **Change**: the current change.
- **Intent**: the path of the plan and its `Overview`, plus the tasks not built, so the reviewer does not report
  them as missing.

Run it even when some task is not done, as long as one builder changed a file; when none did, skip it and say so.

### Acceptance criteria

Check every `AC` of the `Acceptance criteria` of the plan against the working tree, after the review:

1. Run the commands of the `Verification` section of the plan, as the plan writes them, and the end to end checks
   that a command of the project can run.
2. For each AC, gather its evidence: the result of those commands, the `Verification` of the builders of the tasks
   whose `Covers:` name it, and the code at `file:line` that fulfils it.
3. Mark each AC `met`, `not met` or `not verified`, with its evidence in one line.
   It is `not met` when a check fails or a task that covers it is not `done`; `not verified` when no command or code
   can show it, such as a manual step, which goes to `Pending` with the steps for the user.

Run only commands that read or verify: tests, linters, builds and checks, with their outputs outside the repository.

## How to operate

Copy this checklist and track your progress:

```text
Build progress:
- [ ] Step 1: Resolve the plan and the tasks (see `Input`); read the plan whole.
- [ ] Step 2: Check the working tree (see Step 2).
- [ ] Step 3: Order the tasks in waves (see `Waves`) and show them to the user.
- [ ] Step 4: Launch each wave of builders (see `Builder prompt`) and track the status of every task.
- [ ] Step 5: Update the docs of the project (see `Docs`).
- [ ] Step 6: Review the change (see `Review`).
- [ ] Step 7: Verify the acceptance criteria of the plan (see `Acceptance criteria`).
- [ ] Step 8: Report (see `Output format`), stop and return control to the user.
```

### Step 2: Check the working tree

Run `git status --porcelain`.
When it shows changes outside the plan file, tell the user that the builders work on top of them and the review will
include them, and ask whether to go on.

### Step 4: Launch each wave of builders

After each wave, give the user one line per task: its status and, when it is not `done`, why.
Collect from each builder its `Changes`, `Verification`, `Assumptions` and `Pending` for the report.

## Output format

- **Plan**: the path of the plan file.
- **Tasks**: each task with its status (`done`, `partial`, `blocked` or `skipped`) and, when not `done`, why.
- **Changes**: the files changed, by task.
- **Assumptions**: the choices the builders made where the plan was open, or "none".
- **Docs**: each Markdown file updated and what changed in it, or why none needed it.
- **Review**: the output of the skill review: the report, the verdict, the findings and the checks.
- **Acceptance**: each AC with `met`, `not met` or `not verified` and its evidence.
- **Pending**: the failures the builders reported outside their tasks, the tasks to resume with
  `/polix:build <plan> <tasks>`, the changes `AGENTS.md`, `CLAUDE.md` or a protected file need, the ACs not met or
  not verified, and the question of which findings to fix; or
  "none".
