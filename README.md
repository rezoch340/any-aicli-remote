# Grok Remote App

原生 iOS/Android 客户端，用来连接现有的
[`grok-remote`](https://github.com/Amnibro/grok-remote) 服务。

聊天交互参考 OpenMinis，但没有携带 OpenMinis 的本地 Linux、Provider、Agent Loop
等运行时，也没有复制其 GPL 源码。这里只保留远程聊天需要的界面模型：

- 用户消息气泡与通栏 Agent 回复
- 可折叠 thinking
- 工具调用胶囊与详情
- 权限确认卡片
- 会话列表、新建、加载和取消
- WebSocket 流式更新、HTTP 历史补齐与自动重连
- 模型状态和 reasoning effort

## 目录

```text
grok-remote-app/
├── docs/       # grok-remote 协议说明
├── ios/        # SwiftUI
└── android/    # Kotlin + Jetpack Compose
```

## 配对

服务端启动后，`connect.url` 通常类似：

```text
http://<server-ip>:2421/?key=<pairing-key>&auto=1
```

可以把整条链接粘进 App。客户端会保存服务地址，并将 key 放进系统安全存储。
WebSocket 使用 `/ws?key=...`，REST 使用 `X-Grok-Remote-Key`。

## iOS

要求：Xcode 16+、iOS 17+。

```bash
cd ios
xcodegen generate
open GrokRemote.xcodeproj
```

仓库中已经包含生成后的 Xcode 工程；只有修改 `project.yml` 后才需要重新生成。

## Android

要求：Android Studio、JDK 17+、Android SDK 35+。

```bash
cd android
./gradlew :app:assembleDebug
```

APK 输出：

```text
android/app/build/outputs/apk/debug/app-debug.apk
```

安装到当前 USB/无线 ADB 设备：

```bash
adb install -r android/app/build/outputs/apk/debug/app-debug.apk
```

Android 还支持配对深链，便于以后从二维码直接打开：

```text
grokremote://pair?url=http%3A%2F%2F192.168.1.100%3A2421&key=<pairing-key>&cwd=~
```

## 当前 MVP

第一阶段重点是稳定连接和聊天主路径。Android 已支持 Markdown、代码块和表格；
图片/文件上传、LaTeX、Git 面板、语音和后台 Live Activity 会在协议与滚动行为
稳定后继续加入。
