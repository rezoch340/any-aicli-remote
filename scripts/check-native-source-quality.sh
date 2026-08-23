#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"

check_physical_lines() {
  local dir="$1" ext="$2"
  [[ -d "$dir" ]] || return 0
  local failed=0 file lines
  while IFS= read -r -d '' file; do
    lines=$(wc -l < "$file" | awk '{print $1}')
    if (( lines > 600 )); then
      echo "error: $file has $lines physical lines (maximum 600)" >&2
      failed=1
    fi
  done < <(
    find "$dir" -type f -name "*.$ext" \
      ! -path '*/generated/*' \
      ! -path '*/build/*' \
      -print0
  )
  return "$failed"
}

physical_failed=0
ANDROID_SOURCE_DIRS=(
  "$ROOT/android/app/src/main"
  "$ROOT/android/feature/ui/src/main"
  "$ROOT/android/core/model/src/main"
  "$ROOT/android/core/remote/src/main"
  "$ROOT/android/core/storage/src/main"
  "$ROOT/android/core/session/src/main"
  "$ROOT/android/core/chat/src/main"
)
for android_source_dir in "${ANDROID_SOURCE_DIRS[@]}"; do
  check_physical_lines "$android_source_dir" kt || physical_failed=1
done

APPLE_SOURCE_DIRS=(
  "$ROOT/ios/AnyAICLIRemote"
  "$ROOT/ios/AnyAICLIRemoteCore"
  "$ROOT/ios/AnyAICLIRemoteFeature"
  "$ROOT/apple/Shared"
)
for apple_source_dir in "${APPLE_SOURCE_DIRS[@]}"; do
  check_physical_lines "$apple_source_dir" swift || physical_failed=1
done

command -v swift >/dev/null 2>&1 || { echo "error: Swift toolchain is required" >&2; exit 1; }
[[ -f "$ROOT/.swiftlint.yml" ]] || { echo "error: missing .swiftlint.yml" >&2; exit 1; }
echo "==> SwiftLint 0.65.0 command plugin (ios, apple/Shared)"
lint_failed=0
swift package --package-path "$ROOT/tools/swiftlint" plugin --allow-writing-to-package-directory swiftlint --strict \
  --config "$ROOT/.swiftlint.yml" || lint_failed=1
(( physical_failed == 0 && lint_failed == 0 ))
