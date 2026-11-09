# Architecture follow-ups

Two changes to plan after the commit of `ai/plans/limit-size-and-complexity.md`, each as a plan of its own.

## 1. Rename the domain setup

`setup` is a weak domain name: a verb dressed as a noun, which GOARCH-17 admits only as a workaround.
Its use cases act on three different nouns.

- [ ] `install`, `uninstall` and `refresh` write and remove the plugin and the hooks of polix in the settings of
      Claude Code and Codex: they act on the integration of polix with the agents.
      - Option A: `agent/connect`, `agent/disconnect`, `agent/refresh` (ConnectAgent, DisconnectAgent, RefreshAgent).
        `agent/install` would read InstallAgent, as if polix installed the agent.
      - Option B: a domain `integration` (`integration/install`, `integration/uninstall`, `integration/refresh`).
        More exact; it keeps the domain `agent` for the protocol of the hooks (events and answers) only.
- [ ] `check` (`polix config check`) validates `polix.yaml` and the rules: move it to `workflow/check`, or split it
      with the existing `rule/check`.
- [ ] `diagnose` (`polix doctor`) checks the whole machine and repository: move it to `machine/diagnose` or
      `environment/diagnose`, with `setup_finding.go` and `setup_problems.go`.
- [ ] Update with it:
      - the aliases of `importas` (`setupcheck`, `setupdiagnose`...);
      - GOARCH-17, which cites "setup for check, install and doctor" as its example;
      - `docs/architecture.md` and `docs/polix.md`;
      - `tests/setup/`, which becomes `tests/<noun>/`.
- [ ] The commands of the command line do not change.

## 2. Architecture rules for the design of domain entities and use cases

The current rules (GOARCH-1 to 44) decide the shape: where each thing goes and who imports whom.
These rules measure the design itself.
Write them with `/polix:add-rule`, with a check where one can decide them.
Start with 2, 6, 7 and 1: the most design for the least code.

Domain entities:

- [ ] 1. No anemic entities: every exported struct of a domain has at least one method besides `Valid`.
      Check: an AST test that counts the methods per type.
- [x] 2. The domain knows no format: the domain files never import `encoding/json`, `gopkg.in/yaml.v3` nor
      `encoding/xml`.
      Check: add them to the deny list of GOARCH-41-domain in depguard.
      Done: `encoding/json` and `encoding/xml` denied to the domain and the logic, yaml out of their allow list.
- [ ] 3. Typed vocabulary: the closed values of a domain (events, verbs, outcomes) are constants of a named type,
      never loose strings.
      Check: report comparisons of a string field of the domain with a literal outside the domain.
- [ ] 4. Named domain errors: the domain returns sentinel errors (`Err*`) or error types, and the use cases tell
      them apart with `errors.Is` and `errors.As`, never comparing `err.Error()`.
      Check: an AST test on `Error()` inside comparisons.
- [ ] 5. No dead domain: an exported method of a domain that nothing calls outside its package goes away.
      Check: a usage test over the module.

Use cases:

- [ ] 6. Small port: at most N methods per `Port` (for example 8); AnswerHook has about 17 today.
      Check: count the methods of the interface.
- [x] 7. Short `Decide`: at most 4 parameters; more become a domain value.
      UpdateVersion passes 9 today.
      Check: an AST test on the signature.
      Done: GOARCH-45, with `version.Update`, `rule.Selection`, `setup.Setup.Defaulted` and the inspection of
      `setup/check` built by its shell.
- [ ] 8. `Output` is only data: its fields are domain or predeclared types, never functions nor interfaces, as
      GOARCH-33 asks of the port.
      Check: an AST test on the fields.
- [ ] 9. Few domains per use case: a use case imports at most N domains (for example 4); more means two use cases
      or a missing concept.
      Check: the import graph that `tests/architecture` already builds.
- [ ] 10. Covered logic: the `logic*.go` files of each use case pass a coverage threshold (for example 85%).
      Check: `go test -coverprofile` filtered to the logic files.

Left to the review:

- [ ] 11. The verb is the user's: `<Verb><Noun>` says what the user asks (DiagnoseMachine, ConnectAgent), not what
      the code does (ProcessSetup); an extension of GOARCH-17.
- [ ] 12. One entity, one reason to change: methods that split into groups sharing no field are two entities.
- [ ] 13. The entity decides validity: no use case checks an invariant that `Valid` of the domain should
      guarantee; the part of GOARCH-30 no check can decide.

## 3. Rules on the names and the size of a domain

A good domain name is the noun its use cases act on; `setup` passed every current rule.
A deterministic check cannot judge a name, but it can catch the usual smells and leave the rest to the review.

- [ ] 14. Denied domain names: no domain folder is named after a vague bucket (`setup`, `common`, `util`,
      `utils`, `helper`, `helpers`, `manager`, `core`, `misc`, `base`, `shared`, `service`, `handler`, `data`,
      `info`).
      Check: a fixed list in the test of GOARCH-17, extended when a new smell shows up.
- [ ] 15. A use case acts on its noun: the `Decide` of every use case of `internal/app/<noun>/` takes or returns
      at least one type of the domain `<noun>`, and its port speaks of it.
      Check: an AST test on the signature of `Decide`, `Output` and `Port`.
      It would have caught `setup/check`, which only speaks of `workflow` and `rule`.
- [ ] 16. The entity carries the domain: the entity `<Noun>` of GOARCH-2 is used by most of the use cases of its
      domain, not only declared.
      Check: count the use cases of the domain that name the entity.
- [ ] 17. At most 8 use cases per domain: more is a sign of two nouns in one, to split by noun, never by layer.
      Today the largest is `setup`, with 5.
      Check: count the use case folders of each domain.
- [ ] 18. (review) The name of the domain reads with every verb: each `<Verb><Noun>` of the domain is a sentence a
      user would say; a verb that only fits with another noun means the use case belongs to that noun.

## 4. More ideas to weigh

Domain:

- [ ] 19. No god entity: at most N fields per domain struct (for example 12) and N exported methods (for example
      15); `hook.Evaluation` would be the first to look at.
      Check: an AST test.
- [ ] 20. Bounded domain size: at most N lines per domain package (for example 2000), tests aside; past that, the
      noun hides a second one (`workflow` would be the first to look at).
      Check: count the lines of `internal/app/<noun>/*.go`.
- [ ] 21. Stable domains depend on little: a domain that many use cases import imports few other domains
      (afferent against efferent coupling).
      Check: the import graph, with a limit on the domains a widely used domain imports.

Use cases and ports:

- [ ] 22. No flag arguments: neither `Decide` nor a method of a port takes a `bool` parameter; two behaviours are
      two methods, or a domain value says which.
      `version/update` passes several bools to `Decide` today.
      Check: an AST test on the signatures.
- [ ] 23. Ports speak intent, not technology: a method of a port is named after what the use case needs (`Load`,
      `Save`, `Branch`), never after how (`Exec`, `Query`, `Get`, `Post`, `Read` of a path).
      Check: a list of denied verbs, extended when a new one shows up.
- [ ] 24. Every port error is tested: `in_test.go` makes each method of the fake port fail once and checks what
      the response says.
      Check: compare the methods of `Port` with the methods the fake fails in the tests.
- [ ] 25. Errors documented: the section Errors of the document of a use case names every error the shell can
      return (an extension of GOARCH-36).
- [ ] 26. Request and Response are flat: their fields are predeclared or domain types, never pointers to other
      structs of the use case nor maps of structs.
      Check: an AST test on the fields.

Platform:

- [ ] 27. Platform speaks no domain: no identifier of `internal/platform/` uses a noun of `internal/app/`
      (`workflow`, `rule`, `hook`...), so a library never grows domain knowledge through names.
      Check: compare the identifiers of the libraries with the names of the domains.

Tests of the logic:

- [ ] 28. Mutation testing of `logic*.go` on a schedule (not on every stop): it measures whether the tests of the
      pure logic catch changes, which coverage does not.

## 5. Linters to enable in golangci-lint

All exist in the pinned v2.15.0.
Enable each one alone first, count its findings with `golangci-lint run --enable-only <linter> ./...`, and add it
with the code already clean, as the plan of limits does.

Architecture and design:

- [ ] `interfacebloat` (max 8 to 10 methods per interface): rule 6, the small port, for free; tune `max`.
      Enabled as a ratchet at `max: 19`, the port of AnswerHook; the one of DiagnoseSetup went from 20 to 15 by
      merging methods. Lowering it further needs both redesigned, which GOARCH-12 and GOARCH-33 do not let split
      nor embed.
- [x] `exhaustive`: every `switch` on a typed enum lists all its values or has a `default`.
      It pairs with rule 3 (typed vocabulary): a new event or verb fails every switch that forgets it, such as
      the router of AnswerHook.
- [ ] `wrapcheck`: an error from another package is wrapped before it is returned.
      It enforces what `docs/architecture.md` asks of the adapters (wrap with what failed); configure it to ignore
      the domain and the shells, which return errors unchanged.
- [ ] `errname` (enabled) and `err113` (not yet): sentinel errors are named `Err*`, error types `*Error`, and no error is built with
      `errors.New` at the point of return without a sentinel; rule 4 for free.
- [x] `recvcheck`: one type uses the same kind of receiver in all its methods; it backs GOARCH-27 (value
      receivers in the domain) and catches mixed receivers elsewhere.
- [x] `inamedparam`: the parameters of an interface method are named, so a port reads as a contract.
- [x] `iface`: no two identical interfaces and no interface declared but unused.
- [ ] `funcorder`: constructors and exported methods first; it complements GOARCH-13 (types first).
- [ ] `containedctx` and `contextcheck`: no `context.Context` in a struct, and a function that receives a context
      passes it on; it protects the context of the ports and adapters.
- [x] `gochecknoinits`: no `init`; GOARCH-44 for the domain, this for the whole module.

Refactoring and complexity:

- [x] `nestif` (min 4): deep nesting, which gocognit counts only in part.
- [ ] `maintidx`: a maintainability index per function that mixes complexity, volume and lines.
- [ ] `cyclop` with `package-average`: the average complexity of a package, which catches a package of many
      medium functions that gocyclo lets through.
- [ ] `dupl` (threshold about 100 tokens) and `goconst`: duplicated blocks and repeated literals, the first
      candidates of a refactor.
- [ ] `gocritic` with the tags `diagnostic` and `performance`: many small bugs and refactors (appendAssign,
      rangeValCopy, hugeParam...).
- [ ] `nakedret` and `nonamedreturns`: no bare returns in long functions; decide whether named results stay
      allowed (GOARCH-37 and the ports use some today).
- [x] `forcetypeassert`: every type assertion checks its `ok`; `workflow.Cond` as a map relies on them.

Safety and hygiene:

- [ ] `gosec`: polix runs commands and writes files from hooks; review G204 (command with variables) and G304
      (file path from input) and silence each intended finding with its reason (GO-3).
- [x] `errchkjson`: errors of `json.Marshal` are checked, now that the formats live in the in and out files.
- [ ] `misspell` and `dupword`: typos and repeated words in comments and in the texts polix prints.
- [ ] `prealloc`, `predeclared`: small performance and a name that shadows a builtin.

Not recommended for this repository:

- `exhaustruct`: every literal names all fields; too noisy with the domain literals of GOARCH-29.
- `gochecknoglobals`: GOARCH-4 already allows the tables of the domain.
- `ireturn`: AnswerHook returns its `format` interface on purpose.
- `paralleltest` and `thelper`: worth it only if the test suite gets slow.

## 6. Scalability and safety nets beyond the linter

- [ ] Latency budget of a hook: a benchmark of AnswerHook end to end on a large `polix.yaml` and a repository with
      many changed files, failing past a budget (for example 50 ms without runs).
      polix promises milliseconds per hook, and nothing measures it today.
- [ ] Fuzz tests of the translations: `shell.Split`, `yamlfmt.Format`, the formats `claude` and `codex`, and the
      decoders of `workflow/load` and `rule/load` with Go native fuzzing, so input from the agents never panics.
- [ ] Golden tests of the outputs a person reads (`polix graph`, `polix log`, `polix doctor`), so a refactor that
      changes them shows in the diff.
- [ ] A build and test time budget in CI, to notice when the architecture tests (which type check the whole
      module) grow too slow.
- [ ] A coupling report on demand (`go list -deps` per use case), to see which use case pulls in the most of the
      module before it becomes a problem.
