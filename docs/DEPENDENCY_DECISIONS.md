# Dependency and reuse decisions

This file is a quality gate, not a dependency wish list. Before implementing non-trivial
infrastructure, check the repository, the standard library or platform framework, and maintained
libraries. Keep only the smallest compatible dependency or adapter.

## Agent Client Protocol

- Checked: [`coder/acp-go-sdk`](https://github.com/coder/acp-go-sdk), the published Apache-2.0 Go
  SDK for ACP, currently pinned to `v0.13.5`.
- Decision: reuse its typed ACP requests, responses, extension helpers, and protocol behavior wherever
  Grok follows ACP.
- Boundary: Grok-only extension methods stay in the Grok `ProtocolAdapter`; do not duplicate the
  standard ACP request, response, content, session, or notification model in the generic core.
- Exception process: any unsupported Grok extension must be documented and implemented as a thin
  extension around the SDK rather than as a second JSON-RPC stack.

## Grok history

- Checked: [CC-Switch](https://github.com/farion1231/cc-switch) Provider/session-manager design.
- Decision: reuse its proven metadata-first behavior and active/archived source rules as prior art,
  while implementing the parser against Grok's local files in the Grok adapter.
- Shared behavior: timestamp normalization, deterministic deduplication, pagination, canonical path
  containment, and normalized metadata belong to the generic history core and must have one implementation.
- Provider behavior: Grok paths, `summary.json`, `chat_history.jsonl`, and Grok content variants belong
  only to the Grok adapter.
- Implementation boundary: `GrokProvider` is the sole session-directory discovery and summary reader.
  `historydata` contains only the `updates.jsonl` reader used by that adapter; do not add a second Store,
  directory index, working-directory encoder, or summary parser beside it.

## Archived-session compatibility

- Checked: the existing `compat`/`sessionapi` code and Go `encoding/json`.
- Decision: the current `[]string` and legacy `{ids:[...]}` forms are two small, fixed JSON shapes; no
  third-party dependency or hand-written token parser is needed.
- Boundary: `compat.ParseArchivedSessionIDs` is the single reader. Migration and `SetArchived` write only
  the current array through `atomicfile` private atomic writes; other packages must not duplicate compatibility parsing.

## JSON Lines

- Checked: third-party JSONL packages and the Go standard library.
- Decision: use `bufio` plus `encoding/json`; the format is line-delimited JSON and does not justify a
  separate parser dependency.
- Prohibited: custom JSON tokenization, hand-written escaping, or loading an entire history file when
  a bounded streaming scan is sufficient.

## Filesystem confinement and workspace identity

- Checked: path-string containment helpers, third-party sandbox filesystems, and the Go standard
  library's [`os.Root`](https://pkg.go.dev/os#Root), `os.Lstat`, and `os.SameFile` APIs.
- Decision: use `os.Root` for file-descriptor-relative provider history and workspace access. `os.SameFile`
  and `RootIdentity` bind workspace identity and enforce file/Git boundaries; terminal launch uses the
  opened directory fd plus `Fchdir` and `Exec` from the next section to retain the same directory object.
- Boundary: do not add another path sandbox or inode abstraction. Provider/session roots reject symlink
  components, compare directory identity before and after `OpenRoot`, and use relative file names only.

## Terminal working-directory handoff

- Checked `Cmd.Dir`, `/dev/fd`, `ExtraFiles`, `x/sys/unix.Fchdir`, and `moby/sys/reexec` v0.1.0.
- macOS cannot `chdir` via `/dev/fd/3`; path-based `Cmd.Dir` permits TOCTOU races.
- Use `os.Root` + `ExtraFiles` (fd 3) + `reexec` + `Fchdir` + `Exec`; do not use string-path rechecks or a shell `/dev/fd` wrapper instead of the fd-pinned handoff.
- Boundary: provider-origin only, for an existing session/workspace; public APIs cannot invoke this handoff directly.

## YAML skill frontmatter

- Checked: the maintained [`yaml/go-yaml`](https://github.com/yaml/go-yaml) project, its
  `go.yaml.in/yaml/v3` module, the officially recommended `go.yaml.in/yaml/v4` module, and
  [`adrg/frontmatter`](https://github.com/adrg/frontmatter).
- Maintenance finding: the go-yaml maintainers state that v3 is limited to security fixes and direct
  new development to v4. `adrg/frontmatter` has not had a release since 2020 and would add a second
  parser abstraction for delimiter detection that is only a few lines here.
- Decision: pin `go.yaml.in/yaml/v4` at `v4.0.0-rc.6` and decode into one typed frontmatter struct.
  Application code only locates the leading `---` block; it must not parse YAML scalars, quoting,
  booleans, or block strings itself.
- Filesystem boundary: provider adapters supply every discovery root as a typed `(kind, source, path)`
  declaration. The generic scanner never infers `bundled`, `plugin`, or command behavior from directory
  names. It canonicalizes a configured root once, rejects nested or file symlinks, and verifies a regular
  file's identity across `Lstat`, `Open`, and `Stat` before reading it.

## Provider voice transport

- Checked: xAI's official [Python SDK](https://github.com/xai-org/xai-sdk-python), public
  [protocol definitions](https://github.com/xai-org/xai-proto), and its official
  [Text to Speech documentation](https://docs.x.ai/developers/models/text-to-speech), plus community
  Go SDKs.
- Compatibility finding: xAI currently publishes an official Python SDK, not an official Go SDK, and
  the provider's TTS endpoint is a single authenticated HTTP request. Pulling in an unofficial
  general-purpose Go SDK or generated gRPC surface would be materially larger than this use.
- Decision: retain the Go standard library HTTP client for this one endpoint. Keep the request payload,
  provider credential discovery, endpoint, voice list, and response details inside the Grok adapter;
  the generic core exposes only the small `voice.Service` contract.

## Process and listener inspection

- Checked: the Go standard library, operating-system text tools (`tasklist`, `wmic`, `ps`, `lsof`,
  and `ss`), and the maintained BSD-3-Clause
  [`shirou/gopsutil`](https://github.com/shirou/gopsutil) v4 API. The latest stable release checked was
  [`v4.26.7`](https://github.com/shirou/gopsutil/releases/tag/v4.26.7).
- Decision: use `process.NewProcess` with `Cmdline`, `CreateTime`, and `IsRunning`, plus
  `net.Connections("tcp")`, for all read-only process identity and listener discovery. Do not restore
  parallel operating-system command output parsers.
- Measured compatibility: on the supported macOS host, an unprivileged process successfully enumerated
  connections and resolved a newly opened local TCP listener to its own process ID. Tests exercise both
  the current process and a real ephemeral listener. Listener records without a process owner fail closed.
- State migration: `ProcessStart` formats gopsutil's creation timestamp in the existing Unix `ps lstart`
  representation (and the previous Windows WMI representation), so a daemon upgrade does not mistake an
  already managed process for a different owner.
- Boundary: `os/exec` remains only for starting the provider process and the existing Windows termination
  operation; Unix signalling and the ownership state machine remain application lifecycle behavior.

## WebSocket transport

- Existing dependency: [`gorilla/websocket`](https://github.com/gorilla/websocket).
- Decision: reuse the existing transport during the generic-core refactor; do not introduce a second
  WebSocket implementation. A migration requires a measured transport problem and one repository-wide
  replacement, never parallel stacks.

## Child-process log redaction

- Checked: AWS Labs [`ferret-scan`](https://github.com/awslabs/ferret-scan), Docker
  [`portcullis`](https://github.com/docker/portcullis), and structured-logger redaction middleware.
- Compatibility finding: those packages discover broad classes of unknown secrets or wrap structured log
  records. This boundary instead receives one already-known, ephemeral transport secret in arbitrary raw
  child-process bytes; a detector is materially larger and can still miss an otherwise valid random literal.
- Decision: keep one bounded standard-library `io.Writer` in the process package that matches only supplied
  sensitive literals, retains enough trailing bytes to handle secrets split across writes, and writes to a
  mode `0600` log. Provider adapters use environment variables supported by the provider CLI and must not put
  transport secrets in process arguments. Tests cover split writes and a real child process.
- Boundary: do not add provider-specific redaction or regex secret detectors. If output needs unknown-secret
  discovery later, replace this one sink repository-wide with a maintained detector rather than layering both.

## Go identifier quality gate

- Checked: [`varnamelen`](https://github.com/blizzy78/varnamelen), including its maintained
  `golangci-lint` integration, before retaining a repository-specific check.
- Incompatibility: `varnamelen` intentionally makes scope-based decisions and always exempts conventional
  names such as `ctx` and `t`. This repository's explicit contract is different: declarations shorter than
  three characters fail regardless of scope (except the documented technical terms), and a project-specific
  list of compressed words such as `ctx`, `req`, and `resp` must also fail. It also covers declaration kinds
  outside `varnamelen`'s variable-focused rule.
- Decision: keep the small standard-library `go/ast` gate for only those exact rules. Do not add general
  lint checks to it, duplicate compiler/vet analysis, or turn it into a custom linter framework. Re-evaluate
  this exception if an established analyzer gains strict, configurable exemptions and forbidden-word rules.

## Go atomic private-state persistence

Configuration and private state persistence use `github.com/google/renameio/v2` for complete-file
atomic replacement. It was selected over `creachadair/atomicfile` and direct `os.Rename`: renameio
provides a maintained temporary-file-and-rename workflow with explicit permissions. The daemon target
is macOS and Linux; renameio's documented platform support does not include Windows.

## Apple generic-password storage

- Checked: Apple's Security framework documentation for
  [`kSecClassGenericPassword`](https://developer.apple.com/documentation/security/ksecclassgenericpassword),
  [`SecItemAdd`](https://developer.apple.com/documentation/security/secitemadd(_:_:)),
  [`SecItemCopyMatching`](https://developer.apple.com/documentation/security/secitemcopymatching(_:_:)),
  and [TN3137 on macOS keychains](https://developer.apple.com/documentation/technotes/tn3137-on-mac-keychains).
  Apple's APIs already provide encrypted generic-password CRUD, duplicate-item status handling, access
  groups, and portable data-protection-keychain behavior, so a third-party wrapper would duplicate the
  platform contract.
- Decision: for this macOS Launcher, compile and reuse the single
  `apple/Shared/GenericPasswordStore.swift` SecItem primitive. It keys items by service, account, and
  optional access group, uses `kSecAttrAccessibleAfterFirstUnlockThisDeviceOnly`, and opts into
  `kSecUseDataProtectionKeychain` as Apple recommends for cross-platform SecItem behavior. Platform stores
  own only their current/legacy namespaces and migration order; they must not duplicate SecItem dictionaries
  or CRUD.
- macOS policy: a provisioning profile must authorize the restricted keychain access-group entitlement used
  by the data-protection keychain. A pure ad-hoc signature has no such authorization and returns
  `errSecMissingEntitlement`. The launcher therefore uses a file-based fallback only for that exact status.
  When the preferred data-protection item is absent, reads also probe for a prior file-based item and migrate
  it automatically when a later profile-authorized build can access data protection. Other SecItem failures
  always propagate.
- macOS authority: the launcher's pairing secret lives in Keychain. It may read an existing current or
  legacy `0600` file exactly once, write that value to Keychain, and then remove both persistent files. New
  secrets are written only to Keychain.
- daemon boundary: because the daemon accepts `--pairing-secret-file`, the launcher materializes a uniquely named
  `0600` file in the user's temporary directory and passes only that path. The launcher deletes the file
  after a successful health check, a launch failure, or process termination. The secret itself must never
  appear in process arguments or logs. Authenticated launcher requests, including `/config.json`, send only
  the current `X-Any-AI-CLI-Remote-Key`; legacy header names remain read-only migration data and are never
  emitted by first-party clients.
- Local macOS signing: use Xcode and the platform `codesign` tool rather than a custom signing layer. This
  TODO's build script fixes the `-` pseudo-identity for the local ad-hoc signature and provides no optional
  profile or identity switches. The script signs the daemon first, embeds it in the standard `Contents/MacOS`
  nested-code location, and lets Xcode sign the outer app last. `--deep` is used only for final verification,
  not signing, following Apple's
  [nested-code signing guidance](https://developer.apple.com/documentation/xcode/creating-distribution-signed-code-for-the-mac)
  and [code-signing build settings](https://developer.apple.com/documentation/xcode/build-settings-reference).
  Developer ID signing, provisioning, and notarization are deferred to the final-release TODO.

## macOS Launcher process and HTTP lifecycle dependencies

- Checked: Apple Foundation documentation for [`Process.executableURL`](https://developer.apple.com/documentation/foundation/process/executableurl), [`Process.arguments`](https://developer.apple.com/documentation/foundation/process/arguments), [`Process.environment`](https://developer.apple.com/documentation/foundation/process/environment), [`Process.terminationHandler`](https://developer.apple.com/documentation/foundation/process/terminationhandler), [`URLSessionConfiguration`](https://developer.apple.com/documentation/foundation/urlsessionconfiguration), and [`FileHandle.availableData`](https://developer.apple.com/documentation/foundation/filehandle/availabledata).
- Decision: continue using Foundation `Process`, `URLSession`, and `JSONSerialization` with the existing XcodeGen setup. Do not add third-party process, networking, or configuration dependencies: the standard library covers these requirements, while external libraries would duplicate capabilities and expand the dependency surface.
- Boundary: the Launcher does not invoke a shell; it starts the fixed daemon only through `executableURL` plus an argument array. Configuration show/validate/apply is provided by the daemon CLI. HTTP operations use fixed, typed endpoints. Runtime parameters come from `LauncherPolicy.json`, and the pairing URL is returned by the daemon.
