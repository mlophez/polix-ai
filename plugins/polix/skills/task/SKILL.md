---
name: task
description: >
  Adds a task to the backlog of the project in the folder of tasks, from a template with a frontmatter, for /polix:plan.
  It asks only what the request leaves open, never plans the task nor writes code.
  It runs only when the user or the workflow of polix asks for it.
effort: medium
model: sonnet
user-invocable: true
disable-model-invocation: true
argument-hint: "[the task to add, e.g. the export times out with 10,000 rows; empty to take it from the conversation]"
---

# Adding a task

Turn a request into a task file of the backlog: what is wrong or missing, the outcome wanted and when it is done.
A task says what and why, never how: the plan that `/polix:plan` writes from it decides the approach and the steps.
The folders of tasks and of plans are those the start context of polix names, below the root.

## Requirements

- [template.md](template.md) next to this file.
  Without it, stop and tell the user.

## Input

- **Request**: the task to add, from the arguments or the conversation.
  Without either, ask the user.
- **Task**: a file of the folder of tasks to adjust, when the request names or refers to one.
  A task written earlier in this session is the task to adjust.

## Output

- **Task**: the path of the task file, its `type`, `priority` and `status`.
- **Open questions**: those the task keeps, or "none".
- **Next step**: plan it with `/polix:plan <path of the task file>`.

## Limits

- Write only the task file: never the code, a plan, the docs nor another task.
- Never commit nor stage the task file.
- Describe the outcome, never the implementation: no code blocks, snippets, signatures nor steps to build it.
- One task is one change a single plan can deliver; when the request holds several, propose one task for each.
- Never invent a requirement, a cost, a reference nor a figure the request or the user did not give.
- Ask in the language of the session.

## How to operate

Copy this checklist and track your progress:

```text
Task progress:
- [ ] Step 1: Resolve the request and the task to adjust, if any (see `Input`).
- [ ] Step 2: Look for the context in the code, the docs and the backlog (see Step 2).
- [ ] Step 3: Ask in one round what the template needs and the request leaves open (see Step 3).
- [ ] Step 4: Write the task file (see `Task file`).
- [ ] Step 5: Report (see `Output`), stop and return control to the user.
```

### Step 2: Look for the context in the code, the docs and the backlog

1. Read `AGENTS.md` when it is not in context, for the layout and the names of the project.
2. List the folder of tasks and the folder of plans, as the start context names them.
   When a task or plan already covers the request, show it and ask whether to adjust it instead.
3. Find with `rg` the commands, files or symbols the request names, to name them right in `Context`.
   Read only what names them: the plan explores the code later, so never study it in depth.

### Step 3: Ask in one round what the template needs and the request leaves open

- Ask only what changes the task: the `Problem` when its who or why is unclear, the `Outcome`, the `Done when`.
- Ask `type` and `priority` when the request does not settle them, each with the recommended option first.
- Use `AskUserQuestion`, or grouped free text without it, in at most one round of four questions.
- What the user leaves unanswered goes to `Open questions`; never fill it with a choice of yours.
- When nothing is open, skip the step and say so.

## References

### Task file

Path: `<tasks>/<name>.md`, where `<tasks>` is the folder of tasks the start context names; report the path.
`<name>` is short kebab-case English from the request.
When another task already has the name, choose a different one; this applies only to a new task.
Never rename nor move a task once written.

Create it from [template.md](template.md), with its sections in its order, and replace every `<...>`.
Write it in English; only the literals keep their language: the strings, messages, commands and data the request quotes.
Write one sentence per line, never wrapped, with no line over 120 characters and no tables.
Delete `Context` when it holds nothing, but keep every other section; write "none" in one that is empty.

The frontmatter, beyond what its placeholders say:

- `type`: `feature` for a new behaviour, `bug` for a wrong one, `refactor` for a change that keeps it.
  `chore` is for tooling, dependencies or the build; `docs` for the docs only; `idea` for one still to shape.
- `status`: `open` when written; `/polix:plan` sets `planned` with its `plan`.
  Set `done` or `dropped` only when the user says so; the build never touches the task.
- `priority`: `high` when it blocks users or other work, `medium` for the usual, `low` when it can wait.
- `reference`: the ticket, issue or link of the request, or `none`; never invent one.
- `plan`: `none`; `/polix:plan` writes the path of its plan.

The body:

- `Problem` holds the why and `Outcome` the what; neither says how.
- Each item of `Done when` is decidable by a person or a test, with its expected result, never "works well".
- An `idea` may leave `Done when` and `Scope` with "none" and carry its doubts in `Open questions`.

### Adjusting a task

Revise the file in place with targeted edits, never writing it again whole nor elsewhere.
Keep `created`, `plan` and `status`, unless the user asks to change the status.
