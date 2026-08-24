import AnyAICLIRemoteCore
import Foundation

enum ChatNotification {
  case sessionUpdate([String: Any])
  case sessionsChanged
  case permission(PermissionRequest)
  case childAgent(ChildAgentCard)
}

struct PermissionRequest {
  let rpcID: Int
  let question: String
  let options: [PermissionOption]
}

enum ChatNotificationMapper {
  static func map(payload: [String: Any], selectedSessionID: SessionIdentity?) -> ChatNotification? {
    let method = payload.string("method") ?? ""
    if method == ACPWire.Method.childAgentUpdate {
      guard let selectedSessionID,
        let parameters = payload.object("params"),
        ACPWire.matchesSessionIdentity(parameters, expected: selectedSessionID),
        let event = parameters.object("event"),
        let card = ChildAgentPayloadMapper.card(fromEvent: event)
      else { return nil }
      return .childAgent(card)
    }
    if method == ACPWire.Method.sessionUpdate {
      guard let selectedSessionID else { return nil }
      let parameters = payload.object("params") ?? [:]
      guard ACPWire.matchesSessionIdentity(parameters, expected: selectedSessionID) else {
        return nil
      }
      return .sessionUpdate(parameters.object("update") ?? parameters)
    }
    if method == ACPWire.Method.sessionsChanged {
      return .sessionsChanged
    }
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
    return .permission(
      PermissionRequest(
        rpcID: rpcID,
        question: parameters.string("question", "message") ?? "CLI 需要你的确认",
        options: options
      )
    )
  }
}
