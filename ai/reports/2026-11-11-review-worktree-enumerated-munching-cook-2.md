# Review of the watch of the edits made by commands, with the fixes of the first review

2026-11-11; base `123e7db` on the branch `worktree-enumerated-munching-cook`, with the change uncommitted in the
working tree (32 modified files and 5 untracked); main is 4 commits ahead and does not affect the diff.
Intent given: implement `ai/plans/watch-edits-made-by-commands.md` (all six tasks), a snapshot of the working tree on
`pre_command` compared on `post_command`, which restores the files of `guardrails.protected_files` a command changed
and evaluates every other changed path as derived `pre_edit` and `post_edit` whose blocks only inform.
On top, the fixes of the three major findings of `2026-11-11-review-worktree-enumerated-munching-cook.md`: removed
paths are not derived as edits, a snapshot is compared once through an observed `watched` line, and a move of
`refs/stash` skips the watch like a move of HEAD.

## Verdict

approved with comments.

## Findings

- major, security, `internal/app/hook/answer/logic_command.go:26` and `internal/app/hook/answer/logic_watch.go:66`:
  the watch decides whether to act and what is protected from the `polix.yaml` read after the command ran, so a
  command that also breaks `polix.yaml` or rewrites its `protected_files` escapes the restore.
  With an invalid `polix.yaml` the `post_command` only calls `warnUnusable()`, so `watch` never runs; with a valid
  one, `watches(r.guardrails())` and `protected()` use the new globs.
  A `python3 -c` that writes `AGENTS.md` and empties `polix.yaml`, or drops `polix.yaml` from `protected_files`,
  puts neither back, so AC2 fails for these cases; it is no regression and needs the same command to change
  `polix.yaml`.
  Evidence: `if r.ev.Invalid != nil { ... return r.warnUnusable() }` comes before `ans = r.watch()` (line 40), and
  `run` loads the workflow from disk once per call (`port.LoadWorkflow`, in.go); neither "Files a command changed"
  nor Known limits of docs/polix.md mentions it.
  Fix: take what is watched from the snapshot, for example the `polix.yaml` the snapshot tree holds when `polix.yaml`
  changed or the current workflow is unusable, or record in the snapshot what it protects; at least list the case
  under Known limits.
- major, bug, `internal/app/hook/answer/logic_watch.go:87` (`gitMoved`, line 106) and `docs/polix.md:339`: git
  operations that rewrite the working tree without moving HEAD nor `refs/stash` are watched as if the agent made the
  changes: a `git merge` or `git pull` stopped on conflicts, `git cherry-pick`, `git revert` or `git am` with
  conflicts, `git merge --no-commit`, `git cherry-pick -n` and `git stash apply`.
  A protected file the merge brought is put back only in the working tree while the real index keeps the merged
  version, so the resolution commit includes it with the working tree showing the old one, a later `git commit -a`
  reverts it, and for a conflicted `AGENTS.md` a `git add` keeps only ours.
  Up to 25 merged files of others are also derived as edits and `format_on_edit` runs on them, conflict markers
  included.
  Evidence: the only guards are `head != snap.Head` and `stash != snap.Stash`; a conflicted merge leaves HEAD in
  place and `stash apply` does not touch `refs/stash`; docs/polix.md:339 says "so a merge, a checkout or a stash is
  never undone", false for a conflicted merge or pull.
  Fix: record and compare the operation in progress in the snapshot too (`MERGE_HEAD`, `CHERRY_PICK_HEAD`,
  `REVERT_HEAD`, `rebase-merge`, `rebase-apply`, through `git rev-parse -q --verify`), or skip when the
  `post_command` finds one; at least list `git stash apply` and `--no-commit` under Known limits and correct line 339.
- major, bug, `internal/platform/git/git_tree.go:192` (with the sort at line 115): on a case-insensitive file system
  (macOS and Windows by default), a case-only rename of a protected untracked file ends with the file deleted
  instead of restored.
  `os.rename('CLAUDE.md', 'claude.md')` gives `D CLAUDE.md` and `A claude.md`, both matched through `anyPathFold`;
  `restorePaths` walks them sorted, uppercase first, so `checkout-index --force -- CLAUDE.md` writes it back and
  `os.Remove("claude.md")` then deletes that same file.
  The same happens to a tracked file in a repository with `core.ignorecase=false` on such a file system.
  Evidence: the code of lines 192 to 198 and `sort.Strings(paths)` of line 115; not reproduced, since the sandbox of
  the session refused git outside the worktree.
  Fix: do every removal first and the `checkout-index` after, or skip removing a path that folds to a name the tree
  holds; add a test of the case.
- minor, bug, `internal/app/hook/answer/logic_watch.go:190`: `restore` carries out one `Restore` step for every path
  and gives its one error to all of them, so when one of two protected paths fails both are refused with "could not
  put it back", and the warning asks to put back by hand files already restored, though `Repo.Restore` goes on after
  a failure.
  Evidence: `for _, p := range paths { ... if o.Err != nil { reason = ...could not put it back... } }` and
  `strings.Join(paths, ", ")` in the warning (line 199).
  Fix: one `Restore` step per protected path, so each refusal and the warning carry only their own outcome.
- minor, bug, `internal/app/hook/answer/logic_command.go:38`: the snapshot is written before the entries of
  `pre_command`, so a `pre_command` one of its entries blocks still pays the content hash and leaves an orphan
  `snapshot` line, and a `run` of `pre_command` that rewrites files is attributed to the command: its files are
  derived as edits, or restored when protected.
  Evidence: `r.snapshot()` is called before `r.evaluate(ans)`; docs/polix.md, step 1 of "Files a command changed".
  Fix: write the snapshot after the entries of `pre_command`, only when the answer does not block, at the epoch they
  leave, and align answer.md (rule 15) and docs/polix.md.
- minor, consistency, `examples/polix.yaml:186` (and `plugins/polix/skills/workflow/reference.yaml:186`),
  `examples/polix.yaml:231`, `:400`, `:457` and `docs/polix.md:1065`: the annotated comments of `protected_files`,
  `format_on_edit`, `pre_edit` and `post_edit` still describe only the edit tool and the check of the command line,
  not that they also apply to the files a command changed, that `block` there only informs, nor that having any of
  them costs two content hashes per command; Known limits does not say that a line that writes and then moves HEAD
  (`python3 x.py && git commit -am x`) escapes the restore by design.
  Evidence: the comment of `protected_files`, "It denies an edit of a matching path ... and a command that writes
  one (the heuristic of `writes`...)", and that of `pre_edit`, "before a file is edited, once per path".
  Fix: one sentence in each comment pointing to "Files a command changed", the copy of the plugin kept identical,
  and the chained commit under Known limits.

## Checks

- `go vet ./... && go test -count=1 ./...`: exit 0, 43 packages ok, `internal/app/hook/answer`,
  `internal/platform/git`, `tests/architecture`, `tests/setup` and `tests/install` included.
- `gofmt -l .`: no file.
- `golangci-lint run ./...`: 0 issues.
- `go build` of `./cmd/polix` into the scratchpad, and with that binary: `polix rules check --changed`, 34 checks,
  0 failed; `polix config check`, ok, polix.yaml and 49 rules of ai/rules/; `polix config fmt --check`, ok;
  `polix doctor --offline`, ok, warning only for a dev build and the PATH running another polix.

## Held

- GOARCH-1, 3, 5, 11, 12, 13, 15, 23, 24, 32 to 39, 41, 42, 44 and 45 through `polix rules check`: GOARCH-37 on the
  queue of `run`, `case s.Restore != nil` and the new `lookup`; GOARCH-36 with answer.md naming `Stash` and `Restore`
  in `## Port`, 22 methods as in the code.
- GOARCH-16: the tags `json:"-"` of `hookInput` and `json:"tool_use_id"` of `claudeInput` sit on unexported types of
  the in files that decode the input.
- GOARCH-18 and GOARCH-20: the logic only asks steps (`Stash`, `Compare`, `Restore`, `Append`); in.go only
  sequences, and `act` notes the outcome of `Restore`.
- GOARCH-19, GOARCH-10 and GOARCH-42: `Repo.Restore` and `Repo.Stash` live in `internal/platform/git` without
  domain, wrapped by `Adapter`.
- GOARCH-26 to GOARCH-30: `Event.Valid` covers `ByCommand`, `Step.Valid` covers `Restore` with `Source`, `asks`
  counts `Stash` and `Restore`, `Entry.Valid` limits `Stash` to snapshots, `learnTreeDiff` copies its maps,
  `learnStash` follows `learnBranch`, and the literals of `Query`, `Step` and `Event` are validated.
- GOARCH-21: answer.md updated in Request, Logic (rule 15), Port, Errors and Flow; log.md with `Stash`.
- GO-2: English. GO-3: no new `//nolint`. GO-4: in.go has 400 lines, the cap, and logic_watch.go 205.
- XPLAT-1: paths through `filepath.FromSlash` and `IsLocal`, the tests of modes and permissions guarded by
  `runtime.GOOS`, the fake of the shell compares with `ToSlash`; the case folding finding is outside what a test
  covers.
- DOC-1: docs/polix.md, docs/use-cases.md and CHANGELOG.md updated; the schema of polix.yaml did not change.
- The three fixes of the first review are present and tested: `TestWatchRemoved`, `TestWatchSnapshotUsedOnce`,
  `TestWatchStashMoved`, `TestTreeDiff` with removed paths and `TestStash`.
- The replay of the step shape holds: within a pass `watch` reads the state, checks the index of `watched` and only
  then appends it; the journal replays the appends in order, `appendEntry` adds `watched` to the state only after the
  log was read, the facts carry it to the derived events, `Follow` is returned only without a pending step, and the
  derived events never follow, so the queue ends.

## Pending

- The case-insensitive rename and the merge stopped on conflicts were not reproduced in a scratch repository: the
  sandbox of the worktree refused git outside it; both findings come from the code and the behaviour of git.
- The manual end to end check of the plan in an interactive Claude Code session.
- `just check`, not run since it replaces the installed binary; its equivalent ran with a binary of the scratchpad.
- The runs of CI on Linux and Windows (`TestRestore`, `TestStash`, `TestCommandEditsWatched`); only macOS ran.
- `claude plugin validate plugins/polix`, not run: the plugin is not part of the change.
