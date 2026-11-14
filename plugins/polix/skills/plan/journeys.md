# Journeys and criteria by example

Each example shows one kind of journey of `specification.journeys`, then a criterion that checks journeys.
The systems are invented: write the journeys of the plan from its request, its interview and its code.
Each step is an object, and exact data goes in the `literal` of the step that first shows it, with its `id`.
Every other place names that id, never copies its `text`.

## A request and its exact response

The request and the body are part of the contract, so each is the literal of its step, and the body is `exactly`.

```yaml
journeys:
  - id: CUJ-01
    title: A customer checks the status of a claim
    kept: false
    given:
      - step: Claim `C-1001` is `in_review`, with the next step `adjuster visit` on 2026-12-01.
    when:
      - step: The portal sends this request.
        literal:
          id: LIT-01
          match: exactly
          text: |
            GET /claims/C-1001/status
    then:
      - step: The API answers `200` with this body.
        literal:
          id: LIT-02
          match: exactly
          text: |
            {
              "status": "in_review",
              "next_step": "adjuster visit",
              "expected_date": "2026-12-01"
            }
    but:
      - step: The body has no personal data of the customer.
```

## An error, with the state left unchanged

A rejected input names its error and, with `but`, what it must not touch.

```yaml
journeys:
  - id: CUJ-02
    title: An export with an invalid date is rejected
    kept: false
    given:
      - step: The folder `exports/` is empty.
    when:
      - step: |
          The user runs `invoices export --from 2026-13-01`.
    then:
      - step: The command exits with code 2 and writes this line to stderr.
        literal:
          id: LIT-03
          match: exactly
          text: |
            error: --from: invalid date "2026-13-01", expected YYYY-MM-DD
    but:
      - step: No file is written to `exports/`.
```

## A family of errors of one input

Each way the input can be wrong has its own message, so the journey has one step per case, each with its literal.
One journey holds the family: never one journey per message, nor a message only in `interface`.

```yaml
journeys:
  - id: CUJ-07
    title: An export folder that is not valid is rejected
    kept: false
    given:
      - step: The file `invoices.yaml` sets `exports` to each value of the steps below, one run each.
    when:
      - step: |
          The user runs `invoices export --from 2026-11-01` once per value.
    then:
      - step: For `/tmp/out`, the command exits with code 2 and writes this line to stderr.
        literal:
          id: LIT-06
          match: exactly
          text: |
            error: exports "/tmp/out" must be relative to the project, not absolute
      - step: For `../out`, the command exits with code 2 and writes this line to stderr.
        literal:
          id: LIT-07
          match: exactly
          text: |
            error: exports "../out" must be inside the project
      - step: For `out/*`, the command exits with code 2 and writes this line to stderr.
        literal:
          id: LIT-08
          match: exactly
          text: |
            error: exports "out/*" must be a folder, not a glob
    but:
      - step: No file is written in any run.
```

The criterion of the family names the journey once, and its test runs every case:

```yaml
acceptance:
  - id: AC-04
    title: An export folder that is not valid is rejected with its message
    requirement: |
      If `exports` is absolute, outside the project or a glob, then the export shall exit 2 with its message.
    cujs: [CUJ-07]
    verify:
      - run: go test ./internal/config -run TestExportsProblems
        expect: pass, with one case per step of CUJ-07 and its literal
    status: pending
    evidence: none
```

## An output checked in part, with a value that changes

Only the lines the criteria check go in the literal, and the value of each run is a `<variable>`.

```yaml
journeys:
  - id: CUJ-03
    title: An export of a month writes one row per invoice
    kept: false
    given:
      - step: November 2026 has the invoices `F-001` for 120.00 EUR and `F-002` for 80.50 EUR.
    when:
      - step: |
          The user runs `invoices export --from 2026-11-01 --to 2026-11-30 --format csv`.
    then:
      - step: The command exits with code 0 and prints the path `exports/invoices-<timestamp>.csv`.
      - step: That file contains these lines, in this order.
        literal:
          id: LIT-04
          match: contains
          text: |
            number,date,amount
            F-001,2026-11-03,120.00
            F-002,2026-11-17,80.50
    but: []
```

## An internal contract, with code as the actor

The caller is a module, and the contract is a stored format: it goes to `interface` like any public one.

```yaml
journeys:
  - id: CUJ-04
    title: The loader upgrades a state file of version 1
    kept: false
    given:
      - step: The file `.state/session.json` holds this content of version 1.
        literal:
          id: LIT-05
          match: exactly
          text: |
            {
              "version": 1,
              "session": "s-42",
              "entries": []
            }
    when:
      - step: The application loads the state at start.
    then:
      - step: The loader returns the session `s-42` with no entries.
      - step: |
          The file contains `"version": 2` and an empty list `"blocks"`.
    but:
      - step: The file keeps the session `s-42`.
```

## A journey that names a literal of another

The same data in a second journey names the literal by its id, never copies it.

```yaml
journeys:
  - id: CUJ-06
    title: An export retried with the same invalid date is rejected again
    kept: false
    given:
      - step: The export of CUJ-02 was rejected, and `exports/` is still empty.
    when:
      - step: |
          The user runs `invoices export --from 2026-13-01` again.
    then:
      - step: The command exits with code 2 and writes LIT-03 to stderr.
    but:
      - step: No file is written to `exports/`.
```

## A refactor that keeps a behaviour

Nothing new is visible: `outcome` says the behaviour stays the same, and its journeys hold what exists.
Its `interface` gives the structure after the change: the packages, their dependencies and the internal contracts.

```yaml
journeys:
  - id: CUJ-05
    title: An export of a month still writes the same file
    kept: true
    given:
      - step: The invoices of CUJ-03, with the export module split into a reader and a writer.
    when:
      - step: |
          The user runs `invoices export --from 2026-11-01 --to 2026-11-30 --format csv`.
    then:
      - step: The output, the exit code and the file written are the same as in the base version.
    but: []
```

Its criteria compare with the base version, and a structural one has its own check:

```yaml
acceptance:
  - id: AC-02
    title: The export keeps its output
    requirement: The export shall give the same output, errors and exit code as the base version.
    cujs: [CUJ-05]
    verify:
      - run: the differential check of the task, on the inputs of CUJ-03
        expect: no difference with the base version
    status: pending
    evidence: none
  - id: AC-03
    title: The writer does not depend on the reader
    requirement: The writer shall not import the reader.
    cujs: []
    verify:
      - run: go test ./tests/architecture/...
        expect: pass, with the test of the dependency between the writer and the reader
    status: pending
    evidence: none
```

## A criterion that checks journeys

The requirement is one EARS sentence of the general rule, and its journeys are the cases that show it.
It names the journeys and their literals, never their values again.

```yaml
acceptance:
  - id: AC-01
    title: An invalid date is rejected without writing
    requirement: |
      If `--from` or `--to` is not a date, then the export shall exit 2, print its error and write no file.
    cujs: [CUJ-02, CUJ-06]
    verify:
      - run: invoices export --from 2026-13-01
        expect: exit code 2, LIT-03 on stderr and `exports/` still empty
    status: pending
    evidence: none
```
