import Foundation

enum DaemonConfigurationError: LocalizedError, Equatable {
    case invalidJSON
    case rootMustBeObject
    case missingField(String)
    case invalidField(String)

    var errorDescription: String? {
        switch self {
        case .invalidJSON: "Daemon 配置不是有效 JSON"
        case .rootMustBeObject: "Daemon 配置根节点必须是对象"
        case .missingField(let field): "Daemon 配置缺少字段：\(field)"
        case .invalidField(let field): "Daemon 配置字段无效：\(field)"
        }
    }
}

struct DaemonEditableConfigurationPatch: Equatable {
    var bindAddress: String?
    var daemonPort: Int?
    var publicHost: String?
    var agentPort: Int?
}

struct DaemonEditableConfiguration: Equatable {
    var bindAddress: String
    var daemonPort: Int
    var publicHost: String
    var agentPort: Int
    var providerAlwaysApprove: Bool
}

struct CanonicalDaemonConfiguration: Equatable {
    private var rootObject: [String: Any]

    init(data: Data) throws {
        let parsedValue: Any
        do {
            parsedValue = try JSONSerialization.jsonObject(with: data, options: [])
        } catch {
            throw DaemonConfigurationError.invalidJSON
        }
        guard let objectValue = parsedValue as? [String: Any] else {
            throw DaemonConfigurationError.rootMustBeObject
        }
        rootObject = objectValue
    }

    var editable: DaemonEditableConfiguration {
        get throws {
            let network = try dictionary(named: "network")
            let agent = try dictionary(named: "agent")
            let provider = try optionalDictionary(named: "provider")
            let providerOptions = try optionalDictionary(
                in: provider,
                key: "options",
                field: "provider.options"
            )
            return DaemonEditableConfiguration(
                bindAddress: try string(in: network, field: "network.bind"),
                daemonPort: try integer(in: network, field: "network.port"),
                publicHost: optionalString(in: network, field: "network.public_host") ?? "",
                agentPort: try integer(in: agent, field: "agent.port"),
                providerAlwaysApprove: try optionalBooleanString(
                    in: providerOptions,
                    field: "provider.options.always-approve"
                ) ?? false
            )
        }
    }

    mutating func apply(_ editable: DaemonEditableConfiguration) throws {
        var network = try dictionary(named: "network")
        var agent = try dictionary(named: "agent")
        var provider = try optionalDictionary(named: "provider") ?? [:]
        var providerOptions = try optionalDictionary(
            in: provider,
            key: "options",
            field: "provider.options"
        ) ?? [:]
        network["bind"] = editable.bindAddress
        network["port"] = editable.daemonPort
        network["public_host"] = editable.publicHost
        agent["port"] = editable.agentPort
        providerOptions["always-approve"] = editable.providerAlwaysApprove ? "true" : "false"
        provider["options"] = providerOptions
        rootObject["network"] = network
        rootObject["agent"] = agent
        rootObject["provider"] = provider
    }

    mutating func apply(_ patch: DaemonEditableConfigurationPatch) throws {
        var editable = try self.editable
        if let value = patch.bindAddress { editable.bindAddress = value }
        if let value = patch.daemonPort { editable.daemonPort = value }
        if let value = patch.publicHost { editable.publicHost = value }
        if let value = patch.agentPort { editable.agentPort = value }
        try apply(editable)
    }

    mutating func setAgentStopOnExit(_ enabled: Bool) throws {
        var agent = try dictionary(named: "agent")
        agent["stop_on_exit"] = enabled
        rootObject["agent"] = agent
    }

    func serializedData() throws -> Data {
        try JSONSerialization.data(withJSONObject: rootObject, options: [.sortedKeys])
    }

    private func dictionary(named key: String) throws -> [String: Any] {
        guard let value = rootObject[key] else { throw DaemonConfigurationError.missingField(key) }
        guard let dictionary = value as? [String: Any] else { throw DaemonConfigurationError.invalidField(key) }
        return dictionary
    }

    private func optionalDictionary(named key: String) throws -> [String: Any]? {
        guard let value = rootObject[key] else { return nil }
        guard let dictionary = value as? [String: Any] else {
            throw DaemonConfigurationError.invalidField(key)
        }
        return dictionary
    }

    private func optionalDictionary(
        in dictionary: [String: Any]?,
        key: String,
        field: String
    ) throws -> [String: Any]? {
        guard let value = dictionary?[key] else { return nil }
        guard let nestedDictionary = value as? [String: Any] else {
            throw DaemonConfigurationError.invalidField(field)
        }
        return nestedDictionary
    }

    private func string(in dictionary: [String: Any], field: String) throws -> String {
        let key = field.split(separator: ".").last.map(String.init) ?? field
        guard let value = dictionary[key] else { throw DaemonConfigurationError.missingField(field) }
        guard let stringValue = value as? String else { throw DaemonConfigurationError.invalidField(field) }
        return stringValue
    }

    private func optionalString(in dictionary: [String: Any], field: String) -> String? {
        let key = field.split(separator: ".").last.map(String.init) ?? field
        guard let value = dictionary[key] else { return nil }
        return value as? String
    }

    private func integer(in dictionary: [String: Any], field: String) throws -> Int {
        let key = field.split(separator: ".").last.map(String.init) ?? field
        guard let value = dictionary[key] else { throw DaemonConfigurationError.missingField(field) }
        guard let numberValue = value as? NSNumber,
              CFGetTypeID(numberValue) != CFBooleanGetTypeID() else { throw DaemonConfigurationError.invalidField(field) }
        return numberValue.intValue
    }

    private func optionalBooleanString(
        in dictionary: [String: Any]?,
        field: String
    ) throws -> Bool? {
        let key = field.split(separator: ".").last.map(String.init) ?? field
        guard let value = dictionary?[key] else { return nil }
        guard let stringValue = value as? String else {
            throw DaemonConfigurationError.invalidField(field)
        }
        switch stringValue {
        case "true": return true
        case "false": return false
        default: throw DaemonConfigurationError.invalidField(field)
        }
    }

    static func == (left: CanonicalDaemonConfiguration, right: CanonicalDaemonConfiguration) -> Bool {
        (try? left.serializedData()) == (try? right.serializedData())
    }
}
