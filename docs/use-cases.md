# Use cases

Recipes for `polix.yaml`, from a single guardrail to a whole development flow. Each one is a fragment: put it
under the `guardrails:`, the `on:` or the `githooks:` of your workflow (with `version: 1` at the top), validate it with
`polix config check` and draw it with `polix graph`. Every key, condition and verb is described in
[reference.md](reference.md), and a whole annotated workflow is [`examples/polix.yaml`](../examples/polix.yaml);
the design and its limits are in [polix.md](polix.md).

The recipes come in the three sections of `polix.yaml`:

- **`guardrails:`**, the basics every project wants from the first minute: files no agent writes, commands no
  agent runs, branches nobody commits on, a formatter or a linter after each edit and the checks that must pass
  before the agent ends its turn. Each is a short list, a shortcut.
- **`on:`**, where the team orchestrates its flow: the review subagents, the builder, the plan, the ticket. Each
  entry says on which event it applies, when, and what it does.
- **`githooks:`**, the gates git itself runs on every commit, whoever makes it: a person, an IDE or an agent.

`protected_files` and `forbidden_commands` are protections of the engine: they deny before every entry of `on:`,
for every actor, without exceptions, also on the files git ignores. The other three shortcuts are only sugar: polix
expands each into ordinary entries of `on:` when it reads the file, and `polix graph` draws them with the name of
their shortcut. Each of their recipes shows its expansion too: write it in `on:` and tune it when the shortcut
falls short. A polix older than the shortcuts rejects `guardrails:` as an unknown key, so set `requires` to the
release you install.

The entries of `on:` skip the edits of files git ignores: they govern the change, what a commit may hold, so a
build output, a local `.env` or a scratch file never fires a ticket, plan, format or docs entry. Only the
protection of the state of polix, `protected_files` and `forbidden_commands` reach an ignored file.

Every recipe follows the same idea: polix **measures and never judges**. It knows which files changed, which
command ran and with which exit code, which of its own checks passed on which content and which subagent ended on
which content. Judging the code is the job of a subagent; polix makes sure that subagent ran, on the content as it
is now.

Each entry says when it applies (`when`, `unless`) and what it does (`action`: `block`, `run` or `inject`). A
`block` is a gate that holds every time, or a reminder that asks once with `once: true`; a `run` that passed is not
run again on the same content; an `inject` reaches the AI once per session and agent (every time on `start`). The first entry that blocks ends the evaluation of the event. The details are in
[polix.md](polix.md#verbs).

- The basics, with `guardrails:`
  - [Paths no agent writes](#paths-no-agent-writes)
  - [Secrets never land in a commit](#secrets-never-land-in-a-commit)
  - [Destructive commands are left to a person](#destructive-commands-are-left-to-a-person)
  - [No commits on main](#no-commits-on-main)
  - [Format every file the agent edits](#format-every-file-the-agent-edits)
  - [Lint feedback right after each edit](#lint-feedback-right-after-each-edit)
  - [Tests that run themselves](#tests-that-run-themselves)
- The flow of the team, with `on:`
  - Quality gates
    - [Review every change with a reviewer subagent](#review-every-change-with-a-reviewer-subagent)
    - [Document at the end of every task](#document-at-the-end-of-every-task)
    - [Sensitive code gets a security review](#sensitive-code-gets-a-security-review)
  - Process
    - [No ticket, no code](#no-ticket-no-code)
    - [A plan before the first edit](#a-plan-before-the-first-edit)
    - [Unfinished plan steps keep the agent working](#unfinished-plan-steps-keep-the-agent-working)
    - [The main session orchestrates, the builder builds](#the-main-session-orchestrates-the-builder-builds)
    - [The changelog travels with the change](#the-changelog-travels-with-the-change)
  - Safety
    - [Keep production out of reach](#keep-production-out-of-reach)
    - [Hands off generated files and lockfiles](#hands-off-generated-files-and-lockfiles)
    - [The agent cannot switch off its own guardrails](#the-agent-cannot-switch-off-its-own-guardrails)
  - Context
    - [The right rules at the right time](#the-right-rules-at-the-right-time)
    - [Context that survives compaction](#context-that-survives-compaction)
    - [The same checks for the agent and for CI](#the-same-checks-for-the-agent-and-for-ci)
  - Rollout and evidence
    - [Evidence of the process each change followed](#evidence-of-the-process-each-change-followed)
  - [Putting it together: build, document, test, review, commit](#putting-it-together-build-document-test-review-commit)
- The gates of git, with `githooks:`
  - [The message starts with the key of the ticket](#the-message-starts-with-the-key-of-the-ticket)
  - [A message format by the files of the commit](#a-message-format-by-the-files-of-the-commit)
  - [A command that blocks the commit](#a-command-that-blocks-the-commit)
  - [The agent cannot skip the hooks of git](#the-agent-cannot-skip-the-hooks-of-git)
- [Coming from Claude Code hooks](#coming-from-claude-code-hooks)

## Guardrails

The recipes of this layer lead with the shortcut, which is all most projects need, and follow with what it does or
the entries of `on:` it expands into, for those who need more. A file can hold every shortcut at once:

```yaml
version: 1
guardrails:
  protected_files: [".env", ".env.*", "AGENTS.md", "CLAUDE.md"]
  forbidden_commands:
    - pattern: "rm -rf*"
    - pattern: "git push --force*"
      reason: "No force pushes: open a pull request or ask a person"
  protected_branches: [main, "release/*"]
  format_on_edit:
    - run: "gofmt -w {path}"
      path: "*.go"
  run_on_stop:
    - run: "golangci-lint run ./..."
      changed: "*.go"
    - run: "go test ./..."
      changed: ["*.go", "go.mod", "go.sum"]
```

The files and the commands are protections: they deny before every entry of `on:`, also on the files git ignores,
and bind the main session too. The branches, the formatters and the checks become entries, which run before the
entries written in `on:` of the same event.

### Paths no agent writes

polix protects only its own state on its own. Everything else a person wants to keep for themselves goes in
`protected_files`: no actor, the main session included, edits those paths or writes them with a command.

```yaml
guardrails:
  protected_files: [".env", ".env.*", "AGENTS.md", "CLAUDE.md"]
```

An edit of a protected path is denied with `.env is protected in polix.yaml: only a person changes it.`, and a
command that writes one with the path it writes, or the protected glob a folder or a wildcard holds:
`ai holds ai/rules/**, protected in polix.yaml: only a person changes it.` for `rm -rf ai`. The log records the
denial as `guardrails.protected_files`. Unlike an entry of `on:`, it also covers files git ignores, so an ignored
`.env` stays out of reach too, but never an edit outside the repository. On macOS and Windows the globs ignore
case, so `.ENV` is `.env` there.
A command the check of its line misses (`python -c ...`, `bash -c '...'`) is caught after it runs: polix puts a
protected file git does not ignore back as it was before the command and tells the agent (see
[Files a command changed](polix.md#files-a-command-changed)).

To keep only the subagents away and let the main session change a path, use regular entries of `on:`: `path` for
the edits and `writes` for the commands. The authors of the plugin of polix keep their way in:

```yaml
on:
  pre_edit:
    - when: { path: ["polix.yaml", "ai/rules/**"], not: { actor: session } }
      unless: { actor: ["polix:workflow-author", "polix:rules-author"] }
      action:
        block: "{path} is the workflow of the repository: only the main session changes it"
  pre_command:
    - when: { writes: ["polix.yaml", "ai/rules/**"], not: { actor: session } }
      unless: { actor: ["polix:workflow-author", "polix:rules-author"] }
      action:
        block: "Only the main session writes the workflow: {command}"
```

Do not put `polix.yaml` or `ai/rules/` in `protected_files`: it binds the main session too, so the skills
`workflow` and `add-rule` could no longer change them. `writes` is a heuristic: it sees `rm`, `mv`, `cp`, `tee`,
`sed -i` and redirections into the repository, but not an interpreter that opens a file (`python -c ...`). A
program that writes by itself (`rm`, `mv`, `cp`...) is denied on every protected file it names, also one it only
reads (`cp AGENTS.md notes.md`), on a folder holding protected paths and on a wildcard that may reach them
(`rm -rf ai` or `rm -rf ai/*` for `ai/rules/**`, but also `cp a.md docs/` for `docs/secret.md`), and `rm` on the
root. The message names the protected path the folder holds. Reading is fine, even
with the output redirected (`ls ai 2>/dev/null`, `cat AGENTS.md > notes.md`). See
[the details](polix.md#protections).

### Secrets never land in a commit

The simplest way to keep secrets out of a commit is to keep the agent away from the files that hold them:

```yaml
guardrails:
  protected_files: [".env", ".env.*", "*.pem", "*.key", "*.p12"]
```

It works as in [Paths no agent writes](#paths-no-agent-writes), and has no exception, so `.env.example` is denied
too. To let the agent write it, take the globs out of `protected_files` and write an entry of `on:` with an
`unless`. polix skips the entries of `on:` for files git ignores, since they govern what a commit may hold: the
entry never bothers an `.env` that is in `.gitignore`, and fires exactly when the file is **not** ignored, the case
in which the secret would be committed.

```yaml
on:
  pre_edit:
    - when: { path: [".env", ".env.*", "*.pem", "*.key", "*.p12"] }
      unless: { path: ".env.example" }
      action:
        block: "{path} would end up in a commit: keep secrets in an ignored file or a secret store"
```

Both keep the agent away; a commit of a person is gated by `githooks:`, as in
[A message format by the files of the commit](#a-message-format-by-the-files-of-the-commit), whose `pre_commit` blocks
any staged file under `secrets/`.

### Destructive commands are left to a person

Commands that throw away work nobody saved are blocked, in any position of the line.

```yaml
guardrails:
  forbidden_commands:
    - pattern: "*rm -rf*"
    - pattern: "*rm -fr*"
    - pattern: "git reset --hard*"
      reason: "git reset --hard throws away uncommitted work: use git stash, or ask the user to run it."
    - pattern: "git clean -*f*"
    - pattern: "git checkout -- .*"
    - pattern: "git restore .*"
```

The first item whose pattern matches a command of the line denies it, on `pre_command` and `pre_commit`, before
every entry of `on:` and for every actor. An item without a `reason` denies with a default message, `This command
is forbidden by polix.yaml: a person must run it.`; an item with a `reason` with that reason. Both end with the line
of the command, and the log records the denial as `guardrails.forbidden_commands` with the line of the item.

A script that checks whether the command *starts* with `rm` misses `cd build && rm -rf dist`. polix splits the
line the way a shell runs it and matches every command on its own, so it catches it, while `git reset --soft`
still goes through. Narrow the patterns, `*rm -rf /*` or `*rm -rf ~*`, if the agent should keep cleaning its own
build folders.

Each command is matched as written, so this is a guardrail against accidents, not a sandbox: a pattern misses
another spelling of the same command (`rm -fr`, `rm -r -f`, the flags in another order), and a command run through
an interpreter or a substitution (`bash -c 'rm -rf dist'`, `$(...)`). That is why the list carries `rm -rf` and
`rm -fr`, and why the patterns start with `*`, which also covers a wrapper such as `sudo rm -rf /`. A command that is forbidden only on some branches or for some actors is an entry of `on:` with
`command` and those conditions, as in [Keep production out of reach](#keep-production-out-of-reach).

### No commits on main

```yaml
guardrails:
  protected_branches: [main, master]
```

It expands into one regular entry of `pre_commit`, whose `branch` is an anchored regular expression of the names
(`*` in a name matches any text, so `release/*` covers every release branch):

```yaml
on:
  pre_commit:
    # from guardrails.protected_branches
    - when: { branch: "^(?:main|master)$" }
      action:
        block: "{branch} is protected in polix.yaml: create a branch for this change and commit there."
```

`pre_commit` runs for any command line that holds a `git commit`, also inside `git add . && git commit -m ...`.
Copy the entry into `on:` to change its message, or to add an `unless` for a release bot.

`protected_branches` is a gate of the agent only. To keep people off `main` too, write the same gate in
`githooks:`, which git runs on every commit:

```yaml
githooks:
  pre_commit:
    - when: { branch: "^(?:main|master)$" }
      action:
        block: "{branch} is protected: create a branch for this change and commit there."
```

### Format every file the agent edits

The formatter rewrites the file right after each edit, so the diff never carries style noise.

```yaml
guardrails:
  format_on_edit:
    - run: "npx prettier --write {path}"
      path: ["**/*.ts", "**/*.tsx", "**/*.css", "**/*.json"]
    - run: "gofmt -w {path}"
      path: "*.go"
```

Each item expands into one regular entry of `post_edit` that runs its command on the edits of its `path`, or on
every edit without one:

```yaml
on:
  post_edit:
    # from guardrails.format_on_edit
    - when: { path: ["**/*.ts", "**/*.tsx", "**/*.css", "**/*.json"] }
      action:
        run: "npx prettier --write {path}"
    # from guardrails.format_on_edit
    - when: { path: "*.go" }
      action:
        run: "gofmt -w {path}"
```

The filter is `path`, the edited file, not `changed`, which holds for any file not committed yet and would run a
formatter on a file of another kind. `run` executes without a shell and splits the command on spaces before it
replaces `{path}`, so the path stays one argument even when it holds a space: there is no `jq`, no `xargs` and no
quoting to get wrong, and the same item works under PowerShell. When the formatter fails, usually on a syntax
error, the agent gets its output as context, without a block. A formatter that rewrote the file writes no `passed`
line, since the content changed while it ran, so it runs again on the next edit of the path.

### Lint feedback right after each edit

The linter runs on the file the agent just edited, and its output reaches the agent as context, without blocking.
It is the same shortcut as the formatter, with an optional `message` before the output of a failure:

```yaml
guardrails:
  format_on_edit:
    - run: "uv run ruff check {path}"
      path: "**/*.py"
    - run: "npx eslint {path}"
      path: ["**/*.ts", "**/*.tsx"]
      message: "eslint found problems in {path}:"
```

```yaml
on:
  post_edit:
    # from guardrails.format_on_edit
    - when: { path: "**/*.py" }
      action:
        run: "uv run ruff check {path}"
    # from guardrails.format_on_edit
    - when: { path: ["**/*.ts", "**/*.tsx"] }
      action:
        run:
          command: "npx eslint {path}"
          message: "eslint found problems in {path}:"
```

On `post_edit` a failing `run` never blocks, since the edit already happened: the output of the linter reaches the
agent as context. It reads the warnings in the next step and fixes them while the file is still in its head,
instead of finding a wall of lint errors at the end of the task. A lint that passed on a file is not run again
until that file, or any other, changes.

### Tests that run themselves

Instead of asking the agent to run the tests, polix runs them when the agent tries to stop, once per content.

```yaml
guardrails:
  run_on_stop:
    - run: just test
      changed: ["*.go", "go.mod", "go.sum"]
```

Each item expands into one regular entry of `stop` that runs its command when its `changed` holds, or always
without one:

```yaml
on:
  stop:
    # from guardrails.run_on_stop
    - when: { changed: ["*.go", "go.mod", "go.sum"] }
      action:
        run: just test
```

How it works:

- When the run fails, the stop is blocked and the agent gets `just test failed: <error>` and the last 15 lines of
  the output (with `(last 15 of <n> lines)` when there are more), so it goes back to fix them. When it passes, polix writes a `passed` line for `just test` on that branch and content, and the next
  stop does not run it again; any edit makes it run again.
- The items run in their order and the first that fails ends the evaluation, so list the lint before the tests:
  the agent gets one failure per answer, and the tests do not run on code that does not pass the lint.
- A `just test` the agent runs by itself does not count, green or not: polix cannot know what it really ran (an
  alias, another folder, a run still going in the background). Only the runs of polix spare a later one.
- When the result depends on more than the files, as for integration tests against a service, write the entry in
  `on:` with `always: true` in the long form of `run`, which the shortcut does not take, so it runs on every stop:
  an entry of `stop` whose `action` is a `run` of `command: just test-integration` and `always: true`.
- `run` executes without a shell and shares a budget of 10 minutes per hook call. The hook timeout that
  `polix install` writes (900 seconds) is above it.
- A check that should also gate the end of a subagent, or wait for a reviewer, is an entry of `on:`: see
  [The main session orchestrates, the builder builds](#the-main-session-orchestrates-the-builder-builds) and
  [Putting it together](#putting-it-together-build-document-test-review-commit).

The rest of this page is the layer `on:`, where the team orchestrates its flow.

## Quality gates

### Review every change with a reviewer subagent

The agent may not end its turn, nor commit, until a reviewer has looked at the diff as it is now.

```yaml
on:
  # polix observes the end of polix:reviewer by itself, since a finished names it.
  stop:
    - when: { changed: ["src/**", "internal/**"] }
      unless: { finished: "polix:reviewer" }
      action:
        block: "Launch the polix:reviewer subagent on the current diff before you finish"

  pre_commit:
    - unless: { finished: "polix:reviewer" }
      action:
        block: "The diff is not reviewed: launch the polix:reviewer subagent before committing"
```

How it works:

- `finished` names the subagent type: `polix:reviewer` is the read-only reviewer of the plugin, which the skill
  `/polix:review` launches. Use the name of your own agent if you have one. polix observes the end of every
  subagent a `finished` names, so no entry on `subagent_stop` is needed.
- `finished` is tied to the content the reviewer ended on, compared on the files of the `changed` of the entry. If
  the agent fixes a finding in `src/` after the review, it stops holding and the agent has to review again: a review
  never covers code it did not see. An edit of `README.md` keeps it, since it is outside that scope. The entry of
  `pre_commit` has no `changed`, so any edit asks for a new review there.
- The reviewer reports and never fixes, so its run does not change the content it just approved.
- Both are plain blocks, gates that hold on every attempt until the review ran.
- The block asks for the subagent: the agent satisfies the gate by launching `polix:reviewer` itself. Ask for
  `/polix:review` instead, in the block or by running it yourself, when you want its report saved in the folder of
  reports (`ai/reports/` by default), as evidence next to the plan; it launches the same reviewer, so it satisfies
  the gate as well.

When the reviewer runs in the background, the stop above blocks again on every turn until it ends. To ask once
per content instead, make the block of the stop a reminder with `once: true`:

```yaml
on:
  stop:
    - when: { changed: ["src/**", "internal/**"] }
      unless: { finished: "polix:reviewer" }
      action:
        block:
          message: "Launch the polix:reviewer subagent on the current diff before you finish"
          once: true
```

The reminder then stops enforcing: a second stop on the same content passes even if no review ran, and an edit of
`src/` or `internal/` makes it ask again. Keep the plain `block` of `pre_commit` to make the review a hard gate.

### Document at the end of every task

A `documenter` subagent updates the docs and comments the change made stale, and the agent cannot stop until it
has run on the final code.

```yaml
on:
  stop:
    - when: { changed: ["src/**", "internal/**", "cmd/**"] }
      unless: { finished: documenter }
      action:
        block: "Launch the documenter subagent on the final diff before you finish"
```

`documenter` is an agent of your own, for example `.claude/agents/documenter.md`. polix observes its end with the
content as it left it, so `finished: documenter` covers the docs and comments it just wrote.

A cheaper, deterministic variant asks for a change in `docs/` when the code changed, with no subagent at all.
`changed` holds when any file not committed yet matches, so this nudges the agent before it commits. An `inject`
warns without blocking, once per session and agent; `always: true` would repeat it on every commit:

```yaml
on:
  pre_commit:
    - when: { changed: "internal/**" }
      unless: { changed: "docs/**" }
      action:
        inject: "The code changed and docs/ did not: check whether the docs need an update"
```

To ask once per content instead, a reminder blocks the first commit and lets the next one through:

```yaml
on:
  pre_commit:
    - when: { changed: "internal/**" }
      unless: { changed: "docs/**" }
      action:
        block:
          message: "The code changed and docs/ did not: update the docs, or commit again if they need nothing"
          once: true
```

Keep in mind that the documenter edits files, so it changes the content: a `run` that passed before it runs again,
and a `finished` of a reviewer whose scope holds the files it edits stops holding. Document first, then test and
review. See [Putting it together](#putting-it-together-build-document-test-review-commit).

### Sensitive code gets a security review

Some paths deserve a second, specialised reviewer. A subagent type of its own keeps both reviews independent:
`finished: security-reviewer` only holds once that type ended, whatever `polix:reviewer` did.

```yaml
on:
  stop:
    - when: { changed: ["**/auth/**", "**/crypto/**", "**/signing/**", "migrations/**"] }
      unless: { finished: security-reviewer }
      action:
        block: "This change touches auth, crypto, signing or the schema: launch the security-reviewer subagent"

  pre_commit:
    - when: { changed: ["**/auth/**", "**/crypto/**", "**/signing/**", "migrations/**"] }
      unless: { finished: security-reviewer }
      action:
        block: "Security review missing for a sensitive path"
```

A change to a `README` never pays for a security review; a change to the signing code always does. The scope of
both entries is the sensitive paths, so an edit elsewhere after the review keeps it.

## Process

### No ticket, no code

Every change is tracked by a ticket, and the ticket comes from the name of the branch.

```yaml
vars:
  key:   # PLAT-123-add-retry -> PLAT-123
    branch: "^([A-Z][A-Z0-9]+-[0-9]+)"

on:
  # A gentle nudge when the conversation starts on a branch without a ticket.
  prompt:
    - unless: { var: key }
      action:
        inject: "The branch {branch} has no ticket: create a branch like PLAT-123-short-name first."

  # And a hard stop at the first edit.
  pre_edit:
    - unless: { var: key }
      action:
        block: "No ticket in the branch name: create the branch from a ticket"
```

`{key}` is then available in every string of the workflow: in messages, in paths and in `contains`. The nudge of
`prompt` is an `inject`, so it reaches the agent once per session, and again after a compaction. To have the key at
the start of every commit message, whoever commits, see
[The message starts with the key of the ticket](#the-message-starts-with-the-key-of-the-ticket).

### A plan before the first edit

The agent writes a plan for the ticket before it touches the code, and the plan names its ticket.

```yaml
vars:
  key:
    branch: "^([A-Z][A-Z0-9]+-[0-9]+)"

on:
  pre_edit:
    # Writing the plan itself is always allowed.
    - when: { not: { path: "ai/plans/{key}-*.md" } }
      unless: { file: "ai/plans/{key}-*.md" }
      action:
        block: "There is no plan for {key}: write it with /polix:plan"
    - when: { not: { path: "ai/plans/{key}-*.md" } }
      unless: { file: "ai/plans/{key}-*.md", contains: "Ticket: {key}" }
      action:
        block: "The plan of {key} lacks the line 'Ticket: {key}'"
```

The `not` in `when` is the exception: the plan itself can always be written, while every other edit waits for it.
Write an exception this way, or as a condition in `unless`. The plan is a file of the repository, so it is reviewed
and committed with the code it explains.

### Unfinished plan steps keep the agent working

When a plan tracks its steps as a checklist, the agent cannot end its turn while a box is still empty. It is the
deterministic version of asking a model "are all the tasks done?" on every stop.

```yaml
vars:
  key:
    branch: "^([A-Z][A-Z0-9]+-[0-9]+)"

on:
  stop:
    - when: { file: "ai/plans/{key}-*.md", contains: "- [ ]" }
      action:
        block: "The plan of {key} has unchecked steps: finish them, or mark the ones left out as - [-] and say why"
```

`file` with `contains` reads the files of the repository, so the agent ticks the boxes as it goes and the plan
doubles as the progress report of the task. A plain `block` asks on every stop until the boxes are ticked, up to
the loop guard.

### The main session orchestrates, the builder builds

The main session keeps its context clean, since it plans and delegates, and only the `builder` subagent edits code. The
builder cannot hand back work whose tests do not pass.

```yaml
on:
  pre_edit:
    - when: { path: ["src/**", "tests/**"] }
      unless: { actor: builder }
      action:
        block: "Only the builder subagent edits the code: delegate the step to it"

  # polix runs the tests itself when the builder hands the step back.
  subagent_stop:
    - when: { actor: builder, changed: "src/**" }
      action:
        run:
          command: just test
          message: "The tests fail: fix them before you hand the step back"
```

A failing `run` on `subagent_stop` blocks the subagent, not the main session: the builder gets the message and the
output of the tests and keeps working. polix runs the tests itself instead of trusting that the builder ran them,
since a command of the agent never counts as passed. The entries of `subagent_stop` never run when the main
session ends its turn, and blocks of subagents never use up the loop guard of the main session.

### The changelog travels with the change

```yaml
on:
  stop:
    - when: { changed: ["src/**", "internal/**"] }
      unless: { changed: "CHANGELOG.md" }
      action:
        block: "Add an entry to CHANGELOG.md under Unreleased describing this change"
```

The release notes are written by the agent that knows the change best, at the moment it knows it best.

## Safety

### Keep production out of reach

Commands are split the way a shell runs them, so `cd infra && terraform apply -auto-approve` is caught too.

```yaml
on:
  pre_command:
    - when: { command: ["git push*--force*", "git push*-f *"] }
      action:
        block: "No force pushes: open a pull request or ask a person"
    - when: { command: ["git push*origin main*", "git push*origin HEAD:main*"] }
      action:
        block: "No direct pushes to main: open a pull request"
    - when: { command: "terraform apply*", not: { branch: "^main$" } }
      action:
        block: "terraform apply only runs from the pipeline of main"
    - when: { command: ["kubectl*--context*prod*", "aws*--profile*prod*"] }
      action:
        block: "The agent never acts on production: run it yourself, or use a staging context"
```

In a command pattern `*` matches anything, spaces and slashes included, so `kubectl*--context*prod*` holds wherever
the flag appears in the line.

### Hands off generated files and lockfiles

Generated files are rewritten by a tool, not by hand. The agent is sent to the source instead.

```yaml
on:
  pre_edit:
    - when: { path: ["**/*.pb.go", "**/*_generated.go", "**/zz_generated.*"] }
      action:
        block: "{path} is generated: edit its source (.proto, schema...) and run just gen"
    - when: { path: ["package-lock.json", "pnpm-lock.yaml", "go.sum", "poetry.lock", "uv.lock"] }
      action:
        block: "{path} is written by the package manager: change the manifest and let the tool update it"
```

A pattern without `/` matches the base name anywhere, like `.gitignore`, so `go.sum` also covers `tools/go.sum`.
These are regular entries, with a message of their own for each kind of file: the files git ignores are skipped.
To deny them to every actor whatever git ignores, list them in
[`protected_files`](#paths-no-agent-writes) instead.

### The agent cannot switch off its own guardrails

The hooks that call polix live in `.claude/settings.json`. An agent that edits it can turn polix off for the rest
of the session.

```yaml
on:
  pre_edit:
    - when: { path: ".claude/settings.json" }
      action:
        block: "{path} registers the hooks of polix: change it yourself, the agent does not"
```

polix only protects its own state without any entry: `AGENTS.md`, `CLAUDE.md`, `ai/rules/` and `polix.yaml` are
open unless the workflow closes them, as in [Paths no agent writes](#paths-no-agent-writes). This entry keeps the
hooks away from edits, for the main session too. `polix install` still writes them, since it runs as a command and
not as an edit; adding the files to `guardrails.protected_files` instead would also deny the commands of the agent
that write them. A repository where Codex was set up by hand, with `polix install --agent codex` (Codex support is
not implemented), names its hooks file too: `path: [".claude/settings.json", ".codex/hooks.json"]`.

## Context

### The right rules at the right time

Instead of loading every convention into `CLAUDE.md`, polix injects the rules of `ai/rules/` whose scope matches
the file being edited, once per session and agent, at the moment they matter. A subagent has a context of its own,
so it gets the rules on its own first edit, and again on its next run.

```yaml
on:
  start:
    - action:
        inject:
          file: "docs/architecture.md"

  pre_edit:
    - action:
        inject:
          rules: "{path}"
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
`SessionStart` for every source, a compaction included, so a `start` context comes back after each one.

```yaml
vars:
  key:
    branch: "^([A-Z][A-Z0-9]+-[0-9]+)"

on:
  start:
    - action:
        inject: "Branch {branch}, ticket {key}. Use bun, not npm. Run just test before committing."
    - action:
        inject:
          file: "docs/conventions.md"
```

Keep it short: it is injected at every start, resume and compaction. `file` takes one path, not a glob. Rules
injected by an `inject` of `rules` and the other injects that act once come back too: a compaction starts a new
context, so after it each one is due again on its next event.

### The same checks for the agent and for CI

A deterministic rule has a `check`, any file of the repository that passes or fails: a script, run as a program, or
a test, run by its runner of `rules.run` in polix.yaml (`"*_test.go": [go, test, "{dir}", -run, "^Test({names})$"]`, one `go test` per folder for all its rules, each
rule rerun alone only when it fails).
The agent runs the checks of the changed files before it stops, and CI runs the very same ones.

```yaml
on:
  stop:
    - when: { changed: "**" }
      action:
        run:
          rules: changed
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
`base_ref` is empty, and an empty `--base` leaves the choice to `polix.yaml`: the closest branch of `base_branch`,
or a failure when none can be found. On a push to a branch of `base_branch` itself, such as `main`, that branch is its own
base and `--changed` finds nothing to check, so the recipe runs every check on a push. In the hooks `changed` is what is not committed yet; in CI it is
what the branch changed since it forked.

A rule is written once, in `ai/rules/`. The agent is blocked by it locally, and a person who bypasses the agent is
blocked by it in CI.

## Rollout and evidence

### Evidence of the process each change followed

Every decision polix makes, and every line it observed for the workflow, is appended to
`<git-dir>/polix/log.jsonl`, with the entry that caused it and, when it was computed, the content hash. Nobody, agent
included, can edit it. The observed lines say what polix saw by itself: a subagent that `finished`, a run of polix
that `passed`, an entry that `blocked`, an inject made once and the start of a session.

```bash
polix log --tail 20
polix log --tail 0 --json | jq -c 'select(.kind == "observed" and .outcome == "passed")'
polix log --tail 0 --json | jq -c 'select(.kind == "observed" and .outcome == "finished" and .key == "polix:reviewer")'
polix log --tail 0 --json | jq -c 'select(.kind == "decision" and .event == "pre_commit" and .outcome == "block"
  and .branch == "PLAT-123-add-retry" and (.reason | startswith("The diff is not reviewed")))' | wc -l
```

That log answers questions such as *were the tests green on the exact content that was committed?* (a `passed` line
of `just test` with the hash of that content) or *how many times was a commit of this branch blocked for a missing
review?* (the `block` decisions of `pre_commit` with the message of the review gate). `--tail 0` reads the whole log,
not only its last 20 entries. A command the agent ran by itself never appears as passed. For teams that must show
how a change was made, not only what changed, it is evidence produced as a side effect of working.

## Putting it together: build, document, test, review, commit

What polix observes is tied to the content, and every edit changes it. So the order of the phases matters: a phase
that edits files must run before the checks and reviews that should cover its edits.

1. **build**: the agent implements the change.
2. **document**: the documenter edits docs and comments, so it runs before the tests and the review.
3. **test**: polix runs the tests itself on stop, once per content.
4. **review**: the reviewer is read-only, so its `finished` holds until the next edit inside the scope of the
   entry.
5. **commit**: only when the documenter and the reviewer finished on the content being committed, and the tests
   pass on it.

The `finished: documenter` in the `when` of the later stop entries, and the single stop rule, enforce that order:
the tests do not run on code the documenter is about to change, and a failing run ends the evaluation, so the
reviewer is not asked for before the tests are green.

The basics stay in `guardrails:`; the order of the phases is the flow of the team, so it goes in `on:`. The tests
are an entry of `on:` and not of `run_on_stop`, since they wait for the documenter.

```yaml
version: 1
base_branch: main
max_stop_retries: 3
vars:
  key:
    branch: "^([A-Z][A-Z0-9]+-[0-9]+)"

guardrails:
  protected_files: [".env", ".env.*"]
  protected_branches: [main]
  format_on_edit:
    - run: "npx prettier --write {path}"
      path: ["src/**/*.ts", "src/**/*.tsx"]

on:
  pre_edit:
    - unless: { var: key }
      action:
        block: "No ticket in the branch name: create the branch from a ticket"
    - action:
        inject:
          rules: "{path}"

  stop:
    - when: { changed: "src/**" }
      unless: { finished: documenter }
      action:
        block: "Launch the documenter subagent first: it edits files, so it goes before the tests and the review"
    # Tests only once the docs are written, review only once the tests pass: a failing run ends the evaluation.
    - when: { changed: "src/**", finished: documenter }
      action:
        run: just test
    - when: { changed: "src/**", finished: documenter }
      unless: { finished: "polix:reviewer" }
      action:
        block: "Launch the polix:reviewer subagent on the final diff"

  pre_commit:
    - unless: { finished: [documenter, "polix:reviewer"] }
      action:
        block: "Document and review this exact diff before committing ({key})"
    - action:
        run:
          command: just test
          message: "The tests fail on this exact diff ({key})"
```

If the reviewer finds a bug and the agent fixes it in `src/`, `finished: documenter` and
`finished: polix:reviewer` stop holding and `just test` runs again, so the flow runs again on the fixed code.
Nobody has to remember to invalidate anything: the fix is a new content, and polix observed nothing on it yet. The
`just test` of `pre_commit` shares its repeat with the one of the stop, since both run the same command: on a
content the stop already tested, the commit does not run it again.

## The gates of git

The entries of `on.pre_commit` only see the commits the agent makes in a form polix recognises. The section
`githooks` holds the gates git itself runs, on every commit, whoever makes it: a person, an IDE or an agent.
`polix install` declares the hooks of git that run them in the local git configuration (git 2.54 or later), and
`polix doctor` warns when git does not list them. Their vocabulary is small: `block` and `run`, and the conditions
`branch`, `changed` (the files staged for the commit) and, in `commit_msg`, `message` (the first line of the
message). Every `run` runs on every commit, with its output in the terminal: githooks keep no log.

Declaring the entries is enough: a `polix.yaml` with `githooks` runs `polix install --githooks` on every session
start of the AI, by itself and first in `start`, so each person who works on the repository with the AI has the
hooks of git without writing a recipe for it, and the AI learns at the start when git is too old to run them.
Nothing else is needed: no `start` entry is written by hand, and removing the entries of `githooks` turns it off.

### The message starts with the key of the ticket

On a ticket branch, the first line of every commit message starts with the key of the ticket, and merges go
through.

```yaml
vars:
  key:   # PLAT-123-add-retry -> PLAT-123
    branch: "^([A-Z][A-Z0-9]+-[0-9]+)"

githooks:
  commit_msg:
    - when: { branch: "^[A-Z][A-Z0-9]+-[0-9]+" }
      unless: { message: ["^{key}: ", "^Merge "] }
      action:
        block: "Start the message with the key of the ticket: {key}: {message}"
```

`git commit -m "fix the login"` on `PLAT-123-login` fails with
`polix: Start the message with the key of the ticket: PLAT-123: fix the login`, and
`git commit -m "PLAT-123: fix the login"` goes through.

### A message format by the files of the commit

A commit that touches the migrations says so in its message, and nothing of `secrets/` is ever committed, whoever
commits.

```yaml
githooks:
  pre_commit:
    - when: { changed: "secrets/**" }
      action:
        block: "Nothing of secrets/ is committed: unstage it with git restore --staged"
  commit_msg:
    - when: { changed: ["db/migrations/**", "*.sql"] }
      unless: { message: "^migration: " }
      action:
        block: "A commit with migrations starts with \"migration: \""
```

`changed` only looks at the staged files: a migration edited and left out of the commit does not ask for the
prefix.

### A command that blocks the commit

The linter runs on every commit that stages Go code, and its output shows in the terminal of the commit.

```yaml
githooks:
  pre_commit:
    - when: { changed: ["*.go", go.mod] }
      action:
        run:
          command: golangci-lint run ./...
          message: "Fix the lint before committing"
```

A failure aborts the commit with the message and `golangci-lint run ./... failed: exit status 1`.

### The agent cannot skip the hooks of git

`git commit --no-verify` skips both hooks, and `git -c hook.polix-pre-commit.enabled=false commit` one of them.
People may still do it; the agent may not:

```yaml
guardrails:
  forbidden_commands:
    - pattern: "git*--no-verify*"
      reason: "The hooks of git are not skipped: fix what they block"
    - pattern: "git*hook.polix-*"
      reason: "The hooks of git are not skipped: fix what they block"
```

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
  `max_stop_retries` blocks of the same entry on the same content, polix lets the agent stop and logs a
  `loop_guard` decision.
- **Fail closed when the policy is broken** (`onFailure: "block"`): built in for an invalid `polix.yaml`, which
  blocks edits and commands but never a stop. A machine without polix is not covered: Claude Code treats the
  missing command as a non blocking error, and the fragment of [`examples/AGENTS-polix.md`](../examples/AGENTS-polix.md)
  makes the agent offer to install it.
- **Guard configuration against changes** (`ConfigChange`): see
  [The agent cannot switch off its own guardrails](#the-agent-cannot-switch-off-its-own-guardrails) and
  [Paths no agent writes](#paths-no-agent-writes): `AGENTS.md` and `CLAUDE.md` in `protected_files`, and `polix.yaml`
  and `ai/rules/` kept from subagents with `path` and `writes`, so the skills can still change them. polix only
  protects its own state on its own.
- **Log what the agent does** (a `PreToolUse` hook that appends to a file): see
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
- **Context computed by a command**, such as `git log --oneline -5` at session start: `inject` takes a text, a
  file or the rules, not the output of a command.
- **MCP tools**: polix governs the edit tools and `Bash` only.
- **Environment reloads, worktrees, compaction, model switches, elicitations, tasks and teammates** (`CwdChanged`,
  `FileChanged`, `WorktreeCreate`, `PreCompact`, `PreModelSwitch`, `Elicitation`, `TaskCreated`, `TeammateIdle`):
  events polix does not register.
