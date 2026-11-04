---
name: workflow
description: >
  Writes or changes polix.yaml, the workflow polix applies to the sessions of the agents on every event.
  Use it when the user asks to block, require, run, inject or record something in the workflow.
  It never writes ai/rules/: that is the skill add-rule.
effort: medium
model: inherit
user-invocable: true
disable-model-invocation: false
argument-hint: "[what to enforce, e.g. no commit without tests; empty to review the current workflow]"
---

# Writing the workflow

Turn what the user wants to enforce into entries of `polix.yaml`, at the root of the repository.
Conditions are facts of the repository, the event and the state: polix measures and never judges.
Judging the code belongs to a reviewer subagent, such as `polix:rules-reviewer`, whose stop the workflow can record.

## Requirements

- `polix` on the `PATH`.
  Without it, stop and point to the skill `doctor` of polix.

## Input

- **Request**: what to enforce, from the arguments or the conversation.

## Rules

- Extend the current `polix.yaml`; never rewrite unrelated entries.
- Add every new blocking entry with `mode: audit`.
- Give every entry a `why` with its reason.
- Write the reasons of `deny` and `require` in the language of the team, as an instruction the agent can follow.
- Only write `polix.yaml`; rules of the code go to `ai/rules/` with the skill `add-rule` of polix.
- Talk to the user in the language of the session.

## References

### Format

[reference.yaml](reference.yaml) is an annotated example of every option of `polix.yaml`.

## How to operate

Copy this checklist and track your progress:

```text
Workflow progress:
- [ ] Step 1: Read reference.yaml (see `Format`), polix.yaml, ai/rules/ and the Justfile, Makefile or package.json.
- [ ] Step 2: Ask until each wish maps to an event, its conditions and one action.
- [ ] Step 3: Show the entries to add and confirm once before writing.
- [ ] Step 4: Write them and run `polix check` until it reports no problem.
- [ ] Step 5: Run `polix graph` and show the diagram of the events changed.
```

## Output format

- **Entries**: each entry added, with its event and mode.
- **Diagram**: the output of `polix graph` for the events changed.
- **Promotion**: move each entry to `warn` and then `enforce` once `polix log` shows it blocks what it should.
- **Pending**: wishes left without an entry and why, or "none".
