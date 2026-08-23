#!/usr/bin/env bash
set -euo pipefail
readonly REPOSITORY_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
readonly ANDROID_ROOT="$REPOSITORY_ROOT/android"
readonly ADB_BINARY="${ADB:-adb}"
readonly DAEMON_PORT=2421
readonly TARGET_PACKAGE="com.anyaicliremote.app"
readonly TEST_PACKAGE="${TARGET_PACKAGE}.test"
readonly TEST_CLASS="com.anyaicliremote.app.RealDaemonLiveInstrumentedTest"
cleanup() { "$ADB_BINARY" shell pm clear "$TARGET_PACKAGE" >/dev/null 2>&1 || true; "$ADB_BINARY" shell pm clear "$TEST_PACKAGE" >/dev/null 2>&1 || true; "$ADB_BINARY" reverse --remove "tcp:$DAEMON_PORT" >/dev/null 2>&1 || true; }
trap cleanup EXIT
"$ADB_BINARY" get-state >/dev/null
curl --fail --silent --show-error "http://127.0.0.1:$DAEMON_PORT/health" >/dev/null
(
  cd "$ANDROID_ROOT"
  ./gradlew --no-daemon :app:assembleDebug :app:assembleDebugAndroidTest :app:compileDebugAndroidTestKotlin
)
readonly APK_PATH="$(find "$ANDROID_ROOT/app/build/outputs/apk" -name '*debug.apk' -not -name '*androidTest*' | head -n1)"
readonly TEST_APK_PATH="$(find "$ANDROID_ROOT/app/build/outputs/apk" -name '*debug-androidTest.apk' | head -n1)"
"$ADB_BINARY" install -r "$APK_PATH" >/dev/null
"$ADB_BINARY" install -r -t "$TEST_APK_PATH" >/dev/null
security find-generic-password -s com.anyaicliremote.launcher -a pairing-secret -w | "$ADB_BINARY" shell run-as "$TARGET_PACKAGE" sh -c 'umask 077; mkdir -p /data/data/com.anyaicliremote.app/files; cat > /data/data/com.anyaicliremote.app/files/live-e2e-pairing-key'
"$ADB_BINARY" reverse "tcp:$DAEMON_PORT" "tcp:$DAEMON_PORT"
"$ADB_BINARY" shell am instrument -w -e class "$TEST_CLASS" "$TEST_PACKAGE/androidx.test.runner.AndroidJUnitRunner"
