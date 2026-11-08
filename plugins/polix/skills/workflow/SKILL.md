---
name: workflow
description: >
  Writes or changes polix.yaml, the workflow polix applies to the sessions of the agents on every event.
  Use it when the user asks to block, run or inject something in the workflow, or to require a check or a review.
  It never writes the rules (rules.path, ai/rules/ by default): that is the skill add-rule.
effort: medium
model: inherit
user-invocable: true
disable-model-invocation: false
argument-hint: "[what to enforce, e.g. no commit without tests; empty to review the current workflow]"
---

# Writing the workflow

Turn what the user wants to enforce into entries of `polix.yaml`, at the root of the repository.
An entry says when it applies (`when`, `unless`) and what it does (`action`, with one verb: `block`, `run` or
`inject`).
Conditions are facts of the repository, the event and what polix observed by itself: polix measures and never judges.
Judging the code belongs to a reviewer subagent, such as `polix:reviewer` or `polix:rules-reviewer`, whose end
the workflow requires with `finished`.
In `actor` and `finished`, an agent of a plugin goes with its prefix (`polix:reviewer`, never `reviewer`); an agent
of `.claude/agents/` goes by its bare name.

## Requirements

- `polix` on the `PATH`.
  Without it, stop and point to the skill `doctor` of polix.

## Input

- **Request**: what to enforce, from the arguments or the conversation.

## Rules

- Extend the current `polix.yaml`; never rewrite unrelated entries.
- Write a gate that must hold as a plain `block`, which blocks every attempt until its `unless` holds.
- Add `once: true` to a `block` only for a reminder the agent may rightly decline, such as checking whether the
  docs need an update.
- Write a warning as an `inject`, never as a `block`.
- Require a check with a `run` of polix; never with a condition on a command the AI ran, which never counts.
- Require a subagent with `unless: { finished: <type> }`, and name in the `changed` of the `when` every file it
  looks at.
- Write an exception as a `not` in `when`, or as a condition in `unless`; never put `contains` inside `any`.
- Order the entries of an event by priority: the first entry that blocks ends the evaluation.
- Add `always: true` only to a `run` whose result depends on more than the files, or to an `inject` that must
  reach the AI on every event.
- Put the reason of an entry in a YAML comment above it.
- Write the messages of `block`, `run` and `inject` in the language of the team, as an instruction the agent can
  follow.
- When `polix check` names an old name with its new spelling (`if`, `deny`, `record`, `done`, `turn_stop`...), rewrite it
  as told.
- When the change uses a key, event, condition or verb the file did not use yet, set `requires` to the output of
  `polix version`, unless it is `dev` or `requires` is already newer; never lower `requires`.
- Never remove an option `polix check` calls unknown before `polix update` rules out an outdated polix.
- Only write `polix.yaml`; rules of the code go to the folder of rules with the skill `add-rule` of polix.
- When `rules.path`, `plans.path` or `reports.path` changes, tell the user to move the files of the old folder.
- Talk to the user in the language of the session.

## References

### Format

[reference.yaml](reference.yaml) is an annotated example of every option of `polix.yaml`.

## How to operate

Copy this checklist and track your progress:

```text
Workflow progress:
- [ ] Step 1: Read reference.yaml (see `Format`), polix.yaml, the folder of rules and the Justfile, Makefile or package.json.
- [ ] Step 2: Ask until each wish maps to an event, its conditions and one verb.
- [ ] Step 3: Show the entries to add and confirm once before writing.
- [ ] Step 4: Write them and run `polix check` until it reports no problem.
- [ ] Step 5: Run `polix graph` and show the diagram of the events changed.
```

## Output format

- **Entries**: each entry added, with its event, its verb and whether it is a gate, a reminder or a warning.
- **Diagram**: the output of `polix graph` for the events changed.
- **Try it**: how to see each new gate act in a branch, with `polix log`, before it reaches the team.
- **Pending**: wishes left without an entry and why, or "none".
