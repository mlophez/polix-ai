# <One-line description of the change>

## Overview

<For a human who reads nothing else, in plain language: what changes and why, the key decisions and the visible effect.
No paths, code or jargon; a few short paragraphs or bullets.>

## Context

<The problem and the intended outcome, in two or three sentences.>

## Decisions

- <A decision that shapes the tasks, an answer of the interview, or an assumption.
  Label an assumption taken for an unanswered question "Assumption:".>

## Approach

<The technical design: the components and their responsibilities, and the contracts that change.
Contracts are APIs, events, schemas, configuration and command line.
Cover the data model and the error handling.>

### Discarded alternatives

- <Alternative>: <one-line reason>.

## Acceptance criteria

- AC1: <What is checked and the result expected.>

## Out of scope

<What the plan deliberately leaves out and why, or "nothing".>

## Tasks

### Task 1: <What the task delivers>

- Covers: <the acceptance criteria the task fulfils, e.g. `AC1, AC2`, or `none` for a supporting task>.
- Depends on: <the tasks that must be done before this one, e.g. `Task 1`, or `none`.
  Tasks that depend on none of each other and share no file may run at the same time>.
- Files: `<path>` (create), `<path>` (`<function, type or method>`, modify).
- Rules: <the ids of the rules of the folder of rules whose trigger holds for the task and whose `scope` matches its `Files:`.
  E.g. `ARCH-7, STYLE-3`, or `none`>.
- Reuse: `<function, type or utility>` from `<path>`, or `none`.
- Changes: <what changes in each file and why, in prose, never code.
  Name the behaviour, what is added or renamed and the contracts that change>.
- Tests: <the test cases to add or update, in which test file: the behaviour, edge and error cases they check>.
- Verification: `<command>`, expected: <result>.

### Task 2: <What the task delivers>

<Same fields as Task 1, one block per task, in the order they run.>

## Risks

- <What could break>: <how to mitigate it>.

## Verification

- `<command>`: <expected result>.
- <End to end check that needs more than a command, step by step.>

## Missing docs

<Only when `AGENTS.md` has no project facts or the repository has no folder of rules: say which are missing.
Rules are written with `/polix:add-rule`.
Delete the section otherwise.>
