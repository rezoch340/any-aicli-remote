#!/usr/bin/env bash
set -euo pipefail

PROJECT_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
BACKEND_DIRECTORY="$PROJECT_ROOT/backend"

echo "==> Go naming gate"
go run "$PROJECT_ROOT/scripts/naming-gate.go" "$PROJECT_ROOT"

echo "==> Go formatting gate"
UNFORMATTED_FILES="$(gofmt -l "$BACKEND_DIRECTORY" "$PROJECT_ROOT/scripts/naming-gate.go")"
if [[ -n "$UNFORMATTED_FILES" ]]; then
  printf '%s\n' "$UNFORMATTED_FILES" >&2
  echo "Go formatting gate failed" >&2
  exit 1
fi

echo "==> Go tests"
(cd "$BACKEND_DIRECTORY" && go test -count=1 ./...)

echo "==> Go race detector"
(cd "$BACKEND_DIRECTORY" && go test -race -count=1 ./...)

echo "==> Go vet"
(cd "$BACKEND_DIRECTORY" && go vet ./...)
