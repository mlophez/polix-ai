# Reviewer latency

`polix:reviewer` runs with `model: inherit` and `effort: high`, and a run of `build` takes too long because of it.
The effort is not the only cause: it multiplies a heavy procedure that every review round repeats in full.

## Causes

- Every round reviews the whole change from scratch: `build` runs up to three reviews (the first and two after
  fixes), and `Previous` only says in `Pending` which findings are fixed, it does not narrow the scope.
- The checklist is heavy by itself: each changed file whole plus its callers and tests (Step 4), the checks of the
  project with the tests (Step 5), the whole folder of rules (Step 8) and a second check of every finding (Step 10).
- `model: inherit` runs it on the model of the session, Opus in practice.

## Changes, by impact

- [ ] 1. Narrow the review rounds after a fix to what changed since the previous review, both in the gate of
      `polix.yaml` and in `build`: check that the `blocking` and `major` findings of the previous review are fixed
      and review only the delta, not the whole change. See `Design of the delta review`.
- [ ] 2. Lower the default effort of `plugins/polix/agents/reviewer.md` to `medium`, keeping `model: inherit`.
- [ ] 3. Optional, if bugs slip through with 1 and 2: have `build` launch the first review at `high` and the rounds
      after a fix at `medium`, passing `effort` to the Agent tool from the skill.
- [ ] 4. In Step 5, run only the tests of the packages the change touches, not the whole suite, instead of leaving
      it to the judgement of the reviewer ("when it is slow").

Recommended: 1 and 2 together; 3 before going back to `high` on every round.

## Design of the delta review

Today `polix.yaml` cannot express it.
The `finished` line of the reviewer stores the content hash (the git tree) it ended on, and
`unless: { finished: polix:reviewer }` holds only while the current tree equals it on the scope of the entry.
When a fix edits a Go file the gate blocks again with the same fixed message, and neither the session nor the
reviewer know what the first review saw, so the whole diff against `main` is reviewed again.
polix already holds the tree of the last review; it only has to expose it.

- [ ] Engine: a placeholder, for example `{reviewed_tree:polix:reviewer}`, that expands to the tree of the latest
      `finished` line of that type on the branch, the same one `finished` compares.
      `git diff <tree> -- <scope>` then shows what changed since that review, read only and with uncommitted
      changes included.
- [ ] Engine: a condition, for example `reviewed: polix:reviewer`, that holds when any `finished` line of that type
      exists on the branch, whatever its content, to tell the first review from the next ones.
- [ ] `polix.yaml`: split the review gate of `pre_commit` in two entries.

      ```yaml
          # Second and later reviews: only what changed since the last one.
          - when:
              actor: session
              changed: ["*.go", "go.mod", "go.sum"]
              reviewed: polix:reviewer
              any: { changed_lines: 150, changed_files: 5, risk_score: 25 }
            unless: { finished: polix:reviewer }
            action:
              block: |
                The diff changed since the last review.
                1. Run polix:reviewer with `Since: {reviewed_tree:polix:reviewer}`: it reviews only
                   `git diff {reviewed_tree:polix:reviewer}` and checks that the findings of the last review are fixed.
                2. Wait for it, fix what matters, then commit again.

          # First review: the whole diff (the current entry, with not: { reviewed: polix:reviewer }).
      ```

- [ ] `plugins/polix/agents/reviewer.md`: a new input `Since: <tree>`.
      It reviews only the hunks of `git diff <tree>`, reading the rest of each touched file as context, checks that
      the findings of the previous review are fixed and reports only findings on the lines of the delta.
      It needs the previous report: the last `#### Review` of the plan in `build`; outside `build`, passed in the prompt or kept
      in the log.
- [ ] `plugins/polix/skills/build/SKILL.md`: the review rounds after a fix use the same `Since:` input, so there is
      one mechanism, not two.
- [ ] Docs: `docs/polix.md` (placeholder and condition), `docs/use-cases.md` and the changelog.

Risks:

- A fix can break code it does not touch, such as a caller of a function whose signature changed: the reviewer
  still reads the callers of the functions the delta changes (Step 4) and runs the tests of the affected packages.
- A tree pruned by `git gc` makes `git diff <tree>` fail: fall back to the whole review, as `finished` counts a
  lost tree as changed.
- The thresholds (`changed_lines`, `risk_score`) keep measuring the whole diff against the base: they decide
  whether a review is needed, not what it covers.

## Not needed

- Dropping Step 8 (rules) from the reviewer: `polix:rules-reviewer` runs neither in `build` nor in `review`, so
  there is no duplicated work.
