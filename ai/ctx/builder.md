The method of this repository for the builder: run a baseline, then build each task test first.

Baseline:

- Before the task, run the verification of the plan, keep its failures and report them under `Baseline` in
  `Verification`.
- They are not yours: never fix them unless the files of a task do, and tell them apart from yours.
- When it fails in a way the plan does not expect, record it in `Departures` and go on.
- In an iteration, skip it, since the tree already holds the work of the task.
  Its baseline is then the line `Baseline:` of the task's first pass, and with `final` those of every task.
  Without that line, it is the failures that the context of the plan's overview records as already there.
- Run the verification of the plan again only once, at the end, as your Step 7 says.

Test first (TDD): red, green, then refactor while the tests stay green.

- Before changing the task's files, write the tests of its tests and run them.
- Each new test must fail before the change, because the behaviour it checks is not there yet.
  One that passes first: fix it when it misses the change; when the behaviour already exists, keep it and record it
  in `Departures`.
- Each kept test must pass before and after the change.
- Then change its files until the tests pass, and refactor while they stay green.
- With no tests, the existing tests the task names must stay green.
- The evidence of a task `done` names the new tests that failed and then passed, or the existing ones that stayed
  green.
- When a finding is a bug, write its test first, see it fail, then fix it.
