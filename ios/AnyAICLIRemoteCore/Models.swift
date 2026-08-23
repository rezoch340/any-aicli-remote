import Foundation

public enum ConnectionStatus: Equatable {
  case disconnected
  case connecting
  case connected
  case failed(String)

  public var label: String {
    switch self {
    case .disconnected: return "未连接"
    case .connecting: return "连接中"
    case .connected: return "在线"
    case .failed(let message): return message
    }
  }
}

public struct SavedDevice: Codable, Identifiable, Hashable {
  public let id: UUID
  public var name: String
  public var baseURL: URL
}

public enum DeviceHealthStatus: Equatable {
  case checking
  case online
  case offline
}

public struct ServerProfile: Codable, Equatable {
  public var baseURL: URL
  public var key: String

  public init(baseURL: URL, key: String) {
    self.baseURL = baseURL
    self.key = key
  }

  public static func parse(address: String, fallbackKey: String) throws -> ServerProfile {
    var trimmedAddress = address.trimmingCharacters(in: .whitespacesAndNewlines)
    if !trimmedAddress.contains("://") {
      trimmedAddress = "http://" + trimmedAddress
    }
    guard var components = URLComponents(string: trimmedAddress),
      let scheme = components.scheme,
      ["http", "https"].contains(scheme.lowercased()),
      components.host != nil
    else {
      throw ClientError.invalidAddress
    }

    let pairedKey = components.queryItems?.first(where: { $0.name == "key" })?.value
    components.path = ""
    components.query = nil
    components.fragment = nil
    guard let baseURL = components.url else { throw ClientError.invalidAddress }
    let key = (pairedKey ?? fallbackKey).trimmingCharacters(in: .whitespacesAndNewlines)
    guard !key.isEmpty else { throw ClientError.missingKey }
    return ServerProfile(baseURL: baseURL, key: key)
  }
}

public struct SessionIdentity: Hashable {
  public let providerID: String
  public let sessionID: String
}

public struct WorkspaceFile: Identifiable, Equatable, Hashable {
  public let name: String
  public let path: String
  public let relativePath: String
  public let size: Int64
  public let text: Bool
  public let directory: Bool
  public var id: String { path }
  public var uri: String {
    if path.hasPrefix("file:") {
      return URL(string: path)?.absoluteString ?? path
    }
    let absolutePath = path.hasPrefix("/") ? path : "/" + path
    return URL(fileURLWithPath: absolutePath).absoluteString
  }
  public init?(json: [String: Any], directory: Bool) {
    guard let path = json["path"] as? String else { return nil }
    self.path = path
    name = json["name"] as? String ?? (path as NSString).lastPathComponent
    relativePath = json["rel"] as? String ?? json["relativePath"] as? String ?? path
    size = (json["size"] as? NSNumber)?.int64Value ?? 0
    text = json["text"] as? Bool ?? false
    self.directory = directory
  }
}

public struct SessionSummary: Identifiable, Hashable {
  public let providerID: String
  public let sessionID: String
  public var title: String
  public var projectDirectory: String
  public var isResident: Bool
  public var activity: String
  public var createdAt: Date?
  public var lastActiveAt: Date?

  public var id: SessionIdentity {
    SessionIdentity(providerID: providerID, sessionID: sessionID)
  }

  public init?(json: [String: Any], fallbackProviderID: String? = nil) {
    guard let sessionID = Self.string(json, keys: ["sessionId", "session_id", "id"]),
      let providerID = Self.string(json, keys: ["providerId", "provider_id"]) ?? fallbackProviderID,
      !providerID.isEmpty
    else { return nil }
    self.providerID = providerID
    self.sessionID = sessionID
    title = Self.string(json, keys: ["title", "summary"]) ?? "未命名会话"
    projectDirectory = Self.string(json, keys: ["projectDir"]) ?? ""
    isResident = (json["resident"] as? Bool) ?? false
    activity = Self.string(json, keys: ["activity", "status"]) ?? ""
    createdAt = Self.date(json["createdAt"])
    lastActiveAt = Self.date(json["lastActiveAt"])
  }

  private static func string(_ json: [String: Any], keys: [String]) -> String? {
    for key in keys {
      if let value = json[key] as? String, !value.trimmingCharacters(in: .whitespaces).isEmpty {
        return value
      }
    }
    return nil
  }

  private static func date(_ value: Any?) -> Date? {
    if let number = value as? NSNumber {
      let timestamp = number.doubleValue
      return Date(
        timeIntervalSince1970: timestamp > 1_000_000_000_000 ? timestamp / 1000 : timestamp)
    }
    if let text = value as? String {
      if let timestamp = Double(text) {
        return Date(
          timeIntervalSince1970: timestamp > 1_000_000_000_000 ? timestamp / 1000 : timestamp)
      }
      if let date = ISO8601DateFormatter().date(from: text) {
        return date
      }
    }
    return nil
  }
}

public enum ChatBlockKind: String, Equatable {
  case user
  case assistant
  case thinking
  case tool
  case permission
  case plan
  case system
}

public enum ToolRunState: String, Equatable {
  case pending
  case running
  case success
  case failed
  case cancelled

  public init(raw statusValue: String?) {
    let status = (statusValue ?? "").lowercased()
    if status.contains("success") || status.contains("complete") || status == "done" {
      self = .success
    } else if status.contains("fail") || status.contains("error") || status.contains("timeout") {
      self = .failed
    } else if status.contains("cancel") {
      self = .cancelled
    } else if status.contains("run") || status.contains("stream") || status.contains("progress") {
      self = .running
    } else {
      self = .pending
    }
  }
}

public struct PermissionOption: Identifiable, Equatable {
  public let id: String
  public let label: String
  public init(id: String, label: String) { self.id = id; self.label = label }
}

public struct ChatBlock: Identifiable, Equatable {
  public var id: String
  public var kind: ChatBlockKind
  public var text: String = ""
  public var title: String = ""
  public var detail: String = ""
  public var toolState: ToolRunState = .pending
  public var rpcID: Int?
  public var options: [PermissionOption] = []
  public var attachments: [WorkspaceFile] = []

  public init(
    id: String,
    kind: ChatBlockKind,
    text: String = "",
    title: String = "",
    detail: String = "",
    toolState: ToolRunState = .pending,
    rpcID: Int? = nil,
    options: [PermissionOption] = [],
    attachments: [WorkspaceFile] = []
  ) {
    self.id = id
    self.kind = kind
    self.text = text
    self.title = title
    self.detail = detail
    self.toolState = toolState
    self.rpcID = rpcID
    self.options = options
    self.attachments = attachments
  }
}

public struct ModelState: Equatable {
  public var currentModelID = ""
  public var effort = "low"
  public var effortLevels = ["low", "medium", "high", "xhigh"]
  public init() {}
}

public enum ClientError: LocalizedError {
  case invalidAddress
  case missingKey
  case disconnected
  case malformedResponse
  case server(String)

  public var errorDescription: String? {
    switch self {
    case .invalidAddress: return "服务地址无效"
    case .missingKey: return "缺少配对 Key"
    case .disconnected: return "连接已断开"
    case .malformedResponse: return "服务端返回格式无效"
    case .server(let message): return message
    }
  }
}

extension Dictionary where Key == String, Value == Any {
  public func object(_ key: String) -> [String: Any]? { self[key] as? [String: Any] }
  public func array(_ key: String) -> [[String: Any]] { self[key] as? [[String: Any]] ?? [] }
  public func string(_ keys: String...) -> String? {
    for key in keys {
      if let value = self[key] as? String { return value }
    }
    return nil
  }
}
