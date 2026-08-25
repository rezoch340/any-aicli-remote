import AnyAICLIRemoteCore
import Foundation

/// 界面状态的持有者与意图入口。具体职责由协调器承担：设备与配对在 `DeviceCoordinator`，
/// 连接在 `ConnectionCoordinator`，会话在 `SessionCoordinator`，一轮对话与通知在
/// `TurnCoordinator`，工作区文件在 `WorkspaceBrowser`，代际归属与会话清理在 `ChatOwnership`。
///
/// 发布属性对模块内可写：写入方是上述协调器，视图仍只读取它们。
@MainActor
public final class ChatStore: ObservableObject {
  @Published var connection: ConnectionStatus = .disconnected
  @Published var devices: [SavedDevice]
  @Published var activeDeviceID: UUID?
  @Published var navigationResetToken = UUID()
  @Published var deviceMessage = ""
  @Published var deviceMessageIsError = false
  @Published var deviceHealthStatuses: [UUID: DeviceHealthStatus] = [:]
  @Published var sessions: [SessionSummary] = []
  @Published var selectedSession: SessionSummary?
  @Published var blocks: [ChatBlock] = []
  @Published var childAgents: [ChildAgentCard] = []
  @Published var sessionMode = ""
  @Published var sessionNotice = ""
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
  @Published var isSessionLoading = false
  @Published var pendingInteraction: PendingInteraction?

  let client: AnyAICLIRemoteClient
  private let runtimeConfiguration: ClientRuntimeConfiguration
  private(set) var ownership: ChatOwnership!
  private(set) var deviceCoordinator: DeviceCoordinator!
  private(set) var connectionCoordinator: ConnectionCoordinator!
  private(set) var sessionCoordinator: SessionCoordinator!
  private(set) var turnCoordinator: TurnCoordinator!
  private(set) var interactionController: InteractionController!

  public init(
    healthProbe: DeviceHealthProbe? = nil,
    runtimeConfiguration: ClientRuntimeConfiguration = ClientRuntimeConfiguration(),
    deviceRepository: DeviceProfileRepository = DeviceProfileRepository()
  ) {
    self.runtimeConfiguration = runtimeConfiguration
    client = AnyAICLIRemoteClient(runtimeConfiguration: runtimeConfiguration)
    let resolvedHealthProbe =
      healthProbe ?? DeviceHealthProbe(runtimeConfiguration: runtimeConfiguration)
    let loadResult = deviceRepository.loadDevices()
    devices = loadResult.devices
    if let errorMessage = loadResult.errorMessage {
      deviceMessage = errorMessage
      deviceMessageIsError = true
    }
    ownership = ChatOwnership(store: self)
    interactionController = InteractionController(store: self, ownership: ownership)
    deviceCoordinator = DeviceCoordinator(
      store: self, healthProbe: resolvedHealthProbe, deviceRepository: deviceRepository)
    connectionCoordinator = ConnectionCoordinator(store: self, ownership: ownership)
    sessionCoordinator = SessionCoordinator(
      store: self, ownership: ownership, runtimeConfiguration: runtimeConfiguration)
    turnCoordinator = TurnCoordinator(store: self, ownership: ownership)
    client.onNotification = { [weak self] payload in
      self?.turnCoordinator.handleNotification(payload)
    }
    client.onDisconnect = { [weak self] error in
      self?.connectionCoordinator.handleDisconnect(error)
    }
  }

  var healthPollingInterval: TimeInterval { runtimeConfiguration.healthPollingInterval }
  var activeDevice: SavedDevice? { devices.first { $0.id == activeDeviceID } }

  func pairingKey(for deviceID: UUID) throws -> String {
    try deviceCoordinator.pairingKey(for: deviceID)
  }

  @discardableResult
  func saveDevice(id: UUID? = nil, name: String, address: String, pairingKey: String) throws -> UUID {
    try deviceCoordinator.saveDevice(id: id, name: name, address: address, pairingKey: pairingKey)
  }

  func deleteDevice(_ deviceID: UUID) throws { try deviceCoordinator.deleteDevice(deviceID) }

  func reportDeviceError(_ error: Error) { deviceCoordinator.reportDeviceError(error) }

  func deviceHealthStatus(for deviceID: UUID) -> DeviceHealthStatus {
    deviceCoordinator.deviceHealthStatus(for: deviceID)
  }

  func refreshDeviceHealth() async { await deviceCoordinator.refreshDeviceHealth() }

  @discardableResult
  public func importPairingDeepLink(_ pairingDeepLink: URL) -> Bool {
    deviceCoordinator.importPairingDeepLink(pairingDeepLink)
  }

  func connect(to deviceID: UUID) async -> Bool { await connectionCoordinator.connect(to: deviceID) }

  func disconnect() { connectionCoordinator.disconnect() }

  func refreshSessions() async throws { try await sessionCoordinator.refreshSessions() }

  func openSession(_ session: SessionSummary) { sessionCoordinator.openSession(session) }

  func closeSession(_ sessionIdentity: SessionIdentity) {
    sessionCoordinator.closeSession(sessionIdentity)
  }

  func createSession(workingDirectory: String) async -> Bool {
    await sessionCoordinator.createSession(workingDirectory: workingDirectory)
  }

  func send(_ text: String) { turnCoordinator.send(text) }

  func cancel() { turnCoordinator.cancel() }

  func answerInteraction(_ interaction: PendingInteraction, answer: InteractionAnswer) {
    interactionController.answer(interaction, answer: answer)
  }

  func setEffort(_ effort: String) { turnCoordinator.setEffort(effort) }

  func answerPermission(blockID: String, optionID: String?) {
    turnCoordinator.answerPermission(blockID: blockID, optionID: optionID)
  }

  func currentSessionContext(sessionIdentity: SessionIdentity) -> SessionContext? {
    ownership.currentSessionContext(sessionIdentity: sessionIdentity)
  }

  func ownsSession(_ context: SessionContext) -> Bool { ownership.ownsSession(context) }
}
