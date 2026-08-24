# 交接说明

## 1. 当前状态

Phase 0 全栈模块化**已完成并推送**。TODO 5 功能门禁**已解除**，可以开始
Phase 1 backend interaction contract。

全部 commit 已推送到 `origin/main`，工作区除本文件外干净。真机联调发现的
四个缺陷均已修复并验证。

## 2. 未解决问题

无。上一版交接里记录的 iOS 重开会话滚动问题已定位并修复，见第 3 节。

## 3. 本轮完成的工作

### Phase 0 模块化（11 个 ♻️ commit）

三栈最大生产文件从 587 行降到 372 行（372 那个本来就不在清单上）。

- 后端 13 个文件按 owner 拆分。Hub 按方向拆为
  `agentdispatch`/`clientrpc`/`pending` + `clienttransport`/`agenttransport`，
  连接代际与归属校验只在 `agenttransport.go` 保留一份规范实现。
- Android `ChatViewModel` 587 → 143，取消与代际收敛进 `ChatOperationScope`，
  设备/连接/会话/工作区/turn 各有协调器。
- iOS `ChatStore` 536 → 130，同样分工，代际归属收敛进 `ChatOwnership`。

每个后端包都比对了函数清单、导出 API、测试结果三项一致；Android 与 iOS 的
ViewModel/Store 拆分在拆前拆后各跑一次真机 E2E 并逐条比对用例清单。

### 真实链路联调发现并修掉的两个缺陷

**`2951609` 后端：历史重建不再把 provider 内部脚手架当对话**

`/api/sessions/{id}/messages` 原样放行了 Grok CLI 写进 `chat_history.jsonl`
的记账内容。实测一个会话 6 条消息里只有 1 条是真内容，其余是系统提示、
`<user_info>`、`<system-reminder>` skill 目录（含本机绝对路径）、MCP 清单，
真实输入还被包在 `<user_query>` 里。两端都渲染出巨型垃圾气泡。

在适配器边界剥离。标签清单
（`system-reminder`/`user_info`/`rules`/`git_status`）取自 60 个真实会话实测，
不是文档推断。回归：user 轮次 213 → 177，残留脚手架标签为零。

注意：`<user_query>` **不是**每轮都有——多数已存会话的 user 轮次是纯文本无
包装。任何「只保留 user_query 内容」的简化都会把真实消息全丢掉。

**`22a4f3a` Android：代码块补齐语法高亮并修正深色主题**

iOS 用 `SwiftStreamingMarkdown` 自带高亮，Android 只装了 mikepenz 渲染器的
基础包与 m3 包。加入同版本 `-code-android:0.38.1`（不升级），并固定
`SyntaxThemes.default(darkMode = true)`：库默认跟随 `isSystemInDarkTheme()`，
而聊天区恒为深色，浅色系统下会选出深色标点，括号与箭头直接不可见。

### 真机联调发现并修掉的另外两个缺陷

**`8070377` iOS：cell 复用残留上一条消息**

`cellRegistration` 在 `models` 取不到模型时直接 return，没重设
`contentConfiguration`，复用的 cell 于是留着上一条消息的内容。改为就地按
当前 block 补建模型，取不到才清空配置。

**`956d535` 两端：重开会话叠加 provider 历史重放**

打开会话先用 `/messages` 落地权威历史，随后的 `session/load` 会让 provider
把整轮对话作为 `session/update` 重放。两端都把重放直接追加到已有历史上，
于是用户消息、思考与回复各出现两次；进程重启后回显去重表为空，拦不住重放。

两端对称加挂载标记，挂载 RPC 期间丢弃重放的 `session/update`。这是定位过程
里唯一靠日志坐实的结论——中途我基于像素做过几次推断都是错的，最后是在
`SessionCoordinator` 与 `TurnCoordinator` 各插一行 `NSLog` 才看清楚：

```
DBGHIST loaded blocks=2 kinds=user,assistant
DBGEVT user_message_chunk  blocksBefore=2
DBGEVT agent_thought_chunk blocksBefore=3
DBGEVT agent_message_chunk blocksBefore=4
```

接手者若再遇到「界面对不上数据」，建议直接走这条路：先确认后端返回，再打
日志看客户端拿到什么，不要从截图反推。

## 4. 验收命令与本轮结果

| 栈 | 命令 | 本轮结果 |
|---|---|---|
| Backend | `./scripts/check-go-quality.sh` | 通过（38 包 + race + vet） |
| Android | `testDebugUnitTest detekt lintDebug assembleDebug` | 通过 |
| Android | `./scripts/android-connected-e2e.sh` | 16 用例 0 失败 |
| iOS | `xcodegen` + `check-native-source-quality.sh` | 0 违规 / 46 文件 |
| iOS | `xcodebuild test`（模拟器） | 37 用例 0 失败 |
| iOS | `scripts/ios-live-daemon-e2e.sh` | 2 用例全通过 |

`xcodebuild test` 里那 2 个 live 用例是 **SKIPPED**，不能当作 live 链路的验证。
判断 iOS 真实链路必须单独跑 `ios-live-daemon-e2e.sh`。

## 5. 真机联调环境搭法

- Android 模拟器是 **MuMu**（不是 AVD，`emulator -list-avds` 是空的）。
  `adb connect 127.0.0.1:5555`，Android 12 / API 32 / arm64-v8a。
- 守护进程：`go build -o <path> ./cmd/any-aicli-remote-daemon`，
  用 `-pairing-secret-file` 传密钥（600 权限临时文件，**不要进命令行参数**），
  `-provider-path ~/.grok/bin/grok`，默认端口 2421。
- 配对密钥在钥匙串：服务 `com.anyaicliremote.launcher`，账户 `pairing-secret`。
- iOS live E2E 需要 `IOS_SIMULATOR_ID`（iPhone 17 Pro:
  `8B82E4CF-9A2B-4803-A973-28B929ED1F00`）。
- 联调完记得停 daemon、删临时密钥文件、`adb reverse --remove tcp:2421`。

## 6. 遗留的两处设计取舍

- **iOS 发布属性放宽**：写入方移到协调器后，`devices`、`activeDeviceID`、
  `isSessionLoading` 等从 `private(set)` 变为模块内可写。视图仍只读。
- **Android detekt `MatchingDeclarationName`**：跨文件参数契约必须是
  `internal`，而该规则要求「文件里只有一个非 private 类时文件名必须匹配」。
  解法是让屏幕文件统一持有其参数契约，不加 baseline、不做抑制。

## 7. 不要做

不要把 plan board 当 `PlanMessage/PlanBlock` 文本卡；不要继续往
ViewModel/Store/provider.go 塞职责；不要升级依赖；不要创建或切换 worktree。
