import Foundation

enum ConnectionStatus: Equatable {
    case disconnected
    case connecting
    case connected
    case reconnecting
    case failed(String)

    var label: String {
        switch self {
        case .disconnected: return "未连接"
        case .connecting: return "连接中"
        case .connected: return "在线"
        case .reconnecting: return "重连中"
        case .failed(let message): return message
        }
    }
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

struct SessionSummary: Identifiable, Hashable {
    let id: String
    var title: String
    var cwd: String
    var isResident: Bool
    var activity: String
    var updatedAt: Date?

    init?(json: [String: Any]) {
        guard let id = (json["sessionId"] ?? json["session_id"] ?? json["id"]) as? String,
              !id.isEmpty else { return nil }
        self.id = id
        title = Self.string(json, keys: ["remote_title", "title", "generated_title"]) ?? "未命名会话"
        cwd = Self.string(json, keys: ["cwd"]) ?? ""
        isResident = (json["resident"] as? Bool) ?? false
        activity = Self.string(json, keys: ["activity", "status"]) ?? ""
        updatedAt = Self.date(json)
    }

    private static func string(_ json: [String: Any], keys: [String]) -> String? {
        for key in keys {
            if let value = json[key] as? String, !value.trimmingCharacters(in: .whitespaces).isEmpty {
                return value
            }
        }
        return nil
    }

    private static func date(_ json: [String: Any]) -> Date? {
        let keys = ["lastChangeUnixMs", "last_change_unix_ms", "updatedAt", "updated_at", "mtime", "createdAt"]
        for key in keys {
            if let number = json[key] as? NSNumber {
                let raw = number.doubleValue
                return Date(timeIntervalSince1970: raw > 1_000_000_000_000 ? raw / 1000 : raw)
            }
            if let value = json[key] as? String,
               let date = ISO8601DateFormatter().date(from: value) { return date }
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
}

struct ModelState: Equatable {
    var currentModelID = "grok"
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
