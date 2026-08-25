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

- [x] **5. 子 Agent 实时卡片**
  - [x] Backend first: represent each child Agent as a stable typed entity and emit ordered lifecycle/update events rather than asking clients to parse terminal or Markdown text.
  - [x] Prove backend event identity, ordering, reconnect replay, history reconstruction, completion, failure, and cancellation with protocol tests before touching app UI.
  - [x] Android next and iOS after it: render the typed card and validate concurrent child Agents, reconnect/history reconstruction, out-of-order updates, completion, failure, and cancellation on both platforms.
  - Commit: `✨ 聊天：新增子 Agent 实时状态卡片`

- [x] **6. 结构化交互合同（ask / exit plan）**
  - 实测事实（grok 1.0.5 本机双向抓包 + 开源 [`xai-org/grok-build` commit `9fabadea800fa6e2ed8ec91c4f45f02b7e2504f4`](https://github.com/xai-org/grok-build/commit/9fabadea800fa6e2ed8ec91c4f45f02b7e2504f4) 源码交叉确认，见记忆 grok-ask-exit-wire）：ask/exit 是**带 id 的反向请求** `_x.ai/ask_user_question`、`_x.ai/exit_plan_mode`，agent 会阻塞一轮对话等应答。外层 params 是 `{method:"x.ai/...", params:{sessionId,toolCallId,...}}` 双层 wrapper；只有两个 wrapper 键都缺失时才兼容直接 outer 形状。展示帧（session/update 里的 tool_call）另走一条。
  - [x] Grok adapter 归一化：`_x.ai/*` 反向请求 → provider-neutral InteractionRequest（`session/interaction_request`），入向 `NormalizeInteractionRequest`、出向 `DenormalizeInteractionResponse`。exit 应答 `{outcome: approved|cancelled|abandoned, feedback?}`；ask 也可精确应答 `{outcome: cancelled}`；client-neutral ask 的 `accepted.answers` 必须是键为规范十进制问题索引的 map，发数组被 agent 拒；Grok provider response 则由 adapter 映回原始 question 文本，作为 `answers`、`annotations`、`partial_answers` 的键。反向请求侧用 `multiSelect`（驼峰）。
  - [x] 在 `docs/DEPENDENCY_DECISIONS.md` 记录：复用 acp-go-sdk 的 envelope 与 Plan 展示类型；ask/exit 的 `_x.ai/*` 请求/应答为 Grok 私有形状，ACP 无对应物，手写最小双向映射；elicitation 能力 agent 不通告，故不套用。
  - [x] 清理 `internal/provider/grok/protocol.go` 中 `ClassifyReverseRequest` 的 `ask_user` 子串匹配死代码；ask/exit 走 InteractionOperation 精确分类。
  - [x] 中立契约放 `internal/provider/interaction.go`，私有解析只放 `internal/provider/grok/interaction.go`；Hub 复用 permission 的 session 定向 + first-answer-wins + 断连取消，交互失败一律回 JSON-RPC error，unknown 或畸形请求/应答 fail closed。协议测试（grok 层 + hub 往返）覆盖识别、字段归一、拼写、往返、断连与畸形 fail closed。
  - [x] Android 先、iOS 后：实现 pending 交互 UI、ask 表单与 plan 预览/操作；客户端只消费中立 `session/interaction_request`，不解析 provider wrapper。
  - Commit: `✨ 聊天：新增结构化交互与计划确认`

- [ ] **8. 扩展流式与交互 primitive（全部归一化）**
  - 原则：客户端只消费 provider-neutral payload，**绝不解析 grok 原语**；能复用现有中立通道的复用，只在必需时新增一条中立方法。grok primitive 面见记忆/盘点：官方 session/update 扩展变体 ~50，仅少数对远程手机客户端有意义。
  - 归一化落点：`tool_call_delta_chunk`→复用中立 tool-call 更新（append 语义）；`model_changed`/`model_auto_switched`→复用现有 ModelState 通道；`current_mode_update`+`retry_state`→新增一条中立 `session/status_update`（带 mode/retry 枚举）；`feedback_request`（通知式、带 app 层 request_id、应答走 `x.ai/feedback` 调用）→新增中立 feedback 交互通道（通知进 + 应答方法出）。
  - [ ] Phase 1（不改契约）：Android ask 表单追平已发布的 iOS——`InteractionAnswer` 加 annotations（每题 notes）与 cancelAsk；加"先聊一下"（chat_about_this + partialAnswers）、"取消"、每题备注输入；两端补 `chat_about_this` 可达按钮。后端契约已支持，纯客户端。
  - 复核后砍单：`tool_call_delta_chunk` 实为**工具入参**的 raw JSON 片段流（单独非合法 JSON），手机上是噪声非信号，`tool_call`/`tool_call_update` 帧已带有意义状态 → 不做（YAGNI）。`model_changed` 为 app 自身改 effort 所触发、app 已知 → 不单独做；仅 `model_auto_switched`（模型不可用自动切换）罕见但有值，降级为一条归一化系统提示，不铺新 ModelState 管线。
  - [x] Phase 2（新增 1 条中立方法 `session/status_update`）：`current_mode_update`（上游 ACP 标准，客户端直读 → mode badge）+ `retry_state`（grok 私有，后端归一化 retrying/exhausted/failed + attempt/max/rateLimit → 状态行）+ `model_auto_switched`（grok 私有 → 中立 modelSwitch 通知）。后端在 `NormalizeAgentNotification` 前置 `normalizeStatusNotification` 拦 `session/update`、复用 `parseChildAgentEnvelope`，仅重写这两个私有扩展，其余透传（有 passthrough 测试钉住）。retry 字段按实测 camelCase + `type` 判别式读取。两端消费（SessionStatusBar mode 徽章 + 通知行）+ 协议测试（后端 grok 层、Android decoder、iOS mapper/formatter）。
  - [ ] Phase 4（feedback 交互）：`feedback_request`→中立 feedback 交互；daemon 通知归一化 + 应答映回 `x.ai/feedback`（带 request_id）；两端 UI（dismissible）。
  - 每 Phase：后端先（含协议测试）→ Android → iOS；grok 私有解析只在 `internal/provider/grok`，中立契约在 `internal/provider`。
  - Commit：每 Phase 一个 `✨` 提交。

- [ ] **7. 最终发布**
  - [ ] Run the complete Go/macOS/iOS/Android pipeline, privacy and legacy-brand scans, and final real Grok smoke test.
  - [ ] Release signing/notarization and package signature verification happen last, when credentials are available.
  - [ ] Clean historical private identifiers and rename the GitHub repository and local directory to `any-aicli-remote`, then verify the private remote.
  - Commit: `🚀 发布：准备 Any AI CLI Remote 开源版本`
