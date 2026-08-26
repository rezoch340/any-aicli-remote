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
  @Published private(set) var logs = [LogEntry]()
  private var policy: LauncherPolicy?
  private var store: DaemonConfigurationStore?
  private var client: DaemonHTTPClient?
  private var process: ManagedDaemonProcess?
  private var pollingTask: Task<Void, Never>?
  private var secret = ""
  private var secretFileURL: URL?
  private var lifecycleGeneration = LifecycleGeneration()
  private var activated = false
  private var logBuffer: LauncherLogBuffer?
  private var secretRemovalFailureLogged = false
  init(settings: LauncherSettings) {
    self.settings = settings
  }
  var configurationEditable: Bool {
    settings.isConfigurationLoaded && !isReachable && process?.isRunning != true
      && phase != .starting && phase != .stopping
  }
  var showsStopAction: Bool {
    isReachable || process?.isRunning == true || phase == .starting
  }
  var configurationPath: String {
    store?.configurationURL.path ?? DaemonConfigurationPath.defaultURL().path
  }
  func activate() {
    guard !activated else { return }
    activated = true
    do {
      let loadedPolicy = try LauncherPolicy.load()
      guard let executable = DaemonLocator.locate(policy: loadedPolicy) else {
        throw ControllerError.daemonMissing
      }
      policy = loadedPolicy
      daemonExecutablePath = executable.path
      let configurationStore = DaemonConfigurationStore(
        runner: ProcessDaemonCommandRunner(executableURL: executable),
        configurationURL: DaemonConfigurationPath.defaultURL())
      let configurationURL = configurationStore.configurationURL
      let document = try settings.migrateConfiguration(
        store: configurationStore, configurationURL: configurationURL)
      try settings.loadDraft(from: document)
      secret = try PairingSecretStore.loadOrCreate()
      store = configurationStore
      try rebuildClient()
      logBuffer = LauncherLogBuffer(
        maximumChunkCharacters: loadedPolicy.maximumLogChunkCharacters,
        maximumEntries: loadedPolicy.maximumLogEntries,
        redactedValues: { [weak self] in [self?.secret ?? ""] })
      appendLog("配置已加载")
      beginPolling()
    } catch {
      fail(error)
    }
  }
  func refreshDaemonLocation() {
    if let policy {
      daemonExecutablePath = DaemonLocator.locate(policy: policy)?.path ?? ""
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
    Task { await startAfterSave() }
  }
  func saveConfiguration() async -> Bool {
    guard configurationEditable, let store else { return false }
    do {
      let document = try store.save(
        editable: DaemonEditableConfiguration(
          bindAddress: settings.bindAddress,
          daemonPort: settings.daemonPort,
          publicHost: settings.publicHost,
          agentPort: settings.agentPort,
          providerAlwaysApprove: settings.providerAlwaysApprove))
      try settings.loadDraft(from: document)
      try rebuildClient()
      appendLog("配置已保存")
      return true
    } catch {
      fail(error)
      return false
    }
  }
  func stop() async {
    guard let policy else { return }
    let hasManagedProcess = process != nil
    guard hasManagedProcess || isReachable else {
      clearPairing()
      phase = .stopped
      return
    }
    guard phase != .stopping else { return }
    phase = .stopping
    clearPairing()
    do {
      try await client?.stop()
    } catch {
      appendLog("停止请求失败：\(error.localizedDescription)")
    }
    let stoppedAfterApi = await waitForOfflineAndExit(policy: policy)
    if !stoppedAfterApi, let managedProcess = process, managedProcess.isRunning {
      managedProcess.interrupt()
      try? await Task.sleep(for: .seconds(policy.interruptGraceSeconds))
      if managedProcess.isRunning { managedProcess.terminate() }
    }
    let fullyStopped = await waitForOfflineAndExit(policy: policy)
    guard fullyStopped else {
      fail(ControllerError.stopTimeout)
      return
    }
    phase = .stopped
  }
  func restart() {
    Task {
      await stop()
      guard phase == .stopped else { return }
      await startAfterSave()
    }
  }
  func pollHealth() async {
    guard let client else { return }
    do {
      let snapshot = try await client.health()
      health = snapshot
      isReachable = true
      _ = removeSecretFile()
      if phase != .stopping {
        phase = snapshot.ready == true ? .online : .degraded
      }
    } catch {
      health = nil
      isReachable = false
      clearPairing()
      if phase == .stopping {
        return
      }
      if process?.isRunning != true && !isFailedPhase {
        phase = .stopped
      }
      return
    }
    guard phase != .stopping else {
      clearPairing()
      return
    }
    do {
      try await refreshRuntime()
    } catch {
      clearPairing()
      appendLog("运行时配置读取失败：\(error.localizedDescription)")
    }
  }
  func copyPairingURL() {
    copy(pairingURL)
  }
  func copyDeepLink() {
    copy(pairingDeepLink)
  }
  func openPairingURL() {
    if let url = URL(string: pairingURL) {
      NSWorkspace.shared.open(url)
    }
  }
  func clearLogs() {
    logBuffer?.clear()
    logs = []
  }
  func copyLogs() {
    copy(logs.map(\.message).joined(separator: "\n"))
  }
  private func startAfterSave() async {
    guard await saveConfiguration(), let policy, let store else { return }
    guard !daemonExecutablePath.isEmpty else {
      fail(ControllerError.daemonMissing)
      return
    }
    guard removeSecretFile() else {
      fail(ControllerError.secretFileCleanup)
      return
    }
    do {
      secretRemovalFailureLogged = false
      let temporarySecret = try EphemeralDaemonSecretFile.materialize(secret: secret)
      secretFileURL = temporarySecret
      let currentGeneration = lifecycleGeneration.next()
      let executable = URL(fileURLWithPath: daemonExecutablePath)
      let plan = DaemonLaunchPlan(
        configurationURL: store.configurationURL,
        secretFileURL: temporarySecret)
      process = try ManagedDaemonProcess(
        generation: currentGeneration,
        executableURL: executable,
        plan: plan,
        environment: DaemonLocator.launchEnvironment(policy: policy),
        onLog: { [weak self] token, chunk in
          Task { @MainActor in self?.receiveLog(token: token, chunk: chunk) }
        },
        onTermination: { [weak self] token, status in
          Task { @MainActor in self?.terminated(token: token, status: status) }
        })
      phase = .starting
      await pollHealth()
    } catch {
      _ = removeSecretFile()
      fail(error)
    }
  }
  private func rebuildClient() throws {
    guard let policy else { throw ControllerError.policyMissing }
    client = DaemonHTTPClient(
      endpoint: try LocalDaemonEndpoint(
        bindAddress: settings.bindAddress,
        port: settings.daemonPort),
      pairingSecret: secret,
      policy: policy)
  }
  private func beginPolling() {
    guard let policy else { return }
    pollingTask = Task { [weak self] in
      while !Task.isCancelled {
        await self?.pollHealth()
        try? await Task.sleep(for: .seconds(policy.healthPollIntervalSeconds))
      }
    }
  }
  private func refreshRuntime() async throws {
    guard let runtime = try await client?.configuration() else { return }
    guard phase != .stopping else {
      clearPairing()
      return
    }
    if let lanAddress = runtime.lanAddress, !lanAddress.isEmpty {
      settings.lastLANAddress = lanAddress
    }
    let pairing = try RuntimePairing(configuration: runtime)
    guard phase != .stopping else {
      clearPairing()
      return
    }
    pairingURL = pairing.httpURL.absoluteString
    pairingDeepLink = pairing.deepLinkURL.absoluteString
  }
  private func waitForOfflineAndExit(policy: LauncherPolicy) async -> Bool {
    for attempt in 0..<policy.stopPollAttempts {
      await pollHealth()
      let processExited = process?.isRunning != true
      if !isReachable && processExited && process == nil {
        return true
      }
      if attempt + 1 < policy.stopPollAttempts {
        try? await Task.sleep(for: .seconds(policy.stopPollIntervalSeconds))
      }
    }
    return !isReachable && process == nil
  }
  private func receiveLog(token: UInt64, chunk: String) {
    guard lifecycleGeneration.accepts(token) else { return }
    logBuffer?.appendChunk(chunk)
    logs = logBuffer?.entries ?? []
  }
  private func terminated(token: UInt64, status: Int32) {
    guard lifecycleGeneration.accepts(token) else { return }
    logBuffer?.flush()
    logs = logBuffer?.entries ?? []
    process?.close()
    process = nil
    _ = removeSecretFile()
    clearPairing()
    if phase == .stopping {
      phase = .stopped
    } else if status != 0 {
      fail(ControllerError.exited(status))
    }
  }
  private var isFailedPhase: Bool {
    if case .failed = phase { return true }
    return false
  }
  @discardableResult
  private func removeSecretFile() -> Bool {
    guard let secretFileURL else { return true }
    do {
      try EphemeralDaemonSecretFile.remove(at: secretFileURL)
      self.secretFileURL = nil
      return true
    } catch {
      if !secretRemovalFailureLogged {
        appendLog("临时密钥文件删除失败")
        secretRemovalFailureLogged = true
      }
      return false
    }
  }
  private func clearPairing() {
    pairingURL = ""
    pairingDeepLink = ""
  }
  private func appendLog(_ message: String) {
    logBuffer?.append(message)
    logs = logBuffer?.entries ?? logs
  }
  private func fail(_ error: Error) {
    phase = .failed(error.localizedDescription)
    appendLog(error.localizedDescription)
  }
  private func copy(_ value: String) {
    guard !value.isEmpty else { return }
    NSPasteboard.general.clearContents()
    NSPasteboard.general.setString(value, forType: .string)
  }
}
