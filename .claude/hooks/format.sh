#!/usr/bin/env bash
# Formats the Go file Claude just edited and reports readability limits back to Claude.
file=$(jq -r '.tool_input.file_path // empty')
[ -f "$file" ] || exit 0
[[ "$file" == *.go ]] || exit 0

gofmt -w "$file"

PATH="$PATH:$(go env GOPATH)/bin"
problems=""
if command -v gocyclo >/dev/null; then
  complex=$(gocyclo -over 9 "$file")
  [ -n "$complex" ] && problems+="Cyclomatic complexity must be < 10:\n$complex\n"
fi
if [[ "$file" != *_test.go ]]; then
  lines=$(wc -l <"$file")
  [ "$lines" -gt 300 ] && problems+="$file has $lines lines (max 300): split it by responsibility.\n"
fi

if [ -n "$problems" ]; then
  printf "%b" "$problems" >&2
  exit 2
fi
exit 0
