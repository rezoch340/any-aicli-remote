#!/usr/bin/env bash
set -euo pipefail

REPOSITORY_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
ANDROID_ROOT="$REPOSITORY_ROOT/android"
ADB_BINARY="${ADB:-adb}"
TEMP_ROOT="$(mktemp -d "${TMPDIR:-/tmp}/any-aicli-android-e2e.XXXXXX")"
REVERSE_PORT="$(python3 - <<'PY'
import socket

with socket.socket() as probe:
    probe.bind(("", 0))
    print(probe.getsockname()[1])
PY
)"
DAEMON_PROCESS_ID=""

cleanup() {
    if [[ -n "$DAEMON_PROCESS_ID" ]]; then
        kill "$DAEMON_PROCESS_ID" 2>/dev/null || true
        wait "$DAEMON_PROCESS_ID" 2>/dev/null || true
    fi
    "$ADB_BINARY" reverse --remove "tcp:$REVERSE_PORT" >/dev/null 2>&1 || true
    rm -rf "$TEMP_ROOT"
}
trap cleanup EXIT

if ! "$ADB_BINARY" get-state >/dev/null 2>&1; then
    echo "No connected Android device or emulator is available." >&2
    exit 1
fi

"$ADB_BINARY" reverse "tcp:$REVERSE_PORT" "tcp:$REVERSE_PORT"

# Optional real-daemon hook. The file is executed without putting its contents,
# credentials, or provider arguments into this script's process arguments.
readonly DEFAULT_REAL_DAEMON_START_WAIT_SECONDS=1
if [[ -n "${REAL_DAEMON_START_FILE:-}" ]]; then
    if [[ ! -f "$REAL_DAEMON_START_FILE" ]]; then
        echo "REAL_DAEMON_START_FILE does not exist." >&2
        exit 1
    fi
    (
        cd "$TEMP_ROOT"
        REAL_DAEMON_PORT="$REVERSE_PORT" exec bash "$REAL_DAEMON_START_FILE"
    ) >"$TEMP_ROOT/daemon.log" 2>&1 &
    DAEMON_PROCESS_ID=$!
    sleep "${REAL_DAEMON_START_WAIT_SECONDS:-$DEFAULT_REAL_DAEMON_START_WAIT_SECONDS}"
fi

(
    cd "$ANDROID_ROOT"
    ./gradlew --no-daemon \
        :app:assembleDebug \
        :app:assembleDebugAndroidTest \
        :app:testDebugUnitTest
    ./gradlew --no-daemon :app:connectedDebugAndroidTest
)
