# 交接说明

## 当前状态

- `main` 与 `origin/main` 均位于 `fc590e6`，本会话 6 个 commit 已推送，工作区干净。
- item 0–6 完成；**item 8（扩展与归一化）本会话全做完**；item 7（最终发布）待做。
- 三端**静态门禁 + 单测全绿**（Go quality/vet、Android unit/detekt/lint/assemble、iOS SwiftLint + 93 unit），但 UI 改动**未真机验证**——这是提交后的硬门槛，见下「待验」。
- 运行态：daemon 已关闭（2421/2419 无监听）。真机 E2E 前须按现有流程启动 daemon 再 health check，配对密钥在钥匙串（服务 `com.anyaicliremote.launcher`、账户 `pairing-secret`），不要依赖固定 PID 或临时文件。

## 本会话 6 个 commit（都已推送）

1. `7dfe4d8` item 8 Phase 1：Android ask 表单追平已发布的 iOS——`InteractionAnswer` 加 annotations（每题 notes）与 CancelAsk；ask 表单加自由文本框（一物两用：Other 答案 + 备注）、plan 模式露「先聊一下」「跳过」、常驻「取消」「提交」。后端契约早支持，纯客户端。
2. `ac81bb8` item 8 Phase 2：grok 私有 `retry_state`、`model_auto_switched` 后端归一化为中立 `session/status_update`；`current_mode_update`（ACP 标准）客户端直读。两端 `SessionStatusBar`（mode 徽章 + 通知行）。归一化在 `internal/provider/grok/status.go`（复用 `parseChildAgentEnvelope`），中立契约 `internal/provider/status.go`。
3. `13bf7b9` **工具输出 bug**：ACP 的 `ToolCall(Update).content` 是**数组**，两端却当对象读 → `ls` 等输出永丢。归一化修：daemon `normalizeToolContent`（grok/toolcontent.go）把 grok 扁平项 `{type:text,text}` 包成 ACP 标准 `{type:content,content:{...}}`；两端 `upsertTool` 遍历数组取文本。**对 claude 也曾坏，非 grok 特有**（据 zed ACP 规范 tool_call.rs + claude-agent-acp 源码确认）。
4. `dd75f1e` **权限双修**：①卡片看不到命令——命令在 grok 私有 `_meta["x.ai/tool"]`（ACP 禁止客户端读 _meta），故 daemon `ClassifyReverseRequest` 提取命令（grok/permission.go）、hub `applyPermissionTitle` 写进标准 `toolCall.title`，两端只读标准字段。②**iOS 权限从不识别**——`isPermissionRequest` 只认 `permission/request`，但 ACP 标准是 `session/request_permission`，导致 grok 权限卡在 iOS 从不弹；改为 `contains("permission")`，与 daemon/Android 一致。回包 `{outcome:{outcome:selected,optionId}}` 早已正确。
5. `fc590e6` item 8 Phase 4：`feedback_request` 是 xAI 评分/NPS 漏斗，不做客户端 UI。新增 provider 接口 `AutoDeclineNotification`，grok 认出即回 `x.ai/feedback/dismiss {session_id,request_id}`（hub 赋 id/代际校验/复用 replyAgentForGeneration）且不转发客户端。
6. `43892de` TODO 记录。

## 待验（真机，接手第一件事，按此顺序）

装 MuMu（`adb connect 127.0.0.1:5555`、`adb reverse tcp:2421 tcp:2421`、`./gradlew installDebug`）或 iOS 模拟器，接**真实 grok daemon**：

1. **工具输出**（最该验，本会话核心修复）：发「列出当前目录文件」等会触发 bash/ls 的 prompt，确认工具卡**显示命令输出**（此前为空）。
2. **权限卡**：触发需授权的命令（写文件/危险命令），确认卡片**显示要执行的命令**（如「bash: ls -la」）而非笼统「需要你的确认」；**iOS 尤其要验权限卡是否弹出**（此前从不弹）；点允许/拒绝确认生效。
3. **ask 表单**：`/plan` + 让 grok 先 `ask_user_question`，验每题备注框、取消、先聊一下、提交。
4. **状态徽章**：计划模式徽章、弱网下的重试提示（能触发的话）。
5. 回归：`./scripts/android-connected-e2e.sh`、`IOS_SIMULATOR_ID=<uuid> ./scripts/ios-live-daemon-e2e.sh`。

## 归一化原则（勿违背）

- 客户端只消费 provider-neutral / ACP-标准 payload，**绝不解析 grok 私有 wire 或 `_meta` 内命名空间键**（ACP 明令禁止解读 _meta）。grok 偏差一律在 `internal/provider/grok` 适配器边界吸收。
- 本会话所有修复都据 grok 开源（`xai-org/grok-build`）与 zed ACP 规范（`agentclientprotocol/agent-client-protocol`）+ claude 适配（`agentclientprotocol/claude-agent-acp`）交叉确认；改 wire 前先读源码，别猜。

## 验收命令

```sh
./scripts/check-go-quality.sh
cd android && ./gradlew testDebugUnitTest detekt lintDebug assembleDebug
./scripts/check-native-source-quality.sh
cd ios && xcodebuild test -project AnyAICLIRemote.xcodeproj -scheme AnyAICLIRemote -destination 'platform=iOS Simulator,id=<uuid>' -only-testing:AnyAICLIRemoteTests
```

## 不要做

- 不让客户端解析 `_x.ai/*` / `x.ai/tool` 等私有字段；不把职责堆回 ViewModel/ChatStore/reducer。
- iOS 不手改 generated project：先改 `project.yml` 再 `xcodegen generate`（本会话新增 .swift 已按目录 glob 纳入）。
- 不升级依赖；不创建/切换 worktree。
- 命名门禁：Go 禁止 <3 字符局部名（用 `isObject`/`hasMeta` 等）；iOS SwiftLint 禁集合字面量尾逗号。
