import AnyAICLIRemoteCore
import Foundation

/// 会话列举、打开、历史同步、挂载、关闭与新建。
@MainActor
final class SessionCoordinator {
  private unowned let store: ChatStore
  private let ownership: ChatOwnership
  private let runtimeConfiguration: ClientRuntimeConfiguration

  init(store: ChatStore, ownership: ChatOwnership, runtimeConfiguration: ClientRuntimeConfiguration) {
    self.store = store
    self.ownership = ownership
    self.runtimeConfiguration = runtimeConfiguration
  }

  func refreshSessions() async throws {
    guard let context = ownership.currentConnectionContext() else { throw ClientError.disconnected }
    try await refreshSessions(context: context)
  }

  func refreshSessions(context: ConnectionContext) async throws {
    guard ownership.ownsConnection(context) else { throw CancellationError() }
    let refreshedSessions = try await fetchSessions()
    guard !Task.isCancelled, ownership.ownsConnection(context) else { throw CancellationError() }
    store.sessions = refreshedSessions
  }

  func fetchSessions() async throws -> [SessionSummary] {
    try SessionPayloadMapper.sessions(from: await store.client.rest(path: "/api/sessions"))
  }

  func openSession(_ session: SessionSummary) {
    guard let connectionContext = ownership.currentConnectionContext() else { return }
    ownership.sessionLoadTask?.cancel()
    store.isSessionLoading = true
    let requiresSessionLoad = ownership.mountedSessionIdentity != session.id
    if requiresSessionLoad { ownership.mountedSessionIdentity = nil }
    let generation = ownership.advanceSession()
    ownership.resetTurnTracking()
    store.interactionController.clear()
    store.selectedSession = session
    store.blocks = []
    store.childAgents = []
    store.isBusy = false
    store.closeWorkspaceFilePicker(clearSelection: true)
    store.statusMessage = "同步历史"
    let context = SessionContext(
      connection: connectionContext, sessionIdentity: session.id, generation: generation)
    ownership.sessionLoadTask = Task { [weak self] in
      await self?.loadSession(
        session, context: context, requiresSessionLoad: requiresSessionLoad)
    }
  }

  func closeSession(_ sessionIdentity: SessionIdentity) {
    guard store.selectedSession?.id == sessionIdentity else { return }
    ownership.sessionLoadTask?.cancel()
    ownership.sessionLoadTask = nil
    store.isSessionLoading = false
    ownership.advanceSession()
    ownership.resetTurnTracking()
    store.interactionController.clear()
    store.selectedSession = nil
    store.blocks = []
    store.childAgents = []
    store.isBusy = false
    store.closeWorkspaceFilePicker(clearSelection: true)
    store.statusMessage = store.connection == .connected ? "已连接" : ""
  }

  func createSession(workingDirectory: String) async -> Bool {
    guard let context = ownership.currentConnectionContext() else { return false }
    let directory = workingDirectory.trimmingCharacters(in: .whitespacesAndNewlines)
    guard !directory.isEmpty else { return false }
    let previousIdentities = Set(store.sessions.map(\.id))
    do {
      let response = try await store.client.rpc(
        ACPWire.Method.sessionNew,
        params: ACPWire.newSessionParameters(workingDirectory: directory),
        timeout: runtimeConfiguration.sessionCreateTimeout
      )
      guard !Task.isCancelled, ownership.ownsConnection(context) else { return false }
      guard let payload = response as? [String: Any] else {
        throw ClientError.malformedResponse
      }
      do { try await refreshSessions(context: context) } catch {
        guard !Task.isCancelled, ownership.ownsConnection(context) else { return false }
        store.statusMessage = "会话列表刷新失败：\(error.localizedDescription)"
      }
      guard !Task.isCancelled, ownership.ownsConnection(context) else { return false }
      ownership.sessionLoadTask?.cancel()
      ownership.sessionLoadTask = nil
      store.isSessionLoading = false
      ownership.resetTurnTracking()
      store.interactionController.clear()
      store.selectedSession = nil
      store.childAgents = []
      ownership.advanceSession()
      ownership.mountedSessionIdentity = nil
      if let session = try SessionPayloadMapper.createdSession(
        from: payload, sessions: store.sessions, previousSessionIdentities: previousIdentities) {
        store.selectedSession = session
        ownership.mountedSessionIdentity = session.id
        store.statusMessage = "已创建"
      } else {
        store.statusMessage = "已创建，等待历史索引"
      }
      return true
    } catch {
      guard !Task.isCancelled, ownership.ownsConnection(context) else { return false }
      store.statusMessage = error.localizedDescription
      return false
    }
  }

  private func loadSession(
    _ session: SessionSummary, context: SessionContext, requiresSessionLoad: Bool
  ) async {
    defer {
      if ownership.sessionGeneration == context.generation {
        ownership.sessionLoadTask = nil
        store.isSessionLoading = false
      }
    }
    do {
      let response = try await store.client.rest(
        pathComponents: ["api", "sessions", session.sessionID, "messages"],
        query: [URLQueryItem(name: "providerId", value: session.providerID)]
      )
      guard !Task.isCancelled, ownership.ownsSession(context) else { return }
      let history = try SessionPayloadMapper.history(from: response, fallback: session)
      applyAuthoritativeSession(history.session)
      store.blocks = history.blocks
      store.childAgents = history.childAgents
    } catch {
      guard !Task.isCancelled, ownership.ownsSession(context) else { return }
      store.statusMessage = "历史暂不可用：\(error.localizedDescription)"
    }
    guard !Task.isCancelled, ownership.ownsSession(context) else { return }
    guard requiresSessionLoad else {
      store.statusMessage = "在线"
      return
    }
    // 挂载期间 provider 会把整轮对话作为 session/update 重放。上面刚落地的历史快照
    // 已经是权威内容，这里屏蔽重放，避免消息、思考与回复各被追加一遍。
    ownership.isMountingSession = true
    defer { ownership.isMountingSession = false }
    do {
      let response = try await store.client.rpc(
        ACPWire.Method.sessionLoad,
        params: ACPWire.loadSessionParameters(sessionID: session.sessionID),
        timeout: runtimeConfiguration.sessionLoadTimeout
      )
      guard !Task.isCancelled, ownership.ownsSession(context) else { return }
      if let payload = response as? [String: Any] {
        store.modelState = SessionPayloadMapper.modelState(from: payload, current: store.modelState)
      }
      ownership.mountedSessionIdentity = session.id
      store.statusMessage = "在线"
    } catch {
      guard !Task.isCancelled, ownership.ownsSession(context) else { return }
      store.statusMessage = "挂载失败：\(error.localizedDescription)"
    }
  }

  func applyAuthoritativeSession(_ session: SessionSummary) {
    if let index = store.sessions.firstIndex(where: { $0.id == session.id }) {
      store.sessions[index] = session
    } else {
      store.sessions.append(session)
    }
    store.selectedSession = session
  }
}
