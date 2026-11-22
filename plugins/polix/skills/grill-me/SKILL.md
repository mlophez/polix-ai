---
name: grill-me
description: >
  Interviews the user one decision at a time until both share what they want of a change.
  It covers the what and the how only the user should decide, explores the code and returns a brief, editing nothing.
  It runs only when the user or the workflow of the project asks for it.
user-invocable: true
disable-model-invocation: false
argument-hint: "[the change, decision or idea to grill, e.g. split the hooks by agent; empty for the conversation]"
---

# Grilling a change

Interview the user until you both share one understanding of a change, with nothing left assumed.
Map the subject as a design tree: each decision branches into the decisions that hang off it.
The frontier is every open decision whose prerequisites are settled: what you can ask now without guessing answers.
The what comes first: every decision of the how hangs off the decisions of the what.
The brief it returns feeds the skill `spec`, or the agent that edits the code directly in the flow without a plan.
Run on its own, it then asks the user which of the two comes next.
The folder of rules is the one the project names in `AGENTS.md`, `CLAUDE.md` or the session, `ai/rules/` by default.

## Input

- **Subject**: the change, decision or idea, from the arguments, else the conversation; without either, ask for it.
  A single decision is a tree with one root: the same steps, and the sections of `Brief` it leaves empty say "none".
- **Known**: the facts and decisions already settled, from the conversation or from the skill that invokes this one.
  Never search them again nor ask them.
- **Adjust**: a brief confirmed earlier and what reopens it, such as the findings of the subagent `critic` on a plan.
  Then map only the decisions it reopens and those that hang off them.

## Output

- **Brief**: the sections of `Brief`, in its order, written in the language of the session.
- **Pending**: the decisions the user left open or deferred, or "none".
- **Next step**: `plan`, `implement` or `stop`, as the user chose it at Step 10; only when no skill invoked this one.

## Limits

- Never write, edit nor commit a file, nor act on the subject, until the user confirms the brief and chooses at
  Step 10 to implement it.
- Never ask the user a fact you can find in the files, git, the tools or the docs.
- Never settle a decision yourself: put each one to the user, even when you recommend its answer.
- Ask one decision per message, as text with [question.md](question.md), and wait for its answer.
  Never two decisions in one message, nor a list of them, nor `AskUserQuestion` or any dialog of the agent.
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
- [ ] Step 6: Pick the next decision of the frontier, ask it alone (see [question.md](question.md)) and wait for the answer.
- [ ] Step 7: Record the answer and the facts reported, and recompute the frontier (see Step 7).
- [ ] Step 8: Repeat Steps 5 to 7 until the frontier is empty, then ask each deferred decision again, alone.
- [ ] Step 9: Give the `Brief` and ask the user to confirm it (see Step 9).
- [ ] Step 10: When no skill invoked this one, ask the user the next step and follow it (see Step 10).
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

### Step 9: Give the `Brief` and ask the user to confirm it

- Write the `Brief` as text, then ask with [question.md](question.md) whether to confirm it or adjust it, confirm
  first, saying the `Brief` is just above it.
- An adjustment reopens the decisions it touches: go back to Step 6 for them.
- When a skill invoked this one, return the confirmed `Brief` to it and go on with that skill's checklist.
  Skip Step 10: that skill decides what comes next.

### Step 10: Ask the user the next step and follow it

Ask it always once the `Brief` is confirmed, even when the conversation already names the next step.
Ask it as one decision with [question.md](question.md), with three options:

- **Write a plan**: the skill `spec` turns the confirmed `Brief` into a plan, without interviewing again.
- **Implement directly**: this session changes the code from the `Brief`, in the flow without a plan.
  With `Pending` not empty, its description lists those decisions and says only what does not hang off them is built.
- **Stop**: end with the `Brief` and `Pending`, doing nothing more.

Recommend one option, first in the list:

- The next step the user already named in the conversation, when they named one.
- Otherwise **Write a plan** when Step 3 found the change does not fit one review or a reason of the pre-mortem
  with a high impact, and **Implement directly** for every other change.

Then follow the answer:

- **Write a plan**: never start `spec` yourself; tell the user to run it in this session, with the prefix of its
  plugin, so it takes the confirmed `Brief`.
  When the subject is a file of the folder of tasks, give its path as the argument, so `spec` marks the task planned.
- **Implement directly**: change the code from the `Brief`, under the rules and the workflow of the project.
  Write no plan file and leave the status of a task file as it is.
  With `Pending` not empty, build only what does not hang off its decisions, and list them when you end.
- **Stop**: end with the `Output`.

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

### Brief

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

- Ask every question with [question.md](question.md), next to this file, in the language the user writes in.
- Never put a decision with fewer than two alternatives or more than four, nor an alternative without its pros and cons.
  With more than four, drop or merge the weakest: the user may still answer with any alternative of their own.
- A fact only the user knows is a decision too: who suffers the problem, a threshold, the steps of a manual check.
  Its alternatives are two to four hypotheses drawn from the subject, the docs and the code, each with what it implies.
  A hypothesis is never a figure or a cause no source gives: the user may answer with their own.
