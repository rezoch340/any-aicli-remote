import AnyAICLIRemoteCore
import Foundation

@MainActor
public final class ChatStore: ObservableObject {
  @Published var connection: ConnectionStatus = .disconnected
  @Published private(set) var devices: [SavedDevice]
  @Published private(set) var activeDeviceID: UUID?
  @Published private(set) var navigationResetToken = UUID()
  @Published private(set) var deviceMessage = ""
  @Published private(set) var deviceMessageIsError = false
  @Published private(set) var deviceHealthStatuses: [UUID: DeviceHealthStatus] = [:]
  @Published var sessions: [SessionSummary] = []
  @Published var selectedSession: SessionSummary?
  @Published var blocks: [ChatBlock] = []
  @Published var isBusy = false
  @Published var statusMessage = ""
  @Published var modelState = ModelState()
  @Published var selectedFiles: [WorkspaceFile] = []
  @Published var filePickerPath = "."
  @Published var filePickerParent: String?
  @Published var filePickerDirectories: [WorkspaceFile] = []
  @Published var filePickerFiles: [WorkspaceFile] = []
  @Published var filePickerVisible = false
  @Published var filePickerLoading = false
  @Published var filePickerError: String?
  @Published private(set) var isSessionLoading = false

  let client: AnyAICLIRemoteClient
  private let healthProbe: DeviceHealthProbe
  private let runtimeConfiguration: ClientRuntimeConfiguration
  private let deviceRepository: DeviceProfileRepository
  private let pendingUserEchoTracker = PendingUserEchoTracker()
  private var activeTurnID: UUID?
  private var connectionGeneration = UUID()
  private var sessionGeneration = UUID()
  private var sessionLoadTask: Task<Void, Never>?
  private var mountedSessionIdentity: SessionIdentity?
  private var healthProbeGeneration = UUID()

  public init(
    healthProbe: DeviceHealthProbe? = nil,
    runtimeConfiguration: ClientRuntimeConfiguration = ClientRuntimeConfiguration(),
    deviceRepository: DeviceProfileRepository = DeviceProfileRepository()
  ) {
    self.runtimeConfiguration = runtimeConfiguration
    client = AnyAICLIRemoteClient(runtimeConfiguration: runtimeConfiguration)
    self.healthProbe = healthProbe ?? DeviceHealthProbe(runtimeConfiguration: runtimeConfiguration)
    self.deviceRepository = deviceRepository
    let loadResult = deviceRepository.loadDevices()
    devices = loadResult.devices
    if let errorMessage = loadResult.errorMessage {
      deviceMessage = errorMessage
      deviceMessageIsError = true
    }
    client.onNotification = { [weak self] payload in self?.handleNotification(payload) }
    client.onDisconnect = { [weak self] error in self?.handleDisconnect(error) }
  }

  var healthPollingInterval: TimeInterval { runtimeConfiguration.healthPollingInterval }
  var activeDevice: SavedDevice? { devices.first { $0.id == activeDeviceID } }

  func pairingKey(for deviceID: UUID) throws -> String {
    try deviceRepository.pairingKey(for: deviceID)
  }

  @discardableResult
  func saveDevice(id: UUID? = nil, name: String, address: String, pairingKey: String) throws -> UUID {
    let result = try deviceRepository.save(
      id: id, name: name, address: address, pairingKey: pairingKey, devices: devices)
    devices = result.devices
    deviceHealthStatuses[result.deviceID] = .checking
    deviceMessage = "已保存 \(result.device.name)"
    deviceMessageIsError = false
    return result.deviceID
  }

  func deleteDevice(_ deviceID: UUID) throws {
    guard let device = devices.first(where: { $0.id == deviceID }) else { return }
    devices = try deviceRepository.delete(deviceID: deviceID, devices: devices)
    deviceHealthStatuses.removeValue(forKey: deviceID)
    if activeDeviceID == deviceID { disconnect() }
    deviceMessage = "已删除 \(device.name)"
    deviceMessageIsError = false
  }

  func reportDeviceError(_ error: Error) {
    deviceMessage = error.localizedDescription
    deviceMessageIsError = true
  }
  func deviceHealthStatus(for deviceID: UUID) -> DeviceHealthStatus {
    deviceHealthStatuses[deviceID] ?? .checking
  }

  func refreshDeviceHealth() async {
    let devicesSnapshot = devices
    let refreshGeneration = UUID()
    healthProbeGeneration = refreshGeneration
    let currentDeviceIDs = Set(devicesSnapshot.map(\.id))
    deviceHealthStatuses = deviceHealthStatuses.filter { currentDeviceIDs.contains($0.key) }
    for device in devicesSnapshot {
      deviceHealthStatuses[device.id] = .checking
    }

    let monitor = DeviceHealthMonitor(healthProbe: healthProbe)
    let statuses = await monitor.probe(devices: devicesSnapshot)
    guard healthProbeGeneration == refreshGeneration else { return }
    for (deviceID, isOnline) in statuses where devices.contains(where: { $0.id == deviceID }) {
      deviceHealthStatuses[deviceID] = isOnline ? .online : .offline
    }
  }

  @discardableResult
  public func importPairingDeepLink(_ pairingDeepLink: URL) -> Bool {
    do {
      let parsedLink = try PairingDeepLink.parse(pairingDeepLink)
      let existingDevice = devices.first { $0.baseURL == parsedLink.profile.baseURL }
      let name =
        parsedLink.name ?? existingDevice?.name ?? parsedLink.profile.baseURL.host
        ?? ProductIdentifiers.displayName
      _ = try saveDevice(
        id: existingDevice?.id, name: name, address: parsedLink.serviceAddress,
        pairingKey: parsedLink.profile.key)
      disconnect()
      deviceMessage = existingDevice == nil ? "设备已添加，请点击设备连接" : "设备已更新，请点击设备连接"
      deviceMessageIsError = false
      return true
    } catch {
      reportDeviceError(error)
      return false
    }
  }

  func connect(to deviceID: UUID) async -> Bool {
    guard let device = devices.first(where: { $0.id == deviceID }) else { return false }
    if activeDeviceID == deviceID, connection == .connected { return true }
    let attemptID = UUID()
    connectionGeneration = attemptID
    client.disconnect(notify: false)
    clearSessionState()
    activeDeviceID = deviceID
    connection = .connecting
    deviceMessage = "正在连接 \(device.name)"
    deviceMessageIsError = false
    do {
      let profile = ServerProfile(baseURL: device.baseURL, key: try pairingKey(for: deviceID))
      guard !profile.key.isEmpty else { throw ClientError.missingKey }
      let initialize = try await client.connect(profile: profile)
      guard ownsConnectionAttempt(attemptID, deviceID: deviceID) else { return false }
      let refreshedSessions = try await fetchSessions()
      guard ownsConnectionAttempt(attemptID, deviceID: deviceID), client.isConnected else {
        return false
      }
      modelState = SessionPayloadMapper.modelState(from: initialize, current: modelState)
      sessions = refreshedSessions
      connection = .connected
      statusMessage = "已连接"
      deviceMessage = ""
      deviceMessageIsError = false
      return true
    } catch {
      guard ownsConnectionAttempt(attemptID, deviceID: deviceID) else { return false }
      client.disconnect(notify: false)
      activeDeviceID = nil
      connection = .failed(error.localizedDescription)
      statusMessage = error.localizedDescription
      deviceMessage = "连接失败：\(error.localizedDescription)"
      deviceMessageIsError = true
      navigationResetToken = UUID()
      return false
    }
  }

  func disconnect() {
    connectionGeneration = UUID()
    client.disconnect(notify: false)
    activeDeviceID = nil
    clearSessionState()
    connection = .disconnected
    statusMessage = ""
    navigationResetToken = UUID()
  }

  func refreshSessions() async throws {
    guard let context = currentConnectionContext() else { throw ClientError.disconnected }
    try await refreshSessions(context: context)
  }

  func openSession(_ session: SessionSummary) {
    guard let connectionContext = currentConnectionContext() else { return }
    sessionLoadTask?.cancel()
    isSessionLoading = true
    let requiresSessionLoad = mountedSessionIdentity != session.id
    if requiresSessionLoad { mountedSessionIdentity = nil }
    let generation = UUID()
    sessionGeneration = generation
    resetTurnTracking()
    selectedSession = session
    blocks = []
    isBusy = false
    closeWorkspaceFilePicker(clearSelection: true)
    statusMessage = "同步历史"
    let context = SessionContext(
      connection: connectionContext, sessionIdentity: session.id, generation: generation)
    sessionLoadTask = Task { [weak self] in
      await self?.loadSession(session, context: context, requiresSessionLoad: requiresSessionLoad)
    }
  }

  func closeSession(_ sessionIdentity: SessionIdentity) {
    guard selectedSession?.id == sessionIdentity else { return }
    sessionLoadTask?.cancel()
    sessionLoadTask = nil
    isSessionLoading = false
    sessionGeneration = UUID()
    resetTurnTracking()
    selectedSession = nil
    blocks = []
    isBusy = false
    closeWorkspaceFilePicker(clearSelection: true)
    statusMessage = connection == .connected ? "已连接" : ""
  }

  func createSession(workingDirectory: String) async -> Bool {
    guard let context = currentConnectionContext() else { return false }
    let directory = workingDirectory.trimmingCharacters(in: .whitespacesAndNewlines)
    guard !directory.isEmpty else { return false }
    let previousIdentities = Set(sessions.map(\.id))
    do {
      let response = try await client.rpc(
        ACPWire.Method.sessionNew,
        params: ACPWire.newSessionParameters(workingDirectory: directory),
        timeout: runtimeConfiguration.sessionCreateTimeout
      )
      guard !Task.isCancelled, ownsConnection(context) else { return false }
      guard let payload = response as? [String: Any] else {
        throw ClientError.malformedResponse
      }
      do { try await refreshSessions(context: context) } catch {
        guard !Task.isCancelled, ownsConnection(context) else { return false }
        statusMessage = "会话列表刷新失败：\(error.localizedDescription)"
      }
      guard !Task.isCancelled, ownsConnection(context) else { return false }
      sessionLoadTask?.cancel()
      sessionLoadTask = nil
      isSessionLoading = false
      resetTurnTracking()
      selectedSession = nil
      sessionGeneration = UUID()
      mountedSessionIdentity = nil
      if let session = try SessionPayloadMapper.createdSession(
        from: payload, sessions: sessions, previousSessionIdentities: previousIdentities) {
        selectedSession = session
        mountedSessionIdentity = session.id
        statusMessage = "已创建"
      } else {
        statusMessage = "已创建，等待历史索引"
      }
      return true
    } catch {
      guard !Task.isCancelled, ownsConnection(context) else { return false }
      statusMessage = error.localizedDescription
      return false
    }
  }

  func send(_ text: String) {
    guard let session = selectedSession,
      let context = currentSessionContext(sessionIdentity: session.id)
    else { return }
    let trimmedText = text.trimmingCharacters(in: .whitespacesAndNewlines)
    let attachments = selectedFiles
    guard !trimmedText.isEmpty || !attachments.isEmpty else { return }
    let turnID = UUID()
    activeTurnID = turnID
    pendingUserEchoTracker.clear()
    if !trimmedText.isEmpty { pendingUserEchoTracker.begin(text: trimmedText) }
    blocks.append(
      ChatBlock(id: UUID().uuidString, kind: .user, text: trimmedText, attachments: attachments))
    closeWorkspaceFilePicker(clearSelection: true)
    isBusy = true
    statusMessage = "等待助手"
    Task {
      do {
        guard ownsSession(context) else { return }
        let parameters = ACPWire.promptParameters(
          sessionID: session.sessionID,
          text: trimmedText,
          attachments: attachments
        )
        let response = try await client.rpc(ACPWire.Method.sessionPrompt, params: parameters)
        guard !Task.isCancelled, ownsSession(context), activeTurnID == turnID else { return }
        let stopReason = (response as? [String: Any])?.string("stopReason")?.lowercased()
        let wasCancelled = stopReason?.contains("cancel") == true
        finishTurn(
          toolState: wasCancelled ? .cancelled : .success,
          status: wasCancelled ? "已停止" : "完成"
        )
      } catch {
        guard !Task.isCancelled, ownsSession(context), activeTurnID == turnID else { return }
        if error.localizedDescription.lowercased().contains("cancel") {
          finishTurn(toolState: .cancelled, status: "已停止")
        } else {
          blocks.append(
            ChatBlock(id: UUID().uuidString, kind: .system, text: error.localizedDescription))
          finishTurn(toolState: .failed, status: "发送失败")
        }
      }
    }
  }

  func cancel() {
    guard let session = selectedSession,
      let context = currentSessionContext(sessionIdentity: session.id)
    else { return }
    finishTurn(toolState: .cancelled, status: "已停止")
    Task {
      guard ownsSession(context) else { return }
      try? await client.notify(
        ACPWire.Method.sessionCancel,
        params: ACPWire.cancelSessionParameters(sessionID: session.sessionID))
      guard !Task.isCancelled, ownsSession(context) else { return }
      finishTurn(toolState: .cancelled, status: "已停止")
    }
  }

  func setEffort(_ effort: String) {
    guard let session = selectedSession,
      let context = currentSessionContext(sessionIdentity: session.id)
    else { return }
    let modelID = modelState.currentModelID
    Task {
      do {
        guard ownsSession(context) else { return }
        let body: [String: Any] = [
          "providerId": session.providerID,
          "sessionId": session.sessionID,
          "modelId": modelID,
          "effort": effort
        ]
        _ = try await client.rest(path: "/api/effort", method: "POST", body: body)
        guard !Task.isCancelled, ownsSession(context) else { return }
        modelState.effort = effort
      } catch  where !Task.isCancelled && ownsSession(context) {
        statusMessage = "切换失败：\(error.localizedDescription)"
      }
    }
  }

  func answerPermission(blockID: String, optionID: String?) {
    guard let index = blocks.firstIndex(where: { $0.id == blockID }),
      let rpcID = blocks[index].rpcID,
      let sessionIdentity = selectedSession?.id,
      let context = currentSessionContext(sessionIdentity: sessionIdentity)
    else { return }
    Task {
      guard ownsSession(context) else { return }
      try? await client.reply(id: rpcID, result: ACPWire.permissionReplyResult(optionID: optionID))
      guard !Task.isCancelled, ownsSession(context) else { return }
      blocks.removeAll { $0.id == blockID }
    }
  }

  private func refreshSessions(context: ConnectionContext) async throws {
    guard ownsConnection(context) else { throw CancellationError() }
    let refreshedSessions = try await fetchSessions()
    guard !Task.isCancelled, ownsConnection(context) else { throw CancellationError() }
    sessions = refreshedSessions
  }

  private func fetchSessions() async throws -> [SessionSummary] {
    try SessionPayloadMapper.sessions(from: await client.rest(path: "/api/sessions"))
  }

  private func loadSession(
    _ session: SessionSummary, context: SessionContext, requiresSessionLoad: Bool
  ) async {
    defer {
      if sessionGeneration == context.generation {
        sessionLoadTask = nil
        isSessionLoading = false
      }
    }
    do {
      let response = try await client.rest(
        pathComponents: ["api", "sessions", session.sessionID, "messages"],
        query: [URLQueryItem(name: "providerId", value: session.providerID)]
      )
      guard !Task.isCancelled, ownsSession(context) else { return }
      let history = try SessionPayloadMapper.history(from: response, fallback: session)
      applyAuthoritativeSession(history.session)
      blocks = history.blocks
    } catch {
      guard !Task.isCancelled, ownsSession(context) else { return }
      statusMessage = "历史暂不可用：\(error.localizedDescription)"
    }
    guard !Task.isCancelled, ownsSession(context) else { return }
    guard requiresSessionLoad else {
      statusMessage = "在线"
      return
    }
    do {
      let response = try await client.rpc(
        ACPWire.Method.sessionLoad,
        params: ACPWire.loadSessionParameters(sessionID: session.sessionID),
        timeout: runtimeConfiguration.sessionLoadTimeout
      )
      guard !Task.isCancelled, ownsSession(context) else { return }
      if let payload = response as? [String: Any] {
        modelState = SessionPayloadMapper.modelState(from: payload, current: modelState)
      }
      mountedSessionIdentity = session.id
      statusMessage = "在线"
    } catch {
      guard !Task.isCancelled, ownsSession(context) else { return }
      statusMessage = "挂载失败：\(error.localizedDescription)"
    }
  }

  private func handleDisconnect(_ error: Error?) {
    guard activeDeviceID != nil else { return }
    connectionGeneration = UUID()
    client.disconnect(notify: false)
    activeDeviceID = nil
    clearSessionState()
    let message = error?.localizedDescription ?? "连接中断"
    connection = .failed(message)
    statusMessage = message
    deviceMessage = "设备连接已断开"
    deviceMessageIsError = true
    navigationResetToken = UUID()
  }

  private func handleNotification(_ payload: [String: Any]) {
    guard connection == .connected,
      let notification = ChatNotificationMapper.map(
        payload: payload,
        selectedSessionID: selectedSession?.id
      )
    else { return }

    switch notification {
    case .sessionUpdate(let update):
      applyTranscriptUpdate(update)
    case .sessionsChanged:
      guard let context = currentConnectionContext() else { return }
      Task { try? await refreshSessions(context: context) }
    case .permission(let request):
      let options =
        request.options.isEmpty ? [PermissionOption(id: "allow", label: "允许")] : request.options
      blocks.append(
        ChatBlock(
          id: "permission-\(request.rpcID)",
          kind: .permission,
          text: request.question,
          rpcID: request.rpcID,
          options: options
        )
      )
    }
  }

  private func applyTranscriptUpdate(_ update: [String: Any]) {
    switch ChatTranscriptReducer.apply(
      update: update, to: &blocks, pendingUserEchoTracker: pendingUserEchoTracker) {
    case .none: break
    case .busy(let status)
    where ChatTranscriptReducer.shouldMarkTurnBusy(activeTurnID: activeTurnID):
      isBusy = true
      statusMessage = status
    case .busy: break
    case .finished(let state, let status): finishTurn(toolState: state, status: status)
    }
  }

  private func resetTurnTracking() {
    activeTurnID = nil
    pendingUserEchoTracker.clear()
  }
  private func finishTurn(toolState: ToolRunState, status: String) {
    resetTurnTracking()
    blocks = ChatTranscriptReducer.finalizeActiveTools(in: blocks, as: toolState)
    isBusy = false
    statusMessage = status
  }
  private func clearSessionState() {
    sessionLoadTask?.cancel()
    sessionLoadTask = nil
    isSessionLoading = false
    sessionGeneration = UUID()
    mountedSessionIdentity = nil
    resetTurnTracking()
    sessions = []
    selectedSession = nil
    blocks = []
    isBusy = false
    modelState = ModelState()
    closeWorkspaceFilePicker(clearSelection: true)
  }

  private func applyAuthoritativeSession(_ session: SessionSummary) {
    if let index = sessions.firstIndex(where: { $0.id == session.id }) {
      sessions[index] = session
    } else {
      sessions.append(session)
    }
    selectedSession = session
  }

  private func ownsConnectionAttempt(_ attemptID: UUID, deviceID: UUID) -> Bool {
    connectionGeneration == attemptID && activeDeviceID == deviceID
  }

  private func currentConnectionContext() -> ConnectionContext? {
    guard connection == .connected, client.isConnected, let activeDeviceID else { return nil }
    return ConnectionContext(deviceID: activeDeviceID, generation: connectionGeneration)
  }

  private func ownsConnection(_ context: ConnectionContext) -> Bool {
    connection == .connected && client.isConnected && activeDeviceID == context.deviceID
      && connectionGeneration == context.generation
  }

  func currentSessionContext(sessionIdentity: SessionIdentity) -> SessionContext? {
    guard let connection = currentConnectionContext(), selectedSession?.id == sessionIdentity else {
      return nil
    }
    return SessionContext(
      connection: connection, sessionIdentity: sessionIdentity, generation: sessionGeneration)
  }

  func ownsSession(_ context: SessionContext) -> Bool {
    ownsConnection(context.connection) && selectedSession?.id == context.sessionIdentity
      && sessionGeneration == context.generation
  }
}
