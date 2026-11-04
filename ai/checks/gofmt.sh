#!/bin/sh
# GO-1: every Go file of the repository is formatted with gofmt.
set -eu
out=$(gofmt -l cmd internal)
if [ -n "$out" ]; then
	echo "Not formatted (run gofmt -w on them):"
	echo "$out"
	exit 1
fi
