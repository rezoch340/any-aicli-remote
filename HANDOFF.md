# 交接说明

## 1. 当前状态

- 后端 item 5（子 Agent）与 item 6（结构化交互 ask/exit）**已完成、已提交、已推送**，合同冻结。
- **Android item 5 + 6 客户端代码已写完，全量静态门禁通过，但尚未提交、尚未真机验证。**
  工作区有 10 改 + 9 新（`git status` 见下），行数约 +760。
- iOS item 5 + 6 客户端**未开始**。
- item 7 发布未开始。

优先级：Android 这批**先真机验证再提交**（静态门禁不覆盖真实交互），然后按 AGENTS.md
顺序做 iOS。

## 2. 未提交的 Android 改动（本次工作）

按模块拆分，未往 ViewModel/reducer 堆实现——`ChatEventReducer.reduceNotification`
只加了两行分发，状态变换全在独立纯函数/控制器里。

### 新增文件
- `core/model/ChildAgent.kt` — `ChildAgentCard`、`ChildAgentStatus`、`childAgentStatus()`。
- `core/model/Interaction.kt` — `PendingInteraction`、`InteractionKind`、`InteractionQuestion`、
  `InteractionOption`、`InteractionAnswer`（sealed：Accept/ChatAbout/SkipInterview/Approve/
  Cancel/Abandon）。
- `core/chat/ChildAgentReducer.kt` — 纯函数：按 providerChildId 合并、sequence 代际去重、
  终态不被旧事件覆盖、空字段不擦除已有值。
- `core/session/InteractionAnswerCodec.kt` — typed 答案 → 中立 JSON。**answers 必须编码成
  JSON object（键=问题索引串），发数组会被 daemon 拒**。
- `feature/ui/InteractionController.kt` — 应答/取消 pending 交互，带 rpcId 校验。
- `feature/ui/components/ChildAgentStrip.kt` — 横向卡片条（展示态，不渲染 prompt/输出）。
- `feature/ui/components/InteractionSheet.kt` — ModalBottomSheet：AskForm（多问题多选/单选 +
  提交/跳过）、PlanApproval（计划正文 + 批准/取消带 feedback）。

### 改动文件
- `core/remote/ACPEventDecoder.kt` — 加 `ACPEvent.ChildAgentUpdate`、`ACPEvent.Interaction`
  两个变体与解码。
- `core/remote/ACPWire.kt` — 加 `childAgentUpdateMethod`、`interactionRequestMethod` 常量；
  `classifyIncomingRequest` 认 `session/interaction_request` 为 UI_HANDLED；`isPermissionMethod`
  删掉 `ask_user` 子串（现在客户端只收中立方法名，那是死代码）。
- `core/session/SessionController.kt` — `answerInteraction()`；`SessionHistory` 加 `childAgents`；
  `loadHistory` 填充历史快照。
- `core/session/SessionPayloadMapper.kt` — `childAgents()` 解析历史 `childAgents` 数组。
- `feature/ui/ChatUiState.kt` — 加 `childAgents`、`pendingInteraction` 字段。
- `feature/ui/ChatEventReducer.kt` — dispatch 两个新事件（带 session 守卫），委派纯函数。
- `feature/ui/SessionCoordinator.kt` — 打开/关闭会话时重置 childAgents+pendingInteraction，
  历史加载时填充 childAgents。
- `feature/ui/ChatViewModel.kt` — 接入 InteractionController，暴露 answerInteraction/
  dismissInteraction。
- `feature/ui/screens/ChatScreen.kt` — ChatContent 顶部渲染 ChildAgentStrip；根部挂
  InteractionSheet。
- `core/remote/ACPWireTest.kt` — 把 `session/ask_user` 断言改为 `session/interaction_request`。

### 已有测试
- `core/chat/ChildAgentReducerTest.kt`（4）：插入保序、原地合并、乱序 stale 丢弃、空字段不擦除。
- `core/session/InteractionAnswerCodecTest.kt`（5）：accept 的 answers 是 object、approve 无
  feedback、cancel 空 feedback 省略、abandon、chat/skip 的 partialAnswers。

## 3. 接手者第一步：Android 真机验证

静态门禁（unit+detekt+lint+assembleDebug）**已过**，但没验真实交互。必须做：

1. daemon：`(cd backend && go build -o <tmp>/daemon ./cmd/any-aicli-remote-daemon)`，
   `-pairing-secret-file`（600 临时文件）、`-provider-path ~/.grok/bin/grok`、端口 2421。
   配对密钥在钥匙串：服务 `com.anyaicliremote.launcher`、账户 `pairing-secret`。
2. Android 模拟器是 **MuMu**（不是 AVD）：`adb connect 127.0.0.1:5555`，API 32 / arm64。
   `adb reverse tcp:2421 tcp:2421`。`./gradlew installDebug`。
3. **子 Agent 卡片**：发一个会 spawn 子 Agent 的 prompt（真实 grok 会用 explore/code
   子 Agent），看聊天顶部卡片条出现、状态从运行中→已完成。重开会话验历史快照。
4. **交互**：发 `/plan 为空目录设计缓存层，规划前先用 ask_user_question 问我 Redis 还是
   进程内 LRU`。应看到 AskForm 弹出→选一项提交→agent 继续→随后 exit_plan 的 PlanApproval
   弹出→批准。（wire 与应答形状见记忆 `grok-ask-exit-wire`。）
5. `./scripts/android-connected-e2e.sh` 回归。
6. 验完再提交（一个 `✨ 聊天：新增子 Agent 实时状态卡片` 或按 TODO 的 commit 文案）。

## 4. iOS 待做（item 5 + 6，Android 之后）

按冻结的中立 payload 实现等价功能，**不复制 wire 解析**：
- 解码 `session/child_agent_update`、`session/interaction_request`（带 id 反向请求）。
- reducer/state：childAgents 列表（代际去重）、pendingInteraction。
- 应答：`InteractionAnswerCodec` 等价，answers 编码成 map。
- UI：子 Agent 卡片 + 交互 sheet（ask 表单 + plan 批准）。
- 历史：`/messages` 响应的 `childAgents` 快照。
- iOS 参照 `ios/AnyAICLIRemoteFeature` 现有 ChatStore/协调器结构，同样拆模块。

## 5. 冻结的后端合同（客户端只消费中立 payload，绝不解析 _x.ai wrapper）

### 子 Agent（item 5，commit 9549f5a + d08b51d）
- 通知方法 `session/child_agent_update`，params `{sessionId, event}`。
  `event = {eventId, sequence?, occurredAt, replay, kind, agent}`。
  `kind`：started/progress/completed/failed/cancelled/updated。
  `agent`（ChildAgentRecord）字段 camelCase：providerChildId、childSessionId、agentType、
  description、status（running/completed/failed/cancelled/unknown）、startedAt、completedAt、
  toolCallCount、turnCount、modelId、tokensUsed、contextUsagePercent…
- 历史快照：`/api/sessions/{id}/messages` 响应的 `childAgents: [ChildAgentRecord]`。
- 排序/去重靠 `sequence`（来自 eventId 后缀）；未知终态冒泡为 unknown/updated（有测试钉住）。

### 结构化交互（item 6，commit 98e149a）
- daemon 把 grok 私有 `_x.ai/ask_user_question`、`_x.ai/exit_plan_mode`（**带 id 反向请求**）
  归一为中立 `session/interaction_request`（**仍带 id**），params：
  `{kind: ask_question|exit_plan, sessionId, toolCallId, questions[{question,
  options[{label,description}], multiSelect}], planContent, mode}`。
- 应答：客户端对该 id 回 JSON-RPC `result`，中立形状：
  - exit：`{outcome: approved}` / `{outcome: cancelled, feedback?}` / `{outcome: abandoned}`
  - ask：`{outcome: accepted, answers: {"0":["label"]}}`（answers 必须 object）/
    `{outcome: chat_about_this, partialAnswers}` / `{outcome: skip_interview, partialAnswers}`
- daemon 复用 permission 的 session 定向 + first-answer-wins + 断连取消；交互失败一律回
  JSON-RPC error（与 agent 断连后「重现」一致）；unknown/畸形 fail closed。
- 完整实测细节见记忆 `grok-ask-exit-wire`；应答枚举以开源 `xai-org/grok-build` 源码为准。

## 6. TODO 勾选状态

- item 5：后端两条子项已勾（d08b51d）；客户端子项未勾（Android 已写未提交，iOS 未做）。
- item 6：后端四条子项已勾（48a94f1）；客户端子项未勾。
- 顶层 item 5、6 未勾，因为客户端未完成。按 AGENTS.md，勾选与功能提交同一个 commit。

## 7. 验收命令

- 后端：`./scripts/check-go-quality.sh`（当前通过）。
- Android：`testDebugUnitTest detekt lintDebug assembleDebug`（当前通过）+
  `./scripts/android-connected-e2e.sh`（真机，未跑）。
- iOS：`xcodegen` + `check-native-source-quality.sh` + `xcodebuild test` 模拟器 +
  `scripts/ios-live-daemon-e2e.sh`（需 `IOS_SIMULATOR_ID`）。

## 8. 不要做

不要让客户端解析 `_x.ai/*` wrapper；不要把 plan/子 Agent 当文本卡塞进 ChatBlock（已分别用
PendingInteraction / ChildAgentCard 独立建模）；不要往 ViewModel/Store/provider.go 堆职责；
不要升级依赖；不要创建或切换 worktree。
