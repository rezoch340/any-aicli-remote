# Any AI CLI Remote implementation checklist

Every top-level item is one reviewable feature boundary and one Git commit. Do not combine two
unfinished top-level features in one commit. Mark an item complete only after its required tests pass,
then include the checkbox update in that feature's commit.

- [x] **0. Provider 中立后端基线**
  - [x] Establish daemon-idle startup without creating, loading, or resuming sessions; provide per-session workspaces, Grok history, path isolation, secret handling, a public API that never directly forwards to terminal execution, and Go naming/source-size gates.
  - [x] Required evidence is limited to Go quality, lifecycle/session/reverse-security tests, private-identifier scans, and `git diff --check`; this item must not include macOS, iOS, or Android validation.
  - Commit: `♻️ 后端：建立 Provider 中立基线`

- [x] **1. 统一 daemon 配置**
  - [x] Add a versioned, non-secret typed JSON document with daemon-owned defaults, normalization, validation, migration, atomic persistence, and deterministic source precedence.
  - [x] Move operational ports, addresses, paths, timeouts, polling/retry intervals, retention, and resource limits out of call-site literals and inject validated tuning into server, hub, and provider process code.
  - [x] Add side-effect-free `config show`, `config validate`, and `config apply` commands. They must not start the Provider, create a session, or create secret material.
  - Commit: `✨ 配置：建立统一守护进程配置`

- [x] **2. macOS Launcher + CLI 共用配置**
  - [x] Freeze and validate the daemon configuration commands and serialized schema from item 1 before changing Launcher source; replace Launcher-owned defaults and `UserDefaults` service settings with the canonical JSON configuration and daemon validation path.
  - [x] Preserve existing configuration integration, migration, and secret rules. Launch with only the configuration path plus the temporary permission-restricted secret file; keep pairing-link generation and effective runtime data daemon-owned.
  - [x] Add local Launcher functional build/start/stop/restart and QR pairing payload verification. Platform-required ad-hoc signing is only a local build mechanism, not Release signing acceptance.
  - [x] This item must not include Android or iOS.
  - Commit: `♻️ macOS：接入统一守护进程配置`

- [ ] **3. 真实 Grok 的后端+Launcher E2E**
  - [ ] Against the real Grok CLI, prove idle startup has no session, then verify new/load/resume lifecycle, workspace isolation, streaming, cancel, permission, reconnect, file, terminal, and archived history behavior.
  - [ ] Verify Launcher start/stop/restart together with status and QR pairing payload behavior.
  - [ ] This item must not modify or validate Android or iOS; freeze the backend+Launcher E2E contract before clients begin.
  - Commit: `🧪 后端：验证 Grok 与启动器生命周期`

- [ ] **4. 原生客户端功能与 E2E**
  - [ ] Begin only after items 0–3 are completed, checked off, committed, and the protocol contract is frozen. Android first: unit/debug/lint plus connected-device E2E; iOS afterward: unsigned simulator build/tests.
  - [ ] Cover multi-device persistent pairing, disconnected-device navigation, session workspaces, history, streaming/cancel/permission/reconnect, rich-text/file behavior, and current brand naming.
  - Commit: `✨ 客户端：完成原生端功能与联调`

- [ ] **5. 子 Agent 实时卡片**
  - [ ] Backend first: represent each child Agent as a stable typed entity and emit ordered lifecycle/update events rather than asking clients to parse terminal or Markdown text.
  - [ ] Prove backend event identity, ordering, reconnect replay, history reconstruction, completion, failure, and cancellation with protocol tests before touching app UI.
  - [ ] Android next and iOS after it: render the typed card and validate concurrent child Agents, reconnect/history reconstruction, out-of-order updates, completion, failure, and cancellation on both platforms.
  - Commit: `✨ 聊天：新增子 Agent 实时状态卡片`

- [ ] **6. 最终发布**
  - [ ] Run the complete Go/macOS/iOS/Android pipeline, privacy and legacy-brand scans, and final real Grok smoke test.
  - [ ] Release signing/notarization and package signature verification happen last, when credentials are available.
  - [ ] Clean historical private identifiers and rename the GitHub repository and local directory to `any-aicli-remote`, then verify the private remote.
  - Commit: `🚀 发布：准备 Any AI CLI Remote 开源版本`
