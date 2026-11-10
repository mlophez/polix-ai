# polix.yaml reference

Every key, event, condition, verb and placeholder of `polix.yaml`, with its type, its default, what it does and
where it applies. For a whole workflow with every option in place see [`examples/polix.yaml`](../examples/polix.yaml);
for recipes by use case, [use-cases.md](use-cases.md); for the design behind it (protections, state, loop guard,
limits), [polix.md](polix.md).

Validate the file with `polix config check` and draw it with `polix graph`. `polix config check` rejects every
unknown key, event, condition or verb, so a typo never turns into an entry that silently stops applying.
`polix config fmt` formats it in place, keeping its comments (both take another workflow file as an argument, as
`polix config fmt examples/polix.yaml`): the keys of each section in the order of this
reference, with `requirements`, `guardrails` and `on` at the end; the items of a list and the keys you name (variables, globs of
`risk.weights` and `rules.run`) keep their order.

The file has two layers. [`guardrails`](#guardrails) holds the basics every project wants from the first minute,
each a short list: files no agent writes, commands no agent runs, branches nobody commits on, the formatter of each
edited file and the checks that must pass before the agent ends its turn. [`on`](#on) is where the team
orchestrates its flow (the review subagents, the builder, the plan, the ticket), as entries per event. Files no
agent writes and commands no agent runs are protections of the engine, applied before every entry; the other three
shortcuts of `guardrails` are only sugar: polix expands them into entries of `on` when it reads the file, and
`polix graph` draws them with the name of their shortcut.

- [The file at a glance](#the-file-at-a-glance)
- [Top-level keys](#top-level-keys)
- [Guardrails](#guardrails)
- [Entries](#entries)
- [Events](#events)
- [Conditions](#conditions)
- [Verbs](#verbs)
- [Placeholders](#placeholders)
- [Path globs and command patterns](#path-globs-and-command-patterns)
- [How an event is evaluated](#how-an-event-is-evaluated)

## The file at a glance

```yaml
version: 1                       # required
requires: 0.3.0                  # oldest polix that knows this file (run_on_stop needs it)
base_branch: [main, dev]         # branch the change merges into (polix rules check --changed)
max_stop_retries: 3              # loop guard of stop and subagent_stop
requirements: [go, golangci-lint]  # programs needed on the PATH, told to the AI on the first prompt
guardrails:                      # the basics every project wants
  protected_files: [.env, /AGENTS.md]           # path globs no agent writes, before every entry
  forbidden_commands:                           # commands no agent runs, before every entry
    - { pattern: "git push --force*", reason: "No force pushes" }
  protected_branches: [main, "release/*"]       # branches nobody commits on
  format_on_edit:                               # runs after each edit of its path
    - { run: "gofmt -w {path}", path: "*.go" }
  run_on_stop:                                  # must pass before the agent ends its turn
    - { run: "go test ./...", changed: "*.go" }
rules:
  path: ai/rules                 # folder of the rules
  run:                           # runner of each check, by glob
    "*_test.go": [go, test, "{dir}", -run, "^Test({names})$"]
plans:
  path: ai/plans
reports:
  path: ai/reports
risk:
  weights:                       # weight of each path in risk_score
    "*_test.go": 0.5
    "src/core/**": 3
guidance:                        # instructions injected on start
  commit: true
  edit: true
  layout: true
vars:                            # variables from the branch name
  key: { branch: "^([A-Z]+-[0-9]+)" }
on:                              # the entries of each event
  pre_command:
    - when: { command: "terraform apply*" }
      unless: { branch: "^main$" }
      action:
        block: "terraform apply only runs from main"
  pre_commit:
    - when: { changed: "src/**" }
      unless: { finished: polix:reviewer }
      action:
        block: "Lanza polix:reviewer sobre el diff actual"
```

## Top-level keys

Every key but `version` is optional. The key `guardrails`, a map of shortcuts, has a section of its own after
this one: [Guardrails](#guardrails).

Earlier builds named three of them otherwise: `protected` is now [`guardrails.protected_files`](#protected_files),
`base` is now [`base_branch`](#base_branch), and the loop guard is now [`max_stop_retries`](#max_stop_retries).
Only the new names are accepted: `polix config check` reports an old one with its line and its new spelling.

### version

- Type: integer. Required.
- The version of the schema. The only one is `1`; any other value is a problem.

### requires

- Type: a version, as `0.2.0`. Default: none.
- The oldest polix that knows every option the file uses. It is read before the rest of the file: an older polix
  blocks edits and commands asking to run `polix update`, instead of reporting the new options as mistakes, and
  not even the main session may edit `polix.yaml` meanwhile. A development build ignores it.
- `polix config check` rejects a value that is not a version.
- A file that uses `guardrails.run_on_stop` needs a polix that knows it: set `requires` to the release you install, so an
  older polix asks to update instead of reporting it as an unknown key.

### base_branch

- Type: a branch name, or a list of them. Default: `main`.
- The branch the change merges into. Only `polix rules check --changed` (CI) uses it: each candidate is tried
  locally and as `origin/<branch>`, and the one whose merge base is closest to HEAD wins. The hooks never use it:
  `changed` always compares with HEAD.
- `polix config check` rejects an empty name, and warns when no candidate exists.

```yaml
base_branch: main
base_branch: [main, dev]
```

### max_stop_retries

- Type: non-negative integer. Default: `3` (`0` also means 3).
- The loop guard: after this many blocks of the same entry of `stop` or `subagent_stop`, in the same session, by the
  same actor and on the same content, the agent may stop and the log records a `loop_guard` decision. A block let
  through does not end the evaluation, and a subagent let through does not count as `finished`.
- `polix config check` rejects a negative value.

### rules

The rules on how to write the code, which polix injects and checks.

- `path`: the folder of the rules, relative to the root. Default: `ai/rules`. No absolute path, no `..`, no glob,
  not the root itself. polix reads its `*.yaml` but `retired.yaml`, not its subfolders.
- `run`: a map of glob to argv: how the `check` of a rule runs, from the root and without a shell. The glob is
  matched against the path of the check (without `/`, against its base name); the first one that matches wins, in
  the order of the file. A check no glob matches runs as a program (its shebang; on Windows a `.sh` with the bash
  of Git for Windows and a `.py` with `py -3`, `python3` or `python`). There are no default runners.

Placeholders of the argv, only valid in `rules.run`:

- `{file}`: the check, relative to the root, with `/`.
- `{dir}`: its folder as `./<folder>` (`./` at the root).
- `{id}`: the id of the rule, `GOARCH-4`.
- `{name}`: the id with only letters and digits, `GOARCH4`.
- `{names}`: the names of the group joined with `|`, a regex alternation.
- `{files}`, `{dirs}`: one argument per distinct file or folder of the group; each must be a whole argument.

The checks whose argv, with the singular placeholders expanded, is the same share one invocation. When it fails,
each rule runs again alone to tell which ones fail.

```yaml
rules:
  path: ai/rules
  run:
    "tests/architecture/*_test.go": [go, test, "{dir}", -run, "^Test({names})$"]   # one run per folder
    "*_test.go": [go, test, "{dir}", -run, "^Test{name}$"]                          # one run per rule
    "*.py": [uv, run, python, "{file}"]
```

`polix config check` reports an empty argv, an empty glob, an unknown placeholder and `{files}` or `{dirs}` inside
a longer argument.

### plans and reports

- `plans.path`, default `ai/plans`: the folder of the plans the skill `plan` writes.
- `reports.path`, default `ai/reports`: the folder of the reports of audits and reviews.
- Same limits as `rules.path`. polix does not read them: it tells the agent where they are (`guidance.layout`).

### risk

- `weights`: an ordered map of path glob to a non-negative number, the weight of the matching files in
  [`risk_score`](#changed_lines-changed_files-risk_score). The first glob that matches a file wins; a file no glob
  matches weighs 1; `0` leaves it out. List the tests of a critical folder before the folder to make them weigh
  less.
- The score of a set of files is the sum of `weight × log2(1 + added + deleted lines)` of each, rounded.
- Calibrate it with `polix risk`, also on a past commit: `polix risk --commit <rev> --changed 'src/**'`.
- `polix config check` reports an empty glob or a negative weight; a weight that is not a number, or a key of `risk`
  other than `weights`, stops the parse.

```yaml
risk:
  weights:
    "*_test.go": 0.5
    "src/core/**": 3
    "docs/**": 0.2
    "*.lock": 0
```

### guidance

Built-in instructions polix injects on every `start` (startup, resume, clear, compaction), in English, before the
injects of the `start` entries, and only with a valid file. They ask for cooperation and block nothing. Every switch
is `true` when left out; `false` turns it off.

- `commit`: run each commit as a `git commit` of its own, not chained with `&&`, `;` or `|` nor wrapped in `sh -c`,
  so `pre_commit` sees it.
- `edit`: create and change files with the edit tool of the agent, not from the shell, so `pre_edit`, `post_edit`
  and the rules see the change.
- `layout`: the folders of `rules.path`, `plans.path` and `reports.path`.

### vars

- Type: a map of name to `{ branch: <regex> }`. Default: none.
- Each variable is extracted from the current branch name with a Go (RE2) regular expression: its value is the
  first group, or the whole match without groups. When the regex does not match, the variable has no value: the
  condition [`var`](#var) fails and `{name}` is left as is.
- A variable cannot take the name of a built-in placeholder (`path`, `command`, `branch`, `changed_lines`,
  `changed_files`, `risk_score`), and `branch` is its only key.

```yaml
vars:
  key: { branch: "^([A-Z][A-Z0-9]+-[0-9]+)" }   # PLAT-123-add-retry -> PLAT-123
  release: { branch: "^release/[0-9.]+$" }      # release/2.4 -> release/2.4
```

### requirements

- Type: a list of program names. Default: none.
- The programs the repository needs on the PATH, such as the linter, the test runner or the formatter its workflow
  runs. polix checks only these, plus `git`, which polix itself runs and adds to a non-empty list that does not name
  it, so the OK line of `polix doctor` counts it (a missing git is the `fail` of the area `git`, never a
  requirement): it never derives them from the commands of the workflow nor checks versions.
- On the first `prompt` of a session, before the entries, the hook looks them up and, when some are missing, tells
  the AI as a context: `These programs that polix.yaml requires are not on the PATH: <names>. Before doing anything
  else, install them, or ask the user to install them if you cannot.` It is a once-only context: the later prompts
  of the session do not repeat it; it is due again after a `start` of the session (startup, resume, clear,
  compaction) and when the set of missing programs changes; a refused prompt does not count it as told. `start`
  never tells it, and it never blocks.
- `polix doctor` warns once per missing program, in the area `requirements`: `<program> is not on the PATH:
  polix.yaml requires it`; with every program found it gives one OK line, `<n> programs of requirements on the
  PATH`. A missing program never makes it fail; `polix doctor --hook` lists the missing ones.
- The lookup is that of the command of a `run`: the PATH the hooks give polix, not that of an interactive shell. On
  Windows it tries the extensions of `PATHEXT`, so write names without extension (`golangci-lint`, never
  `golangci-lint.exe`, which is missing on macOS and Linux). A shell alias or function, a script without extension
  that only Git Bash runs, and a program found only in the current folder count as missing.
- `polix config check` reports a single name, a map and an item that is not a name at their line, with the form
  the key takes (`requirements takes a list of program names, as [go, golangci-lint]`), an empty name
  (`requirements: a program name cannot be empty`), a name with spaces around it, a path (`bin/tool`, which the
  lookup would resolve against the working folder instead of the PATH) and a repeated name.
- `polix config fmt` places it right before `guardrails`, keeping the order of its programs. A polix older than the
  key reports it as unknown: set `requires` to a release that knows it.

```yaml
requirements: [go, golangci-lint]
```

### on

A map of [event](#events) to a list of [entries](#entries).

## Guardrails

`guardrails` holds five shortcuts, each with exactly one form, so every file reads the same:

- [`protected_files`](#protected_files): a list of path globs;
- [`forbidden_commands`](#forbidden_commands): a list of maps with `pattern` and `reason`;
- [`protected_branches`](#protected_branches): a list of branch names;
- [`format_on_edit`](#format_on_edit): a list of maps with `run`, `path` and `message`;
- [`run_on_stop`](#run_on_stop): a list of maps with `run`, `changed` and `message`.

```yaml
guardrails:
  protected_files: [".env", ".env.*", "AGENTS.md"]
  forbidden_commands:
    - pattern: "rm -rf /*"
    - pattern: "git push --force*"
      reason: "No force pushes: open a pull request"
  protected_branches: [main, "release/*"]
  format_on_edit:
    - { run: "gofmt -w {path}", path: "*.go" }
  run_on_stop:
    - { run: "golangci-lint run ./...", changed: "*.go" }
    - { run: "go test ./...", changed: ["*.go", go.mod, go.sum] }
```

`protected_files` and `forbidden_commands` are protections of the engine: with a valid file, the hook applies them
right after the protection of the state of polix, before every entry of `on` and before the skip of the edits of
files git ignores, to every actor, the main session included, without exceptions. A denial is logged as
`guardrails.protected_files` or `guardrails.forbidden_commands`, and `polix graph` draws both lists in a subgraph
`guardrails` before the events.

`protected_branches`, `format_on_edit` and `run_on_stop` are only sugar: when polix reads the file it expands
each one into ordinary entries of `on`, and from then on the hook, `polix graph`, the repeat rules and the loop
guard see only entries. Each expanded entry takes the line of the item it comes from and the path of its shortcut
as its origin (`guardrails.format_on_edit`), which `polix graph` labels `from guardrails.<shortcut>`. When a
shortcut falls short, write the entries it expands into in `on` and tune them.

`polix config check` reports, each at the line of its item:

- a value in another form than the one of its shortcut, with that form: a single string for `protected_files` or
  `protected_branches`, or a string item in `forbidden_commands`, `format_on_edit` or `run_on_stop`;
- an unknown key inside `guardrails`, with the list of the five, and an unknown key of an item, with its keys;
- an empty glob, pattern or branch name, an item without `pattern` or `run`, a `pattern`, `reason` or `message`
  that is not a text, a `run` that is not a command, and the problems of the entry an item expands into (an
  unknown placeholder in `run`, say).

### protected_files

- Type: a list of path globs.
- Paths no actor writes, the main session included. An edit of a matching path is denied, and so is a command that
  writes files and names one (the heuristic of [`writes`](#writes)), on `pre_command` and `pre_commit`. Reading is
  allowed. It also covers the files git ignores, such as `.env`, but never an edit outside the repository.
- A glob without `/` matches the name in any folder (`.env`); a leading `/` anchors it to the root
  (`/AGENTS.md`); `**` covers a folder (`ai/rules/**`). On macOS and Windows it ignores case.
- Built in, even without this key: nobody writes the state of polix (`<git-dir>/polix/`).
- Messages: `<path> is protected in polix.yaml: only a person changes it.`, and, for a command on a folder or a
  wildcard that holds a protected path, `<folder> holds <glob>, protected in polix.yaml: only a person changes it.`
  (`ai holds ai/rules/**, ...` for `rm -rf ai`).
- No exceptions: to let an actor or a branch through, take the glob out and write a `block` in `on` with `path`,
  `writes` and an `unless`, which then skips the files git ignores.

```yaml
guardrails:
  protected_files: [".env", "AGENTS.md"]
```

### forbidden_commands

- Type: a list of maps, each with:
  - `pattern`: required, a [command pattern](#command-patterns);
  - `reason`: optional, the message for the AI.
- Commands no actor runs, the main session included, in any position of the line: the line is split into its
  commands, and each is matched whole and as written, as for the condition [`command`](#command).
- A heuristic aimed at accidents, not at an agent set on getting around it. A pattern misses another spelling of the
  same command (`rm -fr /`, `git push -f`, the flags in another order or with two spaces), a wrapper
  (`sudo rm -rf /`, `env X=1 ...`) and a command run through an interpreter or a substitution (`bash -c '...'`,
  `$(...)`). List the usual spellings, and start a pattern with `*` to cover a wrapper: `*rm -rf /*`,
  `git push*--force*`, `git push* -f*`.
- On `pre_command` and `pre_commit`, the first item in the order of the file whose pattern matches a command of the
  line denies it, with its `reason` or, without one, `This command is forbidden by polix.yaml: a person must run
  it.`, followed by the line of the command. The log records the line of the item.
- No exceptions: a command forbidden only for some actors or on some branches is a `block` in `on` with
  [`command`](#command) and those conditions.

```yaml
guardrails:
  forbidden_commands:
    - pattern: "rm -rf /*"
    - pattern: "git push --force*"
      reason: "No force pushes: open a pull request"
```

### protected_branches

- Type: a list of branch names. A `*` in a name matches any text, slashes included (`release/*`).
- Branches nobody commits on.
- Expansion: one regular entry on `pre_commit` with [`branch`](#branch), an anchored regular expression of the
  names, whose message asks to create a branch for the change.

```yaml
guardrails:
  protected_branches: [main, "release/*"]
# expands into
on:
  pre_commit:
    - when: { branch: "^(?:main|release/.*)$" }
      action: { block: "{branch} is protected in polix.yaml: create a branch for this change and commit there." }
```

### format_on_edit

- Type: a list of maps, each with:
  - `run`: required, a command as the verb [`run`](#run) takes it, a string split on spaces or a list of
    arguments, placeholders allowed (`{path}` is the edited file);
  - `path`: optional, a path glob or a list of them, matched against the edited file;
  - `message`: optional, the text before the output of a failure.
- The command that formats or lints each edited file. Without `path` it runs on every edited file. A failure never
  blocks, since the edit already happened: its output reaches the AI as context.
- The filter is `path`, the edited file, not `changed`, which holds for any file not committed yet and would format
  a file of another kind.
- Expansion: one regular entry on `post_edit` per item, with `run`, and with the condition `path` when it has one.

```yaml
guardrails:
  format_on_edit:
    - { run: "gofmt -w {path}", path: "*.go" }
    - { run: "npx eslint {path}", path: ["*.ts", "*.tsx"], message: "eslint:" }
# expands into
on:
  post_edit:
    - when: { path: "*.go" }
      action: { run: "gofmt -w {path}" }
    - when: { path: ["*.ts", "*.tsx"] }
      action: { run: { command: "npx eslint {path}", message: "eslint:" } }
```

### run_on_stop

- Type: a list of maps, each with:
  - `run`: required, a command, as in `format_on_edit`;
  - `changed`: optional, a path glob or a list of them: the check runs only when a file not committed yet matches
    (the condition [`changed`](#changed), which also sets the [scope](#scope-of-an-entry) of the entry);
  - `message`: optional, the text before the output of a failure.
- The checks that must pass before the agent ends its turn. A failure blocks the `stop` with its output, up to the
  loop guard, and the first one that fails ends the evaluation, so list the cheaper or more basic check first. A
  check that passed is not run again on the same content.
- Expansion: one regular entry on `stop` per item, with `run`, and with the condition `changed` when it has one.
  For `always`, the end of a subagent or a condition such as `finished`, write the entry in `on`.

```yaml
guardrails:
  run_on_stop:
    - { run: "go test ./...", changed: ["*.go", go.mod, go.sum], message: "The tests fail:" }
# expands into
on:
  stop:
    - when: { changed: ["*.go", go.mod, go.sum] }
      action: { run: { command: "go test ./...", message: "The tests fail:" } }
```

### Order of the expanded entries

In each event the entries come in this order, each group in the order of the file:

1. the entries of `protected_branches`, `format_on_edit` and `run_on_stop`;
2. the entries written in `on`.

So the basics run before the flow of the team, and `protected_files` and `forbidden_commands`, which are no
entries, before both.

## Entries

```yaml
- when: { <condition>: <value>, ... }     # optional: all must hold
  unless: { <condition>: <value>, ... }   # optional: skip the entry when all hold
  action:                                 # required: exactly one verb
    <verb>: <value>
```

- `when`: a map of [conditions](#conditions), all of which must hold. Without it the entry always applies.
- `unless`: a map of conditions; the entry is skipped when all of them hold. Without it nothing skips the entry.
- `action`: a map with exactly one [verb](#verbs): `block`, `run` or `inject`.

An exception to an entry is a `not` in its `when`, or a condition in its `unless`. A YAML comment above the entry
is the place for its reason, for people; the message of a `block` or a `run` is the reason the AI reads.

The entries skip the edits outside the repository and of the files git ignores: they govern the change, what a
commit may hold, so a build output, a local `.env` or a scratch file never fires a ticket, plan, format or docs
entry. Only the protection of the state of polix, [`protected_files`](#protected_files) and
[`forbidden_commands`](#forbidden_commands) reach an ignored file.

## Events

In lifecycle order. Valid verbs and specific conditions per event:

### start

- When: a session starts, resumes, is cleared or is compacted.
- Verbs: `inject`, `run`. It cannot be blocked: a failing `run` reaches the AI as context.
- Every `inject` acts every time on `start`, so the context survives a compaction; `always` has no effect.

### prompt

- When: the user sends a prompt.
- Verbs: `block`, `run`, `inject`. A block rejects the prompt before the model sees it; an inject adds context to it.
  An inject before a block that rejects the prompt is not counted as given.

### pre_edit

- When: before the agent edits a file, once per path.
- Verbs: `block`, `run`, `inject`. A block denies the edit.
- Conditions of its own: [`path`](#path).

### post_edit

- When: after the agent edited a file, once per path.
- Verbs: `run`, `inject`. It cannot be blocked: a failing `run` reaches the AI as context (feedback, not a gate).
- Conditions of its own: [`path`](#path).

### pre_command

- When: before a shell command runs.
- Verbs: `block`, `run`, `inject`. A block denies the command.
- Conditions of its own: [`command`](#command), [`writes`](#writes).

### post_command

- When: after a shell command ran, or failed to run.
- Verbs: `run`, `inject`. It cannot be blocked: a failing `run` reaches the AI as context.
- Conditions of its own: [`command`](#command), [`writes`](#writes), [`exit`](#exit).

### pre_commit

- When: before a command line that holds a `git commit` runs, right after its `pre_command`.
- Verbs: `block`, `run`, `inject`. A block denies the commit.
- Conditions of its own: [`command`](#command), [`writes`](#writes).

### subagent_stop

- When: a subagent is about to end; `actor` is its type.
- Verbs: `block`, `run`. A block keeps the subagent working, up to the loop guard. `inject` is rejected: an agent
  that stops cannot receive context.

### stop

- When: the main session is about to end its turn; `actor` is always `session`. A subagent ends with
  `subagent_stop`, which never runs these entries.
- Verbs: `block`, `run`. A block keeps the session working, up to the loop guard. `inject` is rejected.

## Conditions

Each condition goes in a `when`, an `unless`, a `not` or an `any`. Those that take a list accept a single string
as a list of one. Placeholders are expanded in every string value except `branch`.

### branch

- Value: a regular expression (Go RE2).
- Holds when the current branch matches it. It is not anchored: write `^main$` for a whole match. On a detached
  HEAD the branch is empty. Placeholders are not expanded.
- Events: all.

```yaml
when: { branch: "^(main|dev)$" }
```

### var

- Value: a variable name of [`vars`](#vars), or a list of them.
- Holds when every variable of the list has a value, that is, its regex matched the branch.
- Events: all. `polix config check` rejects a variable not defined in `vars`.

```yaml
unless: { var: key }
when: { var: [key, release] }
```

### path

- Value: a path glob, or a list of them.
- Holds when the edited path matches any of them. On macOS and Windows, whose file names ignore case, it ignores
  case too, so `.ENV` matches `.env` there.
- Events: `pre_edit`, `post_edit` only.

```yaml
when: { path: ["*.ts", "*.tsx"] }
when: { not: { path: "ai/plans/{key}-*.md" } }
```

### changed

- Value: a path glob, or a list of them.
- Holds when some file not committed yet (it differs from HEAD, or it is untracked and not ignored) matches any of
  them. Once committed a file no longer counts: the change is reviewed commit by commit.
- It also sets the scope of the entry: the files the measures, `finished` and a `once` block look at (see
  [Scope](#scope-of-an-entry)).
- Events: all.

```yaml
when: { changed: ["*.go", go.mod, go.sum] }
```

### file

- Value: a path glob, or a list of them.
- Holds when some file of the working tree, tracked or untracked but not ignored, matches any of them. With
  [`contains`](#contains), some matching file must also hold the text.
- Events: all.

```yaml
unless: { file: "ai/plans/{key}-*.md" }
```

### contains

- Value: a text (placeholders allowed).
- Only next to `file`: some file that matches `file` holds this text, as a plain substring.
- Never directly inside `any`, where it would hold on its own; inside a `not` within an `any` it is valid.

```yaml
unless: { file: CHANGELOG.md, contains: "{key}" }
```

### command

- Value: a command pattern, or a list of them.
- Holds when some command of the line matches any pattern. The line is split into its commands
  (`cd x && just test 2>&1 | tail -3` gives `cd x`, `just test 2>&1` and `tail -3`, quotes respected), and each is
  matched whole. See [command patterns](#command-patterns).
- Events: `pre_command`, `post_command`, `pre_commit`.

```yaml
when: { command: ["git push*origin main*", "git push --force*"] }
```

### writes

- Value: a path glob, or a list of them.
- Holds when some command of the line writes a matching path: the target of a redirection into the repository
  (`>`, `>>`, `&>`...; not `2>&1`, `/dev/null` or a path outside), or a file named by a program that writes by
  itself (`rm`, `mv`, `cp`, `tee`, `touch`, `ln`, `chmod`, `install`, `rsync`, `truncate`, `dd of=`, `sed -i`,
  `perl -i`), which also counts on a folder that holds the path or a wildcard that reaches it (`rm -rf ai/*` for
  `ai/rules/**`). Reading never counts.
- A heuristic aimed at accidents: it does not see writes through an interpreter (`python -c`, `bash -c`), a path
  reached through `cd`, or `find -delete`. On macOS and Windows it ignores case.
- Events: `pre_command`, `post_command`, `pre_commit`.

```yaml
when: { writes: ["polix.yaml", "ai/rules/**"], not: { actor: session } }
```

### exit

- Value: an integer.
- Holds when the exit code of the command line equals it. A command that failed to run without an exit code counts
  as `1`. Next to `command`, it only holds when a matching command decides the code of the line: alone, last, or
  followed by `&&` (in `just test | tail`, `tail` decides it).
- Events: `post_command` only.

```yaml
when: { command: "just test*", exit: 1 }
when: { command: "npm install*", exit: 0 }
```

### actor

- Value: `session` (the main session) or a subagent type, or a list of them.
- Holds when who acts is any of them. An agent of a plugin is named with its prefix (`polix:reviewer`,
  `acme:builder`); an agent of `.claude/agents/` by its bare name. On `stop` the actor is always `session`; on
  `subagent_stop` it is the type of the subagent that ends.
- Events: all.

```yaml
unless: { actor: [acme:builder, acme:builder-ui] }
```

### finished

- Value: a subagent type, or a list of them.
- Holds when every subagent of the list ended on this branch, or on another one with the same HEAD commit (a branch
  made with `git checkout -b` after the review), its `subagent_stop` was not refused nor let through by
  the loop guard, and the files of the [scope](#scope-of-an-entry) of the entry did not change since. polix observes
  the end of those subagents by itself: no entry on `subagent_stop` is needed. It never holds without a known
  content hash, so a broken git asks again.
- Events: all.

```yaml
unless: { finished: polix:reviewer }
unless: { finished: [polix:reviewer, acme:security-reviewer] }
```

### changed_lines, changed_files, risk_score

- Value: a non-negative integer. Each holds from that number up.
- They measure the change not committed yet (untracked files whole), over the files of the nearest `changed` (in
  their own map, else in the enclosing one), else the whole change:
  - `changed_lines`: added plus deleted lines;
  - `changed_files`: changed files, deleted and binary included;
  - `risk_score`: the score of [`risk.weights`](#risk).
- When git cannot measure the change they hold, and the call warns (fail closed). The measures are those of the
  start of the call.
- Events: all.

```yaml
when:
  changed: "src/**"
  any:
    changed_lines: 150
    changed_files: 5
    risk_score: 25
```

### not

- Value: a map of conditions.
- Holds unless every nested condition holds: with one nested condition it is its negation; with several, it fails
  only when all of them hold.

```yaml
when: { not: { actor: session, path: "docs/**" } }   # every edit but those of the main session in docs/
```

### any

- Value: a non-empty map of conditions.
- Holds when at least one nested condition holds. It may hold a `not`, and sit inside one. A `changed` inside it
  scopes the measures inside it.

```yaml
when:
  command: "git push*"
  any:
    branch: "^(main|dev)$"
    not: { finished: polix:reviewer }
```

### Scope of an entry

The scope of an entry is the set of files the nearest `changed` of its `when` names, or the whole tree without
one. The measures, `finished` and a `once` block look only at those files: a review of `changed: "src/**"` survives
an edit of `README.md`, and any edit of `src/` asks for a new one. Name in `changed` every file the subagent reviews:
`go.mod` is not `*.go`.

## Verbs

The `action` of an entry holds exactly one verb. Each has a short form, its value alone, and a long form, a map.

### block

Blocks the event with a message for the AI.

- Events: `prompt`, `pre_edit`, `pre_command`, `pre_commit`, `subagent_stop`, `stop`.
- Short form: the message (placeholders allowed).
- Long form:
  - `message`: required, the message;
  - `once`: boolean, default `false`.
- Repeat: a gate blocks every time its entry applies, until its `unless` holds. With `once: true` it is a reminder:
  it blocks the first attempt of an actor in a session on a content (of its scope) and lets the next one through;
  another actor, a new session or an edit inside the scope is reminded again. A rule that must hold has no `once`.

```yaml
action:
  block: "No hay ticket: crea la rama con /ticket"
action:
  block:
    once: true
    message: "Revisa si README.md y docs/ deben cambiar; si no, repite el commit."
```

### run

Runs a command from the root without a shell (no pipes, redirections or globs), or the checks of the rules.

- Events: all. On `prompt`, `pre_edit`, `pre_command`, `pre_commit`, `subagent_stop` and `stop` a failure blocks,
  as a `block` does; on `start`, `post_edit` and `post_command` it reaches the AI as context.
- Short form: the command, a string split on spaces or a list of arguments (placeholders allowed in each; `{path}`
  stays one argument).
- Long form, exactly one of:
  - `command`: the same command;
  - `rules`: `changed` (the checks of the rules whose scope matches the changed files) or a path such as `"{path}"`;
  plus:
  - `message`: the text before the output of a failure (placeholders allowed);
  - `always`: boolean, default `false`.
- A failure answers with the message, a blank line and the last 15 lines of the output (the failing rule ids and
  their output for `rules`).
- Repeat: a run that passed is not run again on the same content, on the same branch or on another one with the
  same HEAD commit, keyed by what it runs (so two entries
  that run `just test` share it, and `{path}` makes one key per path); a failing run runs again on the next event.
  `always: true` runs it every time, for a result that depends on more than the files (network, clock).
- Every run of one hook call shares a budget of 10 minutes.

```yaml
action:
  run: "uv run ruff check {path}"
action:
  run: [npx, prettier, --write, "{path}"]
action:
  run:
    command: just test
    message: "Los tests no pasan sobre este diff:"
action:
  run:
    rules: changed
```

### inject

Injects a text for the AI, as context.

- Events: all but `subagent_stop` and `stop`.
- Short form: the text (placeholders allowed).
- Long form, exactly one of:
  - `text`: a literal text (placeholders allowed);
  - `file`: a file of the repository (placeholders allowed); a missing file injects nothing;
  - `rules`: a path such as `"{path}"`: the rules whose scope matches it, each as `- <id>: <description>`;
  plus:
  - `always`: boolean, default `false`.
- Repeat: each actor gets each expanded text once per session, and again after a compaction or, for a subagent, on
  its next run; `inject: { rules }` gives each rule once per actor and session. A placeholder that changes on every
  event (`{command}`, a measure) makes a new text each time. `always: true` injects it on every event. On `start`
  every inject acts every time.

```yaml
action:
  inject: "Rama {branch}: el flujo es ticket, plan, build, review y commit."
action:
  inject:
    file: docs/flujo.md
action:
  inject:
    rules: "{path}"
action:
  inject:
    always: true
    text: "Los tests fallan: arréglalos antes de seguir."
```

## Placeholders

Expanded in the strings of conditions (except `branch`), verbs and `contains`. An unknown one is a problem.

- `{path}`: the edited path, relative to the root; on `pre_edit` and `post_edit`, empty elsewhere.
- `{command}`: the whole command line; on `pre_command`, `post_command` and `pre_commit`, empty elsewhere.
- `{branch}`: the current branch.
- `{changed_lines}`, `{changed_files}`, `{risk_score}`: the measures of the change not committed yet, over the
  `changed` of the `when` of the entry, else the whole change; `?` when git cannot measure it.
- `{<var>}`: each variable of `vars`; left as is when it has no value.

The placeholders of `rules.run` (`{file}`, `{dir}`, `{id}`, `{name}`, `{names}`, `{files}`, `{dirs}`) only exist
there.

## Path globs and command patterns

### Path globs

Used by `path`, `changed`, `file`, `writes`, `guardrails.protected_files`, the `path` of `format_on_edit`, the
`changed` of `run_on_stop`, `risk.weights` and the scopes of the rules.

- Relative to the root, with `/`; the whole path must match.
- `*` and `?` stay inside a segment; `**` crosses segments.
- A pattern without `/` also matches the base name in any folder: `*.md` matches `docs/guide.md`.
- A leading `/` anchors it to the root, as in `.gitignore`: `/polix.yaml` is not `examples/polix.yaml`.

### Command patterns

Used by `command` and the `pattern` of `forbidden_commands`. Each command of the line is matched whole, with its
surrounding spaces trimmed:

- `*` matches any text, spaces and slashes included; `?` matches one character.
- `just test` only matches the bare command; `just test*` also matches `just test -k x` or `just test 2>&1`.

## How an event is evaluated

The shortcuts `protected_branches`, `format_on_edit` and `run_on_stop` are already entries of `on` by then, in
the [order of the expanded entries](#order-of-the-expanded-entries).

1. The built-in protection comes first: nobody writes the state of polix, even without a workflow.
2. With a valid file, [`protected_files`](#protected_files) and [`forbidden_commands`](#forbidden_commands) come
   next, also on the edits of files git ignores, never on an edit outside the repository. The first protection that
   denies ends the evaluation.
3. Edits outside the repository, or of files git ignores, skip the entries: they govern the change, what a commit
   may hold.
4. The entries of the event run in order. An entry applies when its `when` holds and its `unless` does not, and
   then carries out its verb, unless the repeat rules of that verb skip it.
5. The first entry that actually blocks (a `block`, or a failing `run` where a run blocks, not let through by the
   loop guard) ends the evaluation: the entries after it do not run. The order is the priority, and the AI gets one
   failure per answer.

An invalid file blocks `pre_edit`, `pre_command` and `pre_commit` with its problems (fail closed), except the main
session editing `polix.yaml` or running `polix config check`, `polix config fmt --check`, `polix update`,
`polix doctor` or `polix version`, with no command substitution nor redirection but `2>&1`; on the other events it
only warns, so a broken file never traps a stop in a loop. Its protected files and forbidden
commands do not apply then: only the built-in protection does, and an edit outside the repository or of a file git
ignores goes through. The details are in
[polix.md](polix.md#how-the-entries-of-an-event-are-evaluated).
