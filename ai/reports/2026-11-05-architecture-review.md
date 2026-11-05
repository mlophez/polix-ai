# Review of the Go architecture

Date: 2026-11-05. Base: commit 15c1a46 (main). An opinion on the target architecture of `docs/architecture.md` and
the rules GOARCH-1 to GOARCH-40 of `ai/rules/architecture.yaml`, against SOLID, Clean Architecture and the idioms
of Go. Read in full: the doc, the rules and the use cases `CheckRule` (`internal/rule/`), `AnswerHook`
(`internal/hook/`) and `DiagnoseSetup` (`internal/setup/`); the rest by its layout and sizes.

## Summary

The principles are sound: a pure core with an imperative shell, ports owned by their consumer, packages by
feature, `pkg/` without domain and the architecture decided by AST tests. The problem is the unit of encapsulation:
a use case is a set of files inside a domain package, not a package, which Go does not support. Several rules exist
to simulate the boundary a package would give for free, and they still cannot keep the dependencies of the domain
apart from those of the use cases.

The recommendation, in order: one subpackage per use case, small ports that compose, and `Step` as a sealed
interface or a read port for the hook.

## Strengths

- **Pure core, imperative shell.** `CheckRule` (`internal/rule/check_rule_logic.go`) is a pure function from domain
  values to a decision, tested without fakes. The clock, the version and the system come in the request (`Now`,
  `Version`, `System`) instead of being read by the logic.
- **Ports declared where they are used.** The idiomatic form of DIP in Go ("accept interfaces, return structs").
  The adapters have no constructor, and the compiler checks the port where the entry injects them.
- **Packages by feature, not by layer.** No `models/`, `services/` nor `repositories/`. `pkg/` holds technology
  without domain, and the import graph is checked (GOARCH-7). `cmd/polix/main.go` only wires the commands.
- **Request and Response as DTOs.** The domain carries no transport tags: transport and domain stay apart.
- **Architecture as executable fitness functions.** Deciding the rules with AST tests instead of a wiki is rare
  and valuable.
- **Go details done well.** `context.Context` first in every port, errors wrapped with what failed (`%w`),
  `var _ Port = (*fake)(nil)` in the tests, no `Get*`.

## Findings

### 1. The use case should be a package, not a name prefix

Since several use cases share a package:

- GOARCH-31 makes every top-level name start with the use case (`CheckRule`, `checkRule`) so the names do not
  collide. The result stutters: `rule.CheckRuleCommand`, `hook.AnswerHookCommand`, `version.UpdateVersionCommand`,
  which the Go Code Review Comments ask to avoid.
- GOARCH-10 and GOARCH-40 use AST tests to stop one use case from reaching another. The visibility between Go
  packages gives that for free.
- The domain package imports the frameworks. `internal/rule/check_rule_in.go:9-14` brings `cobra`, `pkg/git` and
  `pkg/cli` into the package `rule`. GOARCH-4 forbids it file by file, but the compiler reasons by package. The
  logic of `DiagnoseSetup` imports the domain of `hook` (`internal/setup/diagnose_setup_logic.go:10`) and with it
  pulls in the cobra command and the adapter of `AnswerHook`. The dependencies of the domain and of the use cases
  are mixed, and the risk of an import cycle grows with every package.

Proposal: `internal/rule` holds only the domain, and `internal/rule/check` holds `Command`, `Request`, `Response`,
`Port`, `Adapter` and the logic. The compiler then enforces the dependency rule of Clean Architecture, the names
stop stuttering, and GOARCH-10, GOARCH-31 and GOARCH-40, with part of GOARCH-4 and GOARCH-9, can be retired or
simplified.

### 2. One port per use case breaks ISP

`AnswerHookPort` has 16 methods and `DiagnoseSetupPort` about 19. "The bigger the interface, the weaker the
abstraction." The rules push towards it: GOARCH-12 allows one interface in the in file, and GOARCH-33 forbids
embedding interfaces. The symptoms are large fakes and long tests (`internal/hook/answer_hook_out_test.go` has
1138 lines).

Proposal: allow several small ports per use case (the repository, the processes, the log), grouped in a struct of
dependencies the entry fills.

### 3. The step shape of AnswerHook costs more than the purity it buys

It is a hand-written effect interpreter:

- `Step` (`internal/hook/hook_step.go`) is a tagged union of 13 optional fields, with a `Valid()` that checks only
  one of them asks.
- `Facts` (`internal/hook/hook_evaluation.go`) carries pairs of `HasX`, maps per epoch and `StateAt`.
- The logic is replayed from the start of the event on every step, consuming a `Journal` of outcomes it already
  got.

A new kind of action touches `Step`, `Facts`, `Evaluation`, the switch of the shell, the port, the adapter and the
fake: the opposite of OCP (shotgun surgery). `internal/hook/answer_hook_logic.go` has 758 lines.

Two reasonable ways out:

- **Keep the step shape with typed steps.** `type Step interface{ step() }`, one concrete type per step and a type
  switch in the shell. The rule "exactly one field asks" and the `Has*` flags disappear.
- **Accept a read-only port in the logic of the hook,** with lazy and memoized reads. The decision of each entry
  stays pure, but the walk no longer replays a journal. The effects (`Run`, `Check`, `Append`) stay in the shell.

### 4. Literal plus Valid, without constructors, goes against the idiom of Go

Invalid values can exist, and every consumer must remember to call `Valid()`. GOARCH-29 checks it with an AST
test, which makes up for the missing constructors instead of removing the need. GOARCH-26 adds ceremony without
value: `Outcome.Valid()` always returns nil (`internal/hook/hook_step.go`, "it never is"). Immutability through
value receivers is misleading with maps and slices, such as those of `Facts` and `Evaluation`: the copy is shallow,
and GOARCH-30 has to ask for them to be copied by hand.

Proposal: for values with real invariants, unexported fields and `NewX(...) (X, error)` ("parse, don't validate");
keep literals for plain data.

### 5. The adapters repeat themselves with drifting semantics

`LoadWorkflow` and `LoadRules` exist in `CheckRuleAdapter`, `AnswerHookAdapter` and `DiagnoseSetupAdapter`, and
each treats a missing file or an invalid rule differently (`ParseFiles` against `ParseFilesAll`, a missing
polix.yaml as the default workflow or as absent). Vertical slices accept some duplication, but these are three
readings of "load the repository" that can drift apart.

Proposal: one shared adapter of the repository, without use case logic, which the adapters wrap.

### 6. The rulebook weighs too much for the size of the project

About 3500 lines of architecture tests (`tests/architecture/`) against about 10000 lines of production code. Some
rules are style, not architecture: GOARCH-13 (types first), GOARCH-14 and GOARCH-36 (the sections of the `.md`).
GOARCH-37, which defines what a condition of the shell may contain, is strict enough to force domain methods whose
only purpose is to satisfy it. The risk is that the rules freeze the design instead of protecting it.

## SOLID

- **SRP:** good at the level of the file; broken in `answer_hook_logic.go` and `Facts`.
- **OCP:** weak where it matters most, the hook (finding 3).
- **LSP:** barely applies, there are no hierarchies; right for Go.
- **ISP:** the weakest point (finding 2).
- **DIP:** right in concept; the compiler does not guarantee it while the domain and the use cases share a package
  (finding 1).

## Recommendation

Keep the pure core, the ports owned by their consumer, `pkg/` without domain, the DTOs and the AST tests. Change,
in this order:

1. One subpackage per use case: it retires several rules and the stutter.
2. Small ports that compose.
3. `Step` as a sealed interface, or a read port for the hook.

Afterwards, review the remaining rules so that each one protects a design decision instead of making up for the
structure.
