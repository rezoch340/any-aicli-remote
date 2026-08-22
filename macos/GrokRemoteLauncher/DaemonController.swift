import AppKit
import Combine
import Foundation

@MainActor
final class DaemonController: ObservableObject {
    let settings: LauncherSettings

    @Published private(set) var phase: DaemonPhase = .stopped
    @Published private(set) var health: HealthSnapshot?
    @Published private(set) var isReachable = false
    @Published private(set) var daemonExecutablePath = ""
    @Published private(set) var pairingURL = ""
    @Published private(set) var pairingDeepLink = ""
    @Published private(set) var logs: [LogEntry] = []

    private var process: Process?
    private var outputPipe: Pipe?
    private var logRemainder = ""
    private var pollingTask: Task<Void, Never>?
    private var isActivated = false
    private var knownSecret = ""
    private var lastReachability = false
    private let session: URLSession

    init(settings: LauncherSettings) {
        self.settings = settings
        let configuration = URLSessionConfiguration.ephemeral
        configuration.timeoutIntervalForRequest = 2
        configuration.timeoutIntervalForResource = 3
        session = URLSession(configuration: configuration)
    }

    var configurationEditable: Bool {
        !isReachable && process?.isRunning != true && phase != .starting && phase != .stopping
    }

    var showsStopAction: Bool {
        isReachable || process?.isRunning == true || phase == .starting
    }

    func activate() {
        guard !isActivated else { return }
        isActivated = true
        refreshDaemonLocation()
        do {
            let secretFile = try PairingSecretStore.loadOrCreate()
            knownSecret = secretFile.secret
            rebuildFallbackPairing()
            appendLog("配对密钥已就绪（安全文件：\(secretFile.fileURL.path)）")
        } catch {
            phase = .failed(error.localizedDescription)
            appendLog("无法准备配对密钥：\(error.localizedDescription)")
        }
        pollingTask = Task { [weak self] in
            while !Task.isCancelled {
                await self?.pollHealth()
                try? await Task.sleep(for: .seconds(2))
            }
        }
    }

    func refreshDaemonLocation() {
        daemonExecutablePath = DaemonLocator.locate()?.path ?? ""
        if daemonExecutablePath.isEmpty {
            appendLog("未找到 grok-remote-daemon")
        } else {
            appendLog("已定位 daemon：\(daemonExecutablePath)")
        }
    }

    func chooseWorkspace() {
        let panel = NSOpenPanel()
        panel.title = "选择 Grok 工作区"
        panel.prompt = "选择"
        panel.canChooseFiles = false
        panel.canChooseDirectories = true
        panel.allowsMultipleSelection = false
        panel.directoryURL = URL(fileURLWithPath: settings.workspace, isDirectory: true)
        if panel.runModal() == .OK, let selection = panel.url {
            settings.workspace = selection.path
            rebuildFallbackPairing()
        }
    }

    func performPrimaryAction() {
        if showsStopAction {
            Task { await stop() }
        } else {
            start()
        }
    }

    func start() {
        if let issue = configurationIssue() {
            phase = .failed(issue)
            appendLog(issue)
            return
        }
        refreshDaemonLocation()
        guard !daemonExecutablePath.isEmpty else {
            phase = .failed("找不到 grok-remote-daemon，请先构建后端")
            return
        }

        let secretFile: PairingSecretStore.Value
        do {
            secretFile = try PairingSecretStore.loadOrCreate()
            knownSecret = secretFile.secret
        } catch {
            phase = .failed(error.localizedDescription)
            appendLog(error.localizedDescription)
            return
        }

        rebuildFallbackPairing()
        phase = .starting
        health = nil
        appendLog("—— 启动 Grok Remote ——")

        let launchedProcess = Process()
        launchedProcess.executableURL = URL(fileURLWithPath: daemonExecutablePath)
        launchedProcess.currentDirectoryURL = URL(fileURLWithPath: settings.workspace, isDirectory: true)
        var arguments = [
            "--cwd", settings.workspace,
            "--bind", settings.bindAddress.trimmingCharacters(in: .whitespacesAndNewlines),
            "--port", String(settings.daemonPort),
            "--agent-port", String(settings.agentPort),
            "--secret-file", secretFile.fileURL.path,
            "--stop-agent-on-exit",
        ]
        let publicHost = settings.publicHost.trimmingCharacters(in: .whitespacesAndNewlines)
        if !publicHost.isEmpty { arguments += ["--public-host", publicHost] }
        launchedProcess.arguments = arguments
        launchedProcess.environment = launchEnvironment()
        launchedProcess.standardInput = FileHandle.nullDevice

        let pipe = Pipe()
        launchedProcess.standardOutput = pipe
        launchedProcess.standardError = pipe
        pipe.fileHandleForReading.readabilityHandler = { [weak self] handle in
            let data = handle.availableData
            if data.isEmpty {
                handle.readabilityHandler = nil
                Task { @MainActor [weak self] in self?.flushLogRemainder() }
                return
            }
            let chunk = String(decoding: data, as: UTF8.self)
            Task { @MainActor [weak self] in self?.consumeLogChunk(chunk) }
        }
        launchedProcess.terminationHandler = { [weak self] terminatedProcess in
            let status = terminatedProcess.terminationStatus
            Task { @MainActor [weak self] in self?.processDidTerminate(status: status) }
        }

        outputPipe = pipe
        process = launchedProcess
        do {
            try launchedProcess.run()
            appendLog("daemon 已启动，PID \(launchedProcess.processIdentifier)")
            Task { await pollHealth() }
        } catch {
            pipe.fileHandleForReading.readabilityHandler = nil
            process = nil
            outputPipe = nil
            phase = .failed(error.localizedDescription)
            appendLog("启动失败：\(error.localizedDescription)")
        }
    }

    func stop() async {
        guard showsStopAction else { return }
        phase = .stopping
        appendLog("正在请求安全停止 daemon 与 Agent…")
        guard let stopEndpoint = localURL(path: "/api/stack/stop") else {
            phase = .failed("Daemon 端口无效")
            return
        }
        var request = URLRequest(url: stopEndpoint)
        request.httpMethod = "POST"
        request.setValue("application/json", forHTTPHeaderField: "Content-Type")
        if !knownSecret.isEmpty { request.setValue(knownSecret, forHTTPHeaderField: "X-Grok-Remote-Key") }
        request.httpBody = Data("{\"keep_agent\":false}".utf8)

        do {
            let (_, response) = try await session.data(for: request)
            guard let http = response as? HTTPURLResponse, (200..<300).contains(http.statusCode) else {
                throw URLError(.badServerResponse)
            }
            appendLog("停止请求已接受")
            for _ in 0..<6 {
                try? await Task.sleep(for: .milliseconds(350))
                await pollHealth()
                if !isReachable { return }
            }
        } catch {
            appendLog("停止接口不可用，改用进程信号：\(error.localizedDescription)")
        }

        if let process, process.isRunning {
            process.interrupt()
            try? await Task.sleep(for: .seconds(1))
            if process.isRunning { process.terminate() }
        } else if isReachable {
            phase = .failed("无法停止当前端口上的 daemon")
        }
    }

    func pollHealth() async {
        guard (1...65535).contains(settings.daemonPort),
              let healthEndpoint = localURL(path: "/health") else { return }
        do {
            let (data, response) = try await session.data(from: healthEndpoint)
            guard let http = response as? HTTPURLResponse, http.statusCode == 200 else {
                throw URLError(.badServerResponse)
            }
            let snapshot = try JSONDecoder().decode(HealthSnapshot.self, from: data)
            health = snapshot
            isReachable = true
            if !lastReachability { appendLog("健康检查已连接") }
            lastReachability = true
            if phase != .stopping {
                phase = snapshot.ready == true ? .online : .degraded
            }
            await refreshRuntimeConfiguration()
        } catch {
            health = nil
            isReachable = false
            if lastReachability { appendLog("daemon 已离线") }
            lastReachability = false
            switch phase {
            case .stopping:
                phase = .stopped
                appendLog("服务已停止")
            case .starting where process?.isRunning == true:
                break
            case .failed:
                break
            default:
                if process?.isRunning != true { phase = .stopped }
            }
        }
    }

    func copyPairingURL() { copy(pairingURL, label: "HTTP 配对链接") }
    func copyDeepLink() { copy(pairingDeepLink, label: "扫码深链") }

    func openPairingURL() {
        guard let pairingEndpoint = URL(string: pairingURL) else { return }
        NSWorkspace.shared.open(pairingEndpoint)
    }

    func clearLogs() { logs.removeAll() }

    func copyLogs() {
        let value = logs.map(\.message).joined(separator: "\n")
        copy(value, label: "日志")
    }

    func rebuildFallbackPairing() {
        let preferredLANAddress = settings.lastLANAddress.isEmpty
            ? LANAddressResolver.currentIPv4()
            : settings.lastLANAddress
        guard !knownSecret.isEmpty,
              let pairing = PairingLinks.pairingURL(
                publicHost: settings.publicHost,
                lanAddress: preferredLANAddress,
                port: settings.daemonPort,
                secret: knownSecret
              ) else { return }
        pairingURL = pairing
        pairingDeepLink = PairingLinks.deepLink(
            pairingURL: pairing,
            workspace: settings.workspace,
            fallbackSecret: knownSecret
        ) ?? ""
    }

    private func configurationIssue() -> String? {
        var isDirectory: ObjCBool = false
        guard FileManager.default.fileExists(atPath: settings.workspace, isDirectory: &isDirectory), isDirectory.boolValue else {
            return "工作区不是有效目录"
        }
        guard (1...65535).contains(settings.daemonPort), (1...65535).contains(settings.agentPort) else {
            return "端口必须在 1 到 65535 之间"
        }
        guard settings.daemonPort != settings.agentPort else { return "Daemon 与 Agent 端口不能相同" }
        guard !settings.bindAddress.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty else {
            return "Bind 地址不能为空"
        }
        return nil
    }

    private func refreshRuntimeConfiguration() async {
        guard let configurationEndpoint = localURL(path: "/config.json") else { return }
        do {
            let (data, response) = try await session.data(from: configurationEndpoint)
            guard let http = response as? HTTPURLResponse, http.statusCode == 200 else { return }
            let runtime = try JSONDecoder().decode(RuntimeConfiguration.self, from: data)
            if let lan = runtime.lanAddress, !lan.isEmpty { settings.lastLANAddress = lan }
            let pairing = runtime.pairingURL.flatMap { URL(string: $0) }?.absoluteString
                ?? PairingLinks.pairingURL(
                    publicHost: settings.publicHost,
                    lanAddress: runtime.lanAddress ?? settings.lastLANAddress,
                    port: settings.daemonPort,
                    secret: knownSecret
                )
            if let pairing {
                pairingURL = pairing
                pairingDeepLink = PairingLinks.deepLink(
                    pairingURL: pairing,
                    workspace: runtime.workingDirectory ?? settings.workspace,
                    fallbackSecret: knownSecret
                ) ?? ""
            }
        } catch {
            rebuildFallbackPairing()
        }
    }

    private func localURL(path: String) -> URL? {
        var components = URLComponents()
        components.scheme = "http"
        components.host = "127.0.0.1"
        components.port = settings.daemonPort
        components.path = path
        return components.url
    }

    private func processDidTerminate(status: Int32) {
        outputPipe?.fileHandleForReading.readabilityHandler = nil
        flushLogRemainder()
        process = nil
        outputPipe = nil
        appendLog("daemon 进程已退出（状态 \(status)）")
        if phase == .stopping {
            phase = .stopped
        } else if status != 0 && !isReachable {
            phase = .failed("daemon 异常退出（状态 \(status)）")
        }
        Task { await pollHealth() }
    }

    private func launchEnvironment() -> [String: String] {
        var environment = ProcessInfo.processInfo.environment
        let home = FileManager.default.homeDirectoryForCurrentUser.path
        let additions = ["\(home)/.grok/bin", "/opt/homebrew/bin", "/usr/local/bin"]
        let current = environment["PATH"] ?? "/usr/bin:/bin:/usr/sbin:/sbin"
        environment["PATH"] = (additions + [current]).joined(separator: ":")
        return environment
    }

    private func consumeLogChunk(_ chunk: String) {
        logRemainder += chunk
        var lines = logRemainder.components(separatedBy: "\n")
        logRemainder = lines.removeLast()
        for line in lines where !line.isEmpty { appendLog(line.trimmingCharacters(in: .newlines)) }
        if logRemainder.count > 8_192 {
            appendLog(String(logRemainder.prefix(8_192)))
            logRemainder.removeFirst(min(8_192, logRemainder.count))
        }
    }

    private func flushLogRemainder() {
        guard !logRemainder.isEmpty else { return }
        appendLog(logRemainder)
        logRemainder = ""
    }

    private func appendLog(_ message: String) {
        var safeMessage = message
        if !knownSecret.isEmpty { safeMessage = safeMessage.replacingOccurrences(of: knownSecret, with: "••••••••") }
        logs.append(LogEntry(date: Date(), message: safeMessage))
        if logs.count > 1_000 { logs.removeFirst(logs.count - 1_000) }
    }

    private func copy(_ value: String, label: String) {
        guard !value.isEmpty else { return }
        NSPasteboard.general.clearContents()
        NSPasteboard.general.setString(value, forType: .string)
        appendLog("已复制\(label)")
    }
}

private enum DaemonLocator {
    static func locate() -> URL? {
        let fileManager = FileManager.default
        var candidates: [URL] = []
        if let bundled = Bundle.main.url(forResource: "grok-remote-daemon", withExtension: nil, subdirectory: "Daemon") {
            candidates.append(bundled)
        }
        if let bundled = Bundle.main.url(forResource: "grok-remote-daemon", withExtension: nil) {
            candidates.append(bundled)
        }
        candidates.append(Bundle.main.bundleURL.appendingPathComponent("Contents/MacOS/grok-remote-daemon"))

        #if DEBUG
        let sourceRoot = URL(fileURLWithPath: #filePath)
            .deletingLastPathComponent()
            .deletingLastPathComponent()
            .deletingLastPathComponent()
        candidates.append(sourceRoot.appendingPathComponent("dist/grok-remote-daemon"))
        #endif

        var cursor = URL(fileURLWithPath: fileManager.currentDirectoryPath, isDirectory: true)
        for _ in 0..<7 {
            candidates.append(cursor.appendingPathComponent("dist/grok-remote-daemon"))
            candidates.append(cursor.appendingPathComponent("grok-remote-app/dist/grok-remote-daemon"))
            let parent = cursor.deletingLastPathComponent()
            if parent == cursor { break }
            cursor = parent
        }
        return candidates.first { fileManager.isExecutableFile(atPath: $0.standardizedFileURL.path) }?.standardizedFileURL
    }
}

private enum PairingSecretStore {
    struct Value { let secret: String; let fileURL: URL }

    static func loadOrCreate() throws -> Value {
        let fileManager = FileManager.default
        let applicationSupportDirectory = try fileManager.url(
            for: .applicationSupportDirectory,
            in: .userDomainMask,
            appropriateFor: nil,
            create: true
        ).appendingPathComponent("GrokRemoteLauncher", isDirectory: true)
        try fileManager.createDirectory(
            at: applicationSupportDirectory,
            withIntermediateDirectories: true,
            attributes: [.posixPermissions: 0o700]
        )
        let secretFileURL = applicationSupportDirectory.appendingPathComponent("pairing-secret")
        if let data = try? Data(contentsOf: secretFileURL),
           let value = String(data: data, encoding: .utf8)?.trimmingCharacters(in: .whitespacesAndNewlines),
           value.count >= 16 {
            try? fileManager.setAttributes([.posixPermissions: 0o600], ofItemAtPath: secretFileURL.path)
            return Value(secret: value, fileURL: secretFileURL)
        }
        var generator = SystemRandomNumberGenerator()
        let secret = (0..<32).map { _ in String(format: "%02x", UInt8.random(in: .min ... .max, using: &generator)) }.joined()
        try Data((secret + "\n").utf8).write(to: secretFileURL, options: .atomic)
        try fileManager.setAttributes([.posixPermissions: 0o600], ofItemAtPath: secretFileURL.path)
        return Value(secret: secret, fileURL: secretFileURL)
    }
}

private enum PairingLinks {
    static func pairingURL(publicHost: String, lanAddress: String, port: Int, secret: String) -> String? {
        let trimmedHost = publicHost.trimmingCharacters(in: .whitespacesAndNewlines)
        var components: URLComponents
        if trimmedHost.isEmpty {
            components = URLComponents()
            components.scheme = "http"
            components.host = lanAddress.isEmpty ? "127.0.0.1" : lanAddress
            components.port = port
        } else {
            let hasScheme = trimmedHost.contains("://")
            guard var parsed = URLComponents(string: hasScheme ? trimmedHost : "http://\(trimmedHost)"), parsed.host != nil else {
                return nil
            }
            if !hasScheme, parsed.port == nil, port != 80 { parsed.port = port }
            components = parsed
        }
        components.path = "/"
        components.queryItems = [URLQueryItem(name: "key", value: secret), URLQueryItem(name: "auto", value: "1")]
        components.fragment = nil
        return components.url?.absoluteString
    }

    static func deepLink(pairingURL: String, workspace: String, fallbackSecret: String) -> String? {
        guard var serverComponents = URLComponents(string: pairingURL), serverComponents.host != nil else { return nil }
        let key = serverComponents.queryItems?.first(where: { $0.name == "key" })?.value ?? fallbackSecret
        serverComponents.query = nil
        serverComponents.fragment = nil
        guard let baseURL = serverComponents.url?.absoluteString, !key.isEmpty else { return nil }
        var deepLinkComponents = URLComponents()
        deepLinkComponents.scheme = "grokremote"
        deepLinkComponents.host = "pair"
        deepLinkComponents.queryItems = [
            URLQueryItem(name: "url", value: baseURL),
            URLQueryItem(name: "key", value: key),
            URLQueryItem(name: "cwd", value: workspace),
        ]
        return deepLinkComponents.url?.absoluteString
    }
}

private enum LANAddressResolver {
    static func currentIPv4() -> String {
        Host.current().addresses.first { address in
            let octets = address.split(separator: ".", omittingEmptySubsequences: false)
            return octets.count == 4
                && !address.hasPrefix("127.")
                && !address.hasPrefix("169.254.")
        } ?? ""
    }
}
