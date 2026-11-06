---
name: plan
description: >
  Writes a self-contained implementation plan in the folder of plans (ai/plans/ by default) for a feature, refactor
  or bugfix, before any code.
  It interviews the user on what the request leaves open and replaces plan mode.
  Use it to plan a change or to adjust a plan; it never writes code, branches or commits.
effort: high
model: inherit
user-invocable: true
disable-model-invocation: false
argument-hint: "[what to plan, e.g. add retries to the webhook sender; empty to take it from the conversation]"
---

# Planning

Turn a request into a plan file that any executor runs without asking: what to change, where, why and how to verify it.
The plan describes the change; it never holds code.
The folder of plans is `plans.path` of `polix.yaml` and the folder of rules is `rules.path`.
Without them, `ai/plans/` and `ai/rules/`; polix names both at session start.

## Requirements

- [template.md](template.md) next to this file.
  Without it, stop and tell the user.

## Input

- **Request**: what to plan, from the arguments or the conversation.
  Without either, ask the user.
- **Plan**: the file of the folder of plans to adjust, when the request names or refers to one.

## Rules

- Never call `EnterPlanMode`.
  When the session is already in plan mode, present the plan with `ExitPlanMode`; once approved, write the plan file.
- Explore read-only: the plan file is the only file you write.
- Define tasks, never code: no code blocks, snippets, implementations or signatures.
  Describe in prose what changes and why, naming the paths, symbols and contracts involved.
- One plan is one change, delivered whole: never phases or follow-up plans.
- When the scope is too big, propose splitting it into plans, each with its outcome and dependencies.
  Split by function, never by layer (data, API, UI) or into preparatory steps.
  Once the user confirms, plan only the part they pick and list the others in `Out of scope`.
- Reference the rules of the folder of rules by id in the `Rules:` of each task, never restate them.
- Name no executor, agent or tool in the plan file.
- Interview in the language of the session.
  Write the plan file in the language of the docs of the repository, English when there are none.
- Write the plan file one sentence per line, each line at most 120 characters, with no tables.

## References

### Project context

- Read `AGENTS.md` and `CLAUDE.md` when they are not in context: purpose, stack, commands, layout and testing.
  Run a command as they write it, never rebuild it.
- Read every file of the folder of rules except `retired.yaml`, whose ids never apply.
- A rule applies when the trigger of its `description` holds and the files touched match its `scope`.
  Without `scope`, it applies to the whole project.
- A rule with `default: true` allows a departure, with its reason in `Decisions`; any other rule is mandatory.
  A task never breaks a mandatory rule: when the design needs it, stop and tell the user.
- Two rules with the same `scope` that contradict each other: stop and report both ids, never choose one.
- The `check` of the rules of the tasks and the commands of `AGENTS.md` feed `Verification`.
- Without facts in `AGENTS.md` or without the folder of rules, follow the conventions of the surrounding code.
  Say what is missing in `Missing docs`.

### Design baseline

Design with it, do not restate it in the plan.
The rules of the project and the surrounding code win over it.

- Reuse existing functions and utilities; find them with `rg` before planning new ones.
- Small units with one responsibility each; build what the request needs, no speculative abstraction.
- Explicit error handling: name the errors and how each one is reported, never swallow them.
- Validate input at the boundaries, and keep pure logic apart from I/O so it is testable without mocks.
- Prefer composition over inheritance.

### Plan file

Path: `<plans>/<name>.md`, where `<plans>` is the folder of plans and `<name>` is short kebab-case English from the
request, with no date.
Example: `ai/plans/add-retry-to-webhook-sender.md`.
When another plan already has the name, choose a different one.

Fill [template.md](template.md): its sections in its order, every `<...>` replaced by what it describes.
Tasks are `### Task 1: ...`, `### Task 2: ...`, ordered, each implementable and verifiable on its own.

- `Acceptance criteria` are numbered `AC1`, `AC2`...
  Never renumber them: the `Covers:` of the tasks point to them.
- Reference code by path and symbol (function, type, method), never by line number.
- The test and edge cases agreed in the interview go to the `Tests:` of their task.
- `Rules:` lists every rule that applies to the task (see `Project context`).

Never write placeholders:

- "TBD", "TODO", "implement later", "fill in details".
- "Add appropriate error handling" or "handle edge cases" without saying which and how.
- "Similar to Task N": tasks are read in isolation.
- Types, functions or files defined neither in a task nor in the code.

### Adjusting a plan

Revise the file in place: keep the ids of its acceptance criteria and add new ones after the last.
Rewrite the tasks the adjustment touches and leave the others as they are.

## How to operate

Copy this checklist and track your progress:

```text
Plan progress:
- [ ] Step 1: Resolve the request and the plan to adjust, if any (see `Input`).
- [ ] Step 2: Read the project context (see `Project context`).
- [ ] Step 3: Explore the docs and the code with `rg` and targeted reads (see Step 3).
- [ ] Step 4: Interview the user on what the request, the docs and the code leave open (see Step 4).
- [ ] Step 5: Write the plan file (see `Plan file`), or revise it (see `Adjusting a plan`).
- [ ] Step 6: Self-review the plan file and fix every gap (see Step 6).
- [ ] Step 7: Stop and return control to the user.
```

### Step 3: Explore the docs and the code with `rg` and targeted reads

- Start from the pages of `docs/` on the area of the request, for the current behaviour and its why.
- Trace the real code paths: the code wins when they disagree, and the gap goes to `Pending`.
- Find the utilities to reuse, the minimal set of files to change, and their tests and test commands.

### Step 4: Interview the user on what the request, the docs and the code leave open

- Ask about the outcome, the acceptance criteria and the limits of the scope when the request leaves them open.
- Ask about the technical design: contracts, data model, error handling, code location, names and task order.
  Also about test and edge cases, and every choice with more than one reasonable option.
- Never ask what the request, the docs or the code already answer; when nothing is open, skip the interview and say so.
- Use `AskUserQuestion` in one batch, or grouped free text without it.
  Give each question the options seen and a recommended default; use the previews for short trade-offs.
- An unanswered question takes the recommended default, recorded as an assumption in `Decisions`.

### Step 6: Self-review the plan file and fix every gap

1. **Coverage**: every requirement and answer of the interview maps to a task or to `Verification`.
   Every acceptance criterion is in a `Covers:`.
2. **Placeholders**: none of the patterns of `Plan file` and no `<...>` left from template.md.
3. **Consistency**: names, contracts and paths match across tasks and with the real code.
4. **Rules**: every id in a `Rules:` exists and is not retired.
   No rule whose `scope` matches the `Files:` of a task is missing from its `Rules:`.
5. **One change**: nothing is deferred to another phase or plan, except the parts of a confirmed split.
6. **No code**: no code blocks, snippets or implementations.

## Output format

- **Plan**: the path of the plan file.
- **Overview**: the `Overview` section of the plan, verbatim.
- **Decisions**: the assumptions taken for unanswered questions, or "none".
- **Next steps**: adjust the plan, implement it, or review the result with `/polix:review`.
- **Pending**: open blockers and stale or missing docs, or "none".
