# AnswerHook

Answers one hook call of Claude Code or Codex: it turns the event of the agent into neutral events, evaluates the
protections and the workflow of polix.yaml for each of them, and answers with the exit code and the output the
agent expects (2 blocks the action, context goes to the session, warnings to the user).

## Trigger

```
polix hook <claude|codex> [--dir <dir>]   < input of the hook on stdin
```

- `<claude|codex>`: the agent behind the call; any other name exits with 1.
- `--dir`: the repository; without it, the `cwd` of the input, else the working directory.

It is the entry point of the hooks of Claude Code; the polix plugin registers it, and people do not run it.
`codex` is accepted, but Codex support is not implemented and untested.

## Request

`Request`, filled by the command:

- `Agent`, from the argument, checked to be `claude` or `codex` (`agent.Agent.Valid`) before the shell runs; it
  chooses the format of the hooks of the agent.
- `Dir`, from `--dir`.
- `Input`, the bytes of stdin as the agent wrote them, in the format of the agent.
- `Version`, the version of the running polix (`build.Current`), which polix.yaml may require.
- `System`, the system polix runs on (`runtime.GOOS`): on macOS and Windows file names ignore case, and on
  Windows `NUL` is the null device.

The formats of the agents are the transport of the entry, not domain: the in files declare the unexported
interface `format`, with `parse`, which turns the input into the native name of the event and the neutral
`agent.Event`s (one `pre_edit` or `post_edit` per edited path, made absolute from the `cwd` of the call; a
`pre_command` whose line runs a `git commit` also gives `pre_commit`), and `render`, which turns an
`agent.Answer` into stdout, stderr and the exit code for the native event. It has one implementation per agent:
`claude` in `in_claude.go` and `codex` in `in_codex.go`, which share the fields of the input both agents write and
the rendering of an answer, in `in.go`.

## Logic

`Decide(ev hook.Evaluation) Output` decides the next step of the evaluation of one event. It has the step
shape of `docs/architecture.md`: a later entry of an event sees what an earlier one did (a `run` that rewrites
files changes the content a later `finished` compares), and the first entry that blocks stops the evaluation, so the
logic cannot plan the event at once.

`hook.Evaluation` (domain) holds the workflow, whether polix.yaml exists (`Present`), why it cannot be used
(`Invalid`), the root, the git directory, the system, the event, the `Facts` of the repository read so far and the
`Journal`, the outcomes of the steps that acted on this event in order. The logic walks the event from its start
every time it is asked: a fact it knows is used, a fact it lacks is the next step; a step that acts takes its
outcome from the journal, in order, and the first one the journal lacks is the next step. `Output.Step`
is that step; when it is none, `Output.Answer` is the answer to the event and `Output.Facts`
what the next event of the call starts from.

`Decide` is a router: it builds the pass of the event (`pass`), and `route`, a `switch` on the name of the event in
`logic.go`, gives the family that answers it, a method of `pass` in a file of its own. Each family reads top-down,
holds only what its events do and calls the engine as a library:

- `session` (`logic_session.go`), for `start` and `prompt`: the session context and the `started` line of `start`
  (rules 4 and 6), the context of the missing programs of `requirements` on `prompt` (rule 14), and the discard of
  the once-only injects of a refused `prompt` (rule 6).
- `edit` (`logic_edit.go`), for `pre_edit` and `post_edit`: the protections of an edit (rule 1), the edits an
  unusable workflow lets through (rule 2) and the skip of the edits outside the change (rule 3).
- `command` (`logic_command.go`), for `pre_command`, `post_command` and `pre_commit`: the protections of a command
  (rule 1), the commands of polix an unusable workflow lets through (rule 2) and the commits outside the change of a
  `pre_commit` (rule 3).
- `stop` (`logic_stop.go`), for `stop` and `subagent_stop`: the loop guard of `max_stop_retries` (rule 10) and the
  `finished` line of `subagent_stop` (rule 9).

The engine is the rest, in the `logic_<topic>.go` files, and has no `if` on the name of an event: the step machinery
and the readers of the facts (`logic_facts.go`), the protections (`logic_protect.go`), the walk of the entries and
their verbs (`logic_entries.go`, `logic_run.go`, `logic_inject.go`), the conditions and the placeholders
(`logic_condition.go`, `logic_placeholder.go`), the parsing of command lines, what they write and where their commits
land (`logic_cmdline.go`, `logic_writes.go`, `logic_commit.go`), and the log (`logic_log.go`). What differs per event
reaches it as fields of the pass: `informed`, set by `Decide` for the events of `workflow.Informed`, turns a failing
`run` into context; `always`, set by `session` on `start`, makes every inject act every time; and `loop`, set by
`stop`, sends every block through the loop guard.

A `hook.Step` asks for one of: the branch, the content hash (of an epoch), the paths that differ between the tree of an
observed line and the current one (`Compare` and `Tree`, kept in the facts per pair of trees), the changed files,
the diff (`Diff`: the
added and deleted lines of each file not committed yet, the same files as the changed ones), the files, the rules, the
log, whether git ignores a path, the real paths of some paths, which programs the PATH does not run (`Programs`, kept
in the facts as `Missing` once per call), the content of a file (of an epoch), or acts: runs
a command, runs the checks of one or several rules (`rule.Run`, with the argv of its runner of `rules.run`) or
appends an entry to the log. A check step may decide several rules: the runs come from `rule.Rules.Plan`, and a failed
shared run is rerun once per rule (`rule.Run.Settle`), each rerun a step of its own, so the block names only the
rules whose own run fails. The epoch is the number of runs carried out before: any run may rewrite files, so the hash and the files
read after it are those of a new epoch, read again.

The rules it follows, as before this use case:

1. Protections, before the entries of the workflow and before the skip of the edits and commits outside the change
   (rule 3); the first one that denies ends the evaluation. Whatever the workflow says and also without one or with
   an invalid one, nobody edits the state of polix (`<git-dir>/polix`, by its real path, without case where file
   names ignore it) nor runs a command that may write and names it, logged as `builtin`. With a valid polix.yaml, no actor, the
   main session included, writes a path of `guardrails.protected_files` (`workflow.Workflow.ProtectedFiles`, matched
   with `anyPathFold`), also when git ignores it but never on an edit outside the repository, by an edit, a
   redirection or a program that writes the files it names (`rm`, `mv`, `cp`, `sed -i`...), its folders and
   wildcards included (`lineWrites`): the reason names the path (`<path> is protected in polix.yaml: only a person
   changes it.`) or the protected glob a folder or a wildcard holds (`<folder> holds <glob>, protected in
   polix.yaml: ...`), and it is logged as `guardrails.protected_files`. Reading a protected path passes. Nor does any
   actor run a command of `guardrails.forbidden_commands` (`workflow.Workflow.ForbiddenCommands`, the first in file
   order whose pattern matches any command of the line, `forbidden`) on `pre_command` or `pre_commit`: the reason
   is its `reason`, or `reasonForbidden`, followed by the line, and it is logged as `guardrails.forbidden_commands`
   with the line of its item.
2. A workflow that cannot be used (it cannot be read or parsed, has problems or requires a newer polix) blocks
   edits and commands with its problems, logged as `invalid`, except an edit outside the change, an edit of
   polix.yaml by the main session (not when it requires a newer polix) and the commands that only check the
   workflow or update polix; any other event only warns, so a stop is never blocked. Problems that may come from a
   newer polix hint to run `polix update` (`hook.Evaluation.Running`, which the shell asks); a name of the old
   vocabulary, reported with its new spelling, does not.
3. Without polix.yaml, for an edit outside the repository or of a file git ignores, and for a `pre_commit` whose
   commits all happen in another repository (`git -C ../other commit`), only the protections apply: the
   entries govern the change, what a commit may hold, so a build output, a local `.env` or a commit in another
   repository never fires them. The check is a strict allowlist: `commitDirs` places the line only when every
   command of it is `git [-C dir]... add|commit` and bash expands nothing in it (`placeable`); each commit folder is
   the cwd after its -C options joined without cleaning, so its real path and `hook.Step.Foreign` resolve a `..` after a
   link as git does. The hook skips only when every folder is outside the root, symlinks resolved, and inside
   another existing repository (`hook.Step.Foreign`, a linked worktree of this one excluded). Anything else is a commit
   of this repository. The other shortcuts of `guardrails` (`protected_branches`, `format_on_edit`,
   `run_before_done`) reach the hook already expanded into entries (`workflow.Entry.From` names the shortcut),
   before those written in `on`.
4. On `start`, the session context (`workflow.Workflow.SessionContext`) is injected as one context first,
   so the contexts of the entries come after it: the template `templates/session_start.md.tmpl` rendered with the
   switches of `guidance` turned on in polix.yaml (commit, edit, layout) and the folders of `rules.path`,
   `plans.path` and `reports.path`; nothing when every switch is off.
5. The entries of the event run in order: one runs when every condition of `when` holds
   and, with `unless`, not every condition of `unless` holds, and carries out the verb of its action
   (`workflow.Action.Verb`). The condition `path` ignores case where file names ignore it (`anyPathFold`), as the
   protection of the state and `writes` do. `block`
   blocks with its message; `run` runs a command or the checks of the rules (`changed`, or a path), and a failure,
   its `message`, a blank line and the output, blocks the event or, on `start`, `post_edit` and
   `post_command` (`workflow.Informed`), reaches the AI as context and is logged as `inform`; `inject` injects a
   text, a file or the rules of a path.
6. Repeat rules, on the observed lines of the log (`state.KindObserved`, read with `state.Query`). A verb that does
   not act because of them is logged as a `skip` decision of its entry and never blocks.
   - `block` blocks every time its entry applies. With `once: true` it is a reminder: it is skipped when the latest
     `blocked` line of its session, actor and line on exactly the current branch (`state.Query.ByBranch`: on a
     detached HEAD, whose branch is empty, the lines of a branch never count) has a tree whose files in the scope of
     the entry did not change (see rule 8), and it writes `blocked`, with the hash, when it blocks. So the first
     attempt of an actor in a session on a content is blocked and the next one goes through; another actor, a new
     session and an edit inside the scope are reminded again, an edit outside it is not.
   - `run` acts once by default: it is skipped when its key passed on exactly the current branch and content hash,
     whoever's entry ran it. The key is what it runs: the expanded arguments joined by single spaces, or `rules` and the
     expanded target, so a `{path}` repeats it per path. Any run of polix that passes, with or without `always`,
     writes `passed` with its key when the hash before and after it is the same; a run during which the content
     changed, or with an unknown hash, writes none, and the hash after it is computed only when the one before it is
     known. With `always: true` it runs on every event.
   - `inject` acts once by default, except on `start`, which always injects: keyed by the session, the
     actor, the line of its entry and the expanded text or file path (one key per rule id for `inject: { rules }`),
     it is skipped while not due and writes `injected`, without hash, when it reaches the AI: the lines are written
     once the answer of the event is known, in the order of the injects, and not at all when the answer to a `prompt`
     refuses, since the AI never sees the contexts of a rejected prompt (on the other events they reach the AI with
     the block, and are written). It is due when it never reached the actor in the session, or when after it a
     `started` of the session or, for a subagent, a `finished` of its type in the session was written: both start a
     new context. A placeholder that changes on every event (a measure, `{command}`) makes a new key each time, so the
     inject acts every time; `{path}` gives once per path.
     With `always: true` it injects on every event and writes nothing.
   - On `start`, after the entries, polix writes `started` when the workflow has a once-only inject or
     `requirements` (`workflow.Workflow.ObservesStart`); the end of a subagent is written only for a once-only inject
     (`workflow.Workflow.InjectsOnce`), since the context of the requirements never goes to a subagent.
   - When the log cannot be read, every verb acts every time, `finished` never holds and the call warns: `polix
     cannot use its log (<cause>): run, inject and the blocks with once act every time, and finished does not hold`.
7. One stop rule, whatever the verb: the first entry that actually blocks, a `block` or a failing `run` on an event
   where it blocks, ends the evaluation. The entries after it never run, so a `stop` answers with one failure
   at a time, in the order of the file, and the next one reruns only the runs that did not pass; the contexts of the
   entries before it reach the agent with the block. A block let through by the loop guard, an `inject`, a skipped
   verb, a passing `run` and a failing `run` on the events where it only informs never end it.
8. `finished: <subagent>` (or a list, all of them) holds when the latest observed `finished` line of that type on
   exactly the current branch, with a hash, has a tree whose paths that differ from the current content
   (`hook.Step.Compare`, answered by `TreeDiff`) are all outside the scope of the entry: the globs of the nearest
   `changed`, else of the `changed` of its `when`, else the whole tree, where any differing path counts. Equal trees
   need no comparison. The log is read first: without such a line the content hash is not computed. A log that
   cannot be read (with the warning of rule 6) and an unknown hash never hold; a tree git no longer has or a
   comparison that fails does not hold either, with a `warn` decision that names the error. A `block` with `once` compares the tree of its latest `blocked` line the same way,
   on the scope of the `changed` of its `when`, and blocks again when the comparison fails.
9. On `subagent_stop`, after the entries, when the answer does not refuse and no block was let through by the loop
   guard, polix writes an observed line `finished`, the key the type of the subagent: with the content hash when a
   `finished` of the workflow names the type (`workflow.Workflow.ObservedSubagents`), so the condition can hold;
   without it, and without computing it, for any subagent when the workflow has a once-only inject, so its injects
   are due again. A type both observed and reset gets one line, with the hash. Without a known hash the `finished`
   condition gets nothing, with a `warn` decision, and the line goes without hash only when the workflow injects
   once. A stop the loop guard let through did not end as asked: it writes `finished` without hash, and only when
   the workflow injects once, so the next run of the subagent gets its injects again but the `finished` condition,
   which ignores the lines without hash, never holds for it. Commands of the AI are never observed and never count as
   passed.
10. Loop guard: every block of `stop` or `subagent_stop`, a `block` or a failing `run`, writes an observed
    `blocked` line with the content hash, and the guard counts those lines per session, event, actor, entry and
    whole hash; an unknown hash is written as `state.UnknownHash`, so those blocks count together. After
    `max_stop_retries` the agent may stop with a warning and the next entries run, and when the log cannot be read or
    written the guard fails open.
11. The content hash is computed only when something needs it: the repeat of a `run` and its `passed` line (before
    and after the run), a `block` with `once`, the loop guard, `finished` and its observed line. An `inject`, the
    `injected` and `started` lines and a `finished` that only resets injects never compute it. When it cannot be
    computed nothing that needs it is observed nor holds, nothing is skipped, and the call warns once: `polix
    cannot hash the content (<cause>): nothing observed is recorded, finished does not hold, and run and the blocks
    with once act every time`.
12. The measures `changed_lines`, `changed_files` and `risk_score` compare the lines, the files or the risk score
    (`diff.Diff.Lines`, `Count` and `Risk` with `risk.weights`) of the diff against their number. They measure
    the files of the nearest `changed`: the one of their own map, else the one of the enclosing map (`any` and
    `not` nest maps), else the whole diff. `any` holds when one of the conditions of its map holds. The diff is read
    once per call (`hook.Facts.Diff`, `hook.Facts.HasDiff`), only when a condition or a string needs it. When it
    cannot be measured (`hook.Facts.DiffErr`), the three measures hold, so a broken git never skips a review, and
    the call warns once: `polix cannot measure the diff (<cause>): changed_lines, changed_files and risk_score hold`.
13. The placeholders `{changed_lines}`, `{changed_files}` and `{risk_score}` of every string of an entry expand to
    the measures over the `changed` of its `when`, else the whole diff, or to `?` when it cannot be measured
    (`measureValues`).
14. On `prompt`, before the entries, when the workflow declares `requirements` (`workflow.Workflow.Requirements`),
    the programs are looked up once per call (`hook.Step.Programs`, answered by `Missing`) and, when some are not on
    the PATH, the AI gets one context: `These programs that polix.yaml requires are not on the PATH: <names>. Before
    doing anything else, install them, or ask the user to install them if you cannot.` It is a once-only inject of
    rule 6 at line 0, keyed by `requirements: ` and the missing names joined by `, `: it is due again after a
    `started` of the session (`ObservesStart` holds with requirements, so `start` writes it) or when the set of missing
    programs changes, its skip is logged as a `skip` decision of the action `requirements`, and a refused prompt does
    not write it. It never blocks. `start` never looks the programs up; without `requirements` nothing is looked up,
    and with every program found nothing is told.

The engine holds the protections and the heuristics of the command lines (`lineWrites`, `namesState`, `within`,
`anyPathFold`, `forbidden`, `matchingCommands`, `commitDirs`), the placeholders (`expandPlaceholders`,
`measureValues`, `branchVars`) and the measures (`measureHolds`). The domain decides the rest: `agent.Answer.Merge`,
`diff.Diff`, `state.State` (`Observed`, `Latest`, `Index`) with `state.Query`, `workflow.Workflow.ObservedSubagents`
and `InjectsOnce`, `rule.Rules` (`Matching` and `Plan`), `rule.Results.Failed`, `rule.Result.Tail` and
`rule.Run.Settle`.

## Response

`Response`, the answers of the events of the call merged (without repeating a text) and rendered for
the native event of the agent: `Stdout`, the JSON with the `additionalContext` of the contexts on the events that
take it; `Stderr`, the reasons and contexts of a block, else one `polix: <warning>` line per warning; and `Code`,
2 for a block and 0 otherwise. The command writes them as they are and exits with `Code`.

## Port

`Port`, declared in `in.go` and implemented by `Adapter` in
`out.go` and, for the log, `out_log.go`. With 20 methods it passes the maximum of 19 of `interfacebloat`, silenced on
its declaration with a `//nolint` and its reason: the hook reads every fact through this one port, and the step shape
needs one method per fact.

- `Open`: opens the repository that contains the folder through `internal/platform/git` and starts the budget of
  10 minutes of every run of the call together.
- `LoadWorkflow`: loads polix.yaml of the root through the field `Workflow` of the adapter, the loader of
  `LoadWorkflow` (`internal/app/workflow/load`) that `cmd/polix` builds once and passes to `Command`: whether it
  exists, and its error as data; without it, no workflow.
- `Locate`: turns the path of an edit into one relative to the root, symlinks resolved.
- `Branch`, `Hash`, `Changed`, `Files` and `Ignored`: the facts of git; `Changed` and `Files` read a failure as no
  files and, like `Branch` and `Ignored`, run detached from the cancellation of the caller.
- `TreeDiff`: the paths that differ between two content hashes (`git.Repo.TreeDiff`, through `git diff-tree`); a
  tree git no longer has, pruned by `git gc`, is its error.
- `Diff`: the added and deleted lines of each file not committed yet (`git.Repo.Stats` against HEAD, untracked
  files read whole), as a validated `diff.Diff`; a failure of git or an invalid diff is its error, kept in the
  facts as a warning. Like `Changed`, it runs detached from the cancellation of the caller.
- `LoadRules`: the rules of the folder it is given (`Workflow.RulesPath`: `rules.path`, `ai/rules/` by default),
  `retired.yaml` left out, ordered by id, invalid ones included, or the error of a file that cannot be read or is
  not yaml (`rule.Loaded.Err`), through the field `Rules` of the adapter, the loader of `LoadRule`
  (`internal/app/rule/load`) that `cmd/polix` builds once and passes to `Command`.
- `LoadState`: the log through the field `State` of the adapter, the loader of `LoadState`
  (`internal/app/state/load`) that `cmd/polix` builds once and passes to `Command`, which `polix log` shares: read
  through `internal/platform/jsonl`, the lines over `state.MaxLine` skipped and counted, every line that decodes
  kept; a log that cannot be read is its error.
- `Real`: the real path of each path.
- `Foreign`: whether a folder is inside an existing git repository other than this one (`git.Repo.Foreign`: its git
  common directory, objects and refs are each another file than ours by `os.SameFile`, so a linked worktree of this
  one or a repository linking into ours is not); an error reads as not, so it runs detached from the cancellation
  of the caller.
- `Missing`: the programs of those it is given that the PATH does not run, in their order, through the field `Look`
  of the adapter, `exec.LookPath`, which `Command` sets: the PATH the hooks give polix, as a command of `run` is
  found; on Windows through the extensions of `PATHEXT`, so `go` finds `go.exe`. A failed lookup is a missing
  program, never an error, and a program found only in the current folder (`exec.ErrDot`) is missing.
- `Contains`: whether a file git does not ignore, matching the patterns of a `file` condition, holds the text of
  its `contains`; one step per search and epoch, the contents never leave the adapter.
- `Read`: the content of a file of the repository.
- `Run`: runs a command from the root through `internal/platform/proc`, within the budget of the call.
- `RunCheck`: runs one invocation from the root, which may decide several rules, with 5 minutes and within the
  budget: the argv of its runner or, when empty, the check of its only rule as a program (on Windows through its interpreter by extension). On Windows a runner whose
  program is `bash`, `sh`, `python`, `python3` or `py` runs the program of that name the PATH finds (an active
  virtualenv wins), unless it is an alias of the Store or, for `bash` and `sh`, the bash of WSL in the Windows folder;
  then the interpreter of its kind (`proc.Command`), with the arguments of the runner kept.
- `Append`: appends an entry to the log, stamped with the time (RFC 3339), encoded as one line by the unexported
  method `encodeLine` with the encoding type `logLine` of `out_log.go`, its reason cut to `state.MaxReason`: the
  shape `LoadState` decodes.

## Errors

The failure policy of the hook, so a broken polix never traps the agent:

- An unknown agent exits with 1.
- An input that cannot be read, or that the format of the agent cannot parse (`claude hook input: <cause>`,
  `codex hook input: <cause>`), prints `polix hook: <error>` and lets the action go on (exit 0).
- Outside a git repository, or without events (a `Read`), everything goes through.
- A workflow that cannot be used is data of the answer (Logic, rule 2), never an error.
- The errors of the steps are data too: an unknown hash warns, a diff that cannot be measured warns and makes the
  measures hold, two trees that cannot be compared make `finished` not hold and a `block` with `once` block again,
  rules that cannot be loaded fail the `run` that needs them (`The rules cannot be loaded`), a log that cannot be
  used lets a stop through and makes every verb act every time, a file
  that cannot be read gives no context, a failing command or check blocks with its output, or informs on the
  events that already happened.

## Flow

1. `Command` checks the agent, reads stdin into a `Request` and injects an `Adapter` with the loaders of the rules,
   of polix.yaml and of the log it was given and the lookup of programs
   (`Adapter{Rules: rules, Workflow: loadWorkflow, State: loadState, Look: exec.LookPath}`) as the `Port`.
2. `run` (shell) chooses the `format` of the agent of the request (`claude` or `codex`) and parses the input with
   it into a `hook.Hook`; without events it answers nothing.
3. It opens the repository from `hook.Hook.Folder` and loads the workflow through the port; outside a repository it
   answers nothing.
4. For each event, it locates the path of an edit through the port and builds its `hook.Evaluation` with the facts the
   previous event left, its problems told for the running polix (`hook.Evaluation.Running`); then it loops from one
   call site: it asks `Decide` for the next step, carries it out through the port and feeds the result back to the
   evaluation (`Know*` for a fact, `Note` for an outcome), until the step is none; the unexported functions `carry`
   (the facts) and `act` (the steps that act) of `in.go` carry out each step. Each pass routes the event to its
   family, which walks its protections, whether git ignores an edited path or where the commits of a `pre_commit`
   land, and the entries, in that order (Logic, rules 1 to 5).
5. It merges the answers of the events (`agent.Answer.Merge`) and renders them with the `format` of the agent
   into the `Response`.
6. `Command` writes stdout and stderr and exits with the code.
