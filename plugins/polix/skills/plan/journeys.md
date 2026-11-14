# Journeys and criteria by example

Each example shows one kind of journey of the `Journeys` of `Outcome`, then a criterion that checks journeys.
The systems are invented: write the journeys of the plan from its request, its interview and its code.

## A request and its exact response

The body is part of the contract, so the step says `exactly` and gives it whole.

##### CUJ-01: A customer checks the status of a claim

- **Given:** claim `C-1001` is `in_review`, with the next step `adjuster visit` on 2026-12-01.
- **When:** the portal sends this request.

  ```http
  GET /claims/C-1001/status
  ```

- **Then:** the API answers `200` with exactly this body.

  ```json
  {
    "status": "in_review",
    "next_step": "adjuster visit",
    "expected_date": "2026-12-01"
  }
  ```

- **But:** the body has no personal data of the customer.

## An error, with the state left unchanged

A rejected input names its error and, with `But:`, what it must not touch.

##### CUJ-02: An export with an invalid date is rejected

- **Given:** the folder `exports/` is empty.
- **When:** the user runs this command.

  ```sh
  invoices export --from 2026-13-01
  ```

- **Then:** the command exits with code 2 and writes exactly this line to stderr.

  ```text
  error: --from: invalid date "2026-13-01", expected YYYY-MM-DD
  ```

- **But:** no file is written to `exports/`.

## An output checked in part, with a value that changes

Only the lines the criteria check go in the block, and the value of each run is a `<variable>`.

##### CUJ-03: An export of a month writes one row per invoice

- **Given:** November 2026 has the invoices `F-001` for 120.00 EUR and `F-002` for 80.50 EUR.
- **When:** the user runs `invoices export --from 2026-11-01 --to 2026-11-30 --format csv`.
- **Then:** the command exits with code 0 and prints the path `exports/invoices-<timestamp>.csv`.
- **And:** that file contains these lines, in this order.

  ```text
  number,date,amount
  F-001,2026-11-03,120.00
  F-002,2026-11-17,80.50
  ```

## An internal contract, with code as the actor

The caller is a module, and the contract is a stored format: it goes to `Interface` like any public one.

##### CUJ-04: The loader upgrades a state file of version 1

- **Given:** the file `.state/session.json` holds exactly this content of version 1.

  ```json
  {
    "version": 1,
    "session": "s-42",
    "entries": []
  }
  ```

- **When:** the application loads the state at start.
- **Then:** the loader returns the session `s-42` with no entries.
- **And:** the file contains `"version": 2` and an empty list `"blocks"`.
- **But:** the file keeps the session `s-42`.

## A refactor that keeps a behaviour

Nothing new is visible: `Outcome` says the behaviour stays the same, and its journeys hold what exists.
Its `Interface` gives the structure after the change: the packages, their dependencies and the internal contracts.

##### CUJ-05 (kept): An export of a month still writes the same file

- **Given:** the invoices of CUJ-03, with the export module split into a reader and a writer.
- **When:** the user runs `invoices export --from 2026-11-01 --to 2026-11-30 --format csv`.
- **Then:** the output, the exit code and the file written are the same as in the base version.

Its criteria compare with the base version, and a structural one has its own check:

- **Requirement:** The export shall give the same output, errors and exit code as the base version for CUJ-03.
- **Requirement:** The writer shall not import the reader.
  Verify it with the architecture tests or a search that finds nothing.

## A criterion that checks journeys

The requirement is one EARS sentence, and its journeys are the cases that show it.

- [ ] AC-01: An invalid date is rejected without writing

#### AC-01: An invalid date is rejected without writing

- **Requirement:** If `--from` or `--to` is not a date, then the export shall exit 2, print its error and write no file.
- **CUJs:** `CUJ-02`.
- **Verify:** `invoices export --from 2026-13-01`, expected: exit code 2, the error of CUJ-02 and `exports/` still empty.
