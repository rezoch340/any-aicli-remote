# 交接说明

## 当前状态

- item 0–6 和 8 已完成；跟随滚动收口与设备真实链路验证也已完成。
- 唯一未开始的顶层工作是 item 7（最终发布）。未明确要求前不要开始签名、公证、仓库改名或发布清理。
- 本轮不修改 iOS 产品代码；iOS 权限卡通过一次临时 focused UI probe 验证，临时测试代码已移除。

## 本轮完成内容

### Android 跟随滚动

- `LazyColumn(reverseLayout = true)`，最新消息位于 index 0。
- 页面内唯一 `follow` 状态控制跟随：流式更新时钉底，用户拖动后暂停，回到底部或点击跳底按钮恢复。
- 已移除 FollowController、transcript-signature `snapshotFlow`、spacer anchor 与分散滚动调用。

### Grok 权限模式

- `AlwaysApprove=false` 时显式使用 `grok --permission-mode default agent ...`，确定性覆盖用户配置中的静默批准模式。
- `AlwaysApprove=true` 时仍使用 `grok agent --always-approve ...`。
- 配对与 transport secret 仍不进入进程参数。

### 真实链路验证

Android / MuMu + 真实 Grok daemon：

- fixture connected E2E 通过，包含长流式跟随、拖动暂停和跳底恢复。
- focused live 5 项通过：配对、工具输出、权限命令标题、ask 取消、plan mode + notes + chat-about-this。

iOS Simulator + 真实 Grok daemon：

- 配对、流式跟随、结构化 ask、plan 修改/放弃均通过。
- focused 权限 probe 通过：`需要确认` 卡片实际出现，并显示 `.ios-live-permission-marker` 命令标题。
- 完整 live 脚本中的 child-agent 用例本次未触发子 Agent，导致 1/5 失败；其余 4/5 通过。该失败不涉及本轮跟滚或权限改动，未借机改动 child-agent 产品功能。
- retry / model auto-switch 状态在本地真实链路中未自然触发。

## 已运行验证

```sh
./scripts/check-go-quality.sh
cd android && ./gradlew testDebugUnitTest detekt assembleDebug
./scripts/check-native-source-quality.sh
./scripts/android-connected-e2e.sh
IOS_SIMULATOR_ID=8B82E4CF-9A2B-4803-A973-28B929ED1F00 ./scripts/ios-live-daemon-e2e.sh
# 另运行 focused iOS permission-card probe；通过后已移除临时测试代码。
```

## 后续边界

- 不恢复 FollowController、transcript hash/signature `snapshotFlow`、spacer anchor、throttle 或第二套跟随状态。
- 客户端不得解析 `_x.ai/*`、`x.ai/tool` 或 `_meta` 命名空间键。
- iOS 工程设置只通过 `project.yml` + `xcodegen generate` 修改。
- item 7 仍需用户明确要求后才能开始。
