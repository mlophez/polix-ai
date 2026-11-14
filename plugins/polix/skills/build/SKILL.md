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
argument-hint: "[plan and optional first task, e.g. ai/plans/add-retries.md 3; empty to take it from the conversation]"
---

# Building

Run a plan to its end, unattended: once it starts, it ends `done`, or `blocked` for what only a person can solve.
You coordinate and never build: each iteration launches the builder, then the verifier and the reviewer in parallel.
You add their reports to the plan, count the iteration, and launch the builder again on what is still open.
The folder of plans is `plans.path` of `polix.yaml`, `ai/plans/` without it.

## Requirements

- A plan file of the folder of plans with `#### Task N:` sections under `### Tasks`, as the skill plan writes it.
  A plan of the earlier format, with `### Task N:` sections, also works.
  Without a plan, stop and offer `/polix:plan`: never build without a plan nor write one yourself.
- The subagents `polix:builder`, `polix:verifier` and `polix:reviewer`.
  `polix:verifier` checks that the change meets the plan, and `polix:reviewer` that its code is well written.
  Without them, as in Codex, read `builder.md`, `verifier.md` and `reviewer.md` in `../../agents/`, next to this skill.
  Follow each yourself when its turn comes, with the `subagent_start` injects of `polix.yaml` for it.
  The rules of this skill win over theirs, and the checks are not independent: say so in the report.

## Input

- **Plan**: the path from the arguments, or the plan named, written or discussed in the conversation.
  Without either, list the plans of the folder of plans and ask the user which one; never pick one yourself.
- **From**: the task number from the arguments, to start there.
  Without it, the first task whose line of `Progress` is not `[x]`, so a task `[-]` is tried again.
  Without `Progress`, Task 1.
- **Branch**: the `branch` of the frontmatter of the plan, the branch it was planned on and is built on.
  When the current branch (`git branch --show-current`) is another one, switch to it with `git switch <branch>`.
  Before switching, run `git status --porcelain`: with changes other than the plan file, stop and ask the user.
  They commit or stash them, or tell you to build where you are; never stash, commit nor discard them yourself.
  When the branch exists neither locally nor on a remote, stop and tell the user; never create it.
  A plan without `branch` builds on the current branch.
- **Approval**: a plan with `status: planned` in its frontmatter is not approved.
  Stop and ask the user to approve it with `/polix:plan <plan>`; never approve it yourself.
  A plan of the earlier format, with a `- Status:` header instead of the frontmatter, needs no approval.
- **Resume**: a plan `in-progress` whose last `### Iteration N` has no `#### Result` was interrupted: resume it.
  Continue that iteration at its first missing part: the builder, the checks, then the result (see `Iteration`).
  Otherwise, when every line of `Progress` is `[x]` and the plan is `done`, tell the user and stop.
  A plan `approved` or `blocked` starts a new iteration at the first task not `[x]`; its earlier ones stay.
  In a plan of the earlier format, `building` stands for `in-progress` and `planned` for `approved`.
  There, `partial` stands for `blocked` and `built` for `done`.

## Output

- **Plan**: the path of the plan file, its `status` and the branch it was built on.
- **Tasks**: each task with its line of `Progress`: `done` with its evidence, `departed` or `blocked` and why.
- **Changes**: the files changed, by task and by fix.
- **Docs**: each Markdown file updated and what changed in it, or why none needed it.
- **Acceptance**: each AC with `met`, `not met` or `not verified` and its evidence.
- **Blockers**: when the plan ends `blocked`, each reason and what a person must do to clear it; or "none".
- **Pending**: the minor findings, the manual checks and the failures that are not the change's.
  Also the changes `AGENTS.md`, `CLAUDE.md` or a protected file need, and the changes made before the run; or "none".
- **Stats**: the build in figures (see `Stats`).

## Rules

- Ask only in `Input`, before Step 4: from then on, never ask the user nor wait for an answer.
  Decide what is open as the builder would (see its `Departures`), and record it.
- Never end your turn before Step 9: a subagent that returns, a failed check or a long run is never the end.
  The run ends only when the plan is done or the loop stops (see `Loop`).
- Never edit the code yourself: only the builder changes it, except in the fallback of `Requirements`.
  You edit only the Markdown docs of the project (see `Docs`) and four parts of the plan.
  Those are the `status` and `iterations` of the frontmatter, the checklist of `Acceptance criteria` and `Iterations`.
- Never commit, stage, stash, reset nor create branches.
  The only checkout is the switch to the plan's `branch` in Step 1 (see `Input`).
- Launch one builder per iteration, for all its open tasks or all its open findings, in the foreground, and wait.
- Count nothing as closed on the builder's word: only the verifier's and the reviewer's reports close it.
- Never run a command that installs, generates or formats, even when the plan or `AGENTS.md` lists it.
- Add what the builder, the verifier and the reviewer report as it is: no findings of your own, and none dropped.
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
- [ ] Step 5: Run iterations until the plan is done or the loop stops (see `Iteration` and `Loop`).
- [ ] Step 6: Mark the acceptance criteria from the last verification (see `Acceptance criteria`).
- [ ] Step 7: Set the plan `done` or `blocked` (see `Status`).
- [ ] Step 8: Gather the stats of the build (see `Stats`).
- [ ] Step 9: Report (see `Output`), stop and return control to the user.
```

### Step 2: Note the changes made before the run

Run `git status --porcelain`.
Leave out the plan and the files of the tasks marked `[x]` or `[-]`.
A task's files are the `files` of its `yaml` block, or its `Files:` in a plan of the earlier format.
Leave out the docs the `Iterations` of the plan record too: earlier iterations changed them.
The paths left are the changes made before the run: keep them for `Pre-existing` and `Pending`, and go on.
The builder works on top of them, and never reverts nor fixes them.

## References

### Iteration

Each iteration is one subsection of `## Iterations`, at the end of the plan, built as it happens:

1. Add `### Iteration N: <today as YYYY-MM-DD>`, where `N` is the `iterations` of the frontmatter plus one.
   Create `## Iterations` when missing, and replace its `none`.
2. **Builder**: launch it on what is open (see `Builder prompt`), wait, and add its report under `#### Builder`.
   Open are the tasks from `From` on that are `[ ]` or `[-]` with `Departure:`; without any, the open findings.
   A task `[-]` with `Blocked:` waits for a person: never send it again in this run.
3. **Docs**: after a builder that changed what a reader sees, update the docs (see `Docs`).
4. **Checks**: launch the verifier and the reviewer in one message, so they run in parallel, and wait for both.
   Add their reports under `#### Verification` and `#### Review`.
   Skip the checks when the builder left a task `[ ]`: the next iteration continues the tasks first.
5. **Result**: add `#### Result` with what the iteration closed and what is still open (see `Loop`).
   Then set `iterations: N` in the frontmatter: the iteration is counted once its result is written.

Add each report as the subagent returns it, with its headings demoted below `####`: a `##` becomes `#####`.
Each line of the plan stays one sentence; keep the reports' lines as they come.

### Loop

Open after an iteration are the tasks not `[x]`, but those `Blocked:`, and the findings of its checks.
Those are the verifier's findings, its criteria `not met` among them, and the reviewer's `blocking` and `major`.
A finding the builder skipped twice for the same reason is closed.

`#### Result` holds two lines:

- **Closed:** the tasks that turned `[x]`, the findings fixed and the criteria that turned `met`, or "nothing".
- **Open:** what is still open, by kind, or "nothing".

An iteration progresses when it closed anything.
The plan is done when nothing is open: the last verification is `met` and the last review has no open finding.
Then go to Step 6.
Stop the loop, leaving the plan `blocked`, in any of these cases:

- Two iterations in a row closed nothing: the work stalled.
- This run reached ten iterations: the guard against a fix that breaks something else every time.
- Only `Blocked:` tasks are left: each waits for a person.

Each reason goes to `Blockers`, with what a person must do: install, grant, adjust the plan or decide.

### Builder prompt

Send the builder this block, in the language of the session:

```text
Plan: <path of the plan file>
Branch: <the branch of Step 1, or "current" for a plan without `branch`>
From: <number and title of the first open task, or "none" to fix findings>
Findings: <"none", or the iteration of the plan whose checks hold the findings to fix, and which ones>
```

To fix findings, name `Iteration N` of the plan and list them.
They are the findings of its `#### Verification`, the criteria `not met` among them.
They are also the `blocking` and `major` findings of its `#### Review`.

### Verifier prompt

Send the verifier this block, in the language of the session:

```text
Plan: <path of the plan file>
Previous: <"Iteration N" of the plan with the previous verification, or "none">
Pre-existing: <the paths changed before the run, from Step 2, or "none">
```

### Reviewer prompt

Send the reviewer this block, in the language of the session:

```text
Plan: <path of the plan file>
Previous: <"Iteration N" of the plan with the previous review, or "none">
Skipped: <the findings of the previous review the builder skipped, with why, or "none">
Pre-existing: <the paths changed before the run, from Step 2, or "none">
```

### Status

Edit only the `status` of the frontmatter of the plan:

- `in-progress` right before the first iteration.
- `done` at the end when three things hold.
  Every line of `Progress` is `[x]`, or the builder reported every task done in a plan without `Progress`.
  The last verification is `met`: every AC `met`, or a manual `not verified` with its steps in `Pending`.
  The last review has no `blocking` nor `major` finding, but those a builder skipped twice for the same reason.
- `blocked` at the end otherwise.

A plan of the earlier format keeps its `- Status:` line and its values: `building`, then `built` or `partial`.
A plan without a status gets none: never add the frontmatter nor the header to it.
A plan without `iterations` in its frontmatter gets it at its first iteration, after `tasks`.

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

Leave the docs alone when the change alters nothing a reader sees, and say so in `#### Result`.

### Acceptance criteria

At Step 6, take the criteria of the last `#### Verification` as they are:

1. Each AC is `met`, `not met` or `not verified`, with its evidence; a manual step goes to `Pending` for the user.
2. When the plan has the checklist `- [ ] AC-NN: <title>`, mark each line, again on every run.
   Write `- [x] AC-NN: <title>. Evidence: <...>.` when `met`.
   Write `- [-] AC-NN: <title>. <Not met or Not verified>: <why>.` otherwise.
   A plan whose criteria have no checklist gets none: never add it.
3. When the loop stopped before a verification of the last change, launch the verifier once more first.
   Add its report to the last iteration, under a second `#### Verification`.

### Stats

Gather the figures of this run from the plan and from git, and give them as a short list:

- **Iterations**: those of this run, and the plan's total in `iterations`.
- **Tasks**: done, departed and blocked, of the total.
- **Criteria**: met, not met and not verified, of the total.
- **Findings**: the verifier's found and fixed, and the reviewer's found and fixed by severity.
- **Agents**: how many times the builder, the verifier and the reviewer ran.
- **Code**: the files changed and the lines added and removed, from `git diff --shortstat` and the untracked files.
