# Review of the current change: carry observations across branches

Date: 2026-11-10; base: `3a8b36b` on branch `worktree-floofy-stirring-pebble`, with uncommitted changes.
Intent (given): implement `ai/plans/carry-observations-across-branches.md`.
polix writes the HEAD commit (`head`) on the observed lines finished with a hash, passed and blocked.
The condition `finished`, the repeat of `run` and the `once` blocks match the lines of the current branch or of another
branch with the same non empty HEAD commit, still comparing the tree on the scope.
Older lines without `head` and repositories without commits match by branch only.
All four tasks of the plan were built, and `docs/reference.md` was also updated outside the plan.

## Verdict

changes requested

## Findings

- blocking, rule, `internal/app/hook/answer/answer.md:139`: the document says that on a detached HEAD only the lines of
  the same commit count, but the code selects every line written on a detached HEAD, whatever its commit, plus the
  lines of any branch with the same commit.
  Evidence: GOARCH-21; `internal/app/state/state.go:140` returns `e.Branch == q.Branch || q.Head != "" && e.Head ==
  q.Head`; `docs/polix.md:668` describes it right ("the lines written on a detached HEAD match each other").
  Fix: rewrite the sentence: on a detached HEAD the lines written on a detached HEAD count, and those of a branch on
  the same commit.
- minor, consistency, `CHANGELOG.md:8`: the change is visible to the user (a review, a passed run and a `once`
  reminder survive `git checkout -b`, and `polix log --json` shows `head`) but `Unreleased` has no entry for it.
  Evidence: `CONTRIBUTING.md:77` asks for a line for each notable change under `Unreleased`; `git status` shows
  `CHANGELOG.md` unchanged.
  Fix: add a line under `### Changed` or `### Fixed` of `Unreleased`.

## Checks

- `go vet ./...`: no output.
- `go test ./...`: every package ok.
- `golangci-lint run ./...`: 0 issues, interfacebloat included (the port keeps 19 methods).
- `gofmt -l .`: no files.
- With a binary built in the scratchpad: `polix config check` ok, `polix config fmt --check` ok,
  `polix rules check --changed` 33 checks and 0 failed, `polix doctor --offline` ok but warnings of the environment.
- `git rev-parse HEAD --abbrev-ref HEAD` in scratch repositories: exit 128 without commits (both empty), the commit
  and `main` with a commit, the same commit and `feat/x` after `checkout -b`.

## Held

- GOARCH-18: `Query.selectsBranch` and `readByHead` are pure.
- GOARCH-19: the git call lives in `internal/platform/git` and the adapter only wraps it.
- GOARCH-20: `carry` only feeds `port.Head` to `KnowBranch`.
- GOARCH-30: `KnowBranch` keeps a name of intent, and the invariants of `Query.Valid` and `Entry.Valid` do not change.
- GOARCH-36: its check passes; `Head` is named in the Port of `answer.md` and in the Response of `log.md`.
- GOARCH-21, but the sentence of the finding: `answer.md`, `load.md` and `log.md` are up to date.
- GO-2: comments and docs in English.
- GO-3: no `//nolint` added.
- GO-4: golangci-lint and the architecture check pass.
- XPLAT-1: no behaviour of one system, and `strings.Fields` absorbs a `\r`.
- DOC-1: `docs/polix.md` is up to date; the schema of `polix.yaml` does not change, so `examples/polix.yaml` needs
  nothing.
- Compatibility: the decoder of the log is not strict, so an older polix reads lines with `head`; without
  `ByBranch`, `selectsBranch` behaves as before.
- Tests cover AC1 to AC7: unknown head, lines without head, detached HEAD in the domain, `Append`, `ReadLog` and
  `polix log --json`.

## Pending

- `just check` was not run by the reviewer as written, since it runs `go install` over the binary of the hooks; its
  checks were run with a scratch binary instead.
- The end to end checks of the plan could not run in the reviewer, blocked by the isolation of the worktree; they
  were run by the build session afterwards and passed.
