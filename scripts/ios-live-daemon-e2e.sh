#!/usr/bin/env bash
set -euo pipefail

simulator_id=${IOS_SIMULATOR_ID:?Set IOS_SIMULATOR_ID to the target simulator UUID.}
keychain_service=${ANY_AI_CLI_REMOTE_LAUNCHER_KEYCHAIN_SERVICE:-com.anyaicliremote.launcher}
keychain_account=${ANY_AI_CLI_REMOTE_LAUNCHER_KEYCHAIN_ACCOUNT:-pairing-secret}
workspace_root=$(cd "$(dirname "$0")/.." && pwd)
secret_directory=$(mktemp -d "${TMPDIR:-/tmp}/any-ai-cli-ios-e2e.XXXXXX")
secret_file="$secret_directory/pairing-key"

cleanup() {
  rm -f "$secret_file"
  rmdir "$secret_directory" 2>/dev/null || true
}
trap cleanup EXIT HUP INT TERM
umask 077
security find-generic-password -s "$keychain_service" -a "$keychain_account" -w > "$secret_file"
chmod 600 "$secret_file"

xcodebuild -skipMacroValidation \
  -project "$workspace_root/ios/AnyAICLIRemote.xcodeproj" \
  -scheme AnyAICLIRemoteLiveE2E \
  -destination "platform=iOS Simulator,id=$simulator_id" \
  -only-testing:AnyAICLIRemoteUITests/DevicePairingUITests/testPairAndOpenSessionListAgainstLiveDaemon \
  -only-testing:AnyAICLIRemoteUITests/DevicePairingUITests/testStreamingResponseAutoScrollAgainstLiveDaemon \
  -only-testing:AnyAICLIRemoteUITests/DevicePairingUITests/testChildAgentCardsAgainstLiveDaemon \
  ANY_AI_CLI_REMOTE_LIVE_PAIRING_KEY_FILE="$secret_file" \
  test
