#!/usr/bin/env bash
set -euo pipefail

PROJECT_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
MACOS_DIRECTORY="$PROJECT_ROOT/macos"
PROJECT_PATH="$MACOS_DIRECTORY/AnyAICLIRemoteLauncher.xcodeproj"
DERIVED_DATA_DIRECTORY="$PROJECT_ROOT/dist/macos-derived-data"
DAEMON_PATH="$PROJECT_ROOT/dist/any-aicli-remote-daemon"
BUILT_APP_PATH="$DERIVED_DATA_DIRECTORY/Build/Products/Release/Any AI CLI Remote Launcher.app"
OUTPUT_APP_PATH="$PROJECT_ROOT/dist/Any AI CLI Remote Launcher.app"
LOCAL_CODE_SIGN_IDENTITY="-"
DAEMON_SIGNING_IDENTIFIER="com.anyaicliremote.daemon"

for required_tool in go xcodegen xcodebuild xcrun codesign; do
  if ! command -v "$required_tool" >/dev/null 2>&1; then
    echo "error: required tool not found: $required_tool" >&2
    exit 1
  fi
done

echo "==> Go quality gate"
"$PROJECT_ROOT/scripts/check-go-quality.sh"

echo "==> Build arm64 Any AI CLI Remote daemon"
mkdir -p "$PROJECT_ROOT/dist"
(
  cd "$PROJECT_ROOT/backend"
  CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 \
    go build -trimpath -o "$DAEMON_PATH" ./cmd/any-aicli-remote-daemon
)
chmod 0755 "$DAEMON_PATH"

if ! /usr/bin/file "$DAEMON_PATH" | /usr/bin/grep -q 'arm64'; then
  echo "error: daemon is not an arm64 Mach-O executable" >&2
  /usr/bin/file "$DAEMON_PATH" >&2
  exit 1
fi

echo "==> Sign daemon"
/usr/bin/codesign \
  --force \
  --sign "$LOCAL_CODE_SIGN_IDENTITY" \
  --identifier "$DAEMON_SIGNING_IDENTIFIER" \
  "$DAEMON_PATH"
/usr/bin/codesign --verify --strict --verbose=2 "$DAEMON_PATH"

echo "==> Generate macOS Xcode project"
xcodegen generate \
  --spec "$MACOS_DIRECTORY/project.yml" \
  --project "$MACOS_DIRECTORY"

echo "==> Test macOS launcher"
TEST_DERIVED_DATA_DIRECTORY=""
cleanup_test_derived_data() {
  if [[ -n "$TEST_DERIVED_DATA_DIRECTORY" ]] && [[ -d "$TEST_DERIVED_DATA_DIRECTORY" ]]; then
    rm -rf "$TEST_DERIVED_DATA_DIRECTORY"
  fi
}
trap cleanup_test_derived_data EXIT
TEST_DERIVED_DATA_DIRECTORY="$(mktemp -d "$PROJECT_ROOT/dist/macos-test-derived-data.XXXXXX")"
xcodebuild \
  -project "$PROJECT_PATH" \
  -scheme AnyAICLIRemoteLauncher \
  -configuration Debug \
  -destination 'platform=macOS' \
  -derivedDataPath "$TEST_DERIVED_DATA_DIRECTORY" \
  ARCHS=arm64 \
  ONLY_ACTIVE_ARCH=YES \
  CODE_SIGNING_ALLOWED=YES \
  CODE_SIGNING_REQUIRED=YES \
  CODE_SIGN_STYLE=Manual \
  CODE_SIGN_IDENTITY="$LOCAL_CODE_SIGN_IDENTITY" \
  DEVELOPMENT_TEAM= \
  build-for-testing
TEST_BUNDLE_PATH="$TEST_DERIVED_DATA_DIRECTORY/Build/Products/Debug/AnyAICLIRemoteLauncherTests.xctest"
TEST_APP_PATH="$TEST_DERIVED_DATA_DIRECTORY/Build/Products/Debug/Any AI CLI Remote Launcher.app"
TEST_DAEMON_PATH="$TEST_APP_PATH/Contents/MacOS/any-aicli-remote-daemon"
if [[ ! -d "$TEST_BUNDLE_PATH" ]]; then
  echo "error: expected test bundle was not produced at $TEST_BUNDLE_PATH" >&2
  exit 1
fi
if [[ ! -x "$TEST_DAEMON_PATH" ]]; then
  echo "error: Debug app does not contain an executable daemon" >&2
  exit 1
fi
ANY_AI_CLI_REMOTE_LAUNCHER_E2E=1 \
ANY_AI_CLI_REMOTE_LAUNCHER_E2E_DAEMON="$TEST_DAEMON_PATH" \
  xcrun xctest "$TEST_BUNDLE_PATH"
cleanup_test_derived_data
trap - EXIT

echo "==> Build macOS launcher"
rm -rf "$DERIVED_DATA_DIRECTORY"
XCODE_SIGNING_ARGUMENTS=(
  "CODE_SIGNING_ALLOWED=YES"
  "CODE_SIGNING_REQUIRED=YES"
  "CODE_SIGN_STYLE=Manual"
  "CODE_SIGN_IDENTITY=$LOCAL_CODE_SIGN_IDENTITY"
  "DEVELOPMENT_TEAM="
)
xcodebuild \
  -project "$PROJECT_PATH" \
  -scheme AnyAICLIRemoteLauncher \
  -configuration Release \
  -destination 'generic/platform=macOS' \
  -derivedDataPath "$DERIVED_DATA_DIRECTORY" \
  ARCHS=arm64 \
  ONLY_ACTIVE_ARCH=NO \
  "${XCODE_SIGNING_ARGUMENTS[@]}" \
  build

if [[ ! -d "$BUILT_APP_PATH" ]]; then
  echo "error: expected app was not produced at $BUILT_APP_PATH" >&2
  exit 1
fi

rm -rf "$OUTPUT_APP_PATH"
/usr/bin/ditto "$BUILT_APP_PATH" "$OUTPUT_APP_PATH"

LAUNCHER_POLICY_PATH="$OUTPUT_APP_PATH/Contents/Resources/LauncherPolicy.json"
if [[ ! -f "$LAUNCHER_POLICY_PATH" ]]; then
  echo "error: app does not contain LauncherPolicy.json at $LAUNCHER_POLICY_PATH" >&2
  exit 1
fi

LAUNCHER_EXECUTABLE="$OUTPUT_APP_PATH/Contents/MacOS/Any AI CLI Remote Launcher"
EMBEDDED_DAEMON="$OUTPUT_APP_PATH/Contents/MacOS/any-aicli-remote-daemon"
if [[ ! -x "$LAUNCHER_EXECUTABLE" ]]; then
  echo "error: app does not contain an executable launcher" >&2
  exit 1
fi
if ! /usr/bin/file "$LAUNCHER_EXECUTABLE" | /usr/bin/grep -q 'arm64'; then
  echo "error: launcher is not arm64" >&2
  exit 1
fi
if [[ ! -x "$EMBEDDED_DAEMON" ]]; then
  echo "error: app does not contain an executable daemon" >&2
  exit 1
fi
if ! /usr/bin/file "$EMBEDDED_DAEMON" | /usr/bin/grep -q 'arm64'; then
  echo "error: embedded daemon is not arm64" >&2
  exit 1
fi
if ! /usr/bin/cmp -s "$DAEMON_PATH" "$EMBEDDED_DAEMON"; then
  echo "error: embedded daemon differs from the freshly built daemon" >&2
  exit 1
fi

echo "==> Verify signatures"
/usr/bin/codesign --verify --strict --verbose=2 "$EMBEDDED_DAEMON"
/usr/bin/codesign --verify --deep --strict --verbose=2 "$OUTPUT_APP_PATH"

for signed_product in "$EMBEDDED_DAEMON" "$OUTPUT_APP_PATH"; do
  signature_information="$(/usr/bin/codesign --display --verbose=4 "$signed_product" 2>&1)"
  if ! /usr/bin/grep -Fxq 'Signature=adhoc' <<< "$signature_information"; then
    echo "error: expected an ad-hoc signature on $signed_product" >&2
    exit 1
  fi
done

echo "==> Built $OUTPUT_APP_PATH"
