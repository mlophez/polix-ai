---
name: build
# Implements a plan of the folder of plans (ai/plans/ by default) with one polix:builder subagent that runs every task
# in order, updates the Markdown docs of the project, reviews the change against the plan with polix:reviewer, has
# the builder fix the blocking and major findings up to twice, reviewing again after each fix, checks the acceptance
# criteria and merges the reports of the run into the plan. It never edits the code itself and never commits.
description: >
  It runs only when the user or the workflow of polix asks for it. Never suggest it nor mention it on your own.
effort: low
model: sonnet
user-invocable: true
disable-model-invocation: false
argument-hint: "[plan and optional first task, e.g. ai/plans/add-retries.md 3; empty to take it from the conversation]"
---

# Building

Run a plan to its end: one `polix:builder` subagent implements every task, in order and in one go.
Then the docs are brought up to date, the change is reviewed against the plan and fixed, each acceptance criterion
is checked, and the run is merged into the plan.
You orchestrate, document and verify; the builder writes the code and the reviewer judges it.
The folder of plans is `plans.path` of `polix.yaml`, `ai/plans/` without it.

## Requirements

- A plan file of the folder of plans with `### Task N:` sections, as the skill plan writes it.
  Without one, stop and offer `/polix:plan`: never build without a plan nor write one yourself.
- The subagents `polix:builder` and `polix:reviewer`.
  Without them, as in Codex, read `../../agents/builder.md` and `../../agents/reviewer.md` next to this skill and
  follow each yourself when its turn comes, with the `subagent_start` injects of `polix.yaml` for it.
  The rules of this skill win over theirs, and the review is not independent: say so in the report.

## Input

- **Plan**: the path from the arguments, or the plan named, written or discussed in the conversation.
  Without either, list the plans of the folder of plans, leaving out the run files (see `Run files`), and ask the
  user which one; never pick one yourself.
- **From**: the task number from the arguments, to start there, for example to resume a build.
  Without it, the first task whose line of `Progress` is not `[x]`, so a task `[-]` is tried again.
  Without `Progress`, Task 1.
- **Resume**: when the plan is `building` or has run files, the last run did not end: resume it (see `Resume`).
  When the plan is back to `planned` and has run files, it was adjusted since: close that run (see `Iterations`)
  with `Status: interrupted`, then start a new iteration.
  Otherwise, when every line of `Progress` is `[x]`, tell the user the `Status` of the plan and stop.

## Rules

- Never edit the code yourself: only the builder changes it, except in the fallback of `Requirements`.
  You edit only the Markdown docs of the project (see `Docs`), the `Status` and `Iterations` of the plan, and the run
  files.
- Never commit, stage, stash, reset, checkout nor create branches.
- Launch one builder for all the tasks and one for each fix, in the foreground, and wait for each: never one per task.
- Never run a command that installs, generates or formats, even when the plan or `AGENTS.md` lists it.
- Relay what the builder and the reviewer report as it is: add no findings of your own and drop none.
- Talk to the user in the language of the session.

## References

### Run files

Each run of the plan is an iteration, and it keeps its reports next to the plan, `<plans>/<plan>.md`:

- `<plans>/<plan>.iterNN.md`: the reports of the builder in iteration `NN`, the build first and each fix after it.
- `<plans>/<plan>.revNN.md`: one review report each, numbered across the iterations of the plan.

`NN` has two digits:

- The iteration is the last `### Iteration NN` of the plan plus one, or the `NN` of the `.iterNN.md` of a run
  that did not end.
- A review is the last `revNN` named in the `Iterations` of the plan or found on disk, plus one.

Start the `.iterNN.md` file with `# Iteration NN of <plan>`, then each builder report under `## Build` or `## Fix N`.
Write each `.revNN.md` file as the reviewer returns its report, under a first line `# Review NN of <plan>`.

### Resume

Resume a run that did not end in the same iteration, at the first step it has not done:

1. The builder, when the `.iterNN.md` is missing or has no `## Build`, from the first task not `[x]`.
2. Else the docs, when the iteration has no `.revNN.md` yet.
3. Else the review rounds, counting the reviews of the iteration on disk and the fixes in its `.iterNN.md`.
4. Else the acceptance criteria.

A task the builder of this run left `[-]` waits for the next run.

### Builder prompt

Send the builder this block, in the language of the session:

```text
Plan: <path of the plan file>
From: <number and title of the first task, resolved in `Input`>
Findings: none
Iteration: <path of the .iterNN.md file, or "none" while it does not exist>
```

To fix a review, the same block with `From: none` and
`Findings: <path of the .revNN.md file>, <the blocking and major findings to fix>`.

### Reviewer prompt

Send the reviewer this block, in the language of the session:

```text
Plan: <path of the plan file>
Previous: <path of the previous .revNN.md file of this iteration, or "none">
Skipped: <the findings of the previous review the builder skipped, with why, or "none">
Iteration: <path of the .iterNN.md file, or "none" while it does not exist>
```

### Status

Edit only the `Status` line of the header of the plan:

- `building` right before launching the builder.
- `built` at the end, when every line of `Progress` is `[x]`, every AC is `met` or a manual `not verified` with its
  steps in `Pending`, and the last review has no `blocking` nor `major` finding; without `Progress`, when the
  builder reported every task done.
- `partial` at the end otherwise.

A plan without `Status` gets none: never add the header to it.

### Docs

Each task updates the docs of what it changes; once the builder ends, bring in line what the tasks missed:

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
After a fix that changes what a reader sees, run these steps again.

### Review rounds

1. Launch the reviewer (see `Reviewer prompt`), wait, and write its report to a `.revNN.md` file.
2. While the last review has `blocking` or `major` findings and fewer than two fixes ran, launch the builder to fix
   them (see `Builder prompt`), wait, add its report to the `.iterNN.md` file, and review again.
3. The findings of the last review still open, and every `minor` one, go to `Pending`.

A fix never reopens a task `[-]`: the next run tries it again, after the user adjusts the plan or as it is.

### Acceptance criteria

Check every `AC` of the `Acceptance criteria` of the plan against the working tree, after the review rounds:

1. Run the commands of the `Verification` section of the plan, as the plan writes them, and the end to end checks
   that a command of the project can run.
2. For each AC, gather its evidence: the result of those commands, the evidence of `Progress` of the tasks whose
   `Covers:` name it, and the code at `file:line` that fulfils it.
3. Mark each AC `met`, `not met` or `not verified`, with its evidence in one line.
   It is `not met` when a check fails or a task that covers it is not `[x]`; `not verified` when no command or
   code can show it, such as a manual step, which goes to `Pending` with the steps for the user.

Run only commands that read or verify: tests, linters, builds and checks, with their outputs outside the repository.

### Iterations

Merge the run files into the plan, so the plan alone holds the final state of every run:

1. Add to the section `## Iterations` at the end of the plan, creating it when missing and replacing its `none`,
   one subsection `### Iteration NN: <today as YYYY-MM-DD>` with one line each:
   - **Tasks**: each task built, `done` or `departed` and why.
   - **Departures**: the departures the builder recorded, or "none".
   - **Docs**: the Markdown files the docs step updated, or "none".
   - **Review revNN**: for each review, its verdict, the findings fixed, skipped with why, and left open.
   - **Acceptance**: each AC with `met`, `not met` or `not verified`, or `not checked` for a run closed before.
   - **Status**: the `Status` the plan ends with, or `interrupted` for a run closed before its end.
2. Delete the run files of the iteration with `rm`, once their content is in the plan.

## How to operate

Copy this checklist and track your progress:

```text
Build progress:
- [ ] Step 1: Resolve the plan, the first task and whether to resume (see `Input`); read the plan whole.
- [ ] Step 2: Check the working tree (see Step 2).
- [ ] Step 3: Show the user the tasks to build, in order.
- [ ] Step 4: Set the plan `building` (see `Status`), launch the builder (see `Builder prompt`) and wait for it.
- [ ] Step 5: Write its report to the `.iterNN.md` file (see `Run files`).
- [ ] Step 6: Update the docs of the project (see `Docs`).
- [ ] Step 7: Review the change against the plan and have the builder fix it (see `Review rounds`).
- [ ] Step 8: Verify the acceptance criteria of the plan (see `Acceptance criteria`).
- [ ] Step 9: Set the plan `built` or `partial` (see `Status`) and merge the run files (see `Iterations`).
- [ ] Step 10: Report (see `Output format`), stop and return control to the user.
```

### Step 2: Check the working tree

Run `git status --porcelain`.
Leave out the plan, its run files, the `Files:` of the tasks marked `[x]` or `[-]` and the docs the `Iterations` of
the plan record: earlier runs changed them.
When it shows other changes, tell the user that the builder works on top of them and the review will include them,
and ask whether to go on.

## Output format

- **Plan**: the path of the plan file, its `Status` and the iteration number.
- **Tasks**: each task with its line of `Progress`: `done` with its evidence, or `departed` and why.
- **Changes**: the files changed, by task and by fix.
- **Departures**: the departures the builder recorded, or "none".
- **Docs**: each Markdown file updated and what changed in it, or why none needed it.
- **Review**: each round: its number, the verdict, the findings and the ones the builder fixed or skipped.
- **Acceptance**: each AC with `met`, `not met` or `not verified` and its evidence.
- **Pending**: the findings still open, the failures the builder reported outside its tasks, the changes `AGENTS.md`,
  `CLAUDE.md` or a protected file need, and the ACs not met or not verified; or "none".
