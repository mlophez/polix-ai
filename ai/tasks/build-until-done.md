# Build until done: the engine part

`/polix:build` now runs unattended (part A, skill and agents): it asks only before it starts, iterates until the
plan is done, and ends `blocked` only when two iterations close nothing, after ten iterations, or when only tasks a
person must unblock are left.
But that still depends on the model following the skill: nothing stops the main session or the builder from ending
their turn early.
Part B makes polix guarantee it, the way it guarantees the other gates: the engine refuses to let the agent stop.

## Done as a command

- [ ] `polix plan status <plan>`: decides whether the plan is done from the plan file alone, read only.
      Done when every line of `Progress` is `[x]`, every criterion of the checklist is `[x]` (or manual, in
      `Pending`), the last `#### Verification` is `met`, and the last `#### Review` has no open `blocking` nor `major`.
      It prints what is still open, one item per line, and exits 0 when done, 1 when not, 2 on a malformed plan.
- [ ] It shares the parser of `polix plan check` (see `plan-check.md`): plan both together, one domain `plan`.
- [ ] `--json` for the skill, and the plans of the earlier format (`- Status:` header, `Covers:` bullets).

## A stop gate while a plan is in progress

- [ ] A recipe and a shortcut in `polix.yaml`: on `stop`, while a plan of the folder of plans is `in-progress`,
      a `run` of `polix plan status` blocks and gives the agent what is still open.
      On `subagent_stop` of `polix:builder`, the same for the tasks from its `From` still `[ ]`.
- [ ] The loop guard (`max_stop_retries`) is the stall detector: it lets the agent stop only when nothing changed
      since the last block, which matches "two iterations that close nothing" of the skill.
      Check whether a stop gate needs its own retries, higher than the repository default of 1.
- [ ] A plan `blocked` or `done` never blocks the stop: only `in-progress` does.
- [ ] Enable it in this repository's `polix.yaml`, and document it in `docs/use-cases.md` and `examples/polix.yaml`.

## No permission prompts during a build

- [ ] A permission prompt stops an unattended run as much as a question does.
      Document the allowlist a build needs (the test, lint and build commands of `AGENTS.md`), or that it runs in
      auto mode, and have `polix doctor` warn when the commands of the plan's verification would ask for permission.

## Open questions

- The stall state: `polix plan status` can read it from the `#### Result` of the last two iterations of the plan.
- Whether the gate reads the plan in progress from the log of the session, or from the one plan `in-progress`.

Next step: plan it with `/polix:plan`, together with `polix plan check`.
