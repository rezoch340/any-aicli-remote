import Foundation

final class DaemonConfigurationStore {
    private let runner: any DaemonCommandRunning
    let configurationURL: URL
    private let fileManager: FileManager

    init(
        runner: any DaemonCommandRunning,
        configurationURL: URL = DaemonConfigurationPath.defaultURL(),
        fileManager: FileManager = .default
    ) {
        self.runner = runner
        self.configurationURL = configurationURL
        self.fileManager = fileManager
    }

    func load() throws -> CanonicalDaemonConfiguration {
        try show()
    }

    func save(editable: DaemonEditableConfiguration) throws -> CanonicalDaemonConfiguration {
        var candidate = try show()
        try candidate.apply(editable)
        return try persist(candidate)
    }

    func bootstrapIfMissing(
        migrating patch: DaemonEditableConfigurationPatch?,
        setAgentStopOnExit: Bool = false
    ) throws -> CanonicalDaemonConfiguration {
        var candidate = try show()
        guard !fileManager.fileExists(atPath: configurationURL.path) else { return candidate }
        if let patch { try candidate.apply(patch) }
        if setAgentStopOnExit { try candidate.setAgentStopOnExit(true) }
        return try persist(candidate)
    }

    private func persist(_ candidate: CanonicalDaemonConfiguration) throws -> CanonicalDaemonConfiguration {
        let data = try candidate.serializedData()
        try validate(data)
        try apply(data)
        return try show()
    }

    private func show() throws -> CanonicalDaemonConfiguration {
        let result = try run(["config", "show", "--config", configurationURL.path], input: nil)
        return try CanonicalDaemonConfiguration(data: result.standardOutput)
    }

    private func validate(_ data: Data) throws {
        _ = try run(["config", "validate", "--config", configurationURL.path, "--input", "-"], input: data)
    }

    private func apply(_ data: Data) throws {
        _ = try run(["config", "apply", "--config", configurationURL.path, "--input", "-"], input: data)
    }

    private func run(_ arguments: [String], input: Data?) throws -> DaemonCommandResult {
        let result = try runner.run(arguments: arguments, standardInput: input)
        guard result.terminationStatus == 0 else {
            let message = String(decoding: result.standardError, as: UTF8.self)
                .trimmingCharacters(in: .whitespacesAndNewlines)
            throw DaemonCommandError.nonZeroStatus(
                result.terminationStatus,
                message.isEmpty ? "未知错误" : message
            )
        }
        return result
    }
}
