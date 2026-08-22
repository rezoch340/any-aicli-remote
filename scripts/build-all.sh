#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"

echo "==> iOS simulator build"
if ! xcodebuild \
    -project "$ROOT/ios/GrokRemote.xcodeproj" \
    -scheme GrokRemote \
    -sdk iphonesimulator \
    -destination 'generic/platform=iOS Simulator' \
    CODE_SIGNING_ALLOWED=NO \
    build; then
  echo "==> iOS Simulator runtime unavailable; type-checking all Swift sources instead"
  SDK="$(xcrun --sdk iphonesimulator --show-sdk-path)"
  find "$ROOT/ios/GrokRemote" -name '*.swift' -print0 | \
    xargs -0 xcrun --sdk iphonesimulator swiftc \
      -typecheck \
      -target arm64-apple-ios17.0-simulator \
      -sdk "$SDK" \
      -module-name GrokRemote
fi

echo "==> Android debug build"
"$ROOT/android/gradlew" -p "$ROOT/android" :app:assembleDebug
