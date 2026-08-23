# Any AI CLI Remote

Any AI CLI Remote 是面向 AI CLI 的原生远程客户端与独立 Go daemon。核心只负责设备配对、
连接、会话生命周期、安全工作区、历史索引和反向 RPC；CLI 的启动方式、协议 method、
模型能力与磁盘会话格式全部由 Provider adapter 实现。

当前只实现 `grok` Provider。通用层已经按多 Provider 边界设计，但不会为尚未接入的 CLI
堆放占位实现。

## 设计约束

- 启动 daemon 只会拉起空闲的 Provider 服务，不创建、加载或恢复任何会话。
- 设备配对只保存服务地址、设备名与密钥，不选择工作区。
- 打开已有会话时，从会话元数据恢复该会话自己的工作区。
- 新建会话时，客户端必须明确提交工作区。
- 文件、终端、Git、Skills 与项目操作始终从当前会话解析工作区，不存在 daemon 全局工作区。
- 通用核心不包含 Grok 的 RPC method、命令行参数或 `~/.grok` 磁盘布局。
- 新数据只写 Any AI CLI Remote 标识；旧 Grok Remote 标识仅在集中式兼容读取和迁移代码中出现。

## 架构

```text
Native iOS / Android
        │ HTTP + WebSocket
        ▼
Any AI CLI Remote daemon
  ├─ authentication / pairing
  ├─ provider registry
  ├─ session metadata + workspace isolation
  ├─ history pagination
  ├─ reverse file / terminal RPC
  └─ Provider + ProtocolAdapter
             └─ grok
                 ├─ grok agent serve lifecycle
                 ├─ Grok JSON-RPC mapping
                 └─ active + archived history reader
```

聊天界面采用原生 SwiftUI 与 Jetpack Compose，支持流式消息、Markdown、代码块、表格、
thinking、工具调用、权限确认、多设备资料、会话历史、新建/加载/取消与断线返回。

## 目录

```text
any-aicli-remote/
├── backend/    # 通用 Go daemon 与 Provider adapters
├── docs/       # 通用传输协议和 Provider 边界
├── ios/        # SwiftUI 客户端
├── android/    # Kotlin + Jetpack Compose 客户端
├── macos/      # SwiftUI 一键启动器
└── scripts/    # 构建与强制质量门禁
```

根目录的 [`AGENTS.md`](AGENTS.md) 是工程质量约束：能复用的逻辑必须复用，跨 Provider
的 registry、路径包含校验、时间归一化、分页和兼容迁移只能存在一份实现。依赖和已有方案的
评估记录在 [`docs/DEPENDENCY_DECISIONS.md`](docs/DEPENDENCY_DECISIONS.md)。

## Go 后端

要求：Go 1.25+，以及已安装并登录的 Grok CLI。

```bash
cd backend
../scripts/check-go-quality.sh
go build -trimpath -o ../dist/any-aicli-remote-daemon ./cmd/any-aicli-remote-daemon
```

`check-go-quality.sh` 会执行命名门禁、Go 单文件 600 行上限、`gofmt`、全量测试、race
detector 与 `go vet`。非专有名词的声明名少于三个字符、使用禁用缩写或把多个职责堆进
超长 Go 文件时会直接失败；GitHub Actions 运行同一门禁。

直接启动（默认 daemon `2421`，Grok Provider 服务 `2419`）：

```bash
./dist/any-aicli-remote-daemon \
  --public-host https://remote.example.com:24443
```

这里没有 `--cwd`：daemon 启动不拥有工作区。首次启动会在 `~/.any-aicli-remote/` 准备
运行数据和配对材料，并可只读迁移旧目录中的兼容数据。Provider 启动后仍为空闲状态，
直到客户端选择已有会话或携带工作区新建会话。

```bash
curl http://127.0.0.1:2421/health
curl -H 'X-Any-AI-CLI-Remote-Key: <pairing-key>' http://127.0.0.1:2421/health/deep
```

只有浅层 `/health` 公开。会触发 Provider 连接/启动探测的 `/health/deep` 必须鉴权。配对密钥
通过系统凭据存储、`*-secret-file` 或环境变量传入；daemon 不接受会把密钥暴露到进程列表和
shell history 的明文 secret 参数。

## Grok 历史

Grok adapter 的历史模型参考
[CC-Switch](https://github.com/farion1231/cc-switch) 的 Provider 设计：同时扫描
`~/.grok/sessions` 与 `~/.grok/archived_sessions`，从 `summary.json` 建立轻量元数据索引，
需要消息时才读取对应的 `chat_history.jsonl`。

统一历史元数据包含 Provider ID、session ID、标题、摘要、工作区、创建/最后活动时间、
来源路径和恢复命令。结果按最后活动时间倒序分页；活动与归档重复项、时间戳格式、富文本
提取和损坏记录都由确定性规则处理。所有来源路径会先 canonicalize，再验证位于允许的
Provider history roots 内，并校验磁盘 session ID 与请求一致。

## macOS 一键启动器

要求：Apple Silicon Mac、Xcode 16+、Go 1.25+ 与
[XcodeGen](https://github.com/yonaskolb/XcodeGen)。

```bash
./scripts/build-macos-app.sh
open "dist/Any AI CLI Remote Launcher.app"
```

启动器会嵌入 arm64 Go daemon。界面只配置设备名称、daemon/Provider 端口、bind 地址和
可选公网地址；它不会选择工作区。启动成功后显示二维码，关闭启动器时只停止本次启动器
能够证明所有权的进程。

默认构建使用 macOS 平台的 ad-hoc 签名：先签 daemon，再把它嵌入标准的
`Contents/MacOS` 位置，最后由 Xcode 签外层 App；脚本会对 daemon 和 App 执行
`codesign --verify --deep --strict`。纯 ad-hoc 签名没有 provisioning profile 授权的
Keychain access group，因此启动器只在系统返回 `errSecMissingEntitlement` 时改用 SecItem
file-based Keychain；其他 Keychain 错误不会被吞掉。更新 ad-hoc 构建后，macOS 可能再次
要求确认该 App 访问登录钥匙串。

若需使用 Data Protection Keychain，提供含 Keychain access group 权限的签名身份、Team 和
provisioning profile；脚本会启用仓库内的 macOS entitlements：

```bash
MACOS_CODE_SIGN_IDENTITY="Apple Development: Example" \
MACOS_DEVELOPMENT_TEAM="TEAMID" \
MACOS_PROVISIONING_PROFILE_SPECIFIER="Any AI CLI Remote Development" \
./scripts/build-macos-app.sh
```

默认产物是本机运行构建，不是 Developer ID/notarized 分发包。

## 设备配对

HTTP 配对地址通常为：

```text
http://<server-ip>:2421/?key=<pairing-key>&auto=1
```

二维码使用：

```text
anyaicliremote://pair?url=<encoded-base-url>&key=<encoded-key>&name=<encoded-device-name>
```

深链没有 `cwd`。客户端将每台设备保存为独立资料，把 key 写入平台安全存储，冷启动先显示
设备列表；选择设备后才连接并读取会话。WebSocket 使用 `/ws?key=...`，REST 使用
`X-Any-AI-CLI-Remote-Key`。旧 scheme/header 只用于升级兼容。

## iOS

要求：Xcode 16+、iOS 17+。

```bash
cd ios
xcodegen generate
open AnyAICLIRemote.xcodeproj
```

旧 bundle ID 与 `com.anyaicliremote.app` 也属于不同 iOS App 容器。Keychain 兼容层会尽力
复制旧配对密钥，但旧 UserDefaults 中的设备 URL/名称不能保证跨沙箱读取；完整无感迁移需要
先由旧 bundle 的过渡版本把设备元数据导出到共享 Keychain/App Group。

## Android

要求：Android Studio、JDK 17+、Android SDK 35+。

```bash
cd android
./gradlew testDebugUnitTest :app:assembleDebug :app:lintDebug
```

APK 输出为 `android/app/build/outputs/apk/debug/app-debug.apk`，可使用：

```bash
adb install -r android/app/build/outputs/apk/debug/app-debug.apk
```

将 Android `applicationId` 从旧包名迁移为 `com.anyaicliremote.app` 后，Android 会把它视为
新应用；系统不允许新包直接读取旧包的私有沙箱。仓库内的偏好迁移覆盖同一沙箱可访问的
旧格式，真正跨 applicationId 的已安装数据迁移需要旧版本先提供显式导出通道。

## License

最终开源许可证尚未确定；当前 [`LICENSE`](LICENSE) 仅记录占位状态。
