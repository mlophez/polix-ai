---
name: architecture
description: >
  Measures the architecture of the repository, or part of it, in nine dimensions scored 1 to 5 on metrics and code.
  It is critical: it names the weakness of each, challenges the declared architecture, and finds where an agent that
  edits the code will err and the cases the design does not model.
  It writes one report with prioritized recommendations and their possible solutions, and changes nothing else.
  It runs only when the user asks for it.
user-invocable: true
disable-model-invocation: true
argument-hint: "[paths and dimensions, e.g. internal/app coupling agent-safety; empty for the whole repository]"
---

# Measuring the architecture

Compute the metrics of the architecture with commands, judge each dimension of [rubric.md](rubric.md) on them and on
the code, and score it.
Judge it as a critic, not as an advocate: look for where it fails, where the declared design is wrong, where an agent
that edits the code will err, and which cases no part of the design handles.
Write one report with the scores, the evidence, the traps and the recommendations with their possible solutions.
The folders of reports and rules are those the project names in `AGENTS.md`, `CLAUDE.md` or the session.
Without one, they are `ai/reports/` and `ai/rules/` below the root.

## Requirements

- [rubric.md](rubric.md) and [template.md](template.md) next to this file.
  Without them, stop and tell the user.
- `git` on the `PATH` and the working directory inside a git repository.
  Without them, list the files with `rg --files`, skip `History metrics` of rubric.md and say so in `Pending`.

## Input

- **Scope**: the paths from the arguments.
  Without them, the whole repository.
- **Dimensions**: the dimensions of rubric.md the arguments name, in any language.
  Without them, all nine.
  The agent traps and the unmodeled cases are reported always, whatever the dimensions named.

## Output

- **Report**: the path of the report file.
- **Scores**: each dimension measured with its score, or `unscored`, and its reason in one line.
- **Top**: the three recommendations that matter most, with the solution recommended for each.
- **Traps**: the agent traps and the unmodeled cases found, and how many have `no mechanical guard`.
- **Pending**: what could not be measured and why, or "none".

## Limits

- Never edit a file of the project: the report file is the only file you write.
- Never run a command that changes the working tree, the index or the history, nor one that installs a tool.
- Give every score the metrics and the findings at `file:line` it rests on.
- Score each dimension by the levels of rubric.md, never by impression.
- Name the main weakness of every dimension measured, even at 5, with its `file:line`; never a weakness without one.
- Never give an overall score: each dimension stands on its own.
- Mark each metric with the command that produced it, or `approximate` when it comes from searching the code.
- Report only findings of architecture: leave the bugs to the audits and the style to the linters.
  A missing check inside one function is a bug; a case no module has a place to handle is an unmodeled case.
- Talk to the user in the language of the session.

## How to operate

Copy this checklist and track your progress:

```text
Architecture progress:
- [ ] Step 1: Resolve the scope and the dimensions (see `Input`) and map the modules (see Step 1).
- [ ] Step 2: Read the declared architecture (see Step 2).
- [ ] Step 3: Compute the metrics of the graph of the whole repository (see `Graph metrics` in rubric.md).
- [ ] Step 4: Compute the metrics of the history (see `History metrics` in rubric.md).
- [ ] Step 5: Read the code the metrics point to (see Step 5).
- [ ] Step 6: Judge and score each dimension of the input (see Step 6).
- [ ] Step 7: Find the agent traps and the unmodeled cases (see Step 7).
- [ ] Step 8: Write the strengths and the recommendations (see `Recommendations`).
- [ ] Step 9: Write the report file from template.md (see `Report file`).
- [ ] Step 10: Relay the `Output` and stop.
```

### Step 1: Map the modules

1. List the files with `git ls-files` and leave out vendored, generated, lock, binary and test data files.
2. Take as module the unit the declared architecture names; without one, the unit of the language.
   A Go or Java package, a Python package, a Rust crate or module, a folder with an index in JavaScript.
3. Count the lines and files of each module with `wc -l`, and note the entry points: `main`, commands, handlers.

### Step 2: Read the declared architecture

Read `AGENTS.md`, `CLAUDE.md`, the README, `docs/`, the decision records, the folder of rules and the architecture
tests or linters (dependency rules of `golangci-lint`, `import-linter`, `dependency-cruiser`, ArchUnit).
Note each layer, module and allowed dependency they declare, and whether a check enforces it.
Without any, `conformance` is `unscored` (see `Conformance` in rubric.md).

### Step 5: Read the code the metrics point to

1. Read whole the modules with the highest fan-in, the highest fan-out, every cycle and the top hotspots.
2. In every other module of the scope, read its public surface and one entry point that uses it.
3. When the scope is too large to read this way, delegate the sweep to at most two `Explore` subagents in one
   message, each with an area, the metrics of its modules and the questions of the dimensions.
4. A report of a subagent is a lead: read yourself each file a finding cites before it reaches the report.

### Step 6: Judge and score each dimension

1. For each dimension, gather its metrics and answer its questions of rubric.md with findings at `file:line`.
2. Give it the level of rubric.md whose description it meets whole; between two levels, take the lower and say why.
3. Name its main weakness: what holds it below 5, or at 5, what would bring it down first.
4. In `conformance`, challenge the declared architecture itself (see `Conformance` in rubric.md).
5. With a scope smaller than the repository, say in the report that the scores hold only for that scope.

### Step 7: Find the agent traps and the unmodeled cases

1. Walk the scope against each kind of `Agent traps` and each family of `Unmodeled cases` of rubric.md.
   Start from the fix concentration, the co-change pairs without an import and the conventions no check enforces.
2. For each trap, write the change an agent would make, what breaks, why no check fails, and the commit that
   confirms it when the history has one.
3. For each unmodeled case, write the case, the module that should handle it and why none does today.
4. Give each one the first viable guard of `Guards` of rubric.md, or mark it `no mechanical guard`.
5. Rank them: the confirmed by the history first, then those with `no mechanical guard`, then the rest.
6. Leave out what has no concrete scenario: a trap must name a change and a break, never a risk in the abstract.

## References

### Recommendations

- Write one recommendation per root cause, with the dimensions it improves, its evidence at `file:line`, its impact
  and its effort (`low`, `medium` or `high`).
- Ground each one on a finding of Step 6; never recommend a practice the code gives no reason for.
- A trap or an unmodeled case the history confirms, or with `no mechanical guard`, is also a recommendation, grouped
  with the others of its root cause.
- Give each a priority:
  - **high**: the root cause of a score of 1 or 2, a cycle, a dependency the declared architecture forbids, or a trap
    the history confirms with `no mechanical guard`.
  - **medium**: the root cause that holds a dimension at 3.
  - **low**: every other one.
- A high one has two or three possible solutions that differ in their idea, not in a detail.
  Each with its idea, its pros, its cons, its effort and its risk.
  Name the solution you recommend and why, and sketch it as before and after in the language of the project.
- Any other one has a single direction, in one or two lines.
- Sort them by priority, then by the number of dimensions they improve, then by effort, the lowest first.
- Write as strengths only what a recommendation could undo, with its evidence, so none undoes it.

### Report file

Path: a file of the folder of reports, `<reports>`.
Name it as the project names its reports: the convention of `AGENTS.md`, of `CLAUDE.md` or of the reports there.
Without one, name it `<reports>/<date>-architecture-<name>.md`, where `<date>` is today as `YYYY-MM-DD`.
`<name>` is short kebab-case English from the scope (`repository` for the whole one).
When the file exists, add a numeric suffix (`-2`) instead of overwriting it.

- Fill `template.md`: its sections in its order, every `<...>` replaced.
- Write it in the language of the docs of the repository, English when there are none.
- One sentence per line, each line at most 120 characters, with no tables.
