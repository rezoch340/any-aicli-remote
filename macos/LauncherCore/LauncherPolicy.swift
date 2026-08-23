import Foundation

struct LauncherPolicy: Codable, Equatable {
    static let supportedSchemaVersion = 1

    let schemaVersion: Int
    let requestTimeoutSeconds: Double
    let resourceTimeoutSeconds: Double
    let healthPollIntervalSeconds: Double
    let stopPollIntervalSeconds: Double
    let stopPollAttempts: Int
    let interruptGraceSeconds: Double
    let maximumLogChunkCharacters: Int
    let maximumLogEntries: Int
    let daemonSearchParentDepth: Int
    let executableSearchPaths: [String]

    enum CodingKeys: String, CodingKey {
        case schemaVersion = "schema_version"
        case requestTimeoutSeconds = "request_timeout_seconds"
        case resourceTimeoutSeconds = "resource_timeout_seconds"
        case healthPollIntervalSeconds = "health_poll_interval_seconds"
        case stopPollIntervalSeconds = "stop_poll_interval_seconds"
        case stopPollAttempts = "stop_poll_attempts"
        case interruptGraceSeconds = "interrupt_grace_seconds"
        case maximumLogChunkCharacters = "maximum_log_chunk_characters"
        case maximumLogEntries = "maximum_log_entries"
        case daemonSearchParentDepth = "daemon_search_parent_depth"
        case executableSearchPaths = "executable_search_paths"
    }

    enum PolicyError: LocalizedError, Equatable {
        case missing(String)
        case invalid(String)

        var errorDescription: String? {
            switch self {
            case .missing(let field):
                return "Launcher policy 缺少字段：\(field)"
            case .invalid(let field):
                return "Launcher policy 字段无效：\(field)"
            }
        }
    }

    init(from decoder: Decoder) throws {
        let values = try decoder.container(keyedBy: CodingKeys.self)
        schemaVersion = try Self.required(.schemaVersion, from: values)
        requestTimeoutSeconds = try Self.required(.requestTimeoutSeconds, from: values)
        resourceTimeoutSeconds = try Self.required(.resourceTimeoutSeconds, from: values)
        healthPollIntervalSeconds = try Self.required(.healthPollIntervalSeconds, from: values)
        stopPollIntervalSeconds = try Self.required(.stopPollIntervalSeconds, from: values)
        stopPollAttempts = try Self.required(.stopPollAttempts, from: values)
        interruptGraceSeconds = try Self.required(.interruptGraceSeconds, from: values)
        maximumLogChunkCharacters = try Self.required(.maximumLogChunkCharacters, from: values)
        maximumLogEntries = try Self.required(.maximumLogEntries, from: values)
        daemonSearchParentDepth = try Self.required(.daemonSearchParentDepth, from: values)
        executableSearchPaths = try Self.required(.executableSearchPaths, from: values)

        guard schemaVersion == Self.supportedSchemaVersion else {
            throw PolicyError.invalid(CodingKeys.schemaVersion.rawValue)
        }
        try Self.validatePositive(requestTimeoutSeconds, key: .requestTimeoutSeconds)
        try Self.validatePositive(resourceTimeoutSeconds, key: .resourceTimeoutSeconds)
        try Self.validatePositive(healthPollIntervalSeconds, key: .healthPollIntervalSeconds)
        try Self.validatePositive(stopPollIntervalSeconds, key: .stopPollIntervalSeconds)
        try Self.validatePositive(interruptGraceSeconds, key: .interruptGraceSeconds)
        try Self.validatePositive(stopPollAttempts, key: .stopPollAttempts)
        try Self.validatePositive(maximumLogChunkCharacters, key: .maximumLogChunkCharacters)
        try Self.validatePositive(maximumLogEntries, key: .maximumLogEntries)
        try Self.validatePositive(daemonSearchParentDepth, key: .daemonSearchParentDepth)
        try Self.validatePaths(executableSearchPaths)
    }

    private static func required<Value: Decodable>(
        _ key: CodingKeys,
        from values: KeyedDecodingContainer<CodingKeys>
    ) throws -> Value {
        guard values.contains(key) else {
            throw PolicyError.missing(key.rawValue)
        }
        do {
            return try values.decode(Value.self, forKey: key)
        } catch {
            throw PolicyError.invalid(key.rawValue)
        }
    }

    private static func validatePositive(_ value: Double, key: CodingKeys) throws {
        guard value.isFinite, value > 0 else {
            throw PolicyError.invalid(key.rawValue)
        }
    }

    private static func validatePositive(_ value: Int, key: CodingKeys) throws {
        guard value > 0 else {
            throw PolicyError.invalid(key.rawValue)
        }
    }

    private static func validatePaths(_ paths: [String]) throws {
        guard !paths.isEmpty else {
            throw PolicyError.invalid(CodingKeys.executableSearchPaths.rawValue)
        }
        guard paths.allSatisfy({ !$0.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty }) else {
            throw PolicyError.invalid(CodingKeys.executableSearchPaths.rawValue)
        }
    }

    static func load(resourceURL: URL) throws -> LauncherPolicy {
        try JSONDecoder().decode(Self.self, from: Data(contentsOf: resourceURL))
    }

    static func load(bundle: Bundle = .main) throws -> LauncherPolicy {
        guard let url = bundle.url(forResource: "LauncherPolicy", withExtension: "json") else {
            throw PolicyError.missing("resource")
        }
        return try load(resourceURL: url)
    }
}
