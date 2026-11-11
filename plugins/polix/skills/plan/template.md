# <One-line description of the change>

- Status: planned
- Risk: <low, medium or high>: <the reason in one line>.
- Reference: <the ticket, issue or link the change comes from, or `none`>.
- Created: <today as YYYY-MM-DD>.
- Size: <N> tasks, about <M> changed lines.

## Overview

<For a human who reads nothing else, in plain language: what changes and why, the key decisions and the visible effect.
No paths, code or jargon; a few short paragraphs or bullets.>

## Request

<The request of the user verbatim, in its language, never summarized nor reworded.>

- <Each answer of the user written as free text in the interview that changes the request, verbatim.
  Delete the bullet when there are none.>

## Context

<How the code behaves today in the area of the change and where the problem is, in two or three sentences.>

## Constraints

- <A limit the solution must respect, and where it comes from: the request, a rule id, `AGENTS.md` or the code.
  Such as compatibility, platforms and versions, dependencies, performance, security, or a contract that must not
  change.
  Write "none" when there are none.>

## Decisions

- <A decision that shapes the tasks, an answer of the interview, or an assumption.
  Label "Assumption:" every choice of yours: a default for an unanswered question or a behaviour the request leaves
  open.>

## Approach

<The technical design: the components and their responsibilities, and the contracts that change.
Contracts are APIs, events, schemas, configuration and command line.
Cover the data model and the error handling.>

### Discarded alternatives

- <Alternative>: <one-line reason>.

## Acceptance criteria

- AC1: <What is checked and the result expected, quoting verbatim the strings, messages and examples of the request.>

## Out of scope

<What the plan deliberately leaves out and why, or "none".>

## Starting point

- Read first: `<path>`: <what it teaches about the area before Task 1>.
- Baseline: `<command>`, expected: <its result before any change, naming the failures that already exist>.

## Progress

- [ ] Task 1: <title of Task 1>
- [ ] Task 2: <title of Task 2>

## Tasks

### Task 1: <What the task delivers>

- Covers: <the acceptance criteria the task fulfils, e.g. `AC1, AC2`, or `none` for a supporting task>.
- High risk: <`none`, or each high-risk item of the task and how its tests guard it>.
- Files: `<path>` (create), `<path>` (`<function, type or method>`, modify).
  <Include the help text and docs that describe what the task changes.
  When another task also changes a file, say which part this one changes.>
- Rules: <the ids of the rules of the folder of rules whose trigger holds for the task and whose `scope` matches
  its `Files:`, e.g. `ARCH-7, STYLE-3`, or `none`>.
- Reuse: `<function, type or utility>` from `<path>`, or `none`.
- Tests: <the tests to write first, in which test file, each labelled `new` with how it fails before the change,
  or `kept` with the previous behaviour it holds: the requirements and observable effects, and the cases they name.
  Or `none` and why: a refactor the existing tests `<test names>` keep, or a change no test can observe>.
- Changes: <what changes in each file to make those tests pass, in prose, never code.
  Name the behaviour, what is added or renamed and the contracts that change>.
- Verification: `<command scoped to the files of the task>`, expected: <result>.
  <For a high-risk task, also the differential check: the commands and inputs to compare against the base version.>

### Task 2: <What the task delivers>

<Same fields as Task 1, one block per task, in the order they run.>

## Risks

- <What could break>: <how to mitigate it>.

## Verification

- `<command>`: <expected result>.
- <End to end check that needs more than a command, step by step.>

## Missing docs

<Only when `AGENTS.md` has no project facts or the repository has no folder of rules: say which are missing.
Delete the section otherwise.>

## Iterations

none
