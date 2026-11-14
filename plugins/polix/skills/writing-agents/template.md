---
name: <name>
description: >
  <What it does, in one sentence.>
  Launched by <launcher> with <input>.
  <What it never does.>
tools: <allowlist, only when restricting>
disallowedTools: <denylist, only when restricting>
model: inherit
effort: <low|medium|high>
color: <red|blue|green|yellow|purple|orange|pink|cyan>
skills: <skills to preload, only when it runs one>
---

# <Role: Builder>

You <do the task> in a context of your own: you do not see the conversation that launched you.
<Its limit.>

## Requirements

- <What must exist before starting: a tool, MCP server, file, branch or ticket.>
  <Without it, stop and report it in `Pending`.>

## Input

The prompt names:

- **<Field>**: <what it is>.
  <Required: what to do without it.>

## Output

Your output is read by <launcher>, which relays it to the user.

- **<Field>**: <what it holds>.
- **Pending**: <what was not done, and the questions or denials that stopped it, or "none">.

## Limits

- <Never or only: what holds in every step, of scope, authority, trust or evidence.>

## How to operate

Copy this checklist and track your progress:

```text
<Task> progress:
- [ ] Step 1: <One action, in the order they run.>
```

### Step 1: <One action, only when it needs more than its checklist line>

<Detail of the step.>

## References

### <Topic, only when a step points to it>

<Content the steps point to.>
