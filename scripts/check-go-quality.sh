#!/usr/bin/env bash
set -euo pipefail

PROJECT_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
BACKEND_DIRECTORY="$PROJECT_ROOT/backend"

echo "==> Go naming gate"
go run "$PROJECT_ROOT/scripts/naming-gate.go" "$PROJECT_ROOT"

echo "==> Go source size gate"
MAXIMUM_GO_FILE_LINES=600
SIZE_GATE_FAILED=0
while IFS= read -r -d '' SOURCE_FILE; do
  SOURCE_LINES="$(wc -l < "$SOURCE_FILE" | tr -d '[:space:]')"
  if (( SOURCE_LINES > MAXIMUM_GO_FILE_LINES )); then
    printf '%s: %d lines exceeds the %d-line Go source limit\n' \
      "${SOURCE_FILE#"$PROJECT_ROOT"/}" "$SOURCE_LINES" "$MAXIMUM_GO_FILE_LINES" >&2
    SIZE_GATE_FAILED=1
  fi
done < <(find "$BACKEND_DIRECTORY" "$PROJECT_ROOT/scripts" -type f -name '*.go' -not -path '*/vendor/*' -print0)
if (( SIZE_GATE_FAILED != 0 )); then
  echo "Go source size gate failed" >&2
  exit 1
fi

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
