# polix plan check

The plan stays in Markdown: it is read by a human reviewer and by the agents, and its content is mostly prose.
But Step 7 of `/polix:plan` (self-review) spends tokens on checks a program can decide, and the model can miss them.
The plan already has enough structure to parse: the YAML frontmatter, the fixed headings (`##### CUJ-NN`,
`#### AC-NN`, `#### Task N`) and the fields with a fixed prefix (`Covers:`, `CUJs:`, `Verify:`, `Tests:`).

## Idea

A command `polix plan check <plan>` that parses the plan file and reports each problem with its line, exit 1 if any.

- [ ] Frontmatter: the keys of `template.md`, a valid `status` and `risk`, and `tasks` equal to the number of tasks.
- [ ] Sections: the sections of `template.md`, in its order and at its levels.
- [ ] Placeholders: no `<...>` of the template left, and none of the patterns `Plan file` forbids (TBD, TODO...).
- [ ] Task data: each task's `yaml` block parses, holds the keys of `template.md` and no others.
      `files` is not empty, every value is one line, and `tests` is a map by test file or `none, <why>`.
- [ ] Coverage: every CUJ has a criterion whose `CUJs:` names it.
      Every criterion has a `Requirement:`, a `Verify:` and a task whose `covers` names it.
      No reference to a CUJ, AC or Task that does not exist.
- [ ] Consistency: the checklist of criteria matches the `#### AC-NN` blocks, and `Progress` matches the
      `#### Task N` headings, by number and title.
- [ ] Format: no line over 120 characters outside the blocks of literal data, not counting `**`.
      No tables, no "Also" chains, and the label of a bullet in bold.
      No paragraph over three sentences, and no blank line between the items of a list.
      It could offer `--fix` for the bold labels and the blank lines, which are mechanical.

## Where it plugs in

- [ ] Step 7 of `/polix:plan` runs it first and fixes what it reports.
      The model keeps only the checks that need judgement: whether `Outcome` solves `Problem`, choices it made alone,
      and readability.
- [ ] `/polix:build` runs it before it starts, and refuses a plan that fails.
- [ ] Optional: a gate of `polix.yaml` that runs it when a file of the folder of plans is written.

## Open questions

- Whether the checks come from `template.md` itself, so the template stays the only source of the format.
- How it treats the plans in the format before the outcome-driven plan, which `/polix:build` still builds.
- The domain and the use case it belongs to under `internal/app/` (likely a `plan` domain with a `check` use case).

Next step: plan it with `/polix:plan`.
