---
name: grill-me
description: >
  Interviews the user one decision at a time until both share what they want of a change.
  It covers the what and the how only the user should decide, explores the code and returns a spec, editing nothing.
  It runs only when the user or the workflow of the project asks for it.
effort: high
model: inherit
user-invocable: true
disable-model-invocation: false
argument-hint: "[the change, decision or idea to grill, e.g. split the hooks by agent; empty for the conversation]"
---

# Grilling a change

Interview the user until you both share one understanding of a change, with nothing left assumed.
Map the subject as a design tree: each decision branches into the decisions that hang off it.
The frontier is every open decision whose prerequisites are settled: what you can ask now without guessing answers.
The what comes first: every decision of the how hangs off the decisions of the what.
The spec it returns feeds the skill `plan`, or the agent that edits the code directly in the flow without a plan.
The folder of rules is the one the project names in `AGENTS.md`, `CLAUDE.md` or the session, `ai/rules/` by default.

## Input

- **Subject**: the change, decision or idea, from the arguments, else the conversation; without either, ask for it.
  A single decision is a tree with one root: the same steps, and the sections of `Spec` it leaves empty say "none".
- **Known**: the facts and decisions already settled, from the conversation or from the skill that invokes this one.
  Never search them again nor ask them.
- **Adjust**: a spec confirmed earlier and what reopens it, such as the findings of the subagent `critic` on a plan.
  Then map only the decisions it reopens and those that hang off them.

## Output

- **Spec**: the sections of `Spec`, in its order, written in the language of the session.
- **Pending**: the decisions the user left open or deferred, or "none".

## Limits

- Never write, edit nor commit a file, and never act on the subject before the user confirms the spec.
- Never ask the user a fact you can find in the files, git, the tools or the docs.
- Never settle a decision yourself: put each one to the user, even when you recommend its answer.
- Ask one decision per call of `AskUserQuestion`, or per message without it, and wait for its answer.
  Never two decisions in one call or message, nor a list of them.
- Ask only the decisions of the how that `What and how` names; leave the rest to whoever builds the change.
- Never end while the frontier has a decision: every branch is visited, none silently assumed.
- Talk to the user in the language they write in (see `Questions`).

## How to operate

Copy this checklist and track your progress:

```text
Grilling progress:
- [ ] Step 1: Resolve the subject, Known and Adjust (see `Input`), and note what the conversation already settles.
- [ ] Step 2: Explore the project and the code the subject touches (see Step 2).
- [ ] Step 3: Draft the approaches, the cases and the pre-mortem (see Step 3).
- [ ] Step 4: Map the design tree and compute the frontier (see Step 4).
- [ ] Step 5: Find the facts the next decisions need as Step 2 says; ask meanwhile one whose facts are known.
- [ ] Step 6: Pick the next decision of the frontier, ask it alone (see `Question`) and wait for the answer.
- [ ] Step 7: Record the answer and the facts reported, and recompute the frontier (see Step 7).
- [ ] Step 8: Repeat Steps 5 to 7 until the frontier is empty, then ask each deferred decision again, alone.
- [ ] Step 9: Give the `Spec` and ask the user to confirm it (see Step 9).
```

### Step 2: Explore the project and the code the subject touches

1. Read `AGENTS.md` and `CLAUDE.md` when they are not in context, and the rules of the folder of rules.
2. Find with `rg` the areas the subject touches: the modules, packages or commands.
3. Explore each area with `rg` and targeted reads, never whole directories, for what `Explore prompt` lists.
4. The code wins over the docs when they disagree, and the gap goes to `Facts`.

Delegate a sweep to an `Explore` subagent only when reading it yourself would flood the context.
Launch at most two, in one message, with the prompt of `Explore prompt`, breadth "medium" and the model `haiku`.
A report is a lead, never a fact: read yourself each file a fact of `Facts` names.

### Step 3: Draft the approaches, the cases and the pre-mortem

1. **Approaches**: when more than one reasonable approach exists, draft two to four distinct ones.
   For each, its core idea, how it fits the architecture and its trade-offs: complexity, risk, effort, blast radius.
   Discard, with a one-line reason, those that break a rule, a constraint or the architecture.
2. **Cases**: walk the change against each item of `Cases` and note those the subject does not settle.
3. **Pre-mortem**: imagine the change shipped and failed a month later; note the likeliest reasons.
   Each gets a mitigation; a mitigation with more than one reasonable option is a decision of the how.
4. **Size**: estimate whether the change fits one review.
   When it does not, the split is the first decision: by function, never by layer nor into preparatory steps.
5. Settle a case only when the subject, the docs, the code or a rule settle it; every other case is a decision.

### Step 4: Map the design tree and compute the frontier

1. Map each open decision under the decisions it depends on.
2. The problem, when unclear enough to change the outcome, and the split hang off nothing: they go first.
3. Every decision of the how hangs off the decisions of the what, so the how is never asked while the what is open.
4. The choice among the approaches hangs off the outcome, and the other decisions of the how hang off it.

### Step 7: Record the answer and the facts reported, and recompute the frontier

- A settled decision unblocks the decisions that hang off it, and an answer may add or prune branches.
- An answer that changes a decision already settled reopens the decisions that hang off it.
- A decision the user defers goes to `Pending` and blocks only its branch.
- An answer that picks none of the alternatives is valid: record it as given and ask about it only when ambiguous.
- Only when the user leaves a decision to you explicitly, take the recommended option with the source `delegated`.

### Step 9: Give the `Spec` and ask the user to confirm it

- Write the `Spec` as text, then ask with `AskUserQuestion` whether to confirm it or adjust it, confirm first.
  The question says the `Spec` is just above it, since the dialog may hide it.
  Without `AskUserQuestion`, ask for the confirmation in the text after the `Spec`.
- An adjustment reopens the decisions it touches: go back to Step 6 for them.
- When a skill invoked this one, return the confirmed `Spec` to it and go on with that skill's checklist.

## References

### What and how

- The what is everything a user or a caller of the change sees: the problem and who suffers it, the outcome, the
  contract, the exact result of each case, the scope, the constraints and the thresholds of the criteria.
- Ask a decision of the how only when it has more than one reasonable option and one of these holds:
  - It changes the behaviour a user or a caller sees.
  - It changes the cost, the risk or the effort of the change.
  - It is hard to undo: the approach, the data model, a contract other modules use, a new dependency, compatibility.
- Never ask a private helper, an internal name nor the order of the work: whoever builds the change decides them.

### Cases

- Inputs: empty, missing, duplicated, too large, malformed, with special characters, at each boundary.
- Failures: each dependency, file, network call or command that can fail, time out or answer late.
- State: the data, configuration and files already there, and how they reach the new behaviour.
- Interactions: the features, commands and callers that share the code or the data the change touches.
- Compatibility: older versions, platforms, settings and users of the contracts that change.
- Operation: concurrent runs, retries, partial runs and how to undo the change.
- Security: who may trigger the change, what it exposes and what it trusts.

### Spec

Every section says "none" when it holds nothing; a fact or a decision lives in one section only.

- **Problem**: who suffers what, its cost and why now; a cost is a figure only when the user or a source gives one.
- **Affected**: the users, teams and systems the change touches.
- **Terms**: each term a reader new to the subject would not know, with its meaning.
- **Outcome**: the result as its users and callers see it, never how it is built.
- **Contract**: each command, option, message, file, format, API and event it adds or changes.
  Give their names, fields, defaults and errors.
- **Journeys**: one per behaviour, boundary and family of errors, as given, when, then and what must not happen.
  The exact data the code must reproduce goes verbatim, once, in the journey that first shows it.
- **Scope**: what the change touches, and what it leaves out and why.
- **Constraints**: the compatibility, platforms, versions and dependencies the change must keep.
- **Criteria**: what proves the outcome; a non-functional one with its threshold, a manual one with its steps.
- **Approach**: the approach chosen and those discarded with a one-line reason, and the design the user settled.
- **Risks**: each reason of the pre-mortem with its mitigation.
- **Decisions**: each decision, one line each, with its answer, `what` or `how`, and its source.
  The source is the subject, the docs, the code, a rule by id, an answer of the user or `delegated`.
- **Facts**: each fact a decision rests on, with its source (`file:line`, a command or a document).

### Explore prompt

Send an `Explore` subagent this block, keeping only the items you need from it:

```text
Area: <the module, package or command to explore, with its paths>
Subject: <the subject, in one or two sentences>
Known: <the paths and facts you already found, so it does not search them again>
Question: <the one thing to find, such as every caller of a symbol>
Return, with paths and symbols, never whole files nor what Known already says:
- The current behaviour of the area and its why, from the pages of docs/ on it and from the code.
- The code paths the subject goes through, and where the docs and the code disagree.
- The utilities, types and patterns to reuse, and the minimal set of files to change.
- The tests of those files and the commands that run them.
- The commands, options and functions that call the code to change.
```

### Questions

- Write every question in the language the user writes in.
  That is the question, its header, the label, description and preview of each option, and the text before the call.
  Only the names of the code, commands and literals keep their spelling.
- Never put a decision with fewer than two alternatives or more than four, nor an alternative without its pros and cons.
  With more than four, drop or merge the weakest: `Other` still takes any alternative the user writes.
- A fact only the user knows is a decision too: who suffers the problem, a threshold, the steps of a manual check.
  Its alternatives are two to four hypotheses drawn from the subject, the docs and the code, each with what it implies.
  A hypothesis is never a figure or a cause no source gives: `Other` takes the answer the user writes.
- Write a longer comparison as text before the call, and keep the question answerable without it.
- Ask an unanswered question again; never fill it with a default.

### Question

Ask each decision with `AskUserQuestion`, one question per call, every field in the language of the session:

- **header**: the number of the decision and the progress, at most 12 characters.
- **question**: the title of the decision and why it matters.
  It stands on its own: it names the subject and what leaves it open, since the dialog may hide the text before it.
- **options**: two to four alternatives, each description with its pros and cons in one or two sentences.
  They say how the change behaves with it and its effort, risk and tests.
- **recommended**: the first option, its label ending in `(Recommended)` and its description saying why.

```json
{
  "header": "Q3 (3 of 5)",
  "question": "Storage of the sessions: where the service writes each session decides how the folder grows and what deleting one costs. Where does it write them?",
  "multiSelect": false,
  "options": [
    {"label": "One JSONL file per day (Recommended)", "description": "No command deletes a single session. Pros: the folder stays small; a day reads in one open. Cons: deleting one session rewrites the whole file."},
    {"label": "One JSONL file per session", "description": "Pros: deleting a session is removing a file; no write contends with another session. Cons: thousands of small files after a few months."},
    {"label": "One SQLite database", "description": "Pros: queries by session or date without parsing. Cons: a new dependency and a binary format no one can read with cat."}
  ]
}
```

### Question as text

Without `AskUserQuestion`, as in Codex, give the decision a number and a title, and say why it matters.
Then list its alternatives with their pros and cons.
End with your recommended alternative and the reason, worded so that "yes" accepts it:

```markdown
**Q3 (3 of 5) - Storage of the sessions**

Where the service writes each session decides how the folder grows and what deleting a session costs.

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
