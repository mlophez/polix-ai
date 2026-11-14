---
name: <name>
description: >
  <What it does, in one sentence.>
  Use it when <trigger>.
  <What it never does, only when easy to confuse.>
effort: <low|medium|high>
model: inherit
user-invocable: true
disable-model-invocation: <false by default; true only when the user alone launches it>
paths: "<glob of the files it works on, only when the model should see it only while on them>"
argument-hint: "<[arguments, e.g. example; what empty means], only when it takes arguments>"
allowed-tools: <exact commands to pre-approve, only when needed>
---

# <Task as a gerund: Committing, Reviewing>

<What the skill does and its limit, in at most 3 sentences.>

## Requirements

- <What must exist before starting: a tool, MCP server, file, branch, ticket or another skill it invokes.>
  <What to do when it is missing.>

## Input

- **<Field>**: <what it is and where it comes from: arguments, conversation, git, Jira>.

## Output

- **<Field>**: <what it holds>.
- **Pending**: <what was not done or is blocked, or "none">.

## Limits

- <Never or only: what holds in every step, of scope, authority, trust or evidence.>

## How to operate

Copy this checklist and track your progress:

```text
<Task> progress:
- [ ] Step 1: <One action, in the order they run.>
- [ ] Step 2: <Stop and return control to the user, only for phases of the development flow.>
```

### Step 1: <One action, only when it needs more than its checklist line>

<Detail of the step.>

## References

### <Topic, only when a step points to it>

<Content the steps point to.>
