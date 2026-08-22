# Grok Remote App

原生 iOS/Android 客户端与独立 Go daemon。Go 后端已经覆盖原 Python
`server.py` 的 REST、WebSocket Hub、会话历史、反向文件/终端 RPC、Agent
生命周期、Remote Loop、Git、Room 与 TTS 接口，不再依赖 Python/aiohttp。

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
├── backend/    # Go daemon（HTTP、WS Hub、Agent 生命周期）
├── docs/       # grok-remote 协议说明
├── ios/        # SwiftUI
├── android/    # Kotlin + Jetpack Compose
└── macos/      # 原生 SwiftUI 配置与一键启动器
```

## Go 后端

要求：Go 1.25+、已安装并登录 Grok CLI。

```bash
cd backend
../scripts/check-go-quality.sh
go build -trimpath -o ../dist/grok-remote-daemon ./cmd/grok-remote-daemon
```

`check-go-quality.sh` 是强制质量门禁：Go 声明名少于 3 个字符会失败（全大写的
标准技术专名除外），压缩缩写会失败，同时执行 `gofmt`、全量测试、race detector
和 `go vet`。后续后端修改必须先通过这条命令；GitHub Actions 也会在推送和
Pull Request 中运行同一门禁。

直接启动（默认 UI/WS `2421`，Grok Agent `2419`）：

```bash
./dist/grok-remote-daemon \
  --cwd /path/to/workspace \
  --public-host https://happy.example.com:20997
```

首次启动会在 `~/.grok/plugin-data/grok-remote/` 生成并保存配对密钥；也可以用
`GROK_AGENT_SECRET`、`--secret` 或 `--secret-file` 指定。daemon 默认自动确保
`grok agent serve` 正常运行，并且只会停止自己能够证明所有权的 Agent 进程。

检查状态：

```bash
curl http://127.0.0.1:2421/health
curl http://127.0.0.1:2421/health/deep
```

完整后端保留 43 个原协议 method/path 路由。旧 Web/PWA 资源不再捆绑；`/`、
`/watch` 和 `/pair` 只提供轻量配对页，聊天界面由原生 App 承担。

## macOS 一键启动器

要求：Apple Silicon Mac、Xcode 16+、Go 1.25+ 与
[XcodeGen](https://github.com/yonaskolb/XcodeGen)。构建原生 SwiftUI App：

```bash
./scripts/build-macos-app.sh
open "dist/Grok Remote Launcher.app"
```

启动器会把 arm64 Go daemon 一并放入 App，不需要另开终端。界面内可以配置：

- Grok 工作区
- daemon 与 Grok Agent 端口
- bind 地址
- 可选公网地址（支持 `https://host:异形端口`）

点击“启动服务”后，启动器会拉起 daemon 和 Grok Agent，等待健康检查通过，再显示
手机配对二维码。Android 与 iOS App 扫码后会通过 `grokremote://pair` 深链直接填入
服务地址、配对密钥和工作区；界面也保留 HTTP 配对链接供复制。关闭窗口或退出 App
会安全停止由本次启动器拥有的 daemon 与 Agent。

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

Android 与 iOS 都支持配对深链，可由 macOS 启动器的二维码直接打开：

```text
grokremote://pair?url=http%3A%2F%2F192.168.1.100%3A2421&key=<pairing-key>&cwd=~
```

## 当前 MVP

第一阶段重点是稳定连接和聊天主路径。Android 已支持 Markdown、代码块和表格；
图片/文件上传、LaTeX、Git 面板、语音和后台 Live Activity 会在协议与滚动行为
稳定后继续加入。
