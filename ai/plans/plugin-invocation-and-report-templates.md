# Run the plugin only on request and give every report a template

## Overview

The plugin `polix` ships skills and subagents that a team can build its development flow on.
Today the model may pick them on its own: most skill descriptions say "Use it when...", which invites the model to
run them whenever it sees fit, and the agent descriptions do not say who may launch them.
After this change every skill and every agent says that it runs only when a person or the workflow of polix asks
for it.
The skills stay callable by the model, so a message of polix such as "Run /polix:review" still works.

Every report also gets a fixed template.
The reports the subagents return carry their template inside the file of the agent, since an agent launched by
polix cannot find files next to it.
The skills that save a report have a template file next to them, as the skill plan already does.
The review, which today only answers in the chat, now also saves its report in the folder of reports, like the audit.

Tests of the plugin package check both conventions, so a new skill or agent cannot skip them.
The docs and the example fragment for governed repositories describe the conventions.

## Context

The plugin is meant to be the default toolbox of a flow that a person or `polix.yaml` drives, not a set of tools the
model reaches for by itself.
Reports are read by people and by the next phase of the flow, so their shape must be stable and easy to find.
The outcome is consistent descriptions, one template per report, a saved review report and tests that keep it so.

## Decisions

- Skills keep their current `disable-model-invocation`: `false` for all but `init`, which stays `true`.
  The invocation limit lives in the description, so polix can keep naming a skill in a `require` or `deny`.
- Agents are limited by their description only; polix gets no new event to deny launching a subagent.
- The templates of the reports of subagents are a fixed section of the file of the agent.
  The skills that save a report file keep their template in a `template.md` next to `SKILL.md`.
- The read-only reporting agents are `auditor`, `reviewer` and `rules-reviewer`.
  The outputs of `rules-author` and `workflow-author`, like those of the skills `doctor`, `add-rule`, `workflow` and
  `init`, are summaries of what they did, not reports, and keep their inline `Output format`.
- The review report is named `<date>-review-<name>.md`, where `<name>` is the branch in kebab-case.
  On the base branch, `<name>` is short kebab-case English from the intent of the change.
- The skill review writes the file, since the agent `reviewer` has no tool to write.
  A `polix:reviewer` launched directly, without the skill, returns its report in the chat and saves no file.
- Assumption: the authoring skills `writing-skills` and `writing-agents` of `~/.claude/skills/` are not part of polix
  and are not changed; the conventions of the plugin live in `docs/polix.md` and in the tests of the plugin package.

## Approach

The conventions have two parts, each applied to the files of `plugins/polix/` and enforced by tests.

Invocation.
Each skill description keeps its first sentence, a verb in third person saying what the skill does.
Its "Use it when..." sentence is replaced by the fixed sentence "It runs only when the user or the workflow of polix
asks for it."
Each agent description keeps what the agent does and what it never does.
Its "Launched by..." sentence is replaced by the fixed sentence "Launch it only when the user or the workflow of
polix asks for it.", followed by the skill that usually launches it when there is one.
Descriptions stay within 3 sentences and 400 characters.

Templates.
A reporting agent has a section `### Report template` under `References`, holding the skeleton of its report: the
headings of its fields in order and a `<...>` placeholder for each.
Its `Output format` says to fill that template, every field, with "none" for an empty one.
The skill audit moves the shape of its merged report from the section `Report file` of `SKILL.md` to
`skills/audit/template.md`; `SKILL.md` keeps the path, naming and writing rules and points to the template.
The skill review gains a section `Report file` and a `skills/review/template.md`, and writes the report after
relaying it.

Tests.
`plugins/plugins_test.go` reads the frontmatter of every skill and agent, collapses the whitespace of the folded
`description`, and checks the fixed sentences.
It checks that every agent whose `tools` field names neither `Edit` nor `Write` has a `### Report template` section.
It checks that every skill whose `SKILL.md` has a section `### Report file` or `### Plan file` has a `template.md`.
A failure names the file and the convention it breaks.

### Discarded alternatives

- `disable-model-invocation: true` on every skill: the model could not run `/polix:review` when polix requires it.
- A `pre_agent` event in the engine to deny launching a subagent: a guarantee the flow does not need yet.
- Serving templates from the binary with a `polix template` command: one more command for a file only agents read.
- A fixed list of reporting agents in the test: a new read-only agent would escape the check.

## Acceptance criteria

- AC1: The description of every skill holds "It runs only when the user or the workflow of polix asks for it." and no
  "Use it" sentence; `init` keeps `disable-model-invocation: true` and the other skills keep `false`.
- AC2: The description of every agent holds "Launch it only when the user or the workflow of polix asks for it." and
  no "Launched by" sentence.
- AC3: `auditor`, `reviewer` and `rules-reviewer` each have a `### Report template` whose fields match their
  `Output format`, and the `Output format` tells the agent to fill it.
- AC4: The skill audit writes its report from `skills/audit/template.md`, with the same title, header paragraph and
  sections as today, and `SKILL.md` no longer describes them inline.
- AC5: The skill review saves its report to `<reports>/<date>-review-<name>.md` from `skills/review/template.md`,
  with a `-2` suffix when the file exists, and relays the path in its answer.
- AC6: The tests of the plugin package fail when a skill or agent breaks a convention of AC1, AC2, AC3 or a skill
  that saves a file lacks its `template.md`, and pass on the plugin of this change.
- AC7: `docs/polix.md`, `README.md`, `docs/use-cases.md` and `examples/AGENTS-polix.md` describe who runs the
  plugin, the templates and the saved review report, consistently with the plugin.
- AC8: `just test`, `golangci-lint run ./...`, `claude plugin validate plugins/polix` and `just check` pass.

## Out of scope

- The new components of the flow (`implement`, `documenter`, `security-reviewer`, `ship`): a later plan, which
  follows these conventions.
- The progress checklist and the `Ticket:` line of the template of the skill plan: they belong to the plan of
  `implement`.
- `AGENTS.md` and `CLAUDE.md`: protected, and they describe the layout without naming the conventions.
- The authoring skills `writing-skills` and `writing-agents` of `~/.claude/skills/`.
- Any change of the engine.

## Tasks

### Task 1: Skill descriptions say who runs them

- Covers: AC1.
- Files: `plugins/polix/skills/add-rule/SKILL.md`, `plugins/polix/skills/audit/SKILL.md`,
  `plugins/polix/skills/doctor/SKILL.md`, `plugins/polix/skills/init/SKILL.md`,
  `plugins/polix/skills/plan/SKILL.md`, `plugins/polix/skills/review/SKILL.md`,
  `plugins/polix/skills/workflow/SKILL.md` (frontmatter `description`, modify).
- Rules: none.
- Reuse: the first sentence of each current description.
- Changes: in each description, replace the "Use it when..." sentence with "It runs only when the user or the
  workflow of polix asks for it."
  Keep the first sentence and the sentence of what the skill never does.
  In `init`, drop the mention of "/polix:init" from the description, since the fixed sentence covers it.
  In `review`, the first sentence also says that the report is saved in the folder of reports.
  Leave `disable-model-invocation` as it is in every skill.
- Tests: covered by the convention tests of Task 6.
- Verification: `claude plugin validate plugins/polix`, expected: no errors.

### Task 2: Agent descriptions say who launches them

- Covers: AC2.
- Files: `plugins/polix/agents/auditor.md`, `plugins/polix/agents/builder.md`, `plugins/polix/agents/reviewer.md`,
  `plugins/polix/agents/rules-author.md`, `plugins/polix/agents/rules-reviewer.md`,
  `plugins/polix/agents/workflow-author.md` (frontmatter `description`, modify).
- Rules: none.
- Reuse: the sentences of each current description on what the agent does and never does.
- Changes: replace the "Launched by..." sentence with "Launch it only when the user or the workflow of polix asks
  for it.", followed in the same sentence by the skill that usually launches it: `audit` for `auditor`, `review`
  for `reviewer`.
  `builder` follows it with the plan and the task, several at once on tasks that do not depend on each other.
  `rules-reviewer`, `rules-author` and `workflow-author` have no launching skill and end the sentence there.
  `rules-reviewer` drops "before a commit or a pull request", which invited the model to launch it by itself.
- Tests: covered by the convention tests of Task 6.
- Verification: `claude plugin validate plugins/polix`, expected: no errors.

### Task 3: Report templates in the reporting agents

- Covers: AC3.
- Files: `plugins/polix/agents/auditor.md`, `plugins/polix/agents/reviewer.md`,
  `plugins/polix/agents/rules-reviewer.md` (sections `References` and `Output format`, modify).
- Rules: none.
- Reuse: the fields of the current `Output format` of each agent, in their order.
- Changes: add a `### Report template` section as the last subsection of `References`, with a fenced markdown
  skeleton of the report: one `##` heading per field of `Output format`, in its order, and a `<...>` placeholder
  under each that says what goes there.
  For findings, the placeholder shows the shape of one finding with the attributes the agent already lists: for
  `auditor` severity, category, confidence, `file:line`, what is wrong, evidence, impact and fix direction; for
  `reviewer` severity, category, `file:line`, what is wrong, evidence and fix direction; for `rules-reviewer` the
  rule id, `file:line`, the code at fault and why.
  Replace the bullet list of `Output format` with an instruction to fill `Report template`, every heading in its
  order, with "none" under an empty one, and keep the sentence that the main session relays the output.
- Tests: covered by the convention tests of Task 6.
- Verification: `claude plugin validate plugins/polix`, expected: no errors.

### Task 4: Template of the audit report

- Covers: AC4.
- Files: `plugins/polix/skills/audit/template.md` (create), `plugins/polix/skills/audit/SKILL.md` (sections
  `Requirements`, `Report file` and `How to operate`, modify).
- Rules: none.
- Reuse: the title, header paragraph and sections described today in the section `Report file` of `SKILL.md`, and
  the template of the skill plan in `plugins/polix/skills/plan/template.md` as the model of the format.
- Changes: create `template.md` with the title `# Audit of <scope>`, the header paragraph as a `<...>` placeholder
  naming the date, the base commit and branch, uncommitted changes, the areas and their auditors and what was out of
  scope, and the sections `Summary`, `Findings`, `Checks`, `Coverage` and `Pending` as `##` headings with a
  placeholder each.
  In `SKILL.md`, add `template.md` to `Requirements`, stopping and telling the user when it is missing.
  Keep in `Report file` the path, the naming, the suffix, the language and the line rules, and replace the
  description of the title, paragraph and sections with "Fill template.md: its sections in its order, every
  `<...>` replaced".
  Step 6 of the checklist points to `template.md`.
- Tests: covered by the convention tests of Task 6.
- Verification: `claude plugin validate plugins/polix`, expected: no errors.

### Task 5: The skill review saves its report

- Covers: AC5.
- Files: `plugins/polix/skills/review/template.md` (create), `plugins/polix/skills/review/SKILL.md` (opening
  paragraph, `Requirements`, `Rules`, `References`, `How to operate` and `Output format`, modify).
- Rules: none.
- Reuse: the section `Report file` of `plugins/polix/skills/audit/SKILL.md` for the path, suffix, language and line
  rules; the fields of the `Report template` of `reviewer` from Task 3.
- Changes: create `template.md` with the title `# Review of <change>`, a header paragraph placeholder with the date,
  the base commit and branch, uncommitted changes and the intent marked as given or inferred, and the sections
  `Verdict`, `Findings`, `Checks`, `Held` and `Pending` as `##` headings with a placeholder each.
  In `SKILL.md`, the opening paragraph says the report is saved in the folder of reports, `reports.path` of
  `polix.yaml`, `ai/reports/` without it.
  Add `template.md` to `Requirements`.
  Replace the rule "Never edit a file during the review." with "Never edit a file of the project: the report file is
  the only file you write."
  Add a `### Report file` section under `References`: path `<reports>/<date>-review-<name>.md`, `<date>` as
  `YYYY-MM-DD`, `<name>` the current branch in kebab-case, or short kebab-case English from the intent when the
  branch is the base; a `-2` suffix when the file exists; written in the language of the docs of the repository,
  English when there are none; one sentence per line, at most 120 characters, no tables; filled from `template.md`.
  Add a checklist step after the relay that writes the report file, before the step that stops.
  Add the field **Report**, the path of the report file, first in `Output format`.
- Tests: covered by the convention tests of Task 6; a manual run in `Verification`.
- Verification: `claude plugin validate plugins/polix`, expected: no errors.

### Task 6: Tests of the conventions of the plugin

- Covers: AC6.
- Files: `plugins/plugins_test.go` (new tests next to `TestFrontmatterNames`, modify).
- Rules: GO-2, GO-3, XPLAT-1.
- Reuse: the frontmatter parsing of `TestFrontmatterNames` and `FS` from `plugins/plugins.go`; extract the parsing to
  a helper of the test file shared by the old and the new tests.
- Changes: add a test that, for every skill, collapses the whitespace of the frontmatter `description` and checks
  it holds the fixed sentence of AC1 and no "Use it".
  Add a test that does the same for every agent with the fixed sentence of AC2 and no "Launched by".
  Add a test that every agent whose frontmatter has a `tools` field without `Edit` and without `Write` has a
  `### Report template` heading in its body.
  Add a test that every skill whose `SKILL.md` has a `### Report file` or `### Plan file` heading has a
  `template.md` in its folder.
  Each failure message names the file and the convention, as `TestWorkflowReference` does.
- Tests: the new tests themselves; check by hand that each fails when its convention is broken in one file, then
  restore the file.
- Verification: `just test`, expected: pass; `golangci-lint run ./...`, expected: no issues.

### Task 7: Docs of the conventions

- Covers: AC7.
- Files: `docs/polix.md` (section "The plugin and the hooks: polix install", modify), `README.md` (the paragraph
  after the list of the harness and the note "The harness is in progress", modify), `docs/use-cases.md` (section
  "Review every change with a reviewer subagent", modify), `examples/AGENTS-polix.md` (modify).
- Rules: GO-2, XPLAT-2.
- Reuse: the current lists of skills and agents in each file.
- Changes: in `docs/polix.md`, after the list of skills and agents, add a paragraph on the conventions: the skills
  and agents run only when the user or the workflow of polix asks for it, as their descriptions say; the reports of
  the agents follow the template inside each agent; `audit` and `review` save their report in the folder of
  reports from their `template.md`; a `polix:reviewer` launched without the skill saves no file; the tests of the
  package `plugins` check it.
  Also update the description of `review` in the list: it saves the report in the folder of reports.
  In `README.md`, replace "A skill only helps the agent" with the fact that a skill or agent runs only when you or
  your `polix.yaml` ask for it, and that polix enforces a phase when `polix.yaml` says so.
  In `docs/use-cases.md`, add to the notes of the section that `/polix:review` saves its report in the folder of
  reports, so each review leaves evidence next to the plan.
  In `examples/AGENTS-polix.md`, add one sentence in Spanish, as the rest of the fragment, telling the agent to run
  the skills and agents of polix only when the user or the workflow of polix asks for them.
- Tests: `TestWorkflowReference` still passes, since `examples/polix.yaml` does not change.
- Verification: `rg -n "Use it when|Launched by" plugins/polix docs README.md`, expected: no match.

## Risks

- A description without a trigger makes the skill less likely to be picked when polix names it loosely: keep the
  messages of `require` and `deny` in the docs naming the exact skill, as `/polix:review`.
- The fixed sentences wrap across lines in the folded YAML: the tests collapse whitespace before matching.
- The audit or review report changes shape by accident while moving it to a template: compare the template with the
  last report in `ai/reports/` and keep the same headings.
- Codex has no agents of plugins and follows `reviewer.md` in the session: the skill review still writes the file,
  so the behaviour is the same.

## Verification

- `just test`: pass.
- `golangci-lint run ./...`: no issues.
- `claude plugin validate plugins/polix`: no errors.
- `just check`: install, `polix check`, `polix rules check` and `polix doctor --offline` pass.
- End to end: run `just install` and start a new session so the plugin reloads.
  Make a small change on a branch and run `/polix:review`.
  Check that `ai/reports/<today>-review-<branch>.md` exists, follows `skills/review/template.md` and that the answer
  gives its path.
  Run `/polix:review` again and check that a `-2` file is written instead of overwriting the first.
- End to end: ask an unrelated question in a session and check that the model does not launch any `polix:` skill or
  agent on its own.
