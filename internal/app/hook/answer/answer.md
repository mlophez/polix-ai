# AnswerHook

Answers one hook call of Claude Code or Codex: it turns the event of the agent into neutral events, evaluates the
protections and the workflow of polix.yaml for each of them, and answers with the exit code and the output the
agent expects (2 blocks the action, context goes to the session, the messages for the user to its `systemMessage`).

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
`pre_command` whose line runs a `git commit` also gives `pre_commit`, decided by `hookInput.commits` on the commands
of `shell.Parse`, nested ones included, whose normalized words start with `git commit`, and by the regular
expression `gitCommit` on a line the parser rejects; the `SubagentStart` of either agent gives one
`subagent_start` whose actor is its `agent_type`), and `render`, which turns an
`agent.Answer` into stdout, stderr and the exit code for the native event. It has one implementation per agent:
`claude` in `in_claude.go` and `codex` in `in_codex.go`, which share the fields of the input both agents write and
the rendering of an answer, in `in.go` and `in_input.go`. Both read `agent_id`, the id of the subagent that acts,
into `agent.Event.AgentID`, only when the actor is a subagent (a `stop` is always of the main session), and the
`source` of `SessionStart` (`startup`, `resume`, `clear`, `compact`) into `agent.Event.Source`, only on `start`; a
missing one is empty, and a value that is not a text (`looseText`) is empty too, without failing the input. On
`stop` and `subagent_stop` they read `background_tasks`, the tasks in flight of the session Claude Code lists, into
`agent.Event.InFlight`, one `agent.Subagent` (its `id` and its `agent_type`) per task of `type` `subagent`,
whatever its `status`, the other tasks left out, and `agent.Event.InFlightKnown`: a missing array, `null`, a value
that is not an array of objects (`backgroundTask`) or a subagent with neither id nor type leaves the list unknown,
without failing the input; an empty array is known and empty. Codex gives no such array, so its list is always
unknown. The Claude format also reads `tool_use_id`, the id Claude Code gives the call
of a tool, the same on `PreToolUse`, `PostToolUse` and `PostToolUseFailure`, into `agent.Event.ToolUse`, which pairs
the snapshot of a command with its `post_command`; the Codex format leaves it empty, since polix knows no such id of
Codex, so its commands pair by the command line.

## Logic

`Decide(ev hook.Evaluation) Output` decides the next step of the evaluation of one event. It has the step
shape of `docs/architecture.md`: a later entry of an event sees what an earlier one did (a `run` that rewrites
files changes the content a later `finished` compares), and the first entry that blocks stops the evaluation, so the
logic cannot plan the event at once.

`hook.Evaluation` (domain) holds the workflow, whether polix.yaml exists (`Present`), why it cannot be used
(`Invalid`), the root, the git directory, the system, the event, how many events of the call come after it
(`Remaining`), the `Facts` of the repository read so far and the
`Journal`, the outcomes of the steps that acted on this event in order. The logic walks the event from its start
every time it is asked: a fact it knows is used, a fact it lacks is the next step; a step that acts takes its
outcome from the journal, in order, and the first one the journal lacks is the next step. `Output.Step`
is that step; when it is none, `Output.Answer` is the answer to the event and `Output.Facts`
what the next event of the call starts from.

`Decide` is a router: it builds the pass of the event (`pass`), and `route`, a `switch` on the name of the event in
`logic.go`, gives the family that answers it, a method of `pass` in a file of its own. Each family reads top-down,
holds only what its events do and calls the engine as a library:

- `session` (`logic_session.go`), for `start`, `prompt` and `subagent_start`: the session context of `start` (rule
  4), the `started` line of `start` and `subagent_start` (rule 6), the context of the missing programs of
  `requirements` on `prompt` (rule 14), and the discard of the once-only injects of a refused `prompt` (rule 6).
  `subagent_start` gets neither the session context nor the requirements, which are told to the main session.
- `edit` (`logic_edit.go`), for `pre_edit` and `post_edit`: the protections of an edit (rule 1), but for an edit a
  command made (rule 16), the edits an unusable workflow lets through (rule 2) and the skip of the edits outside the
  change (rule 3).
- `command` (`logic_command.go`), for `pre_command`, `post_command` and `pre_commit`: the protections of a command
  (rule 1), the commands of polix an unusable workflow lets through (rule 2), the snapshot of the last event
  before a command line runs and the watch of a `post_command` (rule 16) and the commits outside the change of a
  `pre_commit` (rule 3).
- `stop` (`logic_stop.go`), for `stop` and `subagent_stop`: the reconciliation of the log with the subagents in
  flight before the entries (rule 17), the loop guard of `max_stop_retries` (rule 10) and the `finished` line of
  `subagent_stop` (rule 9).

The engine is the rest, in the `logic_<topic>.go` files, and has no `if` on the name of an event: the step machinery
and the readers of the facts (`logic_facts.go`), the protections (`logic_protect.go`), the walk of the entries and
their verbs (`logic_entries.go`, `logic_run.go`, `logic_inject.go`, `logic_notify.go`), the conditions and the placeholders
(`logic_condition.go`, `logic_placeholder.go`), the commands of a command line, what they write and where their
commits land (`logic_cmdline.go`, `logic_writes.go`, `logic_commit.go`), the files a command changed (`logic_watch.go`), and
the log (`logic_log.go`). What differs per event reaches it as fields of the pass: `informed`, set by `Decide` for the
events of `workflow.Informed` and for an edit a command made, turns a failing `run` into context; `byCommand`, set by
`Decide` for an edit a command made (`agent.Event.ByCommand`), turns a `block` into context too; `always`, set by
`session` on `start` and `subagent_start`, makes every inject act every time; and `loop`, set by `stop`, sends every
block through the loop guard.

A command line is read by `shell.Parse` (`internal/platform/shell`), given whether the system is Windows: it parses
the line as bash with `mvdan.cc/sh` into `shell.Command`s, each with its text as written, its words with quotes
removed, the targets of its redirections that write and whether its failure propagates to the line. The commands
nested in it come too, marked with their nesting (`shell.Nesting`): those of subshells, blocks and compound commands,
of substitutions (in the words, the redirections, the list of `for` and the word and patterns of `case`), and of the
literal scripts of `sh -c`, `bash -c`, `zsh -c` and `eval`, past the wrappers (`sudo bash -c`), and of a
here-document (its body as written, expansions included) or a literal here-string given to a shell reading stdin
(`bash <<'EOF'`, `bash - <<EOF`, `sh -s <<< '...'`), directly or through a pipe (`cat <<'EOF' | bash`), parsed
again; a
clause (`if`, `while`, `for`, `case`, a function) and a test (`[[ ]]`) are commands of their own without words;
`command`,
`forbidden_commands`, `writes` and `protected_files` see them all, and the `exit` condition counts a nested command
only when it propagates (the last one of a `-c` script that propagates, never one of a substitution nor of a negated
statement such as `! just test`). A pattern of
`command` and of `forbidden_commands` matches a command as written or its normalized text
(`shell.Command.NormalText`: without leading assignments, the wrappers with their options and the global options of
git, `matchingCommands`), and `program` starts from the normalized words, `git rm` and `git mv` read as `rm` and
`mv`. A line the
parser rejects (an unclosed quote, PowerShell) is read by the heuristic of the package instead, which splits it on its
operators and parses each command with its quotes, as polix did before the parser.

A `hook.Step` asks for one of: the branch and the HEAD commit (`hook.Step.Branch`), what git moves without
moving HEAD (`Refs`: the commit of `refs/stash` and an operation stopped halfway), the content hash (of an epoch),
the paths that differ between the tree of an observed line and the current one and those of them the current one no
longer holds (`Compare` and `Tree`, kept in the facts per pair of trees), the changed files,
the diff (`Diff`: the
added and deleted lines of each file not committed yet, the same files as the changed ones), the files, the rules, the
log, whether git ignores a path, the real paths of some paths, which programs the PATH does not run (`Programs`, kept
in the facts as `Missing` once per call), the content of a file (of an epoch), or acts: runs
a command, runs the checks of one or several rules (`rule.Run`, with the argv of its runner of `rules.run`), puts paths
back as a tree holds them (`Restore` and `Source`) or appends an entry to the log. A check step may decide several
rules: the runs come from `rule.Rules.Plan`, and a failed
shared run is rerun once per rule (`rule.Run.Settle`), each rerun a step of its own, so the block names only the
rules whose own run fails. The epoch is the number of runs carried out before, a restore counting as one: any run may
rewrite files, so the hash and the files read after it are those of a new epoch, read again.

The rules it follows, as before this use case:

1. Protections, before the entries of the workflow and before the skip of the edits and commits outside the change
   (rule 3); the first one that denies ends the evaluation. Whatever the workflow says and also without one or with
   an invalid one, nobody edits the state of polix (`<git-dir>/polix`, by its real path, without case where file
   names ignore it) nor runs a command that may write and names it, logged as `builtin`: the line may write when
   `writesFiles` matches its text without its redirections into `/dev/null` (`withoutNullRedirects`; not `NUL`,
   which under Git Bash may create a file), on any line, or when a command of the parsed line, nested ones
   included, redirects into a file other than `/dev/null`, has a word that writes by itself with the words after it (`writesItself`: `rm` of `find -exec rm` or `sudo -u x rm`
   counts), has a word naming one of the shells or interpreters polix knows, with any version (`interpreters`:
   `bash`, `pwsh`, `perl`, `python3.12`..., without case), runs `.`, or runs a program whose name is not literal
   (`unknownProgram`: `$SHELL -c ...`) (`mayWrite`); the state is named anywhere in the text of the line
   (`namesState`). With a valid polix.yaml, no actor, the main session included, writes a path of
   `guardrails.protected_files` (`workflow.Workflow.ProtectedFiles`, matched
   with `anyPathFold`), also when git ignores it but never on an edit outside the repository, by an edit, a
   redirection or a program that writes the files it names (`rm`, `mv`, `cp`, `sed -i`...), its folders and
   wildcards included (`lineWrites`): the reason names the path (`<path> is protected in polix.yaml: only a person
   changes it.`) or the protected glob a folder or a wildcard holds (`<folder> holds <glob>, protected in
   polix.yaml: ...`), and it is logged as `guardrails.protected_files`. Reading a protected path passes. Nor does any
   actor run a command of `guardrails.forbidden_commands` (`workflow.Workflow.ForbiddenCommands`, the first in file
   order whose pattern matches any command of the line, `forbidden`) on `pre_command` or `pre_commit`: the reason
   is its `reason`, or `reasonForbidden`, followed by the line, and it is logged as `guardrails.forbidden_commands`
   with the line of its item. An edit a command made (rule 16) gets none of the protections of an edit: its
   `post_command` already put the protected files back, and the state of polix lives in the git directory, which no
   tree holds.
2. A workflow that cannot be used (it cannot be read or parsed, has problems or requires a newer polix) blocks
   edits and commands with its problems, logged as `invalid`, except an edit outside the change, an edit of
   polix.yaml by the main session (not when it requires a newer polix) and the commands that only check the
   workflow or update polix (`onlyPolix`: on the line parsed as bash, every command, nested ones included, is one
   of `onlyPolixCommands` as written with no redirection but `2>&1` or `1>&2`, and a clause or a test, a command
   without words, fails closed; on a line the parser rejects, the
   text check of before); any other event only warns, so a stop is never blocked. Problems that may come from a
   newer polix hint to run `polix update` (`hook.Evaluation.Running`, which the shell asks); a name of the old
   vocabulary, reported with its new spelling, does not.
3. Without polix.yaml, for an edit outside the repository or of a file git ignores, and for a `pre_commit` whose
   commits all happen in another repository (`git -C ../other commit`), only the protections apply, and for that
   `pre_commit` the `snapshot` of rule 16, since the rest of its line may still change this repository: the
   entries govern the change, what a commit may hold, so a build output, a local `.env` or a commit in another
   repository never fires them. The check is a strict allowlist: `commitDirs` places the line only when every
   command of it is `git [-C dir]... add|commit` and bash expands nothing in it: on the line parsed as bash, no
   command is nested, has a redirection or a word that is not literal (`plain`), while a comment is no word; on a
   line the parser rejects, the text holds none of the characters that expand (`placeable`); each commit folder is
   the cwd after its -C options joined without cleaning, so its real path and `hook.Step.Foreign` resolve a `..` after a
   link as git does. The hook skips only when every folder is outside the root, symlinks resolved, and inside
   another existing repository (`hook.Step.Foreign`, a linked worktree of this one excluded). Anything else is a commit
   of this repository. The other shortcuts of `guardrails` (`protected_branches`, `format_on_edit`,
   `run_on_stop`) reach the hook already expanded into entries (`workflow.Entry.From` names the shortcut),
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
   its `message`, a blank line and the output, blocks the event or, on `start`, `subagent_start`, `post_edit` and
   `post_command` (`workflow.Informed`), reaches the AI as context and is logged as `inform`; `inject` injects a
   text, a file or the rules of a path; `notify` adds its expanded message to the messages for the user (rule 15),
   never to the AI, logged as a `notify` decision with the text, and a message that expands to nothing shows
   nothing. On an edit a command made (rule 16), which already happened, a `block` and a failing `run` reach the AI
   as context, logged as `inform`, and the evaluation goes on.
6. Repeat rules, on the observed lines of the log of the session of the event (`state.KindObserved`, read with
   `state.Query`): the state the hook reads is the file of that session alone, its subagents included, so a line
   another session wrote never counts for any verb nor condition, and a new session starts from an empty state
   (`claude --resume` keeps the session, and its state). An event without a session reads no state, as a log that
   cannot be read (see below), writes no line and warns as that log does, whether it reads the state or not
   (`Decide`, with the error of `state.Session.File`). A verb that does not act because of them is logged as a `skip`
   decision of its entry and never blocks. The three readers of the lines by branch, the reminder of a `block` with
   `once`, the repeat of a `run` and `finished` (rule 8), select, within the session, the lines of the current branch
   or of the same HEAD commit (`state.Query.ByBranch` with `state.Query.Head`): a branch made from the commit the
   line was written on, as `git checkout -b` gives, reads it too, and on the same branch a line keeps counting after
   a commit moves HEAD. A line without HEAD commit (written by an older polix)
   or an unknown current one (a repository without commits, git failing) matches only by branch; on a detached
   HEAD, whose branch is empty, the lines written on a detached HEAD count, whatever their commit, and so do the
   lines of a branch on the same commit. The lines about a content they read,
   `finished` with a hash, `passed` and `blocked`, carry the HEAD commit (`state.Entry.Head`) when it is known, and so
   does a `snapshot` (rule 16), whose `post_command` compares it, with what git moves without HEAD
   (`state.Entry.Refs`) too; the decisions and the other observed lines never do. Every line of an event of a
   subagent, decisions included, carries its id (`state.Entry.AgentID`) when the agent gives one.
   - `block` blocks every time its entry applies. With `once: true` it is a reminder: it is skipped when the latest
     `blocked` line of its session, actor and line on the current branch or the same HEAD commit has a tree whose
     files in the scope of the entry did not change (see rule 8), and it writes `blocked`, with the hash, when it
     blocks. So the first attempt of an actor in a session on a content is blocked and the next one goes through;
     another actor, a new session and an edit inside the scope are reminded again, an edit outside it is not.
   - `run` acts once by default: it is skipped when its key passed in the session, on the current branch or the same
     HEAD commit, and on the content hash, whoever's entry ran it. The key is what it runs: the expanded arguments
     joined by single spaces, or `rules` and the expanded target, so a `{path}` repeats it per path. Any run of polix that passes, with or without `always`,
     writes `passed` with its key when the hash before and after it is the same; a run during which the content
     changed, or with an unknown hash, writes none, and the hash after it is computed only when the one before it is
     known. With `always: true` it runs on every event.
   - `inject` acts once by default, except on `start` and `subagent_start`, which begin a new context and always
     inject: keyed by the session, the
     actor, the line of its entry and the expanded text or file path (one key per rule id for an `inject` of `rules`),
     it is skipped while not due and writes `injected`, without hash, when it reaches the AI: the lines are written
     once the answer of the event is known, in the order of the injects, and not at all when the answer to a `prompt`
     refuses, since the AI never sees the contexts of a rejected prompt (on the other events they reach the AI with
     the block, and are written). It is due when it never reached the actor in the session, or when after it a
     `started` of the session (written on `start`, which resets every actor) or, for a subagent, a `started` of its
     type (written on its `subagent_start`) or a `finished` of its type in the session was written: each starts a new
     context. The `started` of a subagent resets only its type: the main session and the other types keep theirs. A placeholder that changes on every event (a measure, `{command}`) makes a new key each time, so the
     inject acts every time; `{path}` gives once per path.
     With `always: true` it injects on every event and writes nothing.
   - On `start` and `subagent_start`, after the entries, polix writes `started`, with the actor of the event (the
     subagent type on `subagent_start`, with its id), when the workflow has a once-only inject, `requirements` or an
     `agents_are_running` (`workflow.Workflow.ObservesStart`); the `started` of `start` has the `source` of the start
     as its key, empty when the agent gives none. The end of a subagent is written only for a once-only inject
     (`workflow.Workflow.InjectsOnce`, which skips the injects of `start` and `subagent_start`) or an
     `agents_are_running` (`workflow.Workflow.CountsAgents`, rule 17), since the context of the requirements never
     goes to a subagent.
   - `notify` acts once by default: keyed by the session, the line of its entry and the expanded text, whatever the
     actor, it is skipped, with a `skip` decision, when the log has a `notified` line of that key, or an earlier
     event of the call already showed it (`passNotice`, `notified`). Nothing makes it due again in the session: a
     `started`, a `finished` and another actor never do, since the user is one person. A placeholder that changes its
     text makes a new key. The `notified` lines, without hash, are written only when the whole call does not block
     (`settle`, called by `Decide` once the family answered): the events of one call give one answer, and a blocked
     one shows the user no message (rule 15). So each event that does not block keeps its notices, after those of the
     earlier events, as pending lines (`hook.Facts.Pending`, carried with `hook.Facts.Answered`), and the last event
     of the call (`hook.Evaluation.Last`, from `hook.Evaluation.Remaining`, which the shell sets with
     `hook.Hook.Remaining`) appends them all, in the order they were shown. A block of any event, earlier
     (`hook.Facts.Blocked`) or later (`hook.Facts.Answered`), drops them: the notices stay due and their messages are
     logged as not shown. A line that cannot be written leaves its notice due again. With `always: true` it shows
     every time and writes nothing. The once-only injects keep their own rule above: their contexts reach the agent
     with a block, on stderr, except on a refused `prompt`, which is a call of one event.
   - When the log cannot be read, every verb acts every time, `finished` never holds and the call warns: `polix
     cannot use its log (<cause>): run, inject, notify and the blocks with once act every time, and finished does not
     hold`.
7. One stop rule, whatever the verb: the first entry that actually blocks, a `block` or a failing `run` on an event
   where it blocks, ends the evaluation. The entries after it never run, so a `stop` answers with one failure
   at a time, in the order of the file, and the next one reruns only the runs that did not pass; the contexts of the
   entries before it reach the agent with the block. A block let through by the loop guard, an `inject`, a `notify`,
   a skipped verb, a passing `run` and a failing `run` on the events where it only informs never end it.
8. `finished: <subagent>` (or a list, all of them) holds when the latest observed `finished` line of that type in
   the session, on the current branch or the same HEAD commit (rule 6), with a hash, has a tree whose paths that
   differ from the current content (`hook.Step.Compare`, answered by `TreeDiff`) are all outside the scope of the entry: the globs
   of the nearest `changed`, else of the `changed` of its `when`, else the whole tree, where any differing path counts. Equal trees
   need no comparison. The log is read first: without such a line the content hash is not computed. A log that
   cannot be read (with the warning of rule 6) and an unknown hash never hold; a tree git no longer has or a
   comparison that fails does not hold either, with a `warn` decision that names the error. A `block` with `once` compares the tree of its latest `blocked` line the same way,
   on the scope of the `changed` of its `when`, and blocks again when the comparison fails.
9. On `subagent_stop`, after the entries, when the answer does not refuse and no block was let through by the loop
   guard, polix writes an observed line `finished`, the key the type of the subagent: with the content hash when a
   `finished` of the workflow names the type (`workflow.Workflow.ObservedSubagents`), so the condition can hold;
   without it, and without computing it, for any subagent when the workflow has a once-only inject, so its injects
   are due again, or an `agents_are_running`, so it no longer counts as running (rule 17). A type both observed and
   reset gets one line, with the hash. Without a known hash the `finished` condition gets nothing, with a `warn`
   decision, and the line goes without hash only when the workflow injects once or uses `agents_are_running`. A stop
   the loop guard let through did not end as asked: it writes `finished` without hash, and only when the workflow
   injects once or uses `agents_are_running`, so the next run of the subagent gets its injects again and it no longer
   counts as running, but the `finished` condition, which ignores the lines without hash, never holds for it. The
   line carries the id of the subagent. A refused `subagent_stop` writes nothing: the subagent goes on. Commands of
   the AI are never observed and never count as passed.
10. Loop guard: every block of `stop` or `subagent_stop`, a `block` or a failing `run`, writes an observed
    `blocked` line with the content hash, and the guard counts those lines per session, event, actor, entry and
    whole hash; an unknown hash is written as `state.UnknownHash`, so those blocks count together. A block after
    runs of the event that changed the content also writes `before`, the hash of epoch 0, the content the agent left,
    and the guard counts the chain of those lines (`state.State.Chain`): a block whose stop started from the content
    a previous one ended on counts with it, so a run that writes files the tree does not ignore, the blocking one or
    an earlier one, is still bounded while the agent changes nothing. When those runs changed
    the content, a block let through writes its line too, once the entries of the event are done, with the hash they
    ended on (`writeReleased`), so the chain goes on even when a later run writes again, and the warning names
    `max_stop_retries`. After `max_stop_retries` the agent may
    stop with a warning and the next entries run, and when the log cannot be read or written the guard fails open.
11. The content hash is computed only when something needs it: the repeat of a `run` and its `passed` line (before
    and after the run), a `block` with `once`, the loop guard, `finished` and its observed line, and, when the
    workflow watches, the `snapshot` of the last event before a command line runs (the `pre_command`, or the
    `pre_commit` of a line that commits) and the watch of a `post_command` (rule 16). An `inject`, a
    `notify`, the `injected`, `started`, `notified` and `ended` lines and a `finished` without hash never compute
    it. When it cannot be
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
    its programs followed by git (`workflow.Workflow.Programs`) are looked up once per call (`hook.Step.Programs`,
    answered by `Missing`) and, when some are not on the PATH, the AI gets one context: `These programs that polix.yaml requires are not on the PATH: <names>. Before
    doing anything else, tell the user which ones are missing and propose the commands that would install them;
    install nothing without the user's approval.` It is a once-only inject of
    rule 6 at line 0, keyed by `requirements: ` and the missing names joined by `, `: it is due again after a
    `started` of the session (`ObservesStart` holds with requirements, so `start` writes it) or when the set of missing
    programs changes, its skip is logged as a `skip` decision of the action `requirements`, and a refused prompt does
    not write it. It never blocks. `start` never looks the programs up; without `requirements` nothing is looked up,
    and with every program found nothing is told.
15. The messages for the user (`agent.Answer.Warnings`: the notices of `notify`, the warnings of the pass and of the
    call, such as a log that cannot be read or an unknown hash, the warning of the loop guard and that of a workflow
    that cannot be used)
    are added to the answer at the end of each family, a denial of a protection and a refusal of a workflow that
    cannot be used included (`warn`). Each one goes without the prefix `polix: `, which the render adds once.
    Neither agent reads the output of a blocked event but its reasons, so on a blocked answer they are not shown:
    `unshown` logs each one as a `skip` decision of the action `message`, at line 0, that says the event was blocked
    and the message was not shown to the user. The answers of the events of one call are merged into one, so a block
    of any of them hides the messages of all: `Decide` hands the answer to the facts of the next event
    (`hook.Facts.Answered`), where the messages of an answer that does not block become warnings of the call and a
    block sets `hook.Facts.Blocked`. A later event that blocks logs the messages of the earlier ones, and one that
    does not block logs its own after an earlier block; a warning of the call may then be logged once per event.
16. The files a command changed (`logic_watch.go`), found by their effect rather than by the command line. The
    workflow watches (`watches`) when it is valid and has `guardrails.protected_files` or an entry on `pre_edit` or
    `post_edit`, `format_on_edit` already expanded into them; otherwise nothing below happens and no hash is
    computed for it. The key of a command (`commandKey`) is `agent.Event.ToolUse`, else the command line.
    - `snapshot`: the last event of the call before a command line runs (`lastBefore`, `hook.Evaluation.Last`),
      the `pre_command` or, on a line that commits, its `pre_commit`, writes, when its protections passed and
      neither it nor an earlier event of the call (`hook.Facts.Blocked`) blocked, after its entries, an observed
      `snapshot` line with the content hash of the epoch they left (so a `run` of them that
      rewrites files is not taken for the command), the HEAD commit, the refs git moves without HEAD and the key; an
      unknown hash writes none, with the warning of rule 11.
    - `watch`: a `post_command`, also after a failed command, first reads the latest `snapshot` line of its session,
      actor and key (`state.State.Index`); without one, when an observed `watched` line of the same session, actor and
      key comes after it, or with a log that cannot be read, nothing happens. Otherwise it writes that `watched` line,
      so a snapshot is compared once: a later `post_command` of the same command line whose `pre_command` did not reach
      polix finds no snapshot. When HEAD is another commit than the one of the snapshot (a pull, a checkout, a reset, a
      commit), or the refs git moves without HEAD changed (a `git stash`; a merge, a revert, a cherry-pick, a rebase or
      a `git am` stopped on its conflicts; a merge or a revert with `--no-commit`; a merge with `--squash`; all of which
      rewrite the working tree without moving HEAD), the changes are git's: nothing is restored nor evaluated, and a
      `skip` decision of `builtin` names both values (`gitMoved`). Otherwise it computes the current hash and asks for
      the paths that differ from the tree of the snapshot (`hook.Step.Compare`), and splits them with `anyPathFold` into
      those of `guardrails.protected_files` and the others, each in the order of the comparison.
    - Each protected path goes to a `Restore` step of its own with the tree of the snapshot as `Source`, so each
      gets its own outcome, the paths the command removed last: on a file system that folds case, a path the command
      renamed only in case is removed before its own name is written back. The restores start one new epoch, and the
      command is refused once per path, `<path> is protected in polix.yaml: a command changed it and polix put it
      back.`, each logged as a `deny` of `guardrails.protected_files`. The restore is not bounded.
    - The other paths the current content still holds, a removed one left out, up to `maxCommandEdits` (25), become
      `Output.Follow` (`derive`): a `pre_edit` and then a
      `post_edit` per path, with `agent.Event.ByCommand`, the session, the actor, its agent id, the agent and the cwd of the
      `post_command` and the path located inside the root, which the shell evaluates next in the same call (rules 1
      and 5 say how they differ from an edit of the edit tool). Past 25 paths none is derived: the AI gets one context,
      `A command changed <n> files, more than 25: polix did not evaluate them as edits, so the entries of pre_edit and
      post_edit did not apply to them.`, logged as an `inform` of `builtin`.
    - The entries of the `post_command` run after the watch, its refusals and context merged before theirs.
17. `agents_are_running: true|false` holds when whether subagents of the session other than the one that acts are
    still running is its value, on any event (`agentsRunning`), ranked with `finished` among the conditions. On a
    `stop` or `subagent_stop` whose list of subagents in flight is known (`agent.Event.InFlightKnown`), it counts
    that list without reading the log, the actor left out by its id, or, without one, by its type
    (`othersInFlight`). Otherwise it reads the log: `state.State.Running` walks the observed lines of the session in
    order, a `started` of `subagent_start` opens a subagent, a `finished` or an `ended` closes the one of its id,
    else the oldest open one of its type (without id when the line has one), so the lines without id, of Codex or of
    an older Claude Code or polix, pair by type, and a `started` of `start` whose key is not `compact` (a new session,
    a resume, a clear, or a start without source) closes every one. The actor is left out by its id, else by its type
    (`state.State.RunningOthers`), except on its own `subagent_start`, whose `started` is written after the entries;
    the main session, on `stop` too, counts every subagent. A log that cannot be read counts as no subagent running,
    with the warning of rule 6. polix writes the lines it reads only when the workflow uses the condition
    (`workflow.Workflow.CountsAgents`, rules 6 and 9).
    - Reconciliation (`reconcile`): on `stop` and `subagent_stop`, before the entries, when the workflow uses the
      condition and the list is known, each subagent the log has open in the session that the list does not list,
      the actor aside, gets an observed `ended` line (`state.OutcomeEnded`): about the event, with the type of that
      subagent as the actor and its id, without key nor hash (`writeEnded`). A line with an id is listed by a
      subagent of that id, then each line without id by a subagent of its type not taken yet (`unlisted`). So a
      subagent whose end never reached polix, interrupted, stops counting; without the list, or with one that does
      not decode, the log stays as it is and such a subagent counts until a later stop that lists, or a new session.
      A log that cannot be read reconciles nothing, with the warning of rule 6. The entries never depend on these
      lines: with the list known the condition reads the list.

The engine holds the protections and the heuristics of the command lines (`lineWrites`, `namesState`, `within`,
`anyPathFold`, `forbidden`, `matchingCommands`, `commitDirs`), the watch of the commands (`watches`, `commandKey`,
`maxCommandEdits`), the placeholders (`expandPlaceholders`,
`measureValues`, `branchVars`), the measures (`measureHolds`) and the subagents in flight (`othersInFlight`,
`unlisted`). The domain decides the rest: `agent.Answer.Merge`,
`diff.Diff`, `state.State` (`Observed`, `Latest`, `Index`, `Running`, `RunningOthers`) with `state.Query`,
`workflow.Workflow.ObservedSubagents`, `InjectsOnce` and `CountsAgents`, `rule.Rules` (`Matching` and `Plan`), `rule.Results.Failed`, `rule.Result.Tail` and
`rule.Run.Settle`.

## Response

`Response`, the answers of the events of the call merged (without repeating a text) and rendered for
the native event of the agent: `Stdout`, on exit 0, one JSON object with `systemMessage`, the messages for the user
each as `polix: <message>` joined by new lines, and `hookSpecificOutput`, the `additionalContext` of the contexts
and the `hookEventName`, only on the events that take it; each field is left out when empty, since Codex fails a hook
on a field it does not accept, and stdout is empty with neither. `Stderr`, the reasons and contexts of a block, and
empty otherwise; a block writes nothing to stdout and no message for the user (rule 15). `Code`, 2 for a block and 0
otherwise. The command writes them as they are and exits with `Code`.

## Port

`Port`, declared in `in.go` and implemented by `Adapter` in
`out.go` and, for the log, `out_log.go`. With 22 methods it passes the maximum of 19 of `interfacebloat`, silenced on
its declaration with a `//nolint` and its reason: the hook reads every fact through this one port, and the step shape
needs one method per fact.

- `Open`: opens the repository that contains the folder through `internal/platform/git` and starts the budget of
  10 minutes of every run of the call together.
- `LoadWorkflow`: loads polix.yaml of the root through the field `Workflow` of the adapter, the loader of
  `LoadWorkflow` (`internal/app/workflow/load`) that `cmd/polix` builds once and passes to `Command`: whether it
  exists, and its error as data; without it, no workflow.
- `Locate`: turns the path of an edit into one relative to the root, symlinks resolved.
- `Head`, `Hash`, `Changed`, `Files` and `Ignored`: the facts of git; `Head` gives the current branch, empty on a
  detached HEAD, and the commit HEAD points to, from one `git rev-parse` (`git.Repo.Head`), both empty when git
  fails, as on a repository without commits; `Changed` and `Files` read a failure as no files and, like `Head` and
  `Ignored`, run detached from the cancellation of the caller.
- `Refs`: what git moves without moving HEAD, as one text (`git.Repo.Refs`): the commit `refs/stash` points to,
  through `git rev-parse`, and each operation stopped halfway, by its file in the git directory (`MERGE_HEAD`,
  `CHERRY_PICK_HEAD`, `REVERT_HEAD`, `SQUASH_MSG`, `rebase-merge`, `rebase-apply`) and the commit it names, the
  sha256 of the message for `SQUASH_MSG`; empty when there is neither or git fails. Like `Head`, it runs detached
  from the cancellation of the caller.
- `TreeDiff`: the paths that differ between two content hashes and those of them the second no longer holds
  (`git.Repo.TreeDiff`, through `git diff-tree --name-status`); a tree git no longer has, pruned by `git gc`, is its
  error.
- `Diff`: the added and deleted lines of each file not committed yet (`git.Repo.Stats` against HEAD, untracked
  files read whole), as a validated `diff.Diff`; a failure of git or an invalid diff is its error, kept in the
  facts as a warning. Like `Changed`, it runs detached from the cancellation of the caller.
- `LoadRules`: the rules of the folder it is given (`Workflow.RulesPath`: `rules.path`, `ai/rules/` by default),
  `retired.yaml` left out, ordered by id, invalid ones included, or the error of a file that cannot be read or is
  not yaml (`rule.Loaded.Err`), through the field `Rules` of the adapter, the loader of `LoadRule`
  (`internal/app/rule/load`) that `cmd/polix` builds once and passes to `Command`.
- `LoadState`: the log of the session it is given, the session of the event, which the shell passes: the file
  `<git-dir>/polix/sessions/<name>.jsonl` (`state.DirName`, `state.SessionsDir` and `state.Session.File`: the id as
  it is when made only of `A-Z`, `a-z`, `0-9`, `.`, `_` and `-`, at most 128 characters and not only dots, else its
  lowercase hex sha256), read through the field `State` of the adapter, the loader of `LoadState`
  (`internal/app/state/load`) that `cmd/polix` builds once and passes to `Command`, which `polix log` shares: read
  through `internal/platform/jsonl`, the lines over `state.MaxLine` skipped and counted, every line that decodes
  kept; a missing file is an empty log, a file that cannot be read is its error, and so is a session without an id,
  which names no file. The old `<git-dir>/polix/log.jsonl` is never read.
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
- `Restore`: puts paths back in the working tree as a content hash holds them, removing those it does not hold
  (`git.Repo.Restore`, through a scratch index in the folder of the state, as `Hash`, so the real index of the person
  is never touched); it removes the paths the tree does not hold before writing the others, and its error names
  the first path that failed; a tree git no longer has is an error.
- `Append`: appends an entry to the file of its session (`state.Entry.Session`, named as `LoadState` names it),
  creating it and its folder when missing, stamped with the time (RFC 3339), encoded as one line by the unexported
  method `encodeLine` with the encoding type `logLine` of `out_log.go`, its reason cut to `state.MaxReason`: the
  shape `LoadState` decodes, the id of a subagent as `agent_id`, left out when empty. Each line is one append call,
  without a lock, so the main session and its subagents append to the same file. An entry that is not valid, one
  without a session among them (`state.Entry.Valid`), is not written and is its error. Nothing is written to the old
  `<git-dir>/polix/log.jsonl`, which is left on disk.

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
- The watch of a command (Logic, rule 16) never blocks for its own failures: without a snapshot, with one already
  compared, with an unknown hash or with a log that cannot be read, the `post_command` answers as without it; two
  trees that cannot be compared (a tree pruned by `git gc`, git failing) evaluate nothing and warn: `polix cannot
  compare the content before and after the command (<cause>): it did not check the files it changed`. A path that
  could not be put back is refused with its error, `<path> is protected in polix.yaml: a command changed it and polix
  could not put it back (<cause>).`, logs the `deny` with it and warns, for that path alone: `polix could not put
  back <path>, protected in polix.yaml, after a command changed it (<cause>): put it back by hand`.

## Flow

1. `Command` checks the agent, reads stdin into a `Request` and injects an `Adapter` with the loaders of the rules,
   of polix.yaml and of the log it was given and the lookup of programs
   (`Adapter{Rules: rules, Workflow: loadWorkflow, State: loadState, Look: exec.LookPath}`) as the `Port`.
2. `run` (shell) chooses the `format` of the agent of the request (`claude` or `codex`) and parses the input with
   it into a `hook.Hook`; without events it answers nothing.
3. It opens the repository from `hook.Hook.Folder` and loads the workflow through the port; outside a repository it
   answers nothing.
4. It evaluates the events from a queue, those of the hook first. For each event, it locates the path of an edit
   through the port and builds its `hook.Evaluation` with the facts the previous event left and the events left in
   the queue (`hook.Evaluation.Remaining`), its problems told for the running polix (`hook.Evaluation.Running`); then
   it loops from one call site: it asks `Decide` for the next step, carries it out through the port and feeds the
   result back to the evaluation (`Know*` for a fact, `Note` for an outcome), until the step is none; the unexported
   functions `carry` (the facts of git), `lookup` (the facts of a path, a folder or the programs) and `act` (the
   steps that act, the `Restore` of protected files a command changed among them) of `in.go` carry out each step.
   Each pass routes the event to its family, which walks its protections, whether git ignores an edited path or
   where the commits of a `pre_commit` land, and the entries, in that order (Logic, rules 1 to 5). The events of
   `Output.Follow`, the `pre_edit` and `post_edit` of each file a command changed, join the end of the queue, so
   they are located and evaluated in the same call, their answers merged with the others; they never block, so the
   lines a call writes only when it does not block are written by the last event of the queue.
5. It merges the answers of the events (`agent.Answer.Merge`) and renders them with the `format` of the agent
   into the `Response`.
6. `Command` writes stdout and stderr and exits with the code.
