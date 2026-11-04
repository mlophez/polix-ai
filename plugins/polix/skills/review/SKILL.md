---
name: review
description: >
  Reviews the current change with the subagent polix:reviewer: its intent, bugs, the rules of ai/rules/, its tests
  and its docs, and relays the verdict.
  Use it before a commit or a pull request, or when the user asks to review the change.
  It never fixes what it finds.
effort: medium
model: inherit
user-invocable: true
disable-model-invocation: false
argument-hint: "[range or files, e.g. HEAD~3..HEAD or internal/setup; empty for the current change]"
---

# Reviewing

Review the change with one `polix:reviewer` subagent and relay its report.
You write the intent, which only this session knows; the subagent judges the code.

## Requirements

- The subagent `polix:reviewer`.
  Without it, as in Codex, read `../../agents/reviewer.md` next to this skill and follow it yourself in this session.

## Input

- **Change**: the range or files from the arguments.
  Without them, the current change: the branch against its base plus what is staged, not staged and untracked.
- **Intent**: what the change must achieve, from the conversation: the request, the plan of `ai/plans/` or the ticket.
  Without any, leave it empty and the subagent infers it.

## Rules

- Never edit a file during the review.
- Relay the findings of the subagent as they are: add none of your own and drop none.
- Talk to the user in the language of the session.

## References

### Prompt

Send the subagent this block, in the language of the session:

```text
Change: <range or files, or "the current change">
Intent: <one to three sentences, or "none">
```

## How to operate

Copy this checklist and track your progress:

```text
Review progress:
- [ ] Step 1: Resolve the change (see `Input`); when `git status` and the diff show nothing, tell the user and stop.
- [ ] Step 2: Write the intent from the conversation (see `Input`).
- [ ] Step 3: Launch `polix:reviewer` with the prompt (see `Prompt`) and wait for its report.
- [ ] Step 4: Relay the report (see `Output format`).
- [ ] Step 5: Stop and return control to the user.
```

## Output format

- **Verdict**: the verdict of the subagent and the intent it reviewed, noting when it was inferred.
- **Findings**: the findings, by severity, with `file:line`, what is wrong and the direction of the fix.
- **Checks**: the checks run and their result.
- **Pending**: what the subagent could not review, and the question of which findings to fix.
