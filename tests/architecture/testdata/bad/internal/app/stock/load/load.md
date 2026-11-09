# LoadStock

The good case of a load use case: the loader of the count of a shop, which main builds once.

## Trigger

Not a command: `main` of cmd/app calls `Command` once, which returns the loader, a `stock.Load` (GOARCH-14).

## Request

`Request`: `Shop`.

## Logic

`Decide` validates the count with the domain.

## Response

`Response`: `Loaded`.

## Port

`Port`: `Read`.

## Errors

None: the loader never fails.

## Flow

1. The loader runs the shell, which reads through the port and calls `Decide` once.
