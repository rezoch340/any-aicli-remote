import Foundation

public enum SessionPayloadMapper {
  public struct SessionHistoryResult {
    public let session: SessionSummary
    public let blocks: [ChatBlock]
  }

  public static func sessions(from response: [String: Any]) throws -> [SessionSummary] {
    guard let rows = response["sessions"] as? [[String: Any]] else {
      throw ClientError.malformedResponse
    }
    return rows.compactMap { SessionSummary(json: $0) }.sorted {
      ($0.lastActiveAt ?? $0.createdAt ?? .distantPast)
        > ($1.lastActiveAt ?? $1.createdAt ?? .distantPast)
    }
  }

  public static func history(from response: [String: Any], fallback: SessionSummary) throws
    -> SessionHistoryResult {
    guard let metadata = response.object("session"),
      let session = SessionSummary(
        json: metadata,
        fallbackProviderID: response.string("providerId") ?? fallback.providerID
      ),
      session.id == fallback.id,
      let messages = response["messages"] as? [[String: Any]]
    else {
      throw ClientError.malformedResponse
    }
    return SessionHistoryResult(session: session, blocks: chatBlocks(from: messages))
  }

  public static func createdSession(
    from response: [String: Any],
    sessions: [SessionSummary],
    previousSessionIdentities: Set<SessionIdentity>
  ) throws -> SessionSummary? {
    let sessionID = try createdSessionID(from: response)
    let metadata = response.object("session") ?? response
    let providerID = normalized(
      metadata.string("providerId", "provider_id") ?? response.string("providerId", "provider_id"))
    let projectDirectory = normalized(
      metadata.string("projectDir") ?? response.string("projectDir"))
    if let providerID,
      let indexedSession = sessions.first(where: {
        $0.id == SessionIdentity(providerID: providerID, sessionID: sessionID)
      }) {
      return indexedSession
    }
    if providerID == nil {
      let newIdentities = Set(sessions.map(\.id)).subtracting(previousSessionIdentities)
      let matches = sessions.filter { newIdentities.contains($0.id) && $0.sessionID == sessionID }
      if matches.count == 1 { return matches[0] }
    }
    guard let providerID, let projectDirectory else { return nil }
    var normalizedMetadata = metadata
    normalizedMetadata["providerId"] = providerID
    normalizedMetadata["sessionId"] = sessionID
    normalizedMetadata["projectDir"] = projectDirectory
    return SessionSummary(json: normalizedMetadata)
  }

  public static func createdSessionID(from response: [String: Any]) throws -> String {
    guard
      let sessionID = response.string("sessionId", "session_id")
        ?? response.object("session")?.string("sessionId", "session_id")
    else {
      throw ClientError.malformedResponse
    }
    return sessionID
  }

  public static func modelState(from response: [String: Any], current: ModelState) -> ModelState {
    guard
      let source = response.object("models") ?? response.object("_meta")?.object("modelState")
        ?? response.object("modelState")
    else {
      return current
    }
    var modelState = current
    if let modelID = source.string("currentModelId") { modelState.currentModelID = modelID }
    let availableModels = source["availableModels"] as? [[String: Any]] ?? []
    if let model = availableModels.first(where: {
      $0.string("modelId") == modelState.currentModelID
    }),
      let metadata = model.object("_meta") {
      if let effort = metadata.string("reasoningEffort") { modelState.effort = effort }
      let levels = (metadata["reasoningEfforts"] as? [[String: Any]] ?? [])
        .compactMap { $0.string("value", "id") }
      if !levels.isEmpty { modelState.effortLevels = levels }
    }
    return modelState
  }

  public static func chatBlocks(from messages: [[String: Any]]) -> [ChatBlock] {
    messages.enumerated().compactMap { messageIndex, message in
      guard let role = message.string("role")?.lowercased(),
        let content = message.string("content")
      else { return nil }
      let blockID = "history-\(messageIndex)"
      switch role {
      case "system": return ChatBlock(id: blockID, kind: .system, text: content)
      case "user": return ChatBlock(id: blockID, kind: .user, text: content)
      case "assistant": return ChatBlock(id: blockID, kind: .assistant, text: content)
      case "tool":
        return ChatBlock(
          id: blockID, kind: .tool, title: "工具", detail: content, toolState: .success)
      default: return nil
      }
    }
  }

  private static func normalized(_ value: String?) -> String? {
    guard let value else { return nil }
    let trimmedValue = value.trimmingCharacters(in: .whitespacesAndNewlines)
    return trimmedValue.isEmpty ? nil : trimmedValue
  }
}
