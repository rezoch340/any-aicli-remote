#!/usr/bin/env bash
set -euo pipefail

PROJECT_ROOT="$(cd "$(dirname "$0")/.." && pwd)"

echo "==> Go backend quality and macOS launcher build"
"$PROJECT_ROOT/scripts/build-macos-app.sh"

echo "==> iOS simulator build"
if ! xcodebuild \
    -project "$PROJECT_ROOT/ios/GrokRemote.xcodeproj" \
    -scheme GrokRemote \
    -sdk iphonesimulator \
    -destination 'generic/platform=iOS Simulator' \
    CODE_SIGNING_ALLOWED=NO \
    build; then
  echo "==> iOS Simulator runtime unavailable; type-checking all Swift sources instead"
  SIMULATOR_SDK_PATH="$(xcrun --sdk iphonesimulator --show-sdk-path)"
  find "$PROJECT_ROOT/ios/GrokRemote" -name '*.swift' -print0 | \
    xargs -0 xcrun --sdk iphonesimulator swiftc \
      -typecheck \
      -target arm64-apple-ios17.0-simulator \
      -sdk "$SIMULATOR_SDK_PATH" \
      -module-name GrokRemote
fi

echo "==> Android debug build"
"$PROJECT_ROOT/android/gradlew" -p "$PROJECT_ROOT/android" :app:assembleDebug
