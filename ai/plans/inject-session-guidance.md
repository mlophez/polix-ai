# Inject built-in guidance on commits and edits at session start, switchable from polix.yaml, in English

## Overview

When a session starts in a repository governed by polix, polix will tell the agent two things on its own:
make each commit as a plain, isolated `git commit`, and change files with the agent's edit tool instead of the shell.

Both help polix see what the agent does.
polix recognises a commit and a shell write by heuristics that miss chained or wrapped commands.
Its edit rules and checks only run when a file changes through the edit tool.
The guidance only asks the agent to cooperate: it blocks nothing.

Each piece can be turned off in polix.yaml with a new key, `guidance`, holding one switch per piece.
Both are on when the key is left out, so every governed repository gets them without writing anything.
Repositories without polix.yaml, or with an invalid one, get nothing, as today.

The guidance is written in English, and so will be the message polix gives when it blocks a write to a path the
repository protects, which is in Spanish today.

## Context

The `pre_commit` gate and the `protected` and `writes` checks rely on heuristics over the command line.
They err on commits chained with `&&`, wrapped in `sh -c` or mentioned inside quotes, and do not see files written by
interpreters, `cd` or heredocs.
Edits made from the shell skip `pre_edit` and `post_edit`, and with them the rules injected by `context: { rules }`.
The system prompt of Claude Code in auto mode even encourages editing through Bash.
The outcome: the agent is told at session start to use the forms polix recognises, and each repository can opt out.

## Decisions

- The switch is a new top level key `guidance`, a map of booleans: `commit` and `edit`, both true by default.
- Only two pieces of guidance; the one on skills and subagents was dropped in the interview.
- Injection only: no `warn` nor `deny` on `pre_command` when the agent ignores the guidance.
- It applies only with a present and valid polix.yaml; without it polix keeps applying only its protections.
- The texts are agent neutral ("the edit tool of the agent", not `Edit` or `apply_patch`), so Claude Code and Codex
  share them.
- The texts are in English, as GO-2 asks for the code, although the other texts polix shows to the agent are in
  Spanish.
- The reasons of a block by `protected` move to English too, asked by the user, keeping their meaning and their
  placeholders: for a path, ".env is protected in polix.yaml: only a person changes it."; for a folder or a wildcard
  that holds a protected glob, "ai holds ai/rules/**, protected in polix.yaml: only a person changes it.".
  Only those reasons change; the other texts of polix stay in Spanish.
- The texts allow a commit message by `-m`, `-F` or a heredoc inside `-m`; only chaining and wrapping are discouraged.
- The guidance is injected before the contexts of the `session_start` entries of the repository, so the repository
  has the last word.
- Injecting it is not logged, like any `context` action today.
- Assumption: no release of polix has been published (the repository has no tags), so `requires` and the
  "Known since polix 0.1.0" note of examples/polix.yaml stay as they are.
- Assumption: polix.yaml of this repository keeps the defaults and does not write the key.

## Approach

The workflow domain gains a value `Guidance` (package `internal/workflow`), held by `Workflow` in a new field
`Guidance`.
It has the booleans `Commit` and `Edit` and the keys the file used, so `Valid` rejects an unknown key with the
message format of `unknownKeys` (for example `unknown key "delegation" in guidance (commit, edit)`).
It has one method named after its use, returning the texts of the pieces turned on, in the order commit, edit.
The texts are in English and live in a table of the domain file, built from literals, as GOARCH-4 allows.

The parser (`workflow_parse.go`) adds `guidance` to `topKeys` and decodes it with an unexported `guidanceYAML` whose
two fields are optional booleans, so a missing switch can be told from a false one and defaults to true.
It keeps the keys of the map in an `UnmarshalYAML`, as `rulesYAML` does.
A value that is not a boolean, or a `guidance` that is not a map, is a decode error of yaml.v3, reported by `Parse`
as any other.
`Parse(nil)` (no polix.yaml) also fills both switches with true, so the default workflow is consistent, although the
hook never injects without a present workflow.
`Workflow.Problems` validates the guidance together with the rest, so `polix check` reports its problems and an invalid
guidance makes the workflow invalid (the hook fails closed as for any other problem).

The hook (`AnswerHook` in `internal/hook/answer_hook_logic.go`) injects the texts on `session_start` only.
In `handle`, after the protections, the invalid workflow and the missing workflow have returned, and before the
entries of the event run, it adds each text of the guidance as a context of the answer (`agent.Answer.Inform`).
Every source of `SessionStart` (startup, resume, clear, compact) reaches the same event, so the guidance comes back
after a compaction.
The rendering already puts the contexts of `session_start` in `additionalContext` for both agents, so no change in
`internal/agent`.

Errors: a broken `guidance` is a problem of the workflow; there is no new runtime error, since the texts are static.

### Discarded alternatives

- A list of the pieces turned on (`guidance: [commit, edit]`): a map of booleans states the default and an opt-out
  more plainly.
- A new kind of `context` action under `on.session_start`, one entry per piece: it cannot be on by default without
  writing it.
- Injecting also without polix.yaml: it would change what polix does in plain repositories, where only protections run.
- Reinforcing on `pre_command` with `warn` or `deny`: left out of this change by the user.
- Agent specific texts: one neutral text is simpler and covers Codex, whose tools are still experimental.

## Acceptance criteria

- AC1: With a valid polix.yaml without `guidance`, the answer to `session_start` holds both texts as context, commit
  first.
- AC2: With `guidance: { commit: false }` only the edit text is injected; with `{ edit: false }` only the commit text;
  with both false, none; with `guidance: {}` both.
- AC3: An unknown key in `guidance` makes `polix check` report it with the allowed keys; a non boolean value or a
  `guidance` that is not a map makes `Parse` fail with an error naming polix.yaml.
- AC4: Without polix.yaml, or with an invalid one, nothing of the guidance is injected on `session_start`.
- AC5: The guidance is injected on `session_start` only, never on `prompt`, `pre_*`, `post_*`, `subagent_stop` nor
  `stop`.
- AC6: The contexts of the `session_start` entries of the repository come after the guidance in the same answer.
- AC7: docs/polix.md and examples/polix.yaml document the key, its defaults and when it applies, and
  plugins/polix/skills/workflow/reference.yaml is a copy of examples/polix.yaml.
- AC8: The rendered answer of a `SessionStart` call carries the guidance in `additionalContext` for Claude Code and for
  Codex.
- AC9: A block by `protected`, of an edit or of a command that writes, names the path (and the glob it holds, for a
  folder or a wildcard) and says in English that it is protected in polix.yaml and only a person changes it; no
  Spanish text is left in those reasons.

## Out of scope

- Guidance against invoking skills or subagents without an explicit request: dropped by the user.
- Warnings or denials on `pre_command` when a commit is chained or wrapped, or a file is written from the shell.
- Showing the guidance in `polix graph`.
- The other texts polix shows in Spanish, the reasons of the protection of its state (`ReasonStateEdit`,
  `ReasonStateCommand`) among them: only the reasons of `protected` were asked for.
- The stale `Layout` of AGENTS.md (it lists packages that no longer exist), noted under Pending.

## Tasks

### Task 1: Add the guidance to the workflow domain and its parser

- Covers: AC2, AC3.
- Files: `internal/workflow/workflow_guidance.go` (create), `internal/workflow/workflow_guidance_test.go` (create),
  `internal/workflow/workflow.go` (`Workflow`, `Workflow.Problems`, modify),
  `internal/workflow/workflow_parse.go` (`workflowYAML`, `topKeys`, `decodeNames`, `Parse`, `workflowYAML.workflow`,
  modify), `internal/workflow/workflow_parse_test.go` (modify).
- Rules: GOARCH-1, GOARCH-2, GOARCH-4, GOARCH-9, GOARCH-13, GOARCH-16, GOARCH-18, GOARCH-25, GOARCH-26, GOARCH-27,
  GOARCH-28, GOARCH-29, GOARCH-30, GOARCH-35, GOARCH-38, GOARCH-39, GOARCH-40, GO-2, GO-3, DOC-1.
- Reuse: `validator.unknownKeys`, `mapKeys`, `Invalid` and the pattern of `Rules` in
  `internal/workflow/workflow_runner.go`; the `UnmarshalYAML` pattern of `rulesYAML` in
  `internal/workflow/workflow_parse.go`.
- Changes: `workflow_guidance.go` declares the domain value `Guidance` with exported fields `Commit`, `Edit` and
  `Keys`, its `Valid` (unknown keys, through a `validator` method like `rules`), the table of the two English texts
  and the method that returns the texts of the switches turned on, commit first.
  The commit text asks to run each commit as a `git commit` command of its own, not chained with `&&`, `;` or `|` nor
  inside `sh -c` or `bash -c`, so polix recognises it; it allows the message by `-m`, `-F` or a heredoc.
  The edit text asks to create and change the files of the repository with the edit tool of the agent, never from
  the shell (`sed -i`, `perl -i`, `>`, `>>`, `tee`, heredocs or scripts that write files), because polix applies its
  edit rules and checks only to what goes through that tool.
  `Workflow` gains the field `Guidance` and `Problems` validates it.
  The parser adds the key `guidance`, decodes it through `guidanceYAML` with optional booleans defaulting to true,
  keeps its keys, adds it to `decodeNames`, and fills the defaults also for `Parse(nil)`; the domain value is built
  with a literal and validated, as `rulesYAML.rules` does.
- Tests: in `workflow_guidance_test.go`, the texts for each combination of the switches (both, only commit, only
  edit, none) and their order, and `Valid` with an unknown key.
  In `workflow_parse_test.go`: omitted key gives both true; `guidance: {}` gives both true; `commit: false` keeps edit
  true; an unknown key `delegation` is a problem naming `commit, edit`; a string value such as `yes` and a scalar
  `guidance: true` are errors naming polix.yaml; `Parse(nil)` gives both true.
- Verification: `just test -run 'Guidance|Parse'`, expected: the new and existing tests pass.

### Task 2: Inject the guidance on session_start in the hook

- Covers: AC1, AC4, AC5, AC6, AC8.
- Files: `internal/hook/answer_hook_logic.go` (`answerHookRun.handle`, modify),
  `internal/hook/answer_hook_logic_test.go` (modify), `internal/hook/answer_hook_in_test.go` (modify),
  `internal/hook/answer_hook.md` (Logic, modify).
- Rules: GOARCH-4, GOARCH-10, GOARCH-13, GOARCH-14, GOARCH-18, GOARCH-21, GOARCH-23, GOARCH-31, GOARCH-34, GOARCH-35,
  GOARCH-36, GOARCH-38, GO-2, DOC-1.
- Reuse: `agent.Answer.Inform` from `internal/agent/agent_answer.go`; the method of `workflow.Guidance` from Task 1.
- Changes: `handle` adds, only when the event is `session_start`, each text of `r.ev.Workflow.Guidance` as a context of
  the answer, after the checks of protections, invalid workflow and missing workflow and before the loop over the
  entries, so an invalid or missing workflow injects nothing and the entries' contexts follow the guidance.
  The document of the use case gains a rule in Logic, between the rule of a missing polix.yaml and the rule of the
  entries, saying that on `session_start` the guidance turned on in polix.yaml is injected first.
- Tests: in `answer_hook_logic_test.go`, on `session_start`: a valid workflow without `guidance` answers both texts;
  one with both switches off answers none; a workflow with its own `session_start` context answers the guidance
  before it; a missing workflow and an invalid one inject no guidance (the invalid one keeps its warning); on
  `prompt`, `pre_edit` and `stop` no guidance is injected.
  In `answer_hook_in_test.go`, a `SessionStart` input of Claude Code and one of Codex, through the fake port with a
  valid workflow, render a stdout whose `additionalContext` holds both texts.
- Verification: `just test -run AnswerHook`, expected: the new and existing tests pass.

### Task 3: Document the key in the docs and the annotated example

- Covers: AC7.
- Files: `docs/polix.md` (`## polix.yaml`, `## Limits of the proof of concept`, modify), `examples/polix.yaml`
  (modify), `plugins/polix/skills/workflow/reference.yaml` (modify).
- Rules: GO-2, DOC-1, XPLAT-2.
- Reuse: none.
- Changes: in docs/polix.md, the schema block of `## polix.yaml` gains the `guidance` line with its default, and a
  paragraph after `rules.run` says what each switch injects, that both are on by default, that they apply on every
  `session_start` (startup, resume, clear, compaction) only with a valid polix.yaml, that they come before the
  contexts of the entries, and that they ask for cooperation and block nothing.
  The limit on the `git commit` heuristic points to the commit guidance as the way to keep commits recognisable.
  examples/polix.yaml gains the key `guidance` with both switches, annotated in the style of the other top level
  keys (optional, defaults, when it applies), and reference.yaml is copied from it byte for byte.
- Tests: the existing test of `plugins/plugins_test.go` checks the copy, and the existing test of
  `internal/workflow/workflow_test.go` that validates examples/polix.yaml checks the new key parses.
- Verification: `just test`, expected: all tests pass, the copy and example tests included.

### Task 4: Return the reasons of a block by protected in English

- Covers: AC9.
- Files: `internal/hook/answer_hook_logic.go` (`answerHookRun.protect`, modify),
  `internal/hook/answer_hook_logic_test.go` (modify), `internal/hook/answer_hook_in_test.go` (modify),
  `internal/hook/answer_hook_out_test.go` (`TestAnswerHookProtectedPaths`, modify).
- Rules: GOARCH-4, GOARCH-13, GOARCH-18, GOARCH-21, GOARCH-23, GOARCH-31, GOARCH-34, GOARCH-35, GOARCH-38, GO-2.
- Reuse: none.
- Changes: `protect` builds its three reasons of `protected` (an edit of a protected path, a command that writes a
  protected path, a command that writes a folder or wildcard holding a protected glob) in English, as written in
  `Decisions`, with the same placeholders: the path, the glob and `workflow.File`.
  The reasons of the protection of the state of polix are not touched.
  The document `internal/hook/answer_hook.md` quotes none of these texts, so it does not change.
- Tests: the assertions that look for the Spanish text change to the English one: the stderr of the `apply_patch`
  case in `answer_hook_in_test.go` and the reason checked for every actor in `TestAnswerHookProtectedPaths` of
  `answer_hook_out_test.go`.
  In `answer_hook_logic_test.go`, a case for each of the three reasons checks its full English text: an edit of
  `.env`, a command writing `AGENTS.md`, and `rm -rf ai` with `ai/rules/**` protected naming the glob.
- Verification: `just test -run AnswerHook`, expected: the new and updated tests pass; `rg 'protegido'` over
  `internal/` finds nothing.

## Risks

- An older polix reads a polix.yaml that writes `guidance` and rejects the unknown key: no release exists yet, so no
  `requires` bump is needed; once releases exist, a repository using the key sets `requires`.
- The agent ignores the guidance, since it is only context: the heuristics stay as they are, and reinforcing them on
  `pre_command` is left for a later change.
- The guidance adds tokens to every start and compaction: keep each text to one or two sentences.
- The guidance conflicts with the contexts of a repository: the repository's own contexts come last, and the switches
  turn the guidance off.

## Verification

- `just test`: go vet and every test pass.
- `golangci-lint run ./...`: no findings.
- `just check`: polix installs, and `polix check`, `polix rules check` and `polix doctor --offline` pass on this
  repository.
- `claude plugin validate plugins/polix`: the plugin is valid.
- End to end, after `just install`: pipe to `polix hook claude` a `SessionStart` input with `cwd` in this repository
  and check that `additionalContext` holds both texts; add `guidance: { commit: false }` to a scratch copy of
  polix.yaml in a temporary repository, repeat and check that only the edit text remains; repeat with
  `polix hook codex`.
- Start a new Claude Code session in this repository and check that the agent quotes both pieces of guidance when
  asked what polix injected at startup.
- In a temporary repository whose polix.yaml protects `.env`, pipe to `polix hook claude` a `PreToolUse` edit of
  `.env` and check that it exits with 2 and its stderr gives the English reason.
