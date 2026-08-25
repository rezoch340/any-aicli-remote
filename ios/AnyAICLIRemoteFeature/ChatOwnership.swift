import AnyAICLIRemoteCore
import Foundation

/// 连接与会话的归属判定、代际推进和会话状态清理。
/// 代际比较只在这里实现一份，连接、会话与一轮对话的协调器都调用它，不各自复制判断。
@MainActor
final class ChatOwnership {
  private unowned let store: ChatStore
  let pendingUserEchoTracker = PendingUserEchoTracker()
  var activeTurnID: UUID?
  private(set) var connectionGeneration = UUID()
  private(set) var sessionGeneration = UUID()
  var sessionLoadTask: Task<Void, Never>?
  /// 为 true 时表示正在向 provider 挂载会话，其间收到的 session/update 是历史重放。
  var isMountingSession = false
  var mountedSessionIdentity: SessionIdentity?

  init(store: ChatStore) {
    self.store = store
  }

  @discardableResult
  func advanceConnection() -> UUID {
    connectionGeneration = UUID()
    return connectionGeneration
  }

  @discardableResult
  func advanceSession() -> UUID {
    sessionGeneration = UUID()
    return sessionGeneration
  }

  func ownsConnectionAttempt(_ attemptID: UUID, deviceID: UUID) -> Bool {
    connectionGeneration == attemptID && store.activeDeviceID == deviceID
  }

  func currentConnectionContext() -> ConnectionContext? {
    guard store.connection == .connected, store.client.isConnected, let activeDeviceID = store.activeDeviceID
    else { return nil }
    return ConnectionContext(deviceID: activeDeviceID, generation: connectionGeneration)
  }

  func ownsConnection(_ context: ConnectionContext) -> Bool {
    store.connection == .connected && store.client.isConnected && store.activeDeviceID == context.deviceID
      && connectionGeneration == context.generation
  }

  func currentSessionContext(sessionIdentity: SessionIdentity) -> SessionContext? {
    guard let connection = currentConnectionContext(), store.selectedSession?.id == sessionIdentity
    else {
      return nil
    }
    return SessionContext(
      connection: connection, sessionIdentity: sessionIdentity, generation: sessionGeneration)
  }

  func ownsSession(_ context: SessionContext) -> Bool {
    ownsConnection(context.connection) && store.selectedSession?.id == context.sessionIdentity
      && sessionGeneration == context.generation
  }

  func resetTurnTracking() {
    activeTurnID = nil
    pendingUserEchoTracker.clear()
  }

  func clearSessionState() {
    sessionLoadTask?.cancel()
    sessionLoadTask = nil
    store.isSessionLoading = false
    advanceSession()
    mountedSessionIdentity = nil
    resetTurnTracking()
    store.interactionController.clear()
    store.sessions = []
    store.selectedSession = nil
    store.blocks = []
    store.childAgents = []
    store.sessionMode = ""
    store.sessionNotice = ""
    store.isBusy = false
    store.modelState = ModelState()
    store.closeWorkspaceFilePicker(clearSelection: true)
  }
}
