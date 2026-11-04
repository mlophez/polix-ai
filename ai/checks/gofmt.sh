#!/bin/sh
# GO-1: every Go file of the repository is formatted with gofmt: tracked or new, unless git ignores it. A tracked
# file deleted from the disk is skipped, since gofmt would fail on it.
set -eu
out=$(git ls-files -z --cached --others --exclude-standard -- '*.go' |
	xargs -0 sh -c 'for f; do [ ! -f "$f" ] || printf "%s\0" "$f"; done' sh |
	xargs -0 gofmt -l)
if [ -n "$out" ]; then
	echo "Not formatted (run gofmt -w on them):"
	echo "$out"
	exit 1
fi
