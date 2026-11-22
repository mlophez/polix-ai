---
name: probe
description: >
  Explores one idea with the user and decides whether it is viable, from its hypotheses, their tests and its risks.
  It runs only when the user or the workflow of the project asks for it.
  It never decides the what nor the how of the change, which is the skill grill-me, and edits nothing.
user-invocable: true
disable-model-invocation: false
argument-hint: "[the idea or a task file, e.g. ai/tasks/hooks-for-gemini.md; empty for the conversation]"
---

# Probing an idea

Turn an idea into the hypotheses it rests on, check what the project can check and return a verdict on its viability.
A viable idea goes next to the skill `grill-me`, which decides what to build and how.

## Input

- **Idea**: the idea, or a task file of the folder of tasks, from the arguments, else the conversation.
  Without either, ask for it.

## Output

- **Idea**: the idea in one sentence, and the problem it solves.
- **Hypotheses**: each one with its kind, its status (`confirmed`, `refuted` or `open`) and its evidence or its test.
- **Risks**: each risk with its impact and what would mitigate it.
- **Alternatives**: the cheaper or simpler ways to the same outcome, or "none".
- **Verdict**: `viable`, `viable with conditions` (naming them) or `not viable`, with the reason in one or two lines.
- **Next step**: `grill-me` for a viable idea, the tests of the open hypotheses, or drop it.
- **Pending**: the hypotheses only the user or an experiment can settle, or "none".

## Limits

- Never write, edit nor commit a file.
- Never settle the what nor the how of the change: note them for `grill-me`.
- Mark a hypothesis `confirmed` or `refuted` only with evidence: a `file:line`, a command and its output, or a document.
- Never present a cost, a demand nor a figure no source gives.
- Ask the user only what the files, git, the tools and the docs cannot answer, one question per message.
- Talk to the user in the language they write in.

## How to operate

Copy this checklist and track your progress:

```text
Probe progress:
- [ ] Step 1: Resolve the idea (see `Input`) and restate it with the problem it solves.
- [ ] Step 2: Ask the user what the idea leaves unclear about its problem or its goal, if anything.
- [ ] Step 3: Explore the project the idea touches: `AGENTS.md`, the docs, the code with `rg`, the tasks and plans.
- [ ] Step 4: List the hypotheses the idea rests on (see `Hypotheses`).
- [ ] Step 5: Check each hypothesis the project can answer, and design a cheap test for the rest (see Step 5).
- [ ] Step 6: Run a pre-mortem and list the risks (see Step 6).
- [ ] Step 7: Look for cheaper alternatives to the same outcome.
- [ ] Step 8: Give the verdict (see Step 8).
- [ ] Step 9: Report (see `Output`), stop and return control to the user.
```

### Step 5: Check each hypothesis the project can answer, and design a cheap test for the rest

- Check with reads and read-only commands: `rg`, `git log`, the `--help` of a tool, a dry run.
- For an open hypothesis, design the cheapest test that settles it: a question, a spike, a measure or a prototype.
  Give its effort and what result confirms or refutes it.
- Ask the user the hypotheses only they can settle, such as who needs it and how often.

### Step 6: Run a pre-mortem and list the risks

Imagine the idea built and abandoned six months later, and note the likeliest reasons.
Rate each by impact, `low`, `medium` or `high`, and give what would mitigate it.

### Step 8: Give the verdict

- `not viable` when a refuted hypothesis or a high risk without mitigation blocks the outcome.
- `viable with conditions` when open hypotheses or mitigations must hold first: name each condition.
- `viable` when the key hypotheses are confirmed and every high risk has a mitigation.
- When the idea came from a task file, suggest the user note the verdict in its `Open questions` or drop it.

## References

### Hypotheses

Cover the kinds that apply, one or more hypotheses each:

- **Value**: someone has the problem and the idea solves it better than today.
- **Feasibility**: the project, its dependencies and the platforms allow it.
- **Fit**: it agrees with the architecture, the rules and the direction of the project.
- **Cost**: its effort and its maintenance are worth its value.
- **Adoption**: the users will find it and use it, without breaking what they do today.
