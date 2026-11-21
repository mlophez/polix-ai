# TakeStock

The good case of the document of a use case: the title and every section, once and in order.

## Trigger

`polix stock take` (GOARCH-14).

## Request

`Request`: `Shop`.

## Logic

`Decide` counts the units taken.

## Response

`Response`: `Count`.

## Port

`Port`: `Load`.

## Errors

None.

## Flow

1. The shell reads through the port and calls `Decide` once.
