#!/usr/bin/env bash
set -euo pipefail

PROJECT_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
MACOS_DIRECTORY="$PROJECT_ROOT/macos"
PROJECT_PATH="$MACOS_DIRECTORY/GrokRemoteLauncher.xcodeproj"
DERIVED_DATA_DIRECTORY="$PROJECT_ROOT/dist/macos-derived-data"
DAEMON_PATH="$PROJECT_ROOT/dist/grok-remote-daemon"
BUILT_APP_PATH="$DERIVED_DATA_DIRECTORY/Build/Products/Release/Grok Remote Launcher.app"
OUTPUT_APP_PATH="$PROJECT_ROOT/dist/Grok Remote Launcher.app"

for required_tool in go xcodegen xcodebuild; do
  if ! command -v "$required_tool" >/dev/null 2>&1; then
    echo "error: required tool not found: $required_tool" >&2
    exit 1
  fi
done

echo "==> Go quality gate"
"$PROJECT_ROOT/scripts/check-go-quality.sh"

echo "==> Build arm64 Grok Remote daemon"
mkdir -p "$PROJECT_ROOT/dist"
(
  cd "$PROJECT_ROOT/backend"
  CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 \
    go build -trimpath -o "$DAEMON_PATH" ./cmd/grok-remote-daemon
)
chmod 0755 "$DAEMON_PATH"

if ! /usr/bin/file "$DAEMON_PATH" | /usr/bin/grep -q 'arm64'; then
  echo "error: daemon is not an arm64 Mach-O executable" >&2
  /usr/bin/file "$DAEMON_PATH" >&2
  exit 1
fi

echo "==> Generate macOS Xcode project"
xcodegen generate \
  --spec "$MACOS_DIRECTORY/project.yml" \
  --project "$MACOS_DIRECTORY"

echo "==> Build macOS launcher"
rm -rf "$DERIVED_DATA_DIRECTORY"
xcodebuild \
  -project "$PROJECT_PATH" \
  -scheme GrokRemoteLauncher \
  -configuration Release \
  -destination 'generic/platform=macOS' \
  -derivedDataPath "$DERIVED_DATA_DIRECTORY" \
  ARCHS=arm64 \
  ONLY_ACTIVE_ARCH=NO \
  CODE_SIGNING_ALLOWED=NO \
  CODE_SIGNING_REQUIRED=NO \
  build

if [[ ! -d "$BUILT_APP_PATH" ]]; then
  echo "error: expected app was not produced at $BUILT_APP_PATH" >&2
  exit 1
fi

rm -rf "$OUTPUT_APP_PATH"
/usr/bin/ditto "$BUILT_APP_PATH" "$OUTPUT_APP_PATH"

LAUNCHER_EXECUTABLE="$OUTPUT_APP_PATH/Contents/MacOS/Grok Remote Launcher"
EMBEDDED_DAEMON="$OUTPUT_APP_PATH/Contents/Resources/Daemon/grok-remote-daemon"
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

echo "==> Built $OUTPUT_APP_PATH"
