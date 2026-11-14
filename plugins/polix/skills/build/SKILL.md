---
name: build
description: >
  Implements a plan with polix:builder, then has polix:verifier check it against the plan and polix:reviewer review it.
  It iterates until the plan is done or stalls, without asking once it starts, and records each iteration in the plan.
  It never commits, and returns the stats of the build. It runs only when the user or the workflow of polix asks for it.
effort: low
model: sonnet
user-invocable: true
disable-model-invocation: true
argument-hint: "[plan and optional first task, e.g. add-retries.yaml T3; empty to take it from the conversation]"
---

# Building

Run a plan to its end, unattended: once it starts, it ends `done`, or `blocked` for what only a person can solve.
You coordinate and never build: the builder builds the tasks, then the verifier and the reviewer check them in parallel.
When the checks leave something to fix, you add an iteration with it to the plan and launch the builder on it.
The folder of plans is the one the start context of polix names, below the root.

## Requirements

- A plan file of the folder of plans, in YAML with a list `tasks` of ids `T1`, `T2`..., as the skill plan writes it.
  A plan in Markdown, with `#### Task N:` or `### Task N:` sections, also works (see `Markdown plans`).
  Without a plan, stop and offer `/polix:plan`: never build without a plan nor write one yourself.
- The subagents `polix:builder`, `polix:verifier` and `polix:reviewer`.
  `polix:verifier` checks that the change meets the plan, and `polix:reviewer` that its code is well written.
  Without them, as in Codex, read `builder.md`, `verifier.md` and `reviewer.md` in `../../agents/`, next to this skill.
  Follow each yourself when its turn comes, with the `subagent_start` injects of `polix.yaml` for it.
  The rules of this skill win over theirs, and the checks are not independent: say so in the report.

## Input

- **Plan**: the path from the arguments, or the plan named, written or discussed in the conversation.
  Without either, list the plans of the folder of plans and ask the user which one; never pick one yourself.
- **From**: the task from the arguments, as `T3` or `3`, to start there.
  Without it, the first task whose `status` is not `done`, so a task `departed` is tried again.
- **Branch**: the `branch` of the plan, the branch it was planned on and is built on.
  When the current branch (`git branch --show-current`) is another one, switch to it with `git switch <branch>`.
  Before switching, run `git status --porcelain`: with changes other than the plan file, stop and ask the user.
  They commit or stash them, or tell you to build where you are; never stash, commit nor discard them yourself.
  When the branch exists neither locally nor on a remote, stop and tell the user; never create it.
  A plan without `branch` builds on the current branch.
- **Approval**: a plan with `status: planned` is not approved.
  Stop and ask the user to approve it with `/polix:plan <plan>`; never approve it yourself.
- **Resume**: a plan `in-progress` was interrupted: resume it where it stopped.
  With a last iteration `open`, launch the builder on it; with one `built`, run its checks (see `Loop`).
  Otherwise, build from the first task not `done`, then check.
  When every task is `done` and the plan is `done`, tell the user and stop.
  A plan `approved` or `blocked` builds from the first task not `done`; its earlier iterations stay.

## Output

- **Plan**: the path of the plan file, its `status` and the branch it was built on.
- **Tasks**: each task with its `status`: `done` with its evidence, `departed` or `blocked` and why.
- **Changes**: the files changed, by task and by finding.
- **Docs**: each Markdown file updated and what changed in it, or why none needed it.
- **Acceptance**: each AC with `met`, `not met` or `not verified` and its evidence.
- **Blockers**: when the plan ends `blocked`, each reason and what a person must do to clear it; or "none".
- **Pending**: the minor findings, the manual checks and the failures that are not the change's.
  Also the changes `AGENTS.md`, `CLAUDE.md` or a protected file need, and the changes made before the run; or "none".
- **Stats**: the build in figures (see `Stats`).

## Limits

- Ask only in `Input`, before Step 4: from then on, never ask the user nor wait for an answer.
  Decide what is open as the builder would (see its `Departures`), and record it.
- Never end your turn before Step 9: a subagent that returns, a failed check or a long run is never the end.
  The run ends only when the plan is done or the loop stops (see `Loop`).
- Never edit the code yourself: only the builder changes it, except in the fallback of `Requirements`.
  You edit only the Markdown docs of the project (see `Docs`) and three parts of the plan.
  Those are its `status`, the `status` and `evidence` of each criterion, and the iterations you add and their `status`.
- Edit the plan with targeted edits that keep it valid YAML, never writing it again whole.
- Never commit, stage, stash, reset nor create branches.
  The only checkout is the switch to the plan's `branch` in Step 1 (see `Input`).
- Count nothing as closed on the builder's word: only the verifier's and the reviewer's reports close it.
- Never run a command that installs, generates or formats, even when the plan or `AGENTS.md` lists it.
- Add the findings the verifier and the reviewer report as they are: no findings of your own, and none dropped.
- Talk to the user in the language of the session.

## How to operate

Copy this checklist and track your progress:

```text
Build progress:
- [ ] Step 1: Resolve the plan, switch to its branch, its approval, the first task and resume (see `Input`).
  Read the plan whole.
- [ ] Step 2: Note the changes made before the run (see Step 2).
- [ ] Step 3: Show the user the tasks to build, in order, and go on without waiting.
- [ ] Step 4: Set the plan `in-progress` (see `Status`).
- [ ] Step 5: Build the tasks with the builder (see `Builder prompt`), then update the docs (see `Docs`).
- [ ] Step 6: Check and fix until nothing is open or the loop stops (see `Loop` and `Iteration`).
- [ ] Step 7: Set the plan `done` or `blocked` (see `Status`).
- [ ] Step 8: Gather the stats of the build (see `Stats`).
- [ ] Step 9: Report (see `Output`), stop and return control to the user.
```

### Step 2: Note the changes made before the run

Run `git status --porcelain`.
Leave out the plan and the files of the tasks `done` or `departed`: the `files` of each task.
Leave out the files the findings of the `iterations` of the plan name too: earlier iterations changed them.
The paths left are the changes made before the run: keep them for `Pre-existing` and `Pending`, and go on.
The builder works on top of them, and never reverts nor fixes them.

## References

### Loop

After the builder, check, and repeat until nothing is open:

1. **Checks**: launch the verifier and the reviewer in one message, so they run in parallel, and wait for both.
   Skip them when the builder left a task `pending`: launch it again from that task first.
   Set the iteration the builder worked on, if any, to `status: checked`.
2. **Criteria**: set the criteria from the verification (see `Acceptance criteria`).
3. **Open**: list what is still open.
   - The tasks not `done`, but those `blocked`: each waits for a person.
   - The verifier's findings, its criteria `not met` among them.
   - The reviewer's `blocking` and `major` findings; its `minor` ones go to `Pending`.
   - Leave out a finding the builder skipped twice for the same reason: it goes to `Pending`.
4. **Done**: with nothing open, the loop ends.
5. **Stop**: leave the plan `blocked` in any of these cases, each to `Blockers` with what a person must do.
   - The last two iterations fixed nothing and turned no task `done`: the work stalled.
   - This run reached ten iterations: the guard against a fix that breaks something else every time.
   - Only `blocked` tasks are open: each waits for a person.
6. **Iteration**: otherwise, add an iteration with what is open (see `Iteration`).
   Launch the builder on it (see `Builder prompt`), set it `status: built`, update the docs, and go back to 1.

### Iteration

An iteration is one pass of the builder on what the checks left open, appended to `iterations` of the plan:

- `id`: `ITR-01`, `ITR-02`..., the number of items plus one; the prompts and later findings name it.
- `date`: today as YYYY-MM-DD.
- `status`: `open` while the builder works on it, `built` once it returns, `checked` once the checks ran.
- `tasks`: the ids of the tasks still to do, `pending` or `departed`, or `[]`.
- `findings`: one item per finding to fix, with its own id, `FND-NN`, numbered across the whole plan.
  - `source`: `verification` or `review`.
  - `kind`: the area of a finding of the verification, or the severity and category of one of the review.
  - `where`: the `file:line` or the id of the plan it is about.
  - `problem` and `fix`: what is wrong and what to change, from the report, as literal blocks.
  - `repeats`: the `FND-NN` of an earlier iteration the checks report again, or `none`.
  - `status: open` and `evidence: none`: the builder sets `fixed` or `skipped`, with why.

Copy each finding from its report as it is: never merge, reword nor judge it.
The criteria go to `acceptance`, never to the iteration, and the rest of the reports goes to `Output`.

Two checks of a plan, the first leaving a finding and a task open, and the iteration that fixed them:

```yaml
iterations:
  - id: ITR-01
    date: 2026-11-14
    status: checked
    tasks: [T3]
    findings:
      - id: FND-01
        source: verification
        kind: criterion
        where: internal/export/writer.go:31
        problem: |
          CUJ-03 asks the rows in the order of LIT-04, and they come unsorted.
        fix: |
          Sort the invoices by date before writing them.
        repeats: none
        status: fixed
        evidence: |
          `writer.go` sorts by date; `TestExportRowsSorted` failed before the fix and passes.
      - id: FND-02
        source: review
        kind: major, error handling
        where: internal/export/flags.go:40
        problem: |
          A date that does not parse panics instead of returning the error of LIT-03.
        fix: |
          Return the error of the parser wrapped with the flag name.
        repeats: none
        status: fixed
        evidence: |
          `TestExportDates` covers `2026-02-30`; it panicked before the fix and passes.
```

### Builder prompt

Send the builder this block, in the language of the session:

```text
Plan: <path of the plan file>
Branch: <the branch of Step 1, or "current" for a plan without `branch`>
From: <id and title of the first task to build, or "none" when it works on an iteration>
Iteration: <the `ITR-NN` of the plan to work on, its tasks and findings, or "none">
```

### Verifier prompt

Send the verifier this block, in the language of the session:

```text
Plan: <path of the plan file>
Previous: <the `ITR-NN` of the plan the builder just worked on, or "none">
Pre-existing: <the paths changed before the run, from Step 2, or "none">
```

### Reviewer prompt

Send the reviewer this block, in the language of the session:

```text
Plan: <path of the plan file>
Previous: <the `ITR-NN` of the plan the builder just worked on, or "none">
Skipped: <the `FND-NN` of that iteration the builder skipped, with why, or "none">
Pre-existing: <the paths changed before the run, from Step 2, or "none">
```

### Status

Edit only the top-level `status` of the plan:

- `in-progress` right before the first builder.
- `done` at the end when three things hold.
  Every task is `done`.
  The last verification is `met`: every AC `met`, or a manual `not verified` with its steps in `Pending`.
  The last review has no `blocking` nor `major` finding, but those a builder skipped twice for the same reason.
- `blocked` at the end otherwise.

### Docs

Each task updates the docs of what it changes; after the builder, bring in line what the tasks missed:

1. Read the diff of the change and list what a reader of the docs would see differently.
   That is behaviour, commands, flags, configuration, contracts, files and folders, install or setup steps.
2. Find the Markdown files that describe it with `rg`: `README.md`, the folder of docs and `CHANGELOG.md`.
   The folder of docs is `docs/` or what `AGENTS.md` names.
   Include any other `.md` of the project, except the folders of plans, reports and rules.
3. Update each passage that the change made stale or incomplete.
   Add an entry to the unreleased section of the changelog when the project keeps one.
   Follow the style, language and line format of each file; write nothing the change does not support.
4. Never edit `AGENTS.md`, `CLAUDE.md` nor a file protected by `polix.yaml`: the instructions belong to a person.
   Write the change they need in `Pending`.

Leave the docs alone when the change alters nothing a reader sees, and say so in `Docs` of `Output`.

### Acceptance criteria

After each verification, take its criteria as they are:

1. Each AC is `met`, `not met` or `not verified`, with its evidence; a manual step goes to `Pending` for the user.
2. Set the `status` and `evidence` of each criterion of `acceptance`, again on every run.
   The evidence is the one of the verification, in one line or a literal block.
3. When the loop stopped before a verification of the last change, launch the verifier once more first.

### Stats

Gather the figures of this run from the plan and from git, and give them as a short list:

- **Iterations**: those of this run, and the plan's total, the items of `iterations`.
- **Tasks**: done, departed and blocked, of the total.
- **Criteria**: met, not met and not verified, of the total.
- **Findings**: of the iterations, found, fixed and skipped, by `source`, and the reviewer's `minor` ones apart.
- **Agents**: how many times the builder, the verifier and the reviewer ran.
- **Code**: the files changed and the lines added and removed, from `git diff --shortstat` and the untracked files.

### Markdown plans

A plan in Markdown keeps its format: never convert it.
Its tasks are `Task N`, and its line of `Progress` is their status.
`[x]` is `done`, `[-]` with `Departure:` is `departed`, `[-]` with `Blocked:` is `blocked`, and `[ ]` is `pending`.
Without `Progress`, start at Task 1, and a task is done when the builder reports it so.
A task's files are the `files` of its `yaml` block, or its `Files:` in a plan of the earlier format.

- **Status**: the `status` of its frontmatter.
  A plan with a `- Status:` header instead needs no approval and keeps its values.
  There, `building` stands for `in-progress` and `planned` for `approved`.
  Also, `partial` stands for `blocked` and `built` for `done`.
  A plan without a status gets none: never add the frontmatter nor the header to it.
- **Iteration**: every pass of the builder and its checks, the first one included, is an iteration.
  It is a subsection `### Iteration N: <date>` of `## Iterations`, at the end of the plan.
  `N` is the `iterations` of the frontmatter plus one; create `## Iterations` when missing, and replace its `none`.
  The reports go as they are under `#### Builder`, `#### Verification`, `#### Review` and `#### Result`.
  `#### Result` holds `- **Closed:**` and `- **Open:**`, what the iteration closed and what is still open.
  To fix findings, the builder's `Iteration` names `Iteration N` of the plan and lists the findings to fix.
  Demote their headings below `####`: a `##` becomes `#####`.
  Once `#### Result` is written, set `iterations: N` in the frontmatter; add it after `tasks` when missing.
  A plan `in-progress` whose last `### Iteration N` has no `#### Result` was interrupted: resume it.
- **Acceptance criteria**: when the plan has the checklist `- [ ] AC-NN: <title>`, mark each line, on every run.
  Write `- [x] AC-NN: <title>. Evidence: <...>.` when `met`.
  Write `- [-] AC-NN: <title>. <Not met or Not verified>: <why>.` otherwise.
  A plan whose criteria have no checklist gets none: never add it.
