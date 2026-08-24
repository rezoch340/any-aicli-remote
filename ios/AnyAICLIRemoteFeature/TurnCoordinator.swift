import AnyAICLIRemoteCore
import Foundation

/// 一轮对话的发起、取消、努力度切换、权限应答与来自 provider 的通知归并。
@MainActor
final class TurnCoordinator {
  private unowned let store: ChatStore
  private let ownership: ChatOwnership

  init(store: ChatStore, ownership: ChatOwnership) {
    self.store = store
    self.ownership = ownership
  }

  func send(_ text: String) {
    guard let session = store.selectedSession,
      let context = ownership.currentSessionContext(sessionIdentity: session.id)
    else { return }
    let trimmedText = text.trimmingCharacters(in: .whitespacesAndNewlines)
    let attachments = store.selectedFiles
    guard !trimmedText.isEmpty || !attachments.isEmpty else { return }
    let turnID = UUID()
    ownership.activeTurnID = turnID
    ownership.pendingUserEchoTracker.clear()
    if !trimmedText.isEmpty { ownership.pendingUserEchoTracker.begin(text: trimmedText) }
    store.blocks.append(
      ChatBlock(id: UUID().uuidString, kind: .user, text: trimmedText, attachments: attachments))
    store.closeWorkspaceFilePicker(clearSelection: true)
    store.isBusy = true
    store.statusMessage = "等待助手"
    Task {
      do {
        guard ownership.ownsSession(context) else { return }
        let parameters = ACPWire.promptParameters(
          sessionID: session.sessionID,
          text: trimmedText,
          attachments: attachments
        )
        let response = try await store.client.rpc(ACPWire.Method.sessionPrompt, params: parameters)
        guard !Task.isCancelled, ownership.ownsSession(context), ownership.activeTurnID == turnID
        else { return }
        let stopReason = (response as? [String: Any])?.string("stopReason")?.lowercased()
        let wasCancelled = stopReason?.contains("cancel") == true
        finishTurn(
          toolState: wasCancelled ? .cancelled : .success,
          status: wasCancelled ? "已停止" : "完成"
        )
      } catch {
        guard !Task.isCancelled, ownership.ownsSession(context), ownership.activeTurnID == turnID
        else { return }
        if error.localizedDescription.lowercased().contains("cancel") {
          finishTurn(toolState: .cancelled, status: "已停止")
        } else {
          store.blocks.append(
            ChatBlock(id: UUID().uuidString, kind: .system, text: error.localizedDescription))
          finishTurn(toolState: .failed, status: "发送失败")
        }
      }
    }
  }

  func cancel() {
    store.interactionController.clear()
    guard let session = store.selectedSession,
      let context = ownership.currentSessionContext(sessionIdentity: session.id)
    else { return }
    finishTurn(toolState: .cancelled, status: "已停止")
    Task {
      guard ownership.ownsSession(context) else { return }
      try? await store.client.notify(
        ACPWire.Method.sessionCancel,
        params: ACPWire.cancelSessionParameters(sessionID: session.sessionID))
      guard !Task.isCancelled, ownership.ownsSession(context) else { return }
      finishTurn(toolState: .cancelled, status: "已停止")
    }
  }

  func setEffort(_ effort: String) {
    guard let session = store.selectedSession,
      let context = ownership.currentSessionContext(sessionIdentity: session.id)
    else { return }
    let modelID = store.modelState.currentModelID
    Task {
      do {
        guard ownership.ownsSession(context) else { return }
        let body: [String: Any] = [
          "providerId": session.providerID,
          "sessionId": session.sessionID,
          "modelId": modelID,
          "effort": effort
        ]
        _ = try await store.client.rest(path: "/api/effort", method: "POST", body: body)
        guard !Task.isCancelled, ownership.ownsSession(context) else { return }
        store.modelState.effort = effort
      } catch where !Task.isCancelled && ownership.ownsSession(context) {
        store.statusMessage = "切换失败：\(error.localizedDescription)"
      }
    }
  }

  func answerPermission(blockID: String, optionID: String?) {
    guard let index = store.blocks.firstIndex(where: { $0.id == blockID }),
      let rpcID = store.blocks[index].rpcID,
      let sessionIdentity = store.selectedSession?.id,
      let context = ownership.currentSessionContext(sessionIdentity: sessionIdentity)
    else { return }
    Task {
      guard ownership.ownsSession(context) else { return }
      try? await store.client.reply(
        id: rpcID, result: ACPWire.permissionReplyResult(optionID: optionID))
      guard !Task.isCancelled, ownership.ownsSession(context) else { return }
      store.blocks.removeAll { $0.id == blockID }
    }
  }

  func handleNotification(_ payload: [String: Any]) {
    guard store.connection == .connected,
      let notification = ChatNotificationMapper.map(
        payload: payload,
        selectedSessionID: store.selectedSession?.id
      )
    else { return }

    switch notification {
    case .childAgent(let card):
      store.childAgents = ChildAgentReducer.apply(store.childAgents, incoming: card)
    case .sessionUpdate(let update):
      // session/load 会把整轮对话重放一遍。历史快照已经包含这些内容，再追加就会让
      // 用户消息、思考与回复各出现两次，且重启后回显去重表是空的，拦不住重放。
      guard !ownership.isMountingSession else { return }
      applyTranscriptUpdate(update)
    case .sessionsChanged:
      guard let context = ownership.currentConnectionContext() else { return }
      Task { try? await store.sessionCoordinator.refreshSessions(context: context) }
    case .permission(let request):
      let options =
        request.options.isEmpty ? [PermissionOption(id: "allow", label: "允许")] : request.options
      store.blocks.append(
        ChatBlock(
          id: "permission-\(request.rpcID)",
          kind: .permission,
          text: request.question,
          rpcID: request.rpcID,
          options: options
        )
      )
    case .interaction(let request):
      store.interactionController.receive(request)
    }
  }

  private func applyTranscriptUpdate(_ update: [String: Any]) {
    switch ChatTranscriptReducer.apply(
      update: update, to: &store.blocks,
      pendingUserEchoTracker: ownership.pendingUserEchoTracker) {
    case .none: break
    case .busy(let status)
    where ChatTranscriptReducer.shouldMarkTurnBusy(activeTurnID: ownership.activeTurnID):
      store.isBusy = true
      store.statusMessage = status
    case .busy: break
    case .finished(let state, let status): finishTurn(toolState: state, status: status)
    }
  }

  func finishTurn(toolState: ToolRunState, status: String) {
    ownership.resetTurnTracking()
    store.blocks = ChatTranscriptReducer.finalizeActiveTools(in: store.blocks, as: toolState)
    store.isBusy = false
    store.statusMessage = status
  }
}
