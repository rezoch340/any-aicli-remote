import AnyAICLIRemoteCore
import Foundation

enum ChatNotification {
  case sessionUpdate([String: Any])
  case sessionsChanged
  case permission(PermissionRequest)
  case childAgent(ChildAgentCard)
  case interaction(PendingInteraction)
  case status(SessionStatusUpdate)
  case mode(String)
}

struct PermissionRequest {
  let rpcID: Int
  let question: String
  let options: [PermissionOption]
}

enum ChatNotificationMapper {
  static func map(payload: [String: Any], selectedSessionID: SessionIdentity?) -> ChatNotification? {
    let method = payload.string("method") ?? ""
    if method == ACPWire.Method.interactionRequest {
      return interactionNotification(payload: payload, selectedSessionID: selectedSessionID)
    }
    if method == ACPWire.Method.childAgentUpdate {
      return childAgentNotification(payload: payload, selectedSessionID: selectedSessionID)
    }
    if method == ACPWire.Method.statusUpdate {
      return statusNotification(payload: payload, selectedSessionID: selectedSessionID)
    }
    if method == ACPWire.Method.sessionUpdate {
      return sessionUpdateNotification(payload: payload, selectedSessionID: selectedSessionID)
    }
    if method == ACPWire.Method.sessionsChanged {
      return .sessionsChanged
    }
    return permissionNotification(payload: payload, selectedSessionID: selectedSessionID)
  }

  private static func interactionNotification(
    payload: [String: Any], selectedSessionID: SessionIdentity?
  ) -> ChatNotification? {
    guard let selectedSessionID,
      let request = InteractionPayloadMapper.request(from: payload),
      request.sessionIdentity == selectedSessionID
    else { return nil }
    return .interaction(request)
  }

  private static func childAgentNotification(
    payload: [String: Any], selectedSessionID: SessionIdentity?
  ) -> ChatNotification? {
    guard let selectedSessionID,
      let parameters = payload.object("params"),
      ACPWire.matchesSessionIdentity(parameters, expected: selectedSessionID),
      let event = parameters.object("event"),
      let card = ChildAgentPayloadMapper.card(fromEvent: event)
    else { return nil }
    return .childAgent(card)
  }

  private static func sessionUpdateNotification(
    payload: [String: Any], selectedSessionID: SessionIdentity?
  ) -> ChatNotification? {
    guard let selectedSessionID else { return nil }
    let parameters = payload.object("params") ?? [:]
    guard ACPWire.matchesSessionIdentity(parameters, expected: selectedSessionID) else {
      return nil
    }
    let update = parameters.object("update") ?? parameters
    // current_mode_update is standard ACP; surface the mode as session state
    // rather than a transcript block.
    if (update["sessionUpdate"] as? String) == "current_mode_update" {
      let mode = (update["currentModeId"] as? String) ?? (update["current_mode_id"] as? String) ?? ""
      return .mode(mode)
    }
    return .sessionUpdate(update)
  }

  private static func statusNotification(
    payload: [String: Any], selectedSessionID: SessionIdentity?
  ) -> ChatNotification? {
    guard let selectedSessionID,
      let parameters = payload.object("params"),
      ACPWire.matchesSessionIdentity(parameters, expected: selectedSessionID),
      let status = SessionStatusPayloadMapper.status(from: parameters)
    else { return nil }
    return .status(status)
  }

  private static func permissionNotification(
    payload: [String: Any], selectedSessionID: SessionIdentity?
  ) -> ChatNotification? {
    let method = payload.string("method") ?? ""
    guard ACPWire.isPermissionRequest(method: method),
      let selectedSessionID,
      let rpcID = (payload["id"] as? NSNumber)?.intValue
    else { return nil }
    let parameters = payload.object("params") ?? [:]
    guard ACPWire.matchesSessionIdentity(parameters, expected: selectedSessionID) else {
      return nil
    }
    let options = (parameters["options"] as? [[String: Any]] ?? []).map {
      PermissionOption(
        id: $0.string("optionId", "id") ?? "allow",
        label: $0.string("name", "label") ?? "允许"
      )
    }
    // The daemon writes what is being authorized into the ACP-standard
    // toolCall.title; fall back to legacy fields, then a generic prompt.
    let command = parameters.object("toolCall")?.string("title")
    return .permission(
      PermissionRequest(
        rpcID: rpcID,
        question: command ?? parameters.string("question", "message") ?? "CLI 需要你的确认",
        options: options
      )
    )
  }
}
