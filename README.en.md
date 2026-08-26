[简体中文](README.md) | English

# Any AI CLI Remote

[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)
[![Go backend quality](https://github.com/rezoch340/any-aicli-remote/actions/workflows/go-backend-quality.yml/badge.svg)](https://github.com/rezoch340/any-aicli-remote/actions/workflows/go-backend-quality.yml)
![iOS 17+](https://img.shields.io/badge/iOS-17%2B-000000?logo=apple)
![Android SDK 35+](https://img.shields.io/badge/Android-SDK%2035%2B-3DDC84?logo=android&logoColor=white)
![macOS Apple Silicon](https://img.shields.io/badge/macOS-Apple%20Silicon-000000?logo=apple)

**Native mobile clients and a standalone Go daemon for remotely using an AI CLI on your own machine.**

Any AI CLI Remote is open-source software. It pairs native iOS and Android clients with a host-side daemon that owns authentication, provider lifecycle, session metadata, workspace isolation, history indexing, and reverse RPC for files and terminals. The core is designed around provider adapters, but **grok is currently the only implemented provider**; there are no placeholder integrations for other CLIs.

> [!IMPORTANT]
> This repository does not publish a notarized macOS binary. The macOS launcher is built locally and uses ad-hoc signing by default.

## Overview

The daemon runs beside the AI CLI, while the mobile app connects over HTTP and WebSocket. Pairing records a server URL, device name, and key; it does not select a workspace. A workspace belongs to a session: existing sessions restore their recorded workspace, and new sessions must provide one explicitly.

The project keeps provider-specific commands, RPC methods, capabilities, and on-disk history formats inside provider adapters. Shared code handles cross-provider concerns such as authentication, session routing, canonical path validation, pagination, time normalization, and compatibility migration.

## Features

- Native SwiftUI client for iOS and Jetpack Compose client for Android.
- Streaming chat with Markdown, code blocks, tables, thinking, tool calls, permission prompts, and child-agent status.
- Multiple saved device profiles, QR/deep-link pairing, session history, session creation/loading/cancellation, and reconnect handling.
- Session-scoped workspace access for files, terminals, Git, skills, and project operations.
- A Go daemon with authenticated REST/WebSocket transport and provider lifecycle management.
- A macOS SwiftUI launcher that embeds and manages the arm64 daemon.
- Grok active and archived history indexing with deterministic pagination and defensive path validation.

## Architecture

```text
Native iOS / Android clients
            │ HTTP + WebSocket
            ▼
Any AI CLI Remote daemon
  ├─ authentication and pairing
  ├─ provider registry and lifecycle
  ├─ session metadata and workspace isolation
  ├─ history pagination
  ├─ reverse file and terminal RPC
  └─ Provider + ProtocolAdapter
             └─ grok
                 ├─ grok agent serve lifecycle
                 ├─ Grok JSON-RPC mapping
                 └─ active + archived history reader
```

```text
backend/    Go daemon and provider adapters
docs/       Transport protocol and provider-boundary documentation
ios/        SwiftUI client
android/    Kotlin and Jetpack Compose client
macos/      SwiftUI launcher
scripts/    Build and mandatory quality gates
```

The daemon starts provider services idle; it does not create, load, or resume a session at startup. There is no daemon-global workspace. Provider-specific Grok RPC methods, command-line arguments, and `~/.grok` storage knowledge remain in the Grok adapter.

The Grok history adapter scans both `~/.grok/sessions` and `~/.grok/archived_sessions`. It builds lightweight metadata from `summary.json` and reads `chat_history.jsonl` only when messages are requested. Results are ordered by last activity and paginated; deterministic rules handle duplicate active/archive entries, timestamps, rich-text extraction, and damaged records. Source paths are canonicalized, constrained to allowed history roots, and checked against the requested on-disk session ID.

See [`docs/PROTOCOL.md`](docs/PROTOCOL.md) for the transport contract and [`docs/DEPENDENCY_DECISIONS.md`](docs/DEPENDENCY_DECISIONS.md) for dependency decisions.

## Security model

- Only the shallow `/health` endpoint is public. `/health/deep` is authenticated because it can trigger provider connection or startup probes.
- REST requests use `X-Any-AI-CLI-Remote-Key`; WebSocket connections authenticate with `/ws?key=...`.
- Pairing keys are stored through platform credential storage and supplied to the daemon through credential storage, `*-secret-file` options, or environment variables. The daemon intentionally does not accept plaintext secret command-line arguments that would leak through process listings or shell history.
- File and history paths are canonicalized and checked against the session workspace or permitted provider history roots before access.
- New data uses Any AI CLI Remote identifiers. Legacy Grok Remote identifiers are limited to centralized compatibility reads and migration code.
- The launcher only stops processes whose ownership it can establish.

Treat the daemon as access to the selected workspaces and CLI account. Bind it only to interfaces you intend to expose, protect transport appropriately for your network, keep pairing material secret, and review permission prompts before approving them.

Please report vulnerabilities privately through the repository's [Security Advisories](https://github.com/rezoch340/any-aicli-remote/security/advisories/new), not through a public issue. See [SECURITY.md](SECURITY.md).

## Quick start

### 1. Build the Go daemon

Requirements: Go 1.25+ and an installed, authenticated Grok CLI.

```bash
cd backend
../scripts/check-go-quality.sh
go build -trimpath -o ../dist/any-aicli-remote-daemon ./cmd/any-aicli-remote-daemon
```

The quality gate runs naming and source-size checks, `gofmt`, all Go tests, the race detector, and `go vet`. GitHub Actions runs the same script.

### 2. Start the daemon

```bash
./dist/any-aicli-remote-daemon \
  --public-host https://remote.example.com:24443
```

Defaults are port `2421` for the daemon and `2419` for the Grok provider service. First launch prepares runtime data and pairing material under `~/.any-aicli-remote/`. There is no `--cwd`; a client supplies the workspace when creating a session.

```bash
curl http://127.0.0.1:2421/health
curl -H 'X-Any-AI-CLI-Remote-Key: <pairing-key>' \
  http://127.0.0.1:2421/health/deep
```

### 3. Pair a client

A pairing URL normally has this form:

```text
http://<server-ip>:2421/?key=<pairing-key>&auto=1
```

The QR code uses:

```text
anyaicliremote://pair?url=<encoded-base-url>&key=<encoded-key>&name=<encoded-device-name>
```

The deep link intentionally contains no workspace. Each device is stored as a separate profile, with its key in platform secure storage.

## Platform setup

### iOS

Requirements: Xcode 16+, XcodeGen, and an iOS 17+ target.

```bash
cd ios
xcodegen generate
open AnyAICLIRemote.xcodeproj
```

For an unsigned generic Simulator build:

```bash
xcodebuild -project AnyAICLIRemote.xcodeproj \
  -scheme AnyAICLIRemote \
  -destination 'generic/platform=iOS Simulator' \
  CODE_SIGNING_ALLOWED=NO build
```

### Android

Requirements: Android Studio, JDK 17+, and Android SDK 35+.

```bash
cd android
./gradlew testDebugUnitTest :app:assembleDebug :app:lintDebug
```

The debug APK is written to `android/app/build/outputs/apk/debug/app-debug.apk`.

### macOS launcher

Requirements: an Apple Silicon Mac, Xcode 16+, Go 1.25+, XcodeGen, and an installed/authenticated Grok CLI.

```bash
./scripts/build-macos-app.sh
open "dist/Any AI CLI Remote Launcher.app"
```

The launcher embeds the arm64 daemon and configures the device name, daemon/provider ports, bind address, optional public URL, and provider permission mode. It does not choose a workspace.

The default local build is ad-hoc signed. The script signs the daemon before embedding it, signs the outer app through Xcode, and verifies both with `codesign --verify --deep --strict`. Because an ad-hoc signature has no provisioning-profile Keychain access group, the launcher falls back to its file-based SecItem Keychain only when the system returns `errSecMissingEntitlement`; other Keychain errors remain visible. Updating an ad-hoc build may cause macOS to ask again for login-keychain access.

A properly provisioned development identity, team, and profile can enable the repository's Data Protection Keychain entitlements, but **no Developer ID-signed or notarized macOS distribution is published by this project**.

## Development

Run the complete local validation entry point on macOS:

```bash
./scripts/build-all.sh
```

It builds the macOS launcher and Go daemon, runs the Go quality gate, runs Android static analysis/unit tests/debug builds/lint, generates the iOS project, checks native source quality, builds for a generic iOS Simulator, and runs iOS Simulator tests. Connected Android E2E is opt-in:

```bash
RUN_ANDROID_CONNECTED_E2E=1 \
IOS_SIMULATOR_DESTINATION='platform=iOS Simulator,id=<simulator-id>' \
./scripts/build-all.sh
```

For backend-only changes, run:

```bash
./scripts/check-go-quality.sh
```

When changing behavior shared by platforms, update and test the backend, iOS, and Android representations together. Preserve the provider boundary: reusable policy belongs in shared code, while Grok-specific protocol and storage behavior belongs in the Grok adapter.

## Contributing

Issues and pull requests are welcome:

- [Open an issue](https://github.com/rezoch340/any-aicli-remote/issues/new/choose) for a reproducible bug or a focused feature proposal.
- [Open a pull request](https://github.com/rezoch340/any-aicli-remote/compare) for a reviewed, tested change.

Before contributing, read [CONTRIBUTING.md](CONTRIBUTING.md), the [Code of Conduct](CODE_OF_CONDUCT.md), and [SECURITY.md](SECURITY.md). Please search existing issues first, keep changes focused, reuse existing cross-stack abstractions, add relevant tests, and avoid including pairing keys, user content, private paths, or other sensitive data in reports and fixtures.

## Project status

Any AI CLI Remote is under active development. The source is open, and the current implementation supports **grok only**. The provider boundary is designed for future adapters, but support for any other AI CLI should not be assumed until an implementation is merged and documented.

Platform clients and the macOS launcher are built from source. There is currently no notarized macOS binary release.

## License

Licensed under the [MIT License](LICENSE).
