# Review of the watch of the edits made by commands

2026-11-11; base `123e7db` on the branch `worktree-enumerated-munching-cook`, with the change uncommitted in the
working tree.
Intent given: implement `ai/plans/watch-edits-made-by-commands.md`, a snapshot of the working tree on `pre_command`
compared on `post_command`, which restores the files of `guardrails.protected_files` a command changed and evaluates
every other changed path as derived `pre_edit` and `post_edit` whose blocks only inform; all six tasks were built.

## Verdict

approved with comments.

## Findings

- major, bug, `internal/app/hook/answer/logic_watch.go:143`: a deleted path also becomes a derived `pre_edit` and
  `post_edit`, so `format_on_edit` runs on a file that no longer exists and the agent gets a false failure as context.
  Evidence: `TreeDiff` (`git diff-tree -r --name-only --no-renames`, `internal/platform/git/git_tree.go:102`) lists
  deleted paths and `derive` turns every non-protected path into an edit; `golangci-lint fmt missing.go` exits 3
  with `lstat missing.go: no such file or directory`, so `rm x.go` or `git mv a.go b.go` triggers it in this
  repository.
  Fix: derive edits only for paths present in the current tree (`--diff-filter=d` or a flag per path), or skip
  `post_edit` for deleted ones, and document it in docs/polix.md and answer.md.
- major, bug, `internal/app/hook/answer/logic_watch.go:67`: a snapshot is never consumed nor bounded in age, so with
  the command line as key a `post_command` without a fresh snapshot pairs with an older one of the same line.
  Evidence: the query by outcome, session, actor and key then `st.Latest(q)`, with `head != snap.Head` as the only
  guard (line 79); a person's later edit of a protected file would be put back, and docs/polix.md:340 says such a
  `post_command` answers as without the watch.
  Fix: pair only with an unused snapshot (a consumed marker written by `post_command`, or ignore a snapshot older
  than the latest `post_command` decision of the key), and align the doc sentence.
- major, bug, `internal/app/hook/answer/logic_watch.go:79`: a command that rewrites the working tree without moving
  HEAD (`git stash`, `git restore .`, `git checkout -- .`) is watched, so a person's uncommitted change of a protected
  file is written back after `git stash` and a later `git stash pop` aborts.
  Evidence: the only skip of `watch` is `head != snap.Head`; `git stash` moves `refs/stash`, not HEAD.
  Fix: also skip when `refs/stash` moved, recorded in the snapshot, or list the case under Known limits of
  docs/polix.md as a deliberate decision.

## Checks

- `go vet ./...`: ok.
- `go test ./...`: every package ok, `internal/app/hook/answer`, `internal/platform/git`, `tests/architecture`,
  `tests/setup` and `tests/install` included.
- `golangci-lint run ./...`: 0 issues.
- `gofmt -l .`: no file.
- `polix rules check --changed`: 34 checks, 0 failed.
- `polix config check`: ok, polix.yaml and 49 rules of ai/rules/.
- `golangci-lint fmt missing.go` in a scratch module: exit 3, the evidence of the first finding.

## Held

- GOARCH-1 to GOARCH-45 with checks, through `polix rules check`: GOARCH-37 on the conditions of the queue of `run`,
  GOARCH-36 on the names of answer.md, GOARCH-41 and GOARCH-42 through depguard.
- GOARCH-18: `logic_watch.go` takes and returns data; the restore goes through `hook.Step.Restore` and `Port.Restore`.
- GOARCH-20 and GOARCH-22: in.go only sequences; `tool_use_id` is read in in_claude.go and does not leak past it.
- GOARCH-21: answer.md updated in Request, Logic (rule 15), Port (21 methods), Errors and Flow.
- GOARCH-30: `Event.Valid` covers `ByCommand`, `Step.Valid` covers `Restore` with `Source`, `asks` counts `Restore`.
- GOARCH-10 and GOARCH-19: `Repo.Restore` lives in `internal/platform/git`, without domain, wrapped by the adapter.
- GO-2: code, comments and docs in English.
- GO-3: no new `//nolint`.
- GO-4: in.go has 386 lines, logic_watch.go 185, and the linter passes.
- XPLAT-1: paths through `filepath`, `FromSlash` and `IsLocal`; the tests of modes and permissions are guarded by
  `runtime.GOOS`.
- DOC-1: docs/polix.md, docs/use-cases.md and CHANGELOG.md updated; the schema of polix.yaml did not change.
- Intent: AC1 to AC11 have code and tests, and no change is unrelated to it.

## Pending

- The manual end to end check of the plan in an interactive Claude Code session.
- `just check`, not run by the reviewer since it replaces the installed binary; `polix config fmt --check` and
  `polix doctor --offline` not run by it.
- The runs of CI on Linux and Windows that the plan asks for `TestRestore`; only macOS ran locally.
- `claude plugin validate plugins/polix`, not run: the plugin is not part of the change.
