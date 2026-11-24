# Journeys and criteria by example

Each example shows one kind of journey of `Journeys`, then a criterion that checks journeys.
The systems are invented: write the journeys of the plan from its request, its interview and its code.
Each step is an item of its list, and exact data goes in the literal of the step that first shows it, with its id.
Every other place names that id, never copies its text.

## A request and its exact response

The request and the body are part of the contract, so each is the literal of its step, and the body is `exactly`.

````markdown
#### CUJ-01: A customer checks the status of a claim

As a customer, I want to see where my claim stands, so that I know when to expect the adjuster.

- **Kept**: no

**Given**

- Claim `C-1001` is `in_review`, with the next step `adjuster visit` on 2026-12-01.

**When**

- The portal sends this request.
  LIT-01 (exactly):

  ```text
  GET /claims/C-1001/status
  ```

**Then**

- The API answers `200` with this body.
  LIT-02 (exactly):

  ```json
  {
    "status": "in_review",
    "next_step": "adjuster visit",
    "expected_date": "2026-12-01"
  }
  ```

**But**

- The body has no personal data of the customer.
````

## An error, with the state left unchanged

A rejected input names its error and, with `But`, what it must not touch.

````markdown
#### CUJ-02: An export with an invalid date is rejected

As an accountant, I want a wrong date rejected at once, so that I never get a partial export.

- **Kept**: no

**Given**

- The folder `exports/` is empty.

**When**

- The user runs `invoices export --from 2026-13-01`.

**Then**

- The command exits with code 2 and writes this line to stderr.
  LIT-03 (exactly):

  ```text
  error: --from: invalid date "2026-13-01", expected YYYY-MM-DD
  ```

**But**

- No file is written to `exports/`.
````

## A family of errors of one input

Each way the input can be wrong has its own message, so the journey has one step per case, each with its literal.
One journey holds the family: never one journey per message, nor a message only in `Interface`.

````markdown
#### CUJ-07: An export folder that is not valid is rejected

As an accountant, I want a wrong export folder rejected with its reason, so that I can fix the setting.

- **Kept**: no

**Given**

- The file `invoices.yaml` sets `exports` to each value of the steps below, one run each.

**When**

- The user runs `invoices export --from 2026-11-01` once per value.

**Then**

- For `/tmp/out`, the command exits with code 2 and writes this line to stderr.
  LIT-06 (exactly):

  ```text
  error: exports "/tmp/out" must be relative to the project, not absolute
  ```

- For `../out`, the command exits with code 2 and writes this line to stderr.
  LIT-07 (exactly):

  ```text
  error: exports "../out" must be inside the project
  ```

- For `out/*`, the command exits with code 2 and writes this line to stderr.
  LIT-08 (exactly):

  ```text
  error: exports "out/*" must be a folder, not a glob
  ```

**But**

- No file is written in any run.
````

The criterion of the family names the journey once, and its test runs every case:

```markdown
#### AC-04: An export folder that is not valid is rejected with its message

- **Requirement**: If `exports` is absolute, outside the project or a glob, then the export shall exit 2 with its
  message.
- **Journeys**: CUJ-07
- **Verify**:
  - Run `go test ./internal/config -run TestExportsProblems`.
    Expect: pass, with one case per step of CUJ-07 and its literal.
```

## An output checked in part, with a value that changes

Only the lines the criteria check go in the literal, and the value of each run is a `<variable>`.

````markdown
#### CUJ-03: An export of a month writes one row per invoice

As an accountant, I want the invoices of a month in one file, so that I can load them in the ledger.

- **Kept**: no

**Given**

- November 2026 has the invoices `F-001` for 120.00 EUR and `F-002` for 80.50 EUR.

**When**

- The user runs `invoices export --from 2026-11-01 --to 2026-11-30 --format csv`.

**Then**

- The command exits with code 0 and prints the path `exports/invoices-<timestamp>.csv`.
- That file contains these lines, in this order.
  LIT-04 (contains):

  ```text
  number,date,amount
  F-001,2026-11-03,120.00
  F-002,2026-11-17,80.50
  ```

**But**

- None.
````

## An internal contract, with code as the actor

The caller is a module, and the contract is a stored format: it goes to `Interface` like any public one.

````markdown
#### CUJ-04: The loader upgrades a state file of version 1

As the application, I want an old state file upgraded on load, so that a session survives an update.

- **Kept**: no

**Given**

- The file `.state/session.json` holds this content of version 1.
  LIT-05 (exactly):

  ```json
  {
    "version": 1,
    "session": "s-42",
    "entries": []
  }
  ```

**When**

- The application loads the state at start.

**Then**

- The loader returns the session `s-42` with no entries.
- The file contains `"version": 2` and an empty list `"blocks"`.

**But**

- The file keeps the session `s-42`.
````

## A journey that names a literal of another

The same data in a second journey names the literal by its id, never copies it.

```markdown
#### CUJ-06: An export retried with the same invalid date is rejected again

As an accountant, I want a retry with the same wrong date rejected the same way, so that nothing is half written.

- **Kept**: no

**Given**

- The export of CUJ-02 was rejected, and `exports/` is still empty.

**When**

- The user runs `invoices export --from 2026-13-01` again.

**Then**

- The command exits with code 2 and writes LIT-03 to stderr.

**But**

- No file is written to `exports/`.
```

## A refactor that keeps a behaviour

Nothing new is visible: `Outcome` says the behaviour stays the same, and its journeys hold what exists.
Its `Interface` gives the structure after the change: the packages, their dependencies and the internal contracts.

```markdown
#### CUJ-05: An export of a month still writes the same file

As an accountant, I want the monthly export unchanged by the split, so that my ledger import keeps working.

- **Kept**: yes

**Given**

- The invoices of CUJ-03, with the export module split into a reader and a writer.

**When**

- The user runs `invoices export --from 2026-11-01 --to 2026-11-30 --format csv`.

**Then**

- The output, the exit code and the file written are the same as in the base version.

**But**

- None.
```

Its criteria run the `Kept` tests, which pass on the base version too, and a structural one has its own check.
The `Differential` of the task compares the outputs with the base version; a `Verify` never names it.

```markdown
#### AC-02: The export keeps its output

- **Requirement**: The export shall give the same output, errors and exit code as the base version.
- **Journeys**: CUJ-05
- **Verify**:
  - Run `go test ./internal/export -run '^TestExportMonth$'`.
    Expect: pass, the kept test of CUJ-05, unchanged since the base version.

#### AC-03: The writer does not depend on the reader

- **Requirement**: The writer shall not import the reader.
- **Journeys**: none
- **Verify**:
  - Run `go test ./tests/architecture -run '^TestWriterImports$'`.
    Expect: pass, TestWriterImports finds no import of the reader in the writer.
```

## A criterion that checks journeys

The requirement is one EARS sentence of the general rule, and its journeys are the cases that show it.
It names the journeys and their literals, never their values again.

```markdown
#### AC-01: An invalid date is rejected without writing

- **Requirement**: If `--from` or `--to` is not a date, then the export shall exit 2, print its error and write no
  file.
- **Journeys**: CUJ-02, CUJ-06
- **Verify**:
  - Run `invoices export --from 2026-13-01`.
    Expect: exit code 2, LIT-03 on stderr and `exports/` still empty.
```
