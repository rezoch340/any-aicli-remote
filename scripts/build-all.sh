#!/usr/bin/env bash
set -euo pipefail

PROJECT_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
IOS_PROJECT="$PROJECT_ROOT/ios/AnyAICLIRemote.xcodeproj"
IOS_SCHEME="AnyAICLIRemote"
IOS_GENERIC_SIMULATOR_DESTINATION="generic/platform=iOS Simulator"

resolve_ios_test_destination() {
  if [[ -n "${IOS_SIMULATOR_DESTINATION:-}" ]]; then
    printf '%s\n' "$IOS_SIMULATOR_DESTINATION"
    return
  fi

  local available_destinations
  if ! available_destinations="$(
    xcodebuild \
      -project "$IOS_PROJECT" \
      -scheme "$IOS_SCHEME" \
      -showdestinations
  )"; then
    echo "error: failed to query iOS Simulator destinations" >&2
    return 1
  fi

  local simulator_identifier
  simulator_identifier="$(
    printf '%s\n' "$available_destinations" |
      sed -nE '/platform:iOS Simulator, arch:/s/.*id:([^,[:space:]]+).*/\1/p' |
      sed -n '1p'
  )"
  if [[ -z "$simulator_identifier" ]]; then
    echo "error: no concrete iOS Simulator destination is available" >&2
    echo "hint: install an iOS Simulator runtime or set IOS_SIMULATOR_DESTINATION" >&2
    return 1
  fi

  printf 'platform=iOS Simulator,id=%s\n' "$simulator_identifier"
}

echo "==> Go backend quality and macOS launcher build"
"$PROJECT_ROOT/scripts/build-macos-app.sh"

if ! command -v xcodegen >/dev/null 2>&1; then
  echo "error: required tool not found: xcodegen" >&2
  exit 1
fi

echo "==> Generate iOS Xcode project"
xcodegen generate \
  --spec "$PROJECT_ROOT/ios/project.yml" \
  --project "$PROJECT_ROOT/ios"

echo "==> iOS simulator build"
xcodebuild \
  -skipMacroValidation \
  -project "$IOS_PROJECT" \
  -scheme "$IOS_SCHEME" \
  -destination "$IOS_GENERIC_SIMULATOR_DESTINATION" \
  CODE_SIGNING_ALLOWED=NO \
  build

IOS_TEST_DESTINATION="$(resolve_ios_test_destination)"
echo "==> iOS simulator tests ($IOS_TEST_DESTINATION)"
xcodebuild \
  -skipMacroValidation \
  -project "$IOS_PROJECT" \
  -scheme "$IOS_SCHEME" \
  -destination "$IOS_TEST_DESTINATION" \
  test

echo "==> Android unit tests, debug build, and lint"
"$PROJECT_ROOT/android/gradlew" -p "$PROJECT_ROOT/android" \
  :app:testDebugUnitTest \
  :app:assembleDebug \
  :app:lintDebug
