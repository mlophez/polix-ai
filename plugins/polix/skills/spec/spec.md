---
branch: <the git branch the plan builds on>
status: planned
reference: <the ticket, issue, task file or link the change comes from, or none>
artifacts: []
created: <today as YYYY-MM-DD>
risk:
  level: <low | medium | high>
  reason: <the reason in one line>
agents:
  - name: builder
    model: <sonnet | opus | haiku | fable | inherit>
    effort: <low | medium | high | xhigh | max | inherit>
  - name: verifier
    model: <sonnet | opus | haiku | fable | inherit>
    effort: <low | medium | high | xhigh | max | inherit>
  - name: reviewer
    model: <sonnet | opus | haiku | fable | inherit>
    effort: <low | medium | high | xhigh | max | inherit>
tasks:
  T1: pending
acceptance:
  AC-01: pending
stats:
  start: none
  end: none
  duration: none
  iterations: none
  timeline: []
  code:
    files: none
    lines: none
---

<!--
Template of a plan. Every <...> is replaced, and every comment of this template is deleted from the plan.
Each fact lives in one place: others name it by its id (LIT, CUJ, AC, CON, DEC, T), never restate it.
A literal lives in the step of the first journey that shows it, and every other place names its id.
The frontmatter is the contract the build reads and writes; the body is prose the build never edits.
-->

# <One-line description of the change>

## Overview

<One sentence with the outcome, for a reviewer who reads nothing else.
The overview names no path, identifier or code jargon.>

**What changes**

- <What its users notice, in plain language; one to three items.>

**What does not change**

- <The main limit or what is left out; one to three items.>

**Risk and size**: <the risk in plain words and why, and the size of the change in plain words>

**Example**: <the id of the simplest journey, e.g. CUJ-01, or none when nothing is visible>

### Problem

<Who suffers what today, what it costs them and why it is addressed now, in two or three sentences.
Give the cost as a figure when the request or the interview gives one.>

### Affected

- **<a user, team or system the change reaches>**: <how it reaches it>

### Context

<How the code behaves today in the change's area, in two or three sentences.>

### Terms

- **<a project or domain term>**: <what it means here>

## Specification

### Outcome

<What exists once the change is done, as its users and callers see it: the behaviour, the outputs, the state left.
Give enough detail that two executors would build the same result, never how it is built.
This is the only place that states the behaviour; the others name it.>

### Interface

#### <A command, option, message, file, format, API, event or screen it adds or changes>

- <Its names, fields, defaults and errors, one item each.>

### Journeys

#### CUJ-01: <The journey of a user or caller, named by what they achieve>

As <the actor>, I want <the goal>, so that <the benefit>.

- **Kept**: <no | yes, when it holds an existing behaviour the change puts at risk>

**Given**

- <The starting state, with literal values.>

**When**

- <The input or action, quoting verbatim the command, message or value.>

**Then**

- <The exact result, as the output, the exit code or error and the state left.>
  LIT-01 (<exactly | contains>):

  ```text
  <the literal data, line by line as the code must reproduce it; <variable> for a value of each run>
  ```

**But**

- <What must not happen or must stay unchanged, such as a file not written; "None." when nothing must.>

### Scope

**In**

- <An area the plan touches, such as a module, command, file or screen.>

**Out**

- **<What the plan deliberately leaves out>**: <why>

### Constraints

- **CON-01**: <a limit on how the solution is built, such as compatibility, platforms, versions or a contract kept>
  Source: <request | interview | artifact | rule <id> | docs | code>.

### Acceptance criteria

#### AC-01: <Title of AC-01>

- **Requirement**: <one EARS sentence of the general rule, never the values of its journeys>
- **Journeys**: <CUJ-01, or none for a non-functional criterion>
- **Verify**:
  - Run `<command that runs only the tests or cases of this criterion, never the whole suite>`.
    Expect: <the result, naming the tests or cases that prove this criterion>.
  - Manual: <a step that needs a person>.
    Expect: <its result>.

## Decisions

### DEC-01: <The decision in a few words>

- **On**: <outcome | approach>
- **Choice**: <the choice in a line or two, never the behaviour the outcome states>
- **Source**: <interview | request | docs | code | rule <id> | delegated>
- **Discarded**:
  - **<An alternative not taken>**: <one-line reason>
- **Consequence**: <what it costs or makes possible later, or none>

## Approach

### Design

#### <A module, package or component>

- <Its responsibility, or a change to a type, function or contract other modules use; never a private helper.>

### Runtime

- **<CUJ-01>**: <the components the journey crosses, in order, and what each one does in it>

### Cross-cutting

- **Observability**: <the logs, metrics or traces the change adds or changes, or none>
- **Security**: <who may trigger the change, what it trusts and what it exposes, or none>
- **Privacy**: <the personal data it reads, stores or sends, and where, or none>
- **Existing data**: <how the data, files and settings already there reach the new behaviour, or none>
- **Rollback**: <how to undo the change once shipped, or none>

### Risks

- **<What could break, from the pre-mortem>**: <how to mitigate it, or the cross-cutting concern that covers it>

### Verification

- Run `<a check of the whole project, such as the full test suite, the linter or the build>`.
  Expect: <its result>.

## Tasks

### T1: <What the task delivers>

<What the task delivers and how, in two or three sentences, naming the part of the design it builds.
Never its tests nor its criteria: Tests and Covers hold them.>

- **Covers**: <AC-01, or none for a supporting task>
- **Risk**: <none, or its items of high risk and how its tests and differential guard them>
- **Files**:
  - Create: <the paths the behaviour of the task lives in, or none>
  - Modify: <the paths, or none; never the callers, fixtures and comments that only follow: the build finds them>
- **Expected files**: <n, the files the task is expected to change, callers included>
  <Over 20, split it, or a DEC says why.>
- **Tests**: <none, when it has none, and its summary says why>
  - `<test file>`:
    - New `<TestName>` (<CUJ-NN or AC-NN>): <what it checks>.
    - Kept `<TestName>`.
- **Rules**: <the rule ids, or none>
- **Reuse**:
  - `<function, type or utility>` in `<its path>`: <what for>.
- **Differential**: <none, or for a high-risk task the steps of its differential check against the base version>
