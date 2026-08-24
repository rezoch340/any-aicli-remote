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

- [x] **3. 真实 Grok 的后端+Launcher E2E**
  - [x] Against the real Grok CLI, prove idle startup has no session, then verify new/load/resume lifecycle, workspace isolation, streaming, cancel, permission, reconnect, file, terminal, and archived history behavior.
  - [x] Verify Launcher start/stop/restart together with status and QR pairing payload behavior.
  - [x] This item must not modify or validate Android or iOS; freeze the backend+Launcher E2E contract before clients begin.
  - Commit: `🧪 后端：验证 Grok 与启动器生命周期`

- [x] **4. 原生客户端功能与 E2E**
  - [x] Begin only after items 0–3 are completed, checked off, committed, and the protocol contract is frozen. Android first: unit/debug/lint plus connected-device E2E; iOS afterward: unsigned simulator build/tests.
  - [x] Cover multi-device persistent pairing, disconnected-device navigation, session workspaces, history, streaming/cancel/permission/reconnect, rich-text/file behavior, and current brand naming.
  - Commit: `✨ 客户端：完成原生端功能与联调`

- [x] **4A. 原生客户端代码质量门禁**
  - [x] Android first，拆分接近千行的 ChatViewModel，复用 canonical reducer/协议 helper，清除缩写和魔法值。
  - [x] iOS afterward，拆分接近千行 ChatStore，集中 ACP 方法/参数映射和迁移 helper，清除缩写和魔法值。
  - [x] 使用维护中的 Kotlin/Swift lint 工具建立声明名至少 3 字符（仅允许技术专名）、单文件最多 600 行和复杂度门禁，并完成 Android/iOS 功能与 E2E 回归。
  - Commit: `♻️ 客户端：拆分状态管理并建立质量门禁`

- [x] **4B. 全栈模块化纠偏与真实链路联调**
  - [x] 按真实 owner 拆分后端 13 个大文件、Android `ChatViewModel` 与 iOS `ChatStore`；取消与代际归属在每栈只保留一份规范实现，不使用 facade 套壳、extension 搬运或重复实现。
  - [x] 每次拆分前后比对函数清单、导出 API 与测试结果；Android 与 iOS 在真机/模拟器上拆分前后各跑一次 E2E 并逐条比对用例清单。
  - [x] 以真实 Grok 守护进程完成两端联调，修复联调暴露的四个缺陷：历史重建泄漏 provider 内部脚手架、Android 代码块缺语法高亮、iOS cell 复用残留上一条消息、两端重开会话叠加 provider 历史重放。
  - Commits: `b1815b4..90df229`（♻️ 拆分系列与 🐛 修复系列）

- [ ] **5. 子 Agent 实时卡片**
  - [x] Backend first: represent each child Agent as a stable typed entity and emit ordered lifecycle/update events rather than asking clients to parse terminal or Markdown text.
  - [x] Prove backend event identity, ordering, reconnect replay, history reconstruction, completion, failure, and cancellation with protocol tests before touching app UI.
  - [ ] Android next and iOS after it: render the typed card and validate concurrent child Agents, reconnect/history reconstruction, out-of-order updates, completion, failure, and cancellation on both platforms.
  - Commit: `✨ 聊天：新增子 Agent 实时状态卡片`

- [ ] **6. 结构化交互合同（ask / exit plan）**
  - 实测事实（grok 1.0.5 本机抓取，见记忆 grok-ask-exit-wire）：ask/exit **不是** ACP 反向请求，而是 `session/update` 流里的 `tool_call`，判别式为 `_meta["x.ai/tool"].kind`（`ask_user` / `exit_plan`）。与子 Agent 走同一条 `NormalizeAgentNotification` 归一化管线。
  - [ ] 在 Grok adapter 的 `session/update` 归一化里按 `x.ai/tool.kind` 认出 ask/exit，映射成 provider-neutral typed interaction。`ask_user` 的 `rawInput.questions[{question, options[{label,description}], multi_select}]` 归一为中立 typed 问题；`tool_call` 用 `multi_select`（下划线）而 `tool_call_update` 用 `multiSelect`（驼峰），两处都要认。`plan` 是独立 `sessionUpdate:"plan"`，已是 ACP 形状 `entries[{content,priority,status}]`，直接复用 `coder/acp-go-sdk` 的 `Plan`/`PlanEntry`。exit_plan 的 `rawInput` 为空，计划正文在会话目录 `plan.md`。
  - [ ] 在 `docs/DEPENDENCY_DECISIONS.md` 记录：复用 acp-go-sdk 的 `Plan`/`PlanEntry`；ask 的 questions/options 是 Grok 私有形状（ACP 的 elicitation 走 JSON Schema 且 agent 不通告该能力，故不套用其类型）；`chat_about_this`/`skip_interview` 等结局在 ACP 无对应物需自定义中立枚举。
  - [ ] 清理 `internal/provider/grok/protocol.go` 中 `ClassifyReverseRequest` 的 `ask_user` 子串匹配——ask 从不走反向通道，该分支是死代码，删除以免误导；权限分类改为精确匹配。
  - [ ] 中立契约放 `internal/provider`，provider 私有解析只放 `internal/provider/grok`；Hub 只做 session-scoped first-answer-wins、断连/超时取消与代际校验，unknown 或畸形负载 fail closed。协议测试覆盖 ask/exit 识别、字段归一、两种拼写、未知 kind fail closed、结局回传。
  - [ ] Android 先、iOS 后：实现 pending 交互 UI、ask 表单与 plan 预览/操作；客户端只消费中立 payload，不解析 provider wrapper。
  - Commit: `✨ 聊天：新增结构化交互与计划确认`

- [ ] **7. 最终发布**
  - [ ] Run the complete Go/macOS/iOS/Android pipeline, privacy and legacy-brand scans, and final real Grok smoke test.
  - [ ] Release signing/notarization and package signature verification happen last, when credentials are available.
  - [ ] Clean historical private identifiers and rename the GitHub repository and local directory to `any-aicli-remote`, then verify the private remote.
  - Commit: `🚀 发布：准备 Any AI CLI Remote 开源版本`
