# Rubric of the architecture

The nine dimensions the skill measures, the metrics they rest on and the levels of their score.
A dimension gets the level whose description it meets whole; levels 4 and 2 sit between their neighbours.
Then the agent traps, the unmodeled cases and the guards that close them, which the report lists whatever it scores.

## Graph metrics

Build the graph of the imports between the modules of the whole repository, even when the scope is smaller.
The coupling of a module depends on who uses it.

- Use the tool of the language when it is installed, such as `go list -f '{{.ImportPath}} {{join .Imports " "}}' ./...`,
  `npx --no-install madge --json <src>` or `cargo metadata`.
- Without one, search the import statements with `rg` and mark every metric `approximate`.
- Keep only the edges between modules of the repository; count the external dependencies apart, per module.

Compute:

- **Fan-in** and **fan-out** of each module: the modules that import it and the modules it imports.
- **Instability** of each module: fan-out / (fan-in + fan-out), from 0 (stable) to 1 (volatile).
- **Cycles**: each cycle of the graph, with its modules.
- **Inverted edges**: each edge from a module to another more unstable than it.
- **External reach**: for each external dependency (framework, driver, SDK, client), the modules that import it.

## History metrics

Sample the last 500 commits without merges of the current branch:
`git log --no-merges -n 500 --name-only --format='commit %h %s'`.
Leave out the commits that touch more than 50 files, such as renames and formatting.
With fewer than 30 commits left, skip these metrics and say so in `Pending`: they would be noise.

- **Churn**: the commits that touch each file.
- **Hotspots**: the files ranked high both by churn and by lines.
- **Co-change**: the pairs of modules changed in the same commit at least 5 times, and which of them import neither
  the other.
- **Spread**: the median and the 90th percentile of the modules touched per commit that adds a behaviour.
- **Fix concentration**: the files touched most by the commits whose subject starts with `fix`, `revert` or
  `hotfix`, in any case, and the commits that touch each of them.

## Cohesion

Each module has one responsibility, and what changes together lives together.

- Metrics: lines and files per module, the hotspots, the share of commits of a module that change only that module.
- Questions: can each module be named by one responsibility, without "and"?
  Are there grab-bag modules (`utils`, `common`, `helpers`, `misc`) that grow?
  Do the files of a module use each other, or only share a folder?
- 5: every module has one responsibility one can name, with no grab-bag, and its changes stay inside it.
- 3: several modules mix responsibilities, or a grab-bag module keeps growing.
- 1: no module has a responsibility one can name, or one god module holds most of the logic.

## Coupling

Modules depend on few others, through narrow and explicit contracts.

- Metrics: fan-out per module (median and highest), edges per module, co-change pairs without an import, the size
  of the interfaces and types that cross modules, the shared mutable state.
- Questions: does a module reach the internals of another?
  Does a change of one module force changes in others that do not import it (hidden coupling)?
  Is there global or shared mutable state?
- 5: low fan-out everywhere, narrow contracts, no hidden coupling and no shared mutable state.
- 3: some modules with high fan-out or wide contracts, or a few pairs with hidden coupling.
- 1: most modules depend on most others, or modules share mutable state and change together.

## Growth

A new behaviour is new code in one place, not changes spread across the code.

- Metrics: the spread of the commits that add a behaviour, the hotspots, the files that change in most of them.
- Questions: what must change to add one more case of the main concept (a command, an endpoint, a provider)?
  Is there a switch over kinds, a registry or a central file that every new case edits?
- 5: a new case is a new module or file plus one line of wiring, and the spread stays at one or two modules.
- 3: a new case edits a few known places, such as a switch or a registry, beside its own code.
- 1: a new case edits many modules (shotgun surgery), and the hotspots are where every change collides.

## Replaceability

A piece can be swapped (an adapter, an external dependency, a module) without changing the others.

- Metrics: the external reach of each external dependency, the boundaries with an interface the consumer owns.
- Questions: how many modules change to replace the database, the framework, a provider or the transport?
  Do the types of a framework or an SDK leak into the domain?
  Could a module be swapped by another implementation of the same contract without editing its callers?
- 5: each external dependency lives in one adapter behind a contract its consumer owns, and the domain never sees it.
- 3: the main dependencies sit behind contracts, but some leak their types or are imported by several modules.
- 1: the external dependencies are imported across the code, and the domain is built on their types.

## Dependencies

The dependencies point one way: no cycles, and the stable and abstract never depend on the volatile and concrete.

- Metrics: the cycles, the instability of each module, the inverted edges, the edges from the domain to the
  infrastructure (IO, frameworks, transport).
- Questions: does the core of the domain import infrastructure?
  Do the stable modules, with the highest fan-in, depend on volatile ones?
- 5: no cycles, no inverted edges, and the domain imports no infrastructure.
- 3: no cycles, but some inverted edges or some imports of infrastructure from the domain.
- 1: cycles between modules, or the domain depends on the infrastructure throughout.

## Conformance

The code does what the declared architecture says, and checks enforce it.

- Metrics: the constraints declared, those a test or a linter enforces, and the violations found.
- Questions: does each layer, module and allowed dependency of the docs match the code?
  Which constraints rely only on the reviewers?
- Challenge the declared architecture itself, each finding with the constraint or rule it names:
  - A constraint that protects nothing: breaking it would harm no dimension.
  - A constraint that costs more than it protects: it forces detours, such as layers that only forward or copies.
  - Two constraints that contradict each other, or one the code bypasses because it is impractical.
- `unscored` when nothing declares the architecture: its first recommendation, of high priority, is to declare the
  layers and the allowed dependencies and to check them with tests or linters.
- 5: the code matches every declared constraint, and tests or linters enforce the main ones.
- 3: the code matches most constraints, but some are enforced by nothing, or the docs are out of date.
- 1: the code contradicts the declared architecture in several places.

## Testability

The logic can be tested alone, fast, without the outside world.

- Metrics: the test files per module, the modules without tests, the tests that need a network, a real clock, a
  database or the real file system, the size of the fakes.
- Questions: is the logic pure, with the effects at the edges?
  Can each collaborator be replaced in a test through a seam (an interface, a function, a parameter)?
  Is the architecture itself tested?
- 5: a pure core tested without fakes, seams at every effect, and every module of logic with tests.
- 3: most modules tested, but some logic mixed with effects, or large fakes to test it.
- 1: the logic needs the real outside world to run, or most modules have no tests.

## Simplicity

No more structure than the problem needs.

- Metrics: interfaces with a single implementation and no double in the tests (leave out the contracts at a
  boundary), layers that only forward calls, modules nothing imports that are no entry point.
- Questions: does each abstraction serve a second implementation, a test or a boundary?
  How many hops does a call take from the entry to the logic?
  Is there configuration, generality or extension points nothing uses?
- 5: every abstraction serves a second implementation, a test or a boundary, and no layer only forwards.
- 3: some abstractions or layers nothing needs, which add hops but do not hide the logic.
- 1: the indirection hides the logic: layers that only forward, generality nothing uses, dead modules.

## Agent safety

An agent that edits the code learns every invariant from the files it touches, and a check fails when it breaks one.

- Metrics: the agent traps found and the share with a mechanical guard, the unmodeled cases, the declared conventions
  no check enforces, the fix concentration, the co-change pairs without an import.
- Questions: can an agent learn every invariant of a change from the files it edits, or does one live elsewhere?
  Does a new case need an edit in a second place nothing checks, such as a registry, a list, a doc or a string?
  Are the contracts between modules typed and checked, or strings and conventions?
  Does each failure, platform and concurrent run the system meets have a module that handles it?
  When an agent breaks a declared convention, does a test or a linter fail?
- 5: every invariant is local or checked, every convention has a mechanical guard, no case is unmodeled, and the
  fixes do not concentrate.
- 3: the main constraints have a check, but some conventions or paired edits rely on reading, or some cases have no
  module that handles them.
- 1: most conventions rely on reading, the invariants spread across files with no check, and the history fixes the
  same files again and again.

## Agent traps

A trap is a place where an agent that edits the code will likely err, and no check catches the error.
Look for these kinds, in the scope and in the modules it imports:

- **Non-local invariant**: a rule of a module that holds only because another file, far from it, keeps it.
- **Paired edit**: a change that is right only when a second place changes too, such as a registry, a switch, a
  list of names, a doc, a fixture or a generated file.
- **Implicit convention**: a naming, layout or ordering rule that only the existing code shows, and no doc or check
  states.
- **Stringly contract**: modules that agree on a key, a path, a format or a message by literal strings, with no type
  nor test that ties both ends.
- **Misleading example**: two ways to do one thing, a deprecated pattern still in use, or a name that says other than
  the code does; an agent copies the nearest example.
- **Silent default**: a fallback, a zero value or a swallowed error that turns a wrong edit into wrong behaviour
  instead of a failure.
- **Hand-edited generated code**: generated or vendored files nothing marks, which an agent edits instead of their
  source.

Each trap rests on evidence: the code at `file:line`, a declared constraint, and when the history has one, a commit
of the fix concentration or a co-change pair that confirms it.
A trap without a concrete scenario is no finding: name the change an agent would make, what breaks and why no check
fails.

## Unmodeled cases

An unmodeled case is a case the system meets and no module of the architecture has a place to handle.
Look for these families:

- **Partial failure**: a run that stops between two steps that change state, files or the outside world.
- **Failing dependency**: a command, network call, file or service that fails, times out or answers late.
- **Platform**: another operating system, architecture, shell, encoding or version of a tool the code assumes.
- **Concurrency**: two runs at once, a retry, or a run that starts while another holds the state.
- **Old state**: data, configuration or files of an older version, or missing, that reach the new code.
- **Boundary of a contract**: an input that crosses modules empty, too large or malformed, with no module that owns
  its validation.

Name the module that should handle the case and why none does today.
A missing check inside one function is a bug, not an unmodeled case: it belongs to an audit.

## Guards

Each trap and unmodeled case proposes the first guard of this order that is viable:

1. A test or a linter that fails when the error happens, such as an architecture test or a dependency rule.
2. A rule of the folder of rules with a check that runs it.
3. A rule of the folder of rules without a check.
4. A note in the instructions of the agents, such as `AGENTS.md` or `CLAUDE.md`.

When only a note is viable, mark the finding `no mechanical guard`.
