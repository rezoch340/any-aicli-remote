import Foundation

public enum ACPWire {
  public enum Method {
    public static let sessionLoad = "session/load"
    public static let sessionNew = "session/new"
    public static let sessionPrompt = "session/prompt"
    public static let sessionCancel = "session/cancel"
    public static let sessionUpdate = "session/update"
    public static let childAgentUpdate = "session/child_agent_update"
    public static let sessionsChanged = "sessions/changed"
    public static let interactionRequest = "session/interaction_request"
  }

  public static func promptParameters(sessionID: String, text: String, attachments: [WorkspaceFile])
    -> [String: Any] {
    ["sessionId": sessionID, "prompt": promptBlocks(text: text, attachments: attachments)]
  }

  public static func newSessionParameters(workingDirectory: String) -> [String: Any] {
    ["cwd": workingDirectory, "mcpServers": []]
  }

  public static func loadSessionParameters(sessionID: String) -> [String: Any] {
    ["sessionId": sessionID, "mcpServers": []]
  }

  public static func cancelSessionParameters(sessionID: String) -> [String: Any] {
    ["sessionId": sessionID]
  }

  public static func promptBlocks(text: String, attachments: [WorkspaceFile]) -> [[String: Any]] {
    var blocks: [[String: Any]] = []
    if !text.isEmpty { blocks.append(["type": "text", "text": text]) }
    for file in attachments {
      var link: [String: Any] = [
        "type": "resource_link", "name": file.name, "uri": file.uri,
        "description": file.relativePath
      ]
      if file.size > 0 { link["size"] = file.size }
      blocks.append(link)
    }
    return blocks
  }

  public static func isPermissionRequest(method: String) -> Bool {
    method == "permission/request"
  }

  public static func permissionReplyResult(optionID: String?) -> [String: Any] {
    if let optionID { return ["outcome": ["outcome": "selected", "optionId": optionID]] }
    return ["outcome": ["outcome": "cancelled"]]
  }

  public static func matchesSessionIdentity(_ payload: [String: Any], expected: SessionIdentity) -> Bool {
    payload.string("providerId") == expected.providerID
      && payload.string("sessionId") == expected.sessionID
  }
}
