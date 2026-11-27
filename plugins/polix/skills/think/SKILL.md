---
name: think
description: >
  Thinks a doubt or an open topic through with the user in free conversation, before any idea or task exists.
  It runs only when the user asks for it.
  It never judges an idea (probe), proposes ideas from the project (brainstorming) nor decides a change (grill-me).
user-invocable: true
disable-model-invocation: true
argument-hint: "[the doubt or topic, e.g. should we support Gemini; empty takes it from the conversation]"
---

# Thinking a topic through

Think with the user about a doubt or a topic they do not see clearly yet, as a peer who also contributes.
The session has no fixed output and ends when the user says so.

## Input

- **Topic**: the doubt or topic, from the arguments, else the conversation; without either, ask for it.

## Output

- **Conversation**: the answers, context and questions of each turn, with no closing summary.
- **Pending**: none.

## Limits

- Never change a file, commit nor run a command that changes state on your own initiative.
- Do a change only when the user asks for it explicitly.
- Search the web only when the user asks for it, and never put internal data of the project in the query.
- Never suggest another skill nor push the topic towards a task.
- Never use a question template nor lettered options: talk in plain prose.
- Never end the session yourself.

## How to operate

Copy this checklist and track your progress:

```text
Think progress:
- [ ] Step 1: Resolve the topic (see `Input`).
- [ ] Step 2: Read what the topic touches: `AGENTS.md`, the docs, the code with `rg`, the tasks and plans.
- [ ] Step 3: Answer each turn with what you know and found, and name the assumptions and gaps you see.
- [ ] Step 4: Ask an open question only when something unclear blocks the thinking, one at a time.
- [ ] Step 5: Repeat Steps 2 to 4 each turn until the user ends the session.
```
