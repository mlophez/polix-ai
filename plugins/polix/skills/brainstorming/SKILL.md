---
name: brainstorming
description: >
  Explores the project and proposes new ideas to evolve it, and saves the ideas the user picks in the folder of tasks.
  It runs only when the user asks for it.
  It never judges an idea in depth, which is the skill probe, nor plans nor builds one.
user-invocable: true
disable-model-invocation: true
argument-hint: "[an area or goal to focus on, e.g. the hooks of Codex, onboarding; empty for the whole project]"
---

# Brainstorming

Explore the project, propose distinct ideas to evolve it and save those the user picks as tasks of the backlog.
The folder of tasks is the one the project names in `AGENTS.md`, `CLAUDE.md` or the session, `ai/tasks/` by default.

## Requirements

- [template.md](../task/template.md) of the skill `task`.
  Without it, stop and tell the user.

## Input

- **Focus**: the area, goal or pain to explore, from the arguments or the conversation.
  Without either, the whole project.

## Output

- **Ideas**: every idea proposed, one line each, marked saved or discarded.
- **Tasks**: the path of each task file written.
- **Next step**: the skill `probe` with the path of a task to check its viability, or `grill-me` to shape it.
- **Pending**: the ideas the user left for later, or "none".

## Limits

- Write only the task files of the ideas the user picks: never the code, a plan, the docs nor another file.
- Never commit nor stage a task file.
- Ground every idea on something read: a file at `file:line`, a doc, the history, a task or a plan.
- Never propose an idea a task or a plan of the project already covers; name it instead.
- Never present a cost, a demand nor a figure no source gives.
- Talk to the user in the language they write in.

## How to operate

Copy this checklist and track your progress:

```text
Brainstorming progress:
- [ ] Step 1: Resolve the focus (see `Input`).
- [ ] Step 2: Explore the project for signals (see Step 2).
- [ ] Step 3: Generate the ideas (see Step 3).
- [ ] Step 4: Present the ideas and ask which to save (see Step 4).
- [ ] Step 5: Write one task file per idea picked (see Step 5).
- [ ] Step 6: Report (see `Output`), stop and return control to the user.
```

### Step 2: Explore the project for signals

1. Read `AGENTS.md`, `README.md` and the docs of the focus.
2. List the folder of tasks and the folder of plans, `ai/plans/` by default, to know what is already covered.
3. Read `git log --oneline -n 100` for where the project is moving.
4. Look in the code of the focus with `rg` for what `Signals` lists.

Delegate a sweep to an `Explore` subagent only when reading it yourself would flood the context.
A report is a lead: read yourself each file an idea rests on.

### Step 3: Generate the ideas

- Generate eight to twelve ideas, then keep the five to eight strongest and most distinct.
- Cover at least three of the `Angles`, and include one bold idea that goes against the current direction.
- For each idea, write its title, the problem or opportunity, who gains, the signal it rests on and its effort.
  Rate the effort `low`, `medium` or `high`.

### Step 4: Present the ideas and ask which to save

- Number the ideas and group them by angle.
- Ask in one message which to save, which to discard and which to leave for later.
- The user may reword or merge ideas: record them as given.

### Step 5: Write one task file per idea picked

Follow `Task file` of the skill `task`, in [SKILL.md](../task/SKILL.md), with `type: idea`.
Set `priority` as the user gives it, `medium` otherwise.
Put the signal of the idea in `Context` and what is still unknown in `Open questions`.

## References

### Signals

- `TODO`, `FIXME` and `HACK` comments, and workarounds.
- Gaps between the docs and the code, and limits the docs admit.
- Areas with many fix or revert commits.
- Repeated code or manual steps a command could do.
- Features that exist for one agent, platform or case and not for the others.

### Angles

- Users: what a new or daily user misses or finds hard.
- Reliability: what fails, is fragile or hard to diagnose.
- Reach: new agents, platforms, integrations or kinds of repository.
- Simplicity: what to remove, merge or make implicit.
- Developer experience: the tests, the build, the release and the docs.
