---
name: grill-me
description: >
  Interviews the user one question at a time about a plan, decision or idea until both share one understanding of it.
  It puts each open decision with its alternatives, their pros and cons and a recommended answer, finds the facts itself and acts on nothing.
  It runs only when the user or the workflow of polix asks for it.
effort: high
model: inherit
user-invocable: true
disable-model-invocation: true
argument-hint: "[the plan, decision or idea to grill, e.g. split the hooks by agent; empty for the conversation]"
---

# Grilling a plan

Interview the user until you both share one understanding of a plan, decision or idea, with nothing left assumed.
Map the subject as a design tree: each decision branches into the decisions that hang off it.
The frontier is every open decision whose prerequisites are settled: what you can ask now without guessing answers.

## Input

- **Subject**: the plan, decision or idea, from the arguments, else the conversation; ask for it when neither gives one.

## Output

- **Decisions**: each decision of the tree with the answer the user settled, one line each.
- **Facts**: each fact a decision rests on, with its source (`file:line`, a command or a document).
- **Pending**: the decisions the user left open or deferred, or "none".

## Limits

- Never act on the subject (edit, plan, build, commit) before the user confirms the shared understanding.
- Never ask the user a fact you can find in the files, git, the tools or the docs.
- Never settle a decision yourself: put each one to the user, even when you recommend its answer.
- Ask one decision per call of `AskUserQuestion`, or per message without it, and wait for its answer.
  Never two decisions in one call or message, nor a list of them.
- Never put a decision with fewer than two alternatives or more than four, nor an alternative without its pros and cons.
- Never end while the frontier has a decision: every branch is visited, none silently assumed.
- Talk to the user in the language of the session.

## How to operate

Copy this checklist and track your progress:

```text
Grilling progress:
- [ ] Step 1: Resolve the subject (see `Input`) and note what the conversation and the repository already settle.
- [ ] Step 2: Map the design tree and compute the frontier.
- [ ] Step 3: Launch a subagent for each fact the frontier needs, and do not wait for it before the question.
- [ ] Step 4: Pick the next decision of the frontier, ask it alone (see `Question`) and wait for the answer.
- [ ] Step 5: Record the answer and the facts reported, and recompute the frontier.
- [ ] Step 6: Repeat Steps 3 to 5 until the frontier is empty.
- [ ] Step 7: Give the `Output` and ask the user to confirm the shared understanding.
```

### Step 3: Launch a subagent for each fact the frontier needs, and do not wait for it before the question

Launch the agent `Explore`, with a fresh context, and give it the question the fact answers.
A fact still being searched is an unsettled prerequisite: ask first the decisions that do not hang off it.

### Step 4: Pick the next decision of the frontier, ask it alone and wait for the answer

Pick the decision that unblocks the most branches; on a tie, the one whose answer is hardest to change later.
Tell the user how many decisions the frontier holds, so the question reads as `n of m` known so far.
Ask it with `AskUserQuestion` (see `Question`); without it, as in Codex, write it as text (see `Question as text`).

### Step 5: Record the answer and the facts reported, and recompute the frontier

A settled decision unblocks the decisions that hang off it, and an answer may add or prune branches.
A decision the user defers goes to `Pending` and blocks only its branch.
An answer that picks none of the alternatives is valid: record it as given and ask about it only when it is ambiguous.

### Step 7: Give the `Output` and ask the user to confirm the shared understanding

Write the `Output` as text, then ask with `AskUserQuestion` whether to confirm it or adjust it, confirm first.
The question says the `Output` is just above it, since the dialog may hide it.
An adjustment reopens the decisions it touches: go back to Step 4 for them.
Without `AskUserQuestion`, ask for the confirmation in the text after the `Output`.

## References

### Question

Ask each decision with `AskUserQuestion`, one question per call, every field in the language of the session:

- **header**: the number of the decision and the progress, at most 12 characters.
- **question**: the title of the decision and why it matters.
  It stands on its own: it names the subject and what you found that leaves it open, since the dialog may hide the text before it.
- **options**: two to four alternatives, each description with its pros and cons in one or two sentences.
  With more than four, drop or merge the weakest: `Other` still takes any alternative the user writes.
- **recommended**: the first option, its label ending in `(Recommended)` and its description saying why.

```json
{
  "header": "Q3 (3 of 5)",
  "question": "Storage of the sessions: where polix writes each session decides how the folder grows and what deleting one costs. Where does it write them?",
  "multiSelect": false,
  "options": [
    {"label": "One JSONL file per day (Recommended)", "description": "No command deletes a single session. Pros: the folder stays small; a day reads in one open. Cons: deleting one session rewrites the whole file."},
    {"label": "One JSONL file per session", "description": "Pros: deleting a session is removing a file; no write contends with another session. Cons: thousands of small files after a few months."},
    {"label": "One SQLite database", "description": "Pros: queries by session or date without parsing. Cons: a new dependency and a binary format no one can read with cat."}
  ]
}
```

### Question as text

Without `AskUserQuestion`, give the decision a number and a title, say why it matters, then list its alternatives with their pros and cons.
End with your recommended alternative and the reason, worded so that "yes" accepts it:

```markdown
**Q3 (3 of 5) - Storage of the sessions**

Where polix writes each session decides how the folder grows and what deleting a session costs.

**A. One JSONL file per day**
- Pros: the folder stays small; a day reads in one open.
- Cons: deleting one session rewrites the whole file.

**B. One JSONL file per session**
- Pros: deleting a session is removing a file; no write contends with another session.
- Cons: thousands of small files after a few months.

**C. One SQLite database**
- Pros: queries by session or date without parsing.
- Cons: a new dependency and a binary format no one can read with `cat`.

Recommended: A, one file per day, since no command deletes a single session. Reply yes, or pick another option.
```
