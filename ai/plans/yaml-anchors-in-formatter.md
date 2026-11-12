# Support YAML anchors and aliases in the formatter of polix.yaml and of the rules files

- Status: built
- Risk: high: the formatter rewrites the polix.yaml and the rules files of the users, a file format others edit.
- Reference: none.
- Created: 2026-11-12.
- Size: 2 tasks, about 330 changed lines.

## Overview

Today `polix config fmt` and `polix rules fmt` refuse any file that uses a YAML anchor (`&name`) or alias (`*name`).
`polix config check` accepts them, so a team that shares one action between two entries of `polix.yaml` with an
anchor loses the formatter, and the format run after each edit fails every time.

After this change both commands format such files and keep each anchor and alias where the user wrote it.
They keep refusing, each with a clear message, the cases that would be wrong or ambiguous:

- an anchor or alias in front of a key, as `- &fmt when: ...`, which anchors the word `when` and not the entry;
- an anchor on the whole document;
- the same anchor name defined twice;
- a file where putting the sections in their canonical order would move an alias above its anchor, which is invalid
  YAML: the user moves the anchor up and formats again.

The merge key `<<: *name` stays refused, as `polix config check` already rejects it.

## Request

implement the support of anchors in the formatter

## Context

`internal/platform/yamlfmt` formats a YAML document from the node tree of yaml.v3 and the source lines.
Its function `supported` refuses every node with an anchor and every alias with
`line <n>: anchors and aliases are not supported by the formatter`.
Both `polix config fmt` (`internal/app/workflow/format`) and `polix rules fmt` (`internal/app/rule/format`) call
`yamlfmt.Format` and show that error as the problem of the file.

## Constraints

- Every document without anchors or aliases formats byte for byte as before: the layout is a contract of both commands.
- The final check of `Format` (`same`: same data, same comments) stays and keeps guarding every output.
- `internal/platform/yamlfmt` never imports `internal/app/` (GOARCH-42) and stays without domain (GOARCH-19).
- No non-test Go file over 400 lines nor function over the complexity limits (GO-4); `yamlfmt.go` has 321 lines and
  `yamlfmt_write.go` 350, so new code may need a sibling file such as `yamlfmt_anchor.go`.
- The docs of the behaviour change in the same change (DOC-1), in English (GO-2).

## Decisions

- A reorder that would put an alias above its anchor makes `Format` fail, never write: answer of the interview.
- Anchors and aliases on the keys of a mapping stay refused: answer of the interview.
- The merge key `<<` stays refused: answer of the interview, consistent with `polix config check`.
- An anchor on the root collection and an anchor name defined twice are refused explicitly: answer of the interview.
- The messages follow the style of the other refusals and wrap `ErrUnsupported`, with the line of the offending node:
  answer of the interview.
  The texts are `line <n>: anchors and aliases on keys are not supported by the formatter`,
  `line <n>: the alias *<name> would come before its anchor &<name>: move the anchor up`,
  `line <n>: an anchor on the whole document is not supported by the formatter` and
  `line <n>: the anchor &<name> is defined twice: rename one`.
- An anchor on a block collection is written at the end of the line of its key or of its dash, never followed by the
  first key on the same line: yaml.v3 reads `- &a k: v` as an anchor on the key `k` (checked while planning).

## Approach

All the work is in `internal/platform/yamlfmt`; the two use cases only see fewer errors.

Validation: `supported` stops refusing anchors and aliases on values and refuses instead an anchor or alias on a key,
an anchor on the root, a merge key `<<` (with the existing message `anchors and aliases are not supported`) and an
anchor name defined twice in the document.

Reading: yaml.v3 gives an anchored node the line and column of its `&`, not of its content (checked while planning:
`a: &x |` gives the block scalar column 4, the column of `&x`).
Every reader that uses `Line` or `Column` to find text in the source must skip the anchor and its spaces first:
`bodyHeader` for block scalars, `quoted` for quoted scalars, and the column of a block collection in `readTrivia`
(which must be the column of its first key or dash, not of the anchor, for the comments kept below an item).
An anchored block collection keeps the line of its `&`, which is the line of its key or dash, so the comments above
it stay where they are.

Writing: the writer adds `&<name> ` before the text of an anchored scalar, block scalar header or flow collection,
writes an alias as `*<name>` wherever a value goes (mapping value, sequence item, inside a flow collection), and for
an anchored block collection writes the anchor alone at the end of the line of its key (`run: &test`) or of its
dash (`- &fmt`), then the collection on the lines below at the usual indentation.
A sequence item that is an anchored block collection is therefore no longer written with its first key on the line
of the dash, and the comments above its first key stay above that key, not above the dash.

Order check: after `reorder`, a walk of the tree in output order records each anchor as defined when it reaches it and
fails on the first alias whose anchor it has not reached yet, with the message of the Decisions and the line of that
alias in the source.
It runs before writing, so the generic "does not parse" error of `same` never shows for this case.

### Discarded alternatives

- Move the definition of the anchor to the first node of the output: always formats, but moves the content and its
  comments away from where the user wrote them; rejected in the interview.
- Keep the source order of a mapping whose reorder would cross an anchor: the output silently stops being canonical;
  rejected in the interview.
- Accept anchors on keys: keeps `- &fmt when:` as is, hiding the usual mistake until `polix config check`; rejected in
  the interview.

## Acceptance criteria

- AC1: `polix config fmt` formats a `polix.yaml` with an anchor on a value and its alias, keeps `&<name>` and
  `*<name>` where they were, writes the file, and the result decodes to the same data; a second run changes nothing.
- AC2: anchors and aliases work on every kind of value: plain, quoted and block scalars, flow collections, block
  mappings and block sequences, as the value of a key and as an item of a sequence, and an alias inside a flow
  collection.
- AC3: an anchored block collection is written as `key: &<name>` or `- &<name>` alone on its line, with its content
  below; `- &<name> key: value` is never produced.
- AC4: an anchor or alias in front of a key fails with
  `line <n>: anchors and aliases on keys are not supported by the formatter`, and nothing is written.
- AC5: an anchor on the root collection fails with
  `line <n>: an anchor on the whole document is not supported by the formatter`, and nothing is written.
- AC6: an anchor name defined twice fails with `line <n>: the anchor &<name> is defined twice: rename one`, with the
  line of the second definition, and nothing is written.
- AC7: a merge key `<<: *<name>` still fails with `line <n>: anchors and aliases are not supported by the formatter`.
- AC8: when the order of the keys would put an alias above its anchor, `Format` fails with
  `line <n>: the alias *<name> would come before its anchor &<name>: move the anchor up`, with the line of the alias
  in the source, and nothing is written; with `--check` the same problem is printed.
- AC9: `polix rules fmt` gets the same behaviour, since it shares the formatter.
- AC10: every document without anchors or aliases formats exactly as before the change.
- AC11: the package comment of `yamlfmt`, `internal/app/workflow/format/format.md`,
  `internal/app/rule/format/format.md` and the paragraph of `polix rules fmt` in `docs/polix.md` say what is
  accepted and what is refused.

## Out of scope

- Applying the formatter to `polix.yaml` of other repositories, such as the platform repository that started this.
- How `polix config check`, the log or the `once` keys report entries that share a line through an alias.
- Merge keys and anchors on keys, refused by decision.

## Starting point

- Read first: `internal/platform/yamlfmt/yamlfmt.go` (`Format`, `parse`, `supported`, `reorder`, `same`): the
  pipeline and the safety net.
- Read first: `internal/platform/yamlfmt/yamlfmt_write.go` (`mapping`, `sequence`, `value`, `inline`, `scalar`,
  `quoted`): where each node kind is written, and `prefix` and `leadIndent` for the dash of a sequence item.
- Read first: `internal/platform/yamlfmt/yamlfmt_read.go` (`readBodies`, `bodyHeader`, `readTrivia`, `items`): how
  lines and columns of the nodes locate comments and block scalars.
- Read first: `internal/app/workflow/format/logic_test.go` (`TestDecideProblem`) and
  `internal/app/rule/format/logic_test.go` (`TestDecideProblem`): both use an anchor today as their unformattable
  input.
- Baseline: `just test`, expected: go vet clean and all 46 packages with tests `ok`, no failures.

## Progress

- [x] Task 1: Format anchors and aliases on values, and refuse them on keys, on the root and when repeated. Evidence: new `TestFormatAnchors`, `TestFormatAnchorOnItsOwnLine`, `TestFormatAnchorComments`, `TestFormatAnchorRefusals`, `TestFormatUnsupported` (merge key) and `TestDecideAnchors` failed before the writer and readers changed and pass now; the kept tests of the layout, order, spaced, blocks, `TestFormatRepository` and the two `TestDecideProblem` stay green; `just test` and `golangci-lint run ./...` clean; differential check of `config fmt` and `rules fmt`, `--check` and in place, identical between the base and the new binary on `polix.yaml`, `examples/polix.yaml`, `ai/rules/*.yaml` and the inputs of the layout tests.
- [x] Task 2: Refuse a key order that would put an alias above its anchor. Evidence: new `TestFormatAliasBeforeAnchor`, `TestFormatAliasInLaterSection` (yamlfmt) and `TestDecideAliasBeforeAnchor` (workflow format) failed with the generic "does not parse" error without the check and pass with it; `TestFormatOrder` and the tests of Task 1 stay green; `just test` and `golangci-lint run ./...` clean; differential check of both commands on `polix.yaml`, `examples/polix.yaml` and `ai/rules/*.yaml` identical between the base and the new binary.

## Tasks

### Task 1: Format anchors and aliases on values, and refuse them on keys, on the root and when repeated

- Covers: AC1, AC2, AC3, AC4, AC5, AC6, AC7, AC9, AC10, AC11.
- High risk: a file format others edit, rewritten in place by both commands; the `kept` tests of the layout and the
  differential check guard that documents without anchors format as before, and `same` guards the data.
- Files: `internal/platform/yamlfmt/yamlfmt.go` (`supported`, package comment, modify),
  `internal/platform/yamlfmt/yamlfmt_write.go` (`sequence`, `value`, `inline`, `scalar`, `quoted`, modify),
  `internal/platform/yamlfmt/yamlfmt_read.go` (`bodyHeader`, `readTrivia`, modify),
  `internal/platform/yamlfmt/yamlfmt_anchor.go` (create, only if the files above would pass 400 lines or a function
  the complexity limits), `internal/platform/yamlfmt/yamlfmt_test.go` (modify),
  `internal/platform/yamlfmt/yamlfmt_anchor_test.go` (create, only with `yamlfmt_anchor.go`),
  `internal/app/workflow/format/logic_test.go` (`TestDecideProblem`, modify),
  `internal/app/rule/format/logic_test.go` (`TestDecideProblem`, modify),
  `internal/app/workflow/format/format.md` (the refusals of the layout, modify),
  `internal/app/rule/format/format.md` (the refusals of the layout, modify),
  `docs/polix.md` (the paragraph of `polix rules fmt`, "not YAML, an anchor, an alias", modify).
- Rules: GO-2, GO-3, GO-4, GOARCH-1, GOARCH-7, GOARCH-17, GOARCH-19, GOARCH-35, GOARCH-38, GOARCH-42, GOARCH-43,
  XPLAT-1, DOC-1.
- Reuse: `ErrUnsupported`, `same`, `offset`, `prefix`, `leadIndent` and `TestFormatIdempotent` in
  `internal/platform/yamlfmt`.
- Tests:
  - `new` in `yamlfmt_test.go`, a table test that formats with nil order, for each kind of AC2: an anchored plain
    scalar and its alias, an anchored double-quoted scalar that keeps its quotes, an anchored block scalar `|` with
    its content, an anchored flow mapping and flow sequence, an anchored block mapping as the value of a key, an
    anchored block mapping as a sequence item, an anchored block sequence, an alias as a sequence item and an alias
    inside a flow collection; each compares the exact output, then checks a second pass is identical.
    It fails before the change with `anchors and aliases are not supported by the formatter`.
  - `new` in `yamlfmt_test.go`: a sequence item `- &e` whose mapping has a comment above its second key and a comment
    below its last line, indented deeper than the next item; the output keeps both comments where they were, which
    fails before the change and guards the column of `readTrivia`.
  - `new` in `yamlfmt_test.go`: an anchored block mapping as a sequence item, unformatted with extra indentation, comes
    out as `- &<name>` alone on its line and its keys below (AC3), never `- &<name> key:`.
  - `new` in `yamlfmt_test.go`: the four refusals, each with `errors.Is(err, ErrUnsupported)` and its exact text: an
    anchor on a key (`&k a: 1`), an alias as a key (`*k: 1` after `x: &k a`), an anchor on the root (`&r` above a
    mapping), and an anchor name defined twice with the line of the second; each fails before the change with the old
    generic message.
  - `kept` `TestFormatUnsupported`: the input `a: &x 1\nb: *x\n` moves out of it into the new table of formatted
    inputs; a merge key `a: &x { k: 1 }\nb:\n  <<: *x\n` is added and still fails with the generic message (AC7).
  - `kept` `TestFormatLayout`, `TestFormatOrder`, `TestFormatSpaced`, `TestFormatSpacedOneLine`,
    `TestFormatIdempotent`, `TestFormatEdges`, `TestFormatBlockScalars`, `TestFormatRepository` and the rest of the
    package: unchanged and passing, the layout of documents without anchors holds (AC10).
  - `kept` `TestDecideProblem` of `internal/app/workflow/format`: its anchored input becomes an anchor on a key
    (`a: 1\n&x b: 2\n`), and it still checks a problem containing `anchors` and no write.
  - `kept` `TestDecideProblem` of `internal/app/rule/format`: the file `d.yaml` becomes an anchor on a key
    (`rules:\n  - &x id: A-1\n`), and it still checks the problem of `d.yaml` contains `anchors` and nothing is
    written (AC9).
  - `new` in `internal/app/workflow/format/logic_test.go`: `Decide` on a `polix.yaml` whose `run` of one entry has an
    anchor and another entry an alias to it, unformatted, returns `Changed`, no problem, and a `Write` that keeps
    `&<name>` and `*<name>` (AC1).
- Changes:
  - `supported` accepts an anchor or alias on any value and refuses an anchor or alias on a key, an anchor on the
    root collection (it receives whether the node is the root), a merge key `<<` with the existing generic message,
    and a second definition of an anchor name, tracking the names already seen in source order; each with the
    message of the Decisions and the line of the node.
  - The writers prefix the anchor: `scalar` and the header of a block scalar get `&<name> ` first; `inline` gets it
    for flow collections and writes an alias as `*<name>`; `value` writes `key: &<name>` alone for an anchored block
    collection and `key: *<name>` for an alias; `sequence` writes `- &<name>` alone for an anchored block collection
    and then the collection at `indent+2` with no dash prefix, so its first key goes on the next line, and `- *<name>`
    for an alias.
  - `quoted` and `bodyHeader` skip the anchor and the spaces after it from the column yaml.v3 gives, before they read
    the quote or the header.
  - `readTrivia` takes the column of a block collection from its first item (the key of its first pair, or its first
    item) when the collection has an anchor, instead of the column of the anchor.
  - The package comment, the two `format.md` and `docs/polix.md` replace "anchors, aliases" in their lists of
    refusals with the refused cases of the Decisions, and say that anchors and aliases on values are kept where
    they are written.
- Verification: `just test -run 'TestFormat|TestSame|TestDecide'`, expected: all pass.
  Differential check: build the base with `git archive HEAD | tar -x -C <temporary directory>` and `go build -o
  <temporary directory>/polix-base ./cmd/polix` there, and the working tree with `go build -o <temporary
  directory>/polix-new ./cmd/polix`.
  Copy into temporary repositories `polix.yaml`, `examples/polix.yaml`, `ai/rules/*.yaml` and the unformatted inputs
  of `TestFormatLayout`, and run `polix config fmt --check` and `polix rules fmt --check` with both binaries: output
  and exit code must be identical.
  Then run `polix config fmt` with both on a copy of each and compare the written files byte for byte: identical.
  Only a file with anchors may differ, and only as AC1 asks.

### Task 2: Refuse a key order that would put an alias above its anchor

- Covers: AC8, AC9, AC11.
- High risk: same contract as Task 1; the differential check guards that no file that formatted before now fails.
- Files: `internal/platform/yamlfmt/yamlfmt.go` (`rewrite`, `Format`, package comment, modify),
  `internal/platform/yamlfmt/yamlfmt_anchor.go` (when created by Task 1, else a new function in `yamlfmt.go`),
  `internal/platform/yamlfmt/yamlfmt_test.go` or `yamlfmt_anchor_test.go` (the one of the file changed, modify),
  `internal/app/workflow/format/logic_test.go` (modify), `internal/app/workflow/format/format.md` and
  `internal/app/rule/format/format.md` (modify), `docs/polix.md` (the paragraph of `polix rules fmt`, modify).
- Rules: GO-2, GO-3, GO-4, GOARCH-7, GOARCH-19, GOARCH-35, GOARCH-38, GOARCH-42, XPLAT-1, DOC-1.
- Reuse: `reorder` and `ErrUnsupported` in `internal/platform/yamlfmt`.
- Tests:
  - `new` in the test file of the changed file: with an order that puts key `b` before key `a`, the document
    `a: &x 1\nb: *x\n` fails with `errors.Is(err, ErrUnsupported)` and the text
    `line 2: the alias *x would come before its anchor &x: move the anchor up`; before the change it fails with
    the generic `the formatted document does not parse` error of `same`.
  - `new` in the same file: the same document with an order that keeps `a` first formats and keeps both.
  - `new` in the same file: an alias nested in a later section that the order moves above the section of its anchor,
    as in a `polix.yaml` with `githooks` written before `on`, the anchor in `githooks` and the alias in `on`, formatted
    with the order of `internal/app/workflow/format`, fails with the line of the alias.
  - `new` in `internal/app/workflow/format/logic_test.go`: `Decide` on that `polix.yaml` returns that problem and no
    `Write`, and with `check` true the same problem.
  - `kept` the tests of Task 1 and `TestFormatOrder`: orders that do not cross an anchor format as before.
- Changes:
  - A new function walks the tree after `reorder`, in output order (keys of a mapping, then values, in their new
    order; items of a sequence in order; flow collections included), records each anchor name when it reaches the
    anchored node, and returns the error of the Decisions for the first alias whose name it has not recorded, with the
    line of the alias.
  - `rewrite` (or `Format`) runs it right after `reorder` and before writing, and `Format` returns its error, so
    nothing is written.
  - The package comment, the two `format.md` and `docs/polix.md` add that a file whose canonical order would put an
    alias above its anchor is refused, and that moving the anchor up fixes it.
- Verification: `just test -run 'TestFormat|TestDecide'`, expected: all pass.
  Differential check: repeat the one of Task 1 with both binaries on the same inputs: identical outputs and exit
  codes for every file without anchors, and the files with anchors of Task 1 still format.

## Risks

- A reader that still uses the column of the anchor reads the wrong text, such as `&` as the header of a block
  scalar: the tests of every kind of AC2 compare the exact output, and `same` refuses a changed result.
- An anchored sequence item loses or moves the comments above its first key, since its first key leaves the line of
  the dash: the comment test of Task 1 covers it, and `same` refuses a lost comment.
- A file that formatted before now fails: impossible for files without anchors, which were refused before; the
  differential check confirms the outputs are identical.
- The order check misses an alias inside a flow collection or a nested sequence: Task 2 tests a nested alias, and
  `same` still refuses an output that does not parse.

## Verification

- `just test`: go vet clean and every package `ok`.
- `golangci-lint run ./...`: no findings, including the limits of GO-4.
- `polix rules check`: every rule with a check passes.
- `just build`, then in a temporary git repository with a `polix.yaml` that holds an entry of `on.stop` with
  `run: &test` and an entry of `githooks.pre_commit` with `run: *test`, unformatted: `polix config fmt` writes it
  keeping both, `polix config check` prints ok, and a second `polix config fmt` prints that nothing changed.
- In the same repository, move `githooks` above `on` with the anchor in `githooks` and the alias in `on`:
  `polix config fmt` prints `line <n>: the alias *test would come before its anchor &test: move the anchor up` and
  leaves the file as it was.

## Iterations

### Iteration 01: 2026-11-12

- **Tasks**: Task 1 done; Task 2 done.
- **Departures**: merge key `<<` refused only when its value uses an anchor or alias (`<<: {a: 1}` formats as before,
  AC10); the anchor prefix is written in `inline` (new `bare`), not `scalar`; `yamlfmt_anchor.go` and
  `yamlfmt_anchor_test.go` created; a comment on the anchor line (`a: &x # note`) moves to the first key of the
  collection, stable on a second pass; Fix 1 updated the "block after break" case of `TestFormatAnchors`, which
  recorded the bug; a comment on its own line between an anchor and its block scalar header is still refused by
  `same` ("would lose a comment"), nothing written.
- **Docs**: `docs/polix.md` (merge key added to the refusals of `polix rules fmt`), `CHANGELOG.md` (`### Changed`
  entry under `[Unreleased]`); the builder updated `internal/app/workflow/format/format.md`,
  `internal/app/rule/format/format.md` and `docs/polix.md`.
- **Review rev01**: approved with comments; fixed the major bug of a block scalar header on the line below its anchor
  (Fix 1, `bodyHeader`, `nextContent`, `readBody`, `TestFormatAnchorBlockHeaderBelow`); fixed the minor docs finding
  in the docs step; skipped the major on `ai/tasks/reviewer-latency.md`, a pre-existing untracked file of the user
  outside this change, to leave out of its commit.
- **Review rev02**: approved with comments; fixed the major changelog finding in the docs step; pre-existing and not
  introduced: `a:\n  >\n    one\n  # a comment\nb: 1\n` fails with "would change the data" without anchors too.
- **Acceptance**: AC1 met (end to end: `run: &test`/`*test` formatted, `config check` ok, second run formatted;
  `TestDecideAnchors`); AC2 met (`TestFormatAnchors`, `TestFormatAnchorBlockHeaderBelow`); AC3 met
  (`TestFormatAnchorOnItsOwnLine`); AC4, AC5, AC6 met (`TestFormatAnchorRefusals`, `yamlfmt_anchor.go:28,30,42`);
  AC7 met (`TestFormatUnsupported`, `yamlfmt_anchor.go:46`); AC8 met (`TestFormatAliasBeforeAnchor`,
  `TestDecideAliasBeforeAnchor`, end to end `line 11: the alias *test would come before its anchor &test: move the
  anchor up`, exit 1, file untouched); AC9 met (rule format `TestDecideProblem`, `rules fmt` probe of the reviewers);
  AC10 met (kept layout tests, `TestFormatRepository`, differential checks identical); AC11 met (package comment,
  both `format.md`, `docs/polix.md`).
- **Status**: built.
