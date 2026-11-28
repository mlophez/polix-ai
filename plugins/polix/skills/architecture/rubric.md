# Rubric of the architecture

The eight dimensions the skill measures, the metrics they rest on and the levels of their score.
A dimension gets the level whose description it meets whole; levels 4 and 2 sit between their neighbours.

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
