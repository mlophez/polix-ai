---
title: <One-line description of the change>
status: <planned | approved | in-progress | blocked | done>
risk: <low | medium | high>
risk_reason: <the reason in one line>
reference: <the ticket, issue or link the change comes from, or none>
artifacts: <[] or a list of links or paths>
branch: <the git branch the plan builds on>
created: <today as YYYY-MM-DD>
tasks: <N>
iterations: 0
---

# <One-line description of the change, the same text as title>

## Overview

<One sentence with the outcome, for a reviewer who reads nothing else.
This section names no path, identifier or code jargon outside its example.>

- **What changes:**
  - <What its users notice, in plain language; one to three items.>
- **What it does not do:**
  - <The main limit or what is left out; one to three items.>
- **Risk and size:** <the risk in plain words and why, and the number of tasks>.

<One minimal example from the simplest journey: what a user writes or does, then what they get.
Put each in a fenced block of literal data with one line before it; delete this when nothing is visible.>

### Problem

<Who suffers what today, what it costs them and why it is addressed now, in two or three sentences.
Give the cost as a figure when the request or the interview gives one.>

### Affected

- <A user, team or system that the change reaches, and how.>

### Context

<How the code behaves today in the change's area, in two or three sentences.>

### Terms

- <A project or domain term the plan uses, and what it means here.
  Delete the section when the plan uses none a new reader would not know.>

## Specification

### Outcome

<What exists once the change is done, as its users and callers see it: the behaviour, the outputs and the state left.
Give enough detail that two executors would build the same result.
Never how it is built: that is the `Design` of `Approach`.
This is the only section that states the behaviour; the others point to it.>

#### Interface

<What its users and callers touch: the commands, options, messages, files, formats, APIs, events and screens.
Give each one it adds or changes with its names, fields, defaults and errors, as a nested list.
Quote verbatim the strings, messages and commands the code must reproduce.>

#### Journeys

##### CUJ-01: <the journey of a user or caller, named by what they achieve>

<Head it `CUJ-01 (kept)` when it holds an existing behaviour; delete this note otherwise.>

- **Given:** <the starting state, with literal values>.
- **When:** <the input or action, quoting verbatim the command, message or value>.
- **Then:** <the exact result: the output, the exit code or error, and the state it leaves>.
- **And:** <another step of the same kind as the one before it; repeat or delete it>.
- **But:** <what must not happen or must stay unchanged, such as a file not written; repeat or delete it>.

<When the content of a request, command, body or output is part of the contract, put it under its step.
It is an indented fenced block with the literal data.
The step says whether the block is `exactly` the result or what it `contains`; delete this note otherwise.>

##### CUJ-02: <the journey of a user or caller, named by what they achieve>

<Same fields as CUJ-01: one per behaviour of the outcome, plus its boundaries and its errors.>

### Scope

#### In scope

- <An area the plan touches, such as a module, command, file or screen>: <its journeys, e.g. `CUJ-01`, `CUJ-03`>.

#### Out of scope

- <What the plan deliberately leaves out and why, or "none".>

### Constraints

- <A limit on how the solution is built, such as compatibility, platforms, versions or dependencies.
  It may be the authentication to reuse, or a contract that must not change.
  Say where it comes from: the request, an interview answer, an artifact, a rule, the docs or the code.
  A property of the result that can be measured is a criterion, never a constraint.
  Write "none" when there are none.>

### Acceptance criteria

- [ ] AC-01: <title of AC-01>
- [ ] AC-02: <title of AC-02>

#### AC-01: <title of AC-01>

- **Requirement:** <one EARS sentence, "The <system> shall <response>", after "When", "While", "If..., then" or "Where".
  It reads alone, never pointing to another section, and quotes verbatim the request's strings and messages>.
- **CUJs:** <the journeys of `Journeys` this criterion checks, e.g. `CUJ-01, CUJ-02`, or `none`>.
- **Verify:** `<command>`, expected: <result, naming the tests or cases that prove this criterion>.
  <Or the steps of a check that needs more than a command, one by one, with the result expected of each.>

#### AC-02: <title of AC-02>

<Same fields as AC-01, one block per criterion, in the order of the checklist.>

### Decisions

- <A decision on the outcome, in a line or two: the choice, never the behaviour that `Outcome` already states.
  Give its source: an interview answer, or what the request, the docs, the code or a rule settle.
  Label "Delegated:" a choice the user explicitly left to the planner, with the option taken.>

## Approach

### Design

<How the change is built behind the `Interface` of `Outcome`, which it names without restating.
Give the components and their responsibilities, the data model and the error handling, as a nested list by component.>

#### Runtime

- **`CUJ-01`:** <the components the journey crosses, in order, and what each one does in it>.

#### Cross-cutting concerns

- **Observability:** <the logs, metrics or traces the change adds or changes, or `none`>.
- **Security:** <who may trigger the change, what it trusts and what it exposes, or `none`>.
- **Privacy:** <the personal data it reads, stores or sends, and where, or `none`>.
- **Existing data:** <how the data, files and settings already there reach the new behaviour, or `none`>.
- **Rollback:** <how to undo the change once shipped, or `none`>.

### Decisions

#### <A technical decision: the approach, a data model, a location in the code, a name or the order of the tasks>

- **Source:** <the interview answer, the request, the docs, the code or the rule that settles it.
  Or "Delegated:" when the user explicitly left it to the planner>.
- **Discarded:** <an alternative not taken>: <one-line reason>; repeat it, or `none`.
- **Consequence:** <what the decision costs or makes possible later>.

### Risks

- <What could break, from the pre-mortem>: <how to mitigate it, or the cross-cutting concern that covers it>.

### Starting point

- **Read first:** `<path>`: <what it teaches about the area before Task 1>.
- **Baseline:** `<command>`, expected: <its result before any change, naming the failures that already exist>.

### Verification

- **`<command>`:** <expected result>.
  <Only the whole project's checks, such as the full test suite, the linter and the build.
  What proves a criterion goes to its `Verify:`.>

### Progress

- [ ] Task 1: <title of Task 1>
- [ ] Task 2: <title of Task 2>

### Tasks

#### Task 1: <What the task delivers>

<What the task delivers and how, in two or three sentences, naming the part of `Design` it builds.>

<How its tests prove it, and which behaviours of `Outcome` it completes.
When it is high risk, its items and how its tests and its differential check guard them; otherwise delete this.>

```yaml
covers: [<AC-01>, <AC-02>]
files:
  create:
    <path>: <what it holds>
  modify:
    <path>: <the functions, types or methods it changes, and how>
tests:
  <test file>:
    new:
      <TestName>: <CUJ-NN or AC-NN>; <what it checks>; <how it fails before the change>
    kept: [<TestName>]
rules: [<rule id>]
reuse:
  <function, type or utility> (<path>): <what for>
verification:
  <command scoped to the task's files>: <expected result>
differential:
  - <a step of the differential check against the base version>
```

<The block holds the task's data for the executor; the prose above holds what a reviewer reads.
Every value fits in one line: what needs explaining goes in the prose.
Delete `create`, `modify`, `reuse` or `differential` when empty; `covers: []` for a supporting task.
`tests: none, <why>` for a refactor the existing tests keep, or a change no test can observe.
Delete this note.>

#### Task 2: <What the task delivers>

<Same parts as Task 1, one per task, in the order they run.>

## Iterations

none
