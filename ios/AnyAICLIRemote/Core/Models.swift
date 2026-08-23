import Foundation

enum ConnectionStatus: Equatable {
    case disconnected
    case connecting
    case connected
    case failed(String)

    var label: String {
        switch self {
        case .disconnected: return "未连接"
        case .connecting: return "连接中"
        case .connected: return "在线"
        case .failed(let message): return message
        }
    }
}

struct SavedDevice: Codable, Identifiable, Hashable {
    let id: UUID
    var name: String
    var baseURL: URL
}

enum DeviceHealthStatus: Equatable {
    case checking
    case online
    case offline
}

struct ServerProfile: Codable, Equatable {
    var baseURL: URL
    var key: String

    static func parse(address: String, fallbackKey: String) throws -> ServerProfile {
        var raw = address.trimmingCharacters(in: .whitespacesAndNewlines)
        if !raw.contains("://") { raw = "http://" + raw }
        guard var components = URLComponents(string: raw),
              let scheme = components.scheme,
              ["http", "https"].contains(scheme.lowercased()),
              components.host != nil else {
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

struct SessionIdentity: Hashable {
    let providerID: String
    let sessionID: String
}

struct WorkspaceFile: Identifiable, Equatable, Hashable {
    let name: String
    let path: String
    let relativePath: String
    let size: Int64
    let text: Bool
    let directory: Bool
    var id: String { path }
    var uri: String { path.hasPrefix("file:") ? (URL(string: path)?.absoluteString ?? path) : URL(fileURLWithPath: path.hasPrefix("/") ? path : "/" + path).absoluteString }
    init?(json: [String: Any], directory: Bool) {
        guard let path = json["path"] as? String else { return nil }
        self.path = path
        name = json["name"] as? String ?? (path as NSString).lastPathComponent
        relativePath = json["rel"] as? String ?? json["relativePath"] as? String ?? path
        size = (json["size"] as? NSNumber)?.int64Value ?? 0
        text = json["text"] as? Bool ?? false
        self.directory = directory
    }
}

struct SessionSummary: Identifiable, Hashable {
    let providerID: String
    let sessionID: String
    var title: String
    var projectDirectory: String
    var isResident: Bool
    var activity: String
    var createdAt: Date?
    var lastActiveAt: Date?

    var id: SessionIdentity {
        SessionIdentity(providerID: providerID, sessionID: sessionID)
    }

    init?(json: [String: Any], fallbackProviderID: String? = nil) {
        guard let sessionID = Self.string(json, keys: ["sessionId", "session_id", "id"]),
              let providerID = Self.string(json, keys: ["providerId", "provider_id"]) ?? fallbackProviderID,
              !providerID.isEmpty else { return nil }
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
            return Date(timeIntervalSince1970: timestamp > 1_000_000_000_000 ? timestamp / 1000 : timestamp)
        }
        if let text = value as? String {
            if let timestamp = Double(text) {
                return Date(timeIntervalSince1970: timestamp > 1_000_000_000_000 ? timestamp / 1000 : timestamp)
            }
            if let date = ISO8601DateFormatter().date(from: text) {
                return date
            }
        }
        return nil
    }
}

enum ChatBlockKind: String, Equatable {
    case user
    case assistant
    case thinking
    case tool
    case permission
    case plan
    case system
}

enum ToolRunState: String, Equatable {
    case pending
    case running
    case success
    case failed
    case cancelled

    init(raw: String?) {
        let value = (raw ?? "").lowercased()
        if value.contains("success") || value.contains("complete") || value == "done" { self = .success }
        else if value.contains("fail") || value.contains("error") || value.contains("timeout") { self = .failed }
        else if value.contains("cancel") { self = .cancelled }
        else if value.contains("run") || value.contains("stream") || value.contains("progress") { self = .running }
        else { self = .pending }
    }
}

struct PermissionOption: Identifiable, Equatable {
    let id: String
    let label: String
}

struct ChatBlock: Identifiable, Equatable {
    var id: String
    var kind: ChatBlockKind
    var text: String = ""
    var title: String = ""
    var detail: String = ""
    var toolState: ToolRunState = .pending
    var rpcID: Int?
    var options: [PermissionOption] = []
    var attachments: [WorkspaceFile] = []
}

struct ModelState: Equatable {
    var currentModelID = ""
    var effort = "low"
    var effortLevels = ["low", "medium", "high", "xhigh"]
}

enum ClientError: LocalizedError {
    case invalidAddress
    case missingKey
    case disconnected
    case malformedResponse
    case server(String)

    var errorDescription: String? {
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
    func object(_ key: String) -> [String: Any]? { self[key] as? [String: Any] }
    func array(_ key: String) -> [[String: Any]] { self[key] as? [[String: Any]] ?? [] }
    func string(_ keys: String...) -> String? {
        for key in keys {
            if let value = self[key] as? String { return value }
        }
        return nil
    }
}
