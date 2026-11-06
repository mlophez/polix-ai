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

The polix plugin registers it in the hooks of the agents; people do not run it.

## Request

`AnswerHookRequest`, filled by the command:

- `Agent`, from the argument, checked to be `claude` or `codex` before the shell runs.
- `Dir`, from `--dir`.
- `Input`, the bytes of stdin as the agent wrote them.
- `Version`, the version of the running polix (`build.Current`), which polix.yaml may require.
- `System`, the system polix runs on (`runtime.GOOS`): on macOS and Windows file names ignore case, and on
  Windows `NUL` is the null device.

## Logic

`AnswerHook(ev Evaluation) AnswerHookOutput` decides the next step of the evaluation of one event. It has the step
shape of `docs/architecture.md`: a later entry of an event sees what an earlier one did (a `record` makes a later
`done` hold, a `run` that rewrites files changes the hash a later `done` is pinned to), and `allow` and a `deny`
that blocks stop the evaluation, so the logic cannot plan the event at once.

`Evaluation` (domain) holds the workflow, whether polix.yaml exists (`Present`), why it cannot be used
(`Invalid`), the root, the git directory, the system, the event, the `Facts` of the repository read so far and the
`Journal`, the outcomes of the steps that acted on this event in order. The logic walks the event from its start
every time it is asked: a fact it knows is used, a fact it lacks is the next step; a step that acts takes its
outcome from the journal, in order, and the first one the journal lacks is the next step. `AnswerHookOutput.Step`
is that step; when it is none, `AnswerHookOutput.Answer` is the answer to the event and `AnswerHookOutput.Facts`
what the next event of the call starts from.

A `Step` asks for one of: the branch, the content hash (of an epoch), the changed files, the files, the rules, the
log, whether git ignores a path, the real paths of some paths, the content of a file (of an epoch), or acts: runs
a command, runs the checks of one or several rules (`rule.Run`, with the argv of its runner of `rules.run`) or
appends an entry to the log. A check step may decide several rules: the runs come from `rule.Plan`, and a failed
shared run is rerun once per rule (`rule.Run.Settle`), each rerun a step of its own, so the block names only the
rules whose own run fails. The epoch is the number of runs carried out before: any run may rewrite files, so the hash and the files
read after it are those of a new epoch, read again.

The rules it follows, as before this use case:

1. Protections, whatever the workflow says and also without one: nobody edits the state of polix
   (`<git-dir>/polix`, by its real path, without case where file names ignore it) nor runs a command that may write
   and names it; no actor, the main session included, writes a path of `protected`, by an edit, a redirection or a
   program that writes the files it names (`rm`, `mv`, `cp`, `sed -i`...), its folders and wildcards included. A
   protection blocks and is logged as `builtin`.
2. A workflow that cannot be used (it cannot be read or parsed, has problems or requires a newer polix) blocks
   edits and commands with its problems, logged as `invalid`, except an edit outside the change, an edit of
   polix.yaml by the main session (not when it requires a newer polix) and the commands that only check the
   workflow or update polix; any other event only warns, so a stop is never blocked. Problems that may come from a
   newer polix hint to run `polix update` (`Unusable`).
3. Without polix.yaml, and for an edit outside the repository or of a file git ignores, only the protections apply.
4. On `session_start`, the texts of the `guidance` turned on in polix.yaml (`workflow.Guidance.Instructions`) are
   injected as contexts first, so the contexts of the entries come after them.
5. The entries of the event run in order: one runs when every condition of `if` holds and, with `unless`, not
   every condition of `unless` holds. `deny` blocks and stops the evaluation; `allow` stops it; `require` blocks
   and goes on; `run` runs a command or the checks of the rules (`changed`, or a path) and blocks when it fails,
   recording its fact only when it passed on a content that did not change; `context` injects a file, a text or
   the rules of a path (each rule once per session); `record` records a fact on the current content.
6. The mode of an entry: `warn` turns a block into a context and a warning, `audit` only logs it.
7. Loop guard: a block of `stop` or `subagent_stop` is counted per session, event, actor, entry and content hash;
   after `max_stop_blocks` the agent may stop with a warning, and when the log cannot be read or written the guard
   fails open.
8. Facts hold only on a known content hash; when it cannot be computed nothing is recorded nor holds, and the
   call warns once.

The domain decides the rest: `agent.Agent.Parse` and `Render` (the formats of the agents), `agent.Answer.Merge`,
`Writes`, `NamesState`, `Within`, `Expand`, `Vars`, `MatchingCommands`, `state.State` (`Done`, `Injected`,
`StopBlocks`), `rule.Matching`, `rule.Result.Tail`, `rule.Plan` and `rule.Run.Settle`.

## Response

`AnswerHookResponse`, the answers of the events of the call merged (without repeating a text) and rendered for
the native event of the agent: `Stdout`, the JSON with the `additionalContext` of the contexts on the events that
take it; `Stderr`, the reasons and contexts of a block, else one `polix: <warning>` line per warning; and `Code`,
2 for a block and 0 otherwise. The command writes them as they are and exits with `Code`.

## Port

`AnswerHookPort`, declared in `answer_hook_in.go` and implemented by `AnswerHookAdapter` in
`answer_hook_out.go`:

- `Open`: opens the repository that contains the folder through `pkg/git` and starts the budget of 10 minutes of
  every run of the call together.
- `LoadWorkflow`: reads polix.yaml with `workflow.Parse`: whether it exists, and its error as data.
- `Locate`: turns the path of an edit into one relative to the root, symlinks resolved.
- `Branch`, `Hash`, `Changed`, `Files` and `Ignored`: the facts of git; `Changed` and `Files` read a failure as no
  files and, like `Branch` and `Ignored`, run detached from the cancellation of the caller.
- `LoadRules`: the rules of `ai/rules/`, `retired.yaml` left out, ordered by id.
- `LoadState`: the log through `pkg/jsonl`, the lines over `state.MaxLine` skipped and counted.
- `Real`: the real path of each path.
- `Contains`: whether a file git does not ignore, matching the patterns of a `file` condition, holds the text of
  its `contains`; one step per search and epoch, the contents never leave the adapter.
- `Read`: the content of a file of the repository.
- `Run`: runs a command from the root through `pkg/proc`, within the budget of the call.
- `RunCheck`: runs one invocation from the root, which may decide several rules, with 5 minutes and within the
  budget: the argv of its runner or, when empty, the check of its only rule as a program (on Windows through its interpreter by extension). On Windows a runner whose
  program is `bash`, `sh`, `python`, `python3` or `py` runs the program of that name the PATH finds (an active
  virtualenv wins), unless it is an alias of the Store or, for `bash` and `sh`, the bash of WSL in the Windows folder;
  then the interpreter of its kind (`proc.Command`), with the arguments of the runner kept.
- `Append`: appends an entry to the log, stamped with the time (RFC 3339), its reason cut to `state.MaxReason`.

## Errors

The failure policy of the hook, so a broken polix never traps the agent:

- An unknown agent exits with 1.
- An input that cannot be read or parsed prints `polix hook: <error>` and lets the action go on (exit 0).
- Outside a git repository, or without events (a `Read`), everything goes through.
- A workflow that cannot be used is data of the answer (Logic, rule 2), never an error.
- The errors of the steps are data too: an unknown hash warns, rules that cannot be loaded fail the `run` that
  needs them (`No se pueden cargar las rules`), a log that cannot be used lets a stop through, a file that cannot
  be read gives no context, a failing command or check blocks with its output.

## Flow

1. `AnswerHookCommand` checks the agent, reads stdin into an `AnswerHookRequest` and injects an empty
   `AnswerHookAdapter` as the `AnswerHookPort`.
2. `answerHook` (shell) parses the input with the format of the agent into a `Hook`; without events it answers
   nothing.
3. It opens the repository from `Hook.Folder` and loads the workflow through the port; outside a repository it
   answers nothing.
4. For each event, it locates the path of an edit through the port and builds its `Evaluation` with the facts the
   previous event left; then it loops from one call site: it asks `AnswerHook` for the next step, carries it out
   through the port and feeds the result back to the evaluation (`Know*` for a fact, `Note` for an outcome), until
   the step is none.
5. It merges the answers of the events (`agent.Answer.Merge`) and renders them for the agent
   (`agent.Agent.Render`) into the `AnswerHookResponse`.
6. `AnswerHookCommand` writes stdout and stderr and exits with the code.
