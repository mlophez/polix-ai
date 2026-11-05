# Use cases

Recipes for `polix.yaml`, from a single guardrail to a whole development flow. Each one is a fragment: put its
entries under the `on:` of your workflow (with `version: 1` at the top), validate it with `polix check` and draw
it with `polix graph`. The full reference of every option is [`examples/polix.yaml`](../examples/polix.yaml);
the design and its limits are in [polix.md](polix.md).

Every recipe follows the same idea: polix **measures and never judges**. It knows which files changed, which
command ran and with which exit code, which subagent stopped and which facts hold on the current content. Judging
the code is the job of a subagent; polix makes sure that subagent ran, on the content as it is now.

- Quality gates
  - [Review every change with a reviewer subagent](#review-every-change-with-a-reviewer-subagent)
  - [Document at the end of every task](#document-at-the-end-of-every-task)
  - [Tests that run themselves](#tests-that-run-themselves)
  - [Lint feedback right after each edit](#lint-feedback-right-after-each-edit)
  - [Format every file the agent edits](#format-every-file-the-agent-edits)
  - [Sensitive code gets a security review](#sensitive-code-gets-a-security-review)
- Process
  - [No ticket, no code](#no-ticket-no-code)
  - [A plan before the first edit](#a-plan-before-the-first-edit)
  - [Unfinished plan steps keep the agent working](#unfinished-plan-steps-keep-the-agent-working)
  - [The main session orchestrates, the builder builds](#the-main-session-orchestrates-the-builder-builds)
  - [The changelog travels with the change](#the-changelog-travels-with-the-change)
- Safety
  - [Destructive commands are left to a person](#destructive-commands-are-left-to-a-person)
  - [Keep production out of reach](#keep-production-out-of-reach)
  - [Hands off generated files and lockfiles](#hands-off-generated-files-and-lockfiles)
  - [Secrets never land in a commit](#secrets-never-land-in-a-commit)
  - [Paths no agent writes](#paths-no-agent-writes)
  - [The agent cannot switch off its own guardrails](#the-agent-cannot-switch-off-its-own-guardrails)
  - [No commits on main](#no-commits-on-main)
- Context
  - [The right rules at the right time](#the-right-rules-at-the-right-time)
  - [Context that survives compaction](#context-that-survives-compaction)
  - [The same checks for the agent and for CI](#the-same-checks-for-the-agent-and-for-ci)
- Rollout and evidence
  - [Try a guardrail before enforcing it](#try-a-guardrail-before-enforcing-it)
  - [A record of every infrastructure command](#a-record-of-every-infrastructure-command)
  - [Evidence of the process each change followed](#evidence-of-the-process-each-change-followed)
- [Putting it together: build, document, test, review, commit](#putting-it-together-build-document-test-review-commit)
- [Coming from Claude Code hooks](#coming-from-claude-code-hooks)

## Quality gates

### Review every change with a reviewer subagent

The agent may not end its turn, nor commit, until a reviewer has looked at the diff as it is now.

```yaml
on:
  subagent_stop:
    # The reviewer finished: record the fact on the content it reviewed.
    - if: { actor: "polix:reviewer" }
      record: reviewed

  stop:
    - if: { changed: ["src/**", "internal/**"] }
      unless: { done: reviewed }
      require: "Run /polix:review on the current diff before you finish"

  pre_commit:
    - unless: { done: reviewed }
      deny: "The diff is not reviewed: run /polix:review before committing"
```

How it works:

- `actor` is the subagent type: `polix:reviewer` is the read-only reviewer of the plugin, which the skill
  `/polix:review` launches. Use the name of your own agent if you have one.
- `reviewed` is tied to the content hash. If the agent fixes a finding after the review, the fact stops holding
  and it has to review again: a review never covers code it did not see.
- The reviewer reports and never fixes, so its run does not change the content it just approved.

### Document at the end of every task

A `documenter` subagent updates the docs and comments the change made stale, and the agent cannot stop until it
has run on the final code.

```yaml
on:
  subagent_stop:
    - if: { actor: documenter }
      record: documented

  stop:
    - if: { changed: ["src/**", "internal/**", "cmd/**"] }
      unless: { done: documented }
      require: "Launch the documenter subagent on the final diff before you finish"
```

`documenter` is an agent of your own, for example `.claude/agents/documenter.md`. The fact is recorded when the
subagent stops, so it covers the docs it just wrote.

A cheaper, deterministic variant asks for a change in `docs/` when the code changed, with no subagent at all.
`changed` holds when any file not committed yet matches, so this nudges the agent before it commits:

```yaml
on:
  pre_commit:
    - if: { changed: "internal/**" }
      unless: { changed: "docs/**" }
      mode: warn
      deny: "The code changed and docs/ did not: check whether the docs need an update"
```

Keep in mind that the documenter edits files, so it changes the content hash: facts recorded before it, like
`tested` or `reviewed`, stop holding. Document first, then test and review. See
[Putting it together](#putting-it-together-build-document-test-review-commit).

### Tests that run themselves

Instead of asking the agent to run the tests, polix runs them when the agent tries to stop, once per content.

```yaml
on:
  # A green run by the agent counts too.
  post_command:
    - if: { command: "just test*", exit: 0 }
      record: tested

  stop:
    - if: { changed: ["*.go", "go.mod", "go.sum"] }
      unless: { done: tested }
      run: { command: ["just", "test"], record: tested }
```

How it works:

- When the run fails, the stop is blocked and the agent gets the last 15 lines of the output, so it goes back to
  fix them. When it passes, it records `tested` on that content, and the next stop does not run them again.
- `exit: 0` only holds when `just test` decides the exit code of the line: `just test | tail -3` is decided by
  `tail`, so a piped run never counts as green.
- `run` executes without a shell and shares a budget of 10 minutes per hook call. The hook timeout that
  `polix install` writes (900 seconds) is above it.

### Lint feedback right after each edit

The linter runs on the file the agent just edited, and its output reaches the agent as context, without blocking.

```yaml
on:
  post_edit:
    - if: { path: "**/*.py" }
      mode: warn
      run: { command: ["uv", "run", "ruff", "check", "{path}"] }
    - if: { path: ["**/*.ts", "**/*.tsx"] }
      mode: warn
      run: { command: ["npx", "eslint", "{path}"] }
```

In `warn` mode the agent reads the warnings in the next step and fixes them while the file is still in its head,
instead of finding a wall of lint errors at the end of the task.

### Format every file the agent edits

The formatter rewrites the file right after each edit, so the diff never carries style noise.

```yaml
on:
  post_edit:
    - if: { path: ["**/*.ts", "**/*.tsx", "**/*.css", "**/*.json"] }
      run: { command: ["npx", "prettier", "--write", "{path}"] }
    - if: { path: "*.go" }
      run: { command: ["gofmt", "-w", "{path}"] }
```

`run` executes without a shell and passes `{path}` as one argument, so there is no `jq`, no `xargs` and no quoting
to get wrong, and the same entry works under PowerShell. When the formatter fails, usually on a syntax error, the
agent gets its output.

### Sensitive code gets a security review

Some paths deserve a second, specialised reviewer. A separate fact keeps both reviews independent.

```yaml
on:
  subagent_stop:
    - if: { actor: security-reviewer }
      record: security-reviewed

  stop:
    - if: { changed: ["**/auth/**", "**/crypto/**", "**/signing/**", "migrations/**"] }
      unless: { done: security-reviewed }
      require: "This change touches auth, crypto, signing or the schema: launch the security-reviewer subagent"

  pre_commit:
    - if: { changed: ["**/auth/**", "**/crypto/**", "**/signing/**", "migrations/**"] }
      unless: { done: security-reviewed }
      deny: "Security review missing for a sensitive path"
```

A change to a `README` never pays for a security review; a change to the signing code always does.

## Process

### No ticket, no code

Every change is tracked by a ticket, and the ticket comes from the name of the branch.

```yaml
vars:
  key: { branch: "^([A-Z][A-Z0-9]+-[0-9]+)" }   # PLAT-123-add-retry -> PLAT-123

on:
  # A gentle nudge when the conversation starts on a branch without a ticket.
  prompt:
    - unless: { var: key }
      context: { text: "The branch {branch} has no ticket: create a branch like PLAT-123-short-name first." }

  # And a hard stop at the first edit.
  pre_edit:
    - unless: { var: key }
      deny: "No ticket in the branch name: create the branch from a ticket"
```

`{key}` is then available in every string of the workflow: in messages, in paths and in `contains`.

### A plan before the first edit

The agent writes a plan for the ticket before it touches the code, and the plan names its ticket.

```yaml
vars:
  key: { branch: "^([A-Z][A-Z0-9]+-[0-9]+)" }

on:
  pre_edit:
    # Writing the plan itself is always allowed.
    - if: { path: "ai/plans/{key}-*.md" }
      allow: true
    - unless: { file: "ai/plans/{key}-*.md" }
      deny: "There is no plan for {key}: write it with /polix:plan"
    - unless: { file: "ai/plans/{key}-*.md", contains: "Ticket: {key}" }
      deny: "The plan of {key} lacks the line 'Ticket: {key}'"
```

`allow: true` stops the evaluation, so the entries below it never block the plan. The plan is a file of the
repository, so it is reviewed and committed with the code it explains.

### Unfinished plan steps keep the agent working

When a plan tracks its steps as a checklist, the agent cannot end its turn while a box is still empty. It is the
deterministic version of asking a model "are all the tasks done?" on every stop.

```yaml
vars:
  key: { branch: "^([A-Z][A-Z0-9]+-[0-9]+)" }

on:
  stop:
    - if: { file: "ai/plans/{key}-*.md", contains: "- [ ]" }
      require: "The plan of {key} has unchecked steps: finish them, or mark the ones left out as - [-] and say why"
```

`file` with `contains` reads the files of the repository, so the agent ticks the boxes as it goes and the plan
doubles as the progress report of the task.

### The main session orchestrates, the builder builds

The main session keeps a clean context: it plans and delegates, and only the `builder` subagent edits code. The
builder cannot hand back work whose tests do not pass.

```yaml
on:
  pre_edit:
    - if: { path: ["src/**", "tests/**"] }
      unless: { actor: builder }
      deny: "Only the builder subagent edits the code: delegate the step to it"

  subagent_stop:
    - if: { actor: builder, changed: "src/**" }
      unless: { done: tested }
      require: "Run just test before you hand the step back"
```

`require` on `subagent_stop` blocks the subagent, not the main session: the builder gets the message and keeps
working. Blocks of subagents never use up the loop guard of the main session.

### The changelog travels with the change

```yaml
on:
  stop:
    - if: { changed: ["src/**", "internal/**"] }
      unless: { changed: "CHANGELOG.md" }
      require: "Add an entry to CHANGELOG.md under Unreleased describing this change"
```

The release notes are written by the agent that knows the change best, at the moment it knows it best.

## Safety

### Destructive commands are left to a person

Commands that throw away work nobody saved are blocked, wherever they appear in the line.

```yaml
on:
  pre_command:
    - if: { command: ["rm -rf*", "git reset --hard*", "git clean -*f*", "git checkout -- .*", "git restore .*"] }
      deny: "This line destroys work that may not be saved: ask the user to run it. Line: {command}"
```

A script that checks whether the command *starts* with `rm` misses `cd build && rm -rf dist`. polix splits the
line the way a shell runs it and matches every command on its own, so it catches it, while `git reset --soft`
still goes through. Narrow the patterns, `rm -rf /*` or `rm -rf ~*`, if the agent should keep cleaning its own
build folders.

### Keep production out of reach

Commands are split the way a shell runs them, so `cd infra && terraform apply -auto-approve` is caught too.

```yaml
on:
  pre_command:
    - if: { command: ["git push*--force*", "git push*-f *"] }
      deny: "No force pushes: open a pull request or ask a person"
    - if: { command: ["git push*origin main*", "git push*origin HEAD:main*"] }
      deny: "No direct pushes to main: open a pull request"
    - if: { command: "terraform apply*", not: { branch: "^main$" } }
      deny: "terraform apply only runs from the pipeline of main"
    - if: { command: ["kubectl*--context*prod*", "aws*--profile*prod*"] }
      deny: "The agent never acts on production: run it yourself, or use a staging context"
```

In a command pattern `*` matches anything, spaces and slashes included, so `kubectl*--context*prod*` holds wherever
the flag appears in the line.

### Hands off generated files and lockfiles

Generated files are rewritten by a tool, not by hand. The agent is sent to the source instead.

```yaml
on:
  pre_edit:
    - if: { path: ["**/*.pb.go", "**/*_generated.go", "**/zz_generated.*"] }
      deny: "{path} is generated: edit its source (.proto, schema...) and run just gen"
    - if: { path: ["package-lock.json", "pnpm-lock.yaml", "go.sum", "poetry.lock", "uv.lock"] }
      deny: "{path} is written by the package manager: change the manifest and let the tool update it"
```

A pattern without `/` matches the base name anywhere, like `.gitignore`, so `go.sum` also covers `tools/go.sum`.

### Secrets never land in a commit

```yaml
on:
  pre_edit:
    - if: { path: [".env", ".env.*", "*.pem", "*.key", "*.p12"] }
      unless: { path: ".env.example" }
      deny: "{path} would end up in a commit: keep secrets in an ignored file or a secret store"
```

polix skips the workflow for files git ignores, so this entry never bothers an `.env` that is in `.gitignore`.
It fires exactly when the file is **not** ignored: the case in which the secret would be committed.

### Paths no agent writes

polix protects only its own state on its own. Everything else a person wants to keep for themselves goes in
`protected`: no actor, the main session included, edits those paths or writes them with a command.

```yaml
version: 1
protected: [".env", ".env.*", "AGENTS.md", "CLAUDE.md"]
```

Unlike the workflow, `protected` also covers files git ignores, so an ignored `.env` stays out of reach too.

To keep only the subagents away and let the main session change a path, use entries: `path` for the edits and
`writes` for the commands. The authors of the plugin of polix keep their way in:

```yaml
on:
  pre_edit:
    - if: { path: ["polix.yaml", "ai/rules/**"], not: { actor: session } }
      unless: { actor: ["polix:workflow-author", "polix:rules-author"] }
      deny: "{path} is the workflow of the repository: only the main session changes it"
  pre_command:
    - if: { writes: ["polix.yaml", "ai/rules/**"], not: { actor: session } }
      unless: { actor: ["polix:workflow-author", "polix:rules-author"] }
      deny: "Only the main session writes the workflow: {command}"
```

Do not put `polix.yaml` or `ai/rules/` in `protected`: it binds the main session too, so the skills `workflow`
and `add-rule` could no longer change them. `writes` is a heuristic: it sees `rm`, `mv`, `cp`, `tee`, `sed -i` and
redirections into the repository, but not an interpreter that opens a file (`python -c ...`). A program that writes
by itself (`rm`, `mv`, `cp`...) is denied on every protected file it names, also one it only reads
(`cp AGENTS.md notes.md`), on a folder holding protected paths and on a wildcard that may reach them (`rm -rf ai` or
`rm -rf ai/*` for `ai/rules/**`, but also `cp a.md docs/` for `docs/secret.md`), and `rm` on the root. Reading is
fine, even with the output redirected (`ls ai 2>/dev/null`, `cat AGENTS.md > notes.md`). See
[the details](polix.md#protections).

### The agent cannot switch off its own guardrails

The hooks that call polix live in `.claude/settings.json` and `.codex/hooks.json`. An agent that edits them can
turn polix off for the rest of the session.

```yaml
on:
  pre_edit:
    - if: { path: [".claude/settings.json", ".codex/hooks.json"] }
      deny: "{path} registers the hooks of polix: change it yourself, the agent does not"
```

polix only protects its own state without any entry: `AGENTS.md`, `CLAUDE.md`, `ai/rules/` and `polix.yaml` are
open unless the workflow closes them, as in [Paths no agent writes](#paths-no-agent-writes). This entry keeps the
hooks away from edits, for the main session too. `polix install` still writes them, since it runs as a command and
not as an edit; adding the files to `protected` instead would also deny the commands of the agent that write
them.

### No commits on main

```yaml
on:
  pre_commit:
    - if: { branch: "^(main|master)$" }
      deny: "Do not commit on {branch}: create a branch for this change"
```

`pre_commit` runs for any command line that holds a `git commit`, also inside `git add . && git commit -m ...`.

## Context

### The right rules at the right time

Instead of loading every convention into `CLAUDE.md`, polix injects the rules of `ai/rules/` whose scope matches
the file being edited, once per session, at the moment they matter.

```yaml
on:
  session_start:
    - context: { file: "docs/architecture.md" }

  pre_edit:
    - context: { rules: "{path}" }
```

With a rule like this in `ai/rules/frontend.yaml`, the agent reads it the first time it edits a component, and
never while it works on the backend:

```yaml
rules:
  - id: FE-1
    description: Components never call the API directly; they go through the hooks of src/api/.
    scope: ["src/components/**"]
    deterministic: false
```

### Context that survives compaction

When the context window fills up, Claude Code compacts the conversation and details get lost. polix answers
`SessionStart` for every source, a compaction included, so a `session_start` context comes back after each one.

```yaml
vars:
  key: { branch: "^([A-Z][A-Z0-9]+-[0-9]+)" }

on:
  session_start:
    - context: { text: "Branch {branch}, ticket {key}. Use bun, not npm. Run just test before committing." }
    - context: { file: "docs/conventions.md" }
```

Keep it short: it is injected at every start, resume and compaction. `file` takes one path, not a glob. Rules
injected by `context: { rules }` are not repeated after a compaction, since polix injects each one once per session.

### The same checks for the agent and for CI

A deterministic rule has a `check`, any file of the repository that passes or fails: a script, run as a program, or
a test, run by its runner of `rules.run` in polix.yaml (`"*_test.go": [go, test, "{dir}", -run, "^Test{name}$"]`).
The agent runs the checks of the changed files before it stops, and CI runs the very same ones.

```yaml
on:
  stop:
    - if: { changed: "**" }
      run: { rules: changed }
```

```yaml
# .github/workflows/ci.yml
- uses: actions/checkout@v4
  with:
    fetch-depth: 0 # the base branch and the merge base must be in the clone
- if: github.event_name == 'pull_request'
  run: polix rules check --changed --base="${{ github.base_ref }}"
- if: github.event_name != 'pull_request'
  run: polix rules check
```

The checkout needs `fetch-depth: 0` so that the base branch and the merge base are in the clone. On a pull
request, `--base` is the branch it merges into, and polix fails when it does not exist. Outside a pull request
`base_ref` is empty, and an empty `--base` leaves the choice to `polix.yaml`: the closest branch of `base`, or a
failure when none can be found. On a push to a branch of `base` itself, such as `main`, that branch is its own
base and `--changed` finds nothing to check, so the recipe runs every check on a push. In the hooks `changed` is what is not committed yet; in CI it is
what the branch changed since it forked.

A rule is written once, in `ai/rules/`. The agent is blocked by it locally, and a person who bypasses the agent is
blocked by it in CI.

## Rollout and evidence

### Try a guardrail before enforcing it

A new entry starts in `audit`: it blocks nothing and says nothing, it only logs what it would have blocked.

```yaml
on:
  pre_edit:
    - unless: { actor: builder }
      mode: audit       # then warn, then enforce
      deny: "Only the builder subagent edits the project"
```

After a few days of real sessions, read what it would have done:

```bash
polix log --json | jq -c 'select(.kind == "decision" and .outcome == "audit")'
```

When there are no false positives, move it to `warn` (the agent is told, nothing is blocked) and then to
`enforce`. Nobody's afternoon is broken by a guardrail that was wrong.

### A record of every infrastructure command

An entry in `audit` mode never blocks and never speaks, but it logs every time it holds. Pointed at the commands
that touch infrastructure, it becomes a record of what the agent ran against it.

```yaml
on:
  pre_command:
    - if: { command: ["kubectl*", "terraform*", "helm*", "aws*", "gcloud*"] }
      mode: audit
      deny: "Infrastructure command: {command}"
```

```bash
polix log --json | jq -r 'select(.outcome == "audit") | "\(.at)  \(.actor)  \(.reason)"'
```

polix only sees the edit tools and `Bash`: calls to MCP tools do not reach it.

### Evidence of the process each change followed

Every fact and every decision is appended to `<git-dir>/polix/log.jsonl`, with the entry that caused it and the
content hash. Nobody, agent included, can edit it.

```bash
polix log --tail 20
polix log --json | jq -c 'select(.kind == "fact")'
```

That log answers questions such as *were the tests green on the exact content that was committed?* or *how many
times did the reviewer block this branch?*. For teams that must show how a change was made, not only what changed,
it is evidence produced as a side effect of working.

## Putting it together: build, document, test, review, commit

Facts are tied to the content, and every edit changes it. So the order of the phases matters: a phase that edits
files must run before the facts that should cover its edits.

1. **build**: the agent implements the change.
2. **document**: the documenter edits docs and comments, so it runs before the tests and the review.
3. **test**: polix runs the tests itself on stop, once per content.
4. **review**: the reviewer is read-only, so its fact survives until the next edit.
5. **commit**: only when every fact holds on the content being committed.

The `done` conditions in the `if` of the stop entries enforce that order: the tests do not run on code the
documenter is about to change, and the reviewer is not asked for before the tests are green.

```yaml
version: 1
base: main
max_stop_blocks: 3
vars:
  key: { branch: "^([A-Z][A-Z0-9]+-[0-9]+)" }

on:
  pre_edit:
    - unless: { var: key }
      deny: "No ticket in the branch name: create the branch from a ticket"
    - context: { rules: "{path}" }

  post_command:
    - if: { command: "just test*", exit: 0 }
      record: tested

  subagent_stop:
    - if: { actor: documenter }
      record: documented
    - if: { actor: "polix:reviewer" }
      record: reviewed

  stop:
    - if: { changed: "src/**" }
      unless: { done: documented }
      require: "Launch the documenter subagent first: it edits files, so it goes before the tests and the review"
    # Tests only once the docs are written, review only once the tests pass.
    - if: { changed: "src/**", done: documented }
      unless: { done: tested }
      run: { command: ["just", "test"], record: tested }
    - if: { changed: "src/**", done: [documented, tested] }
      unless: { done: reviewed }
      require: "Run /polix:review on the final diff"

  pre_commit:
    - unless: { done: [documented, tested, reviewed] }
      deny: "Document, test and review this exact diff before committing ({key})"
```

If the reviewer finds a bug and the agent fixes it, `documented`, `tested` and `reviewed` all stop holding, and the
flow runs again on the fixed code. Nobody has to remember to invalidate anything: the fix is a new content, and
no fact was recorded on it yet.

## Coming from Claude Code hooks

The [hooks guide of Claude Code](https://code.claude.com/docs/en/hooks-guide) and its
[reference](https://code.claude.com/docs/en/hooks) show what a hook script can do. This is where each of their
examples lands in polix, and what stays outside of it. polix keeps every other hook of `.claude/settings.json`
when it installs its own, so the two live side by side.

What polix does, without a script:

- **Block `rm -rf` and other dangerous commands** (`PreToolUse` on `Bash`): see
  [Destructive commands are left to a person](#destructive-commands-are-left-to-a-person) and
  [Keep production out of reach](#keep-production-out-of-reach). polix matches each command of the line, not the
  line as a whole.
- **Format or lint after each edit** (`PostToolUse` on `Edit|Write`): see
  [Format every file the agent edits](#format-every-file-the-agent-edits) and
  [Lint feedback right after each edit](#lint-feedback-right-after-each-edit).
- **Block edits to protected files** such as `.env` or `package-lock.json`: see
  [Paths no agent writes](#paths-no-agent-writes),
  [Secrets never land in a commit](#secrets-never-land-in-a-commit) and
  [Hands off generated files and lockfiles](#hands-off-generated-files-and-lockfiles).
- **Tell the agent a file is generated** (`additionalContext`): the generated-files recipe blocks the edit and
  points to the source in the same message.
- **Re-inject context after compaction** (`SessionStart` with matcher `compact`): see
  [Context that survives compaction](#context-that-survives-compaction).
- **Keep the agent working until the tests pass** (a `Stop` command, or an agent hook that runs the suite): see
  [Tests that run themselves](#tests-that-run-themselves). The run is deterministic, costs no model call and is not
  repeated on content that already passed.
- **Ask a model whether every task is done** (a prompt hook on `Stop`): see
  [Unfinished plan steps keep the agent working](#unfinished-plan-steps-keep-the-agent-working) for the
  deterministic version, or [Review every change](#review-every-change-with-a-reviewer-subagent) when it needs
  judgement: the judgement goes to a subagent, and polix checks that it ran on the current content.
- **Keep a subagent working** (`SubagentStop`): see
  [The main session orchestrates, the builder builds](#the-main-session-orchestrates-the-builder-builds).
- **Avoid a stop hook that never lets go** (`stop_hook_active`, the cap of eight blocks): built in. After
  `max_stop_blocks` blocks of the same entry on the same content, polix lets the agent stop and logs a
  `loop_guard` decision.
- **Fail closed when the policy is broken** (`onFailure: "block"`): built in for an invalid `polix.yaml`, which
  blocks edits and commands but never a stop. A machine without polix is not covered: Claude Code treats the
  missing command as a non blocking error, and the fragment of [`examples/AGENTS-polix.md`](../examples/AGENTS-polix.md)
  makes the agent offer to install it.
- **Guard configuration against changes** (`ConfigChange`): see
  [The agent cannot switch off its own guardrails](#the-agent-cannot-switch-off-its-own-guardrails) and
  [Paths no agent writes](#paths-no-agent-writes): `AGENTS.md` and `CLAUDE.md` in `protected`, and `polix.yaml`
  and `ai/rules/` kept from subagents with `path` and `writes`, so the skills can still change them. polix only
  protects its own state on its own.
- **Log what the agent does** (a `PreToolUse` hook that appends to a file): see
  [A record of every infrastructure command](#a-record-of-every-infrastructure-command) and
  [Evidence of the process each change followed](#evidence-of-the-process-each-change-followed).

What stays outside of polix, because it is not policy or polix does not listen to it:

- **Notifications, sounds and terminal titles** (`Notification`, `terminalSequence`): personal preferences, better
  kept in the user settings.
- **Approving permission prompts or changing the permission mode** (`PermissionRequest`): polix blocks, injects
  context or lets the action go on. It never grants a permission the user did not grant.
- **Rewriting what the agent sends or receives** (`updatedInput`, `updatedToolOutput`): polix never changes a tool
  call, so what the agent ran is what the log says it ran.
- **Checking the text of a prompt**: a `prompt` entry can block or add context, but its conditions are facts of the
  repository, not the words of the user.
- **Context computed by a command**, such as `git log --oneline -5` at session start: `context` takes a text or a
  file, not the output of a command.
- **MCP tools**: polix governs the edit tools and `Bash` only.
- **Environment reloads, worktrees, compaction, model switches, elicitations, tasks and teammates** (`CwdChanged`,
  `FileChanged`, `WorktreeCreate`, `PreCompact`, `PreModelSwitch`, `Elicitation`, `TaskCreated`, `TeammateIdle`):
  events polix does not register.
