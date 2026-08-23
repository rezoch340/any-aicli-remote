import Foundation

struct DaemonCommandResult: Equatable {
    let standardOutput: Data
    let standardError: Data
    let terminationStatus: Int32
}

protocol DaemonCommandRunning {
    func run(arguments: [String], standardInput: Data?) throws -> DaemonCommandResult
}

enum DaemonCommandError: LocalizedError {
    case nonZeroStatus(Int32, String)

    var errorDescription: String? {
        guard case let .nonZeroStatus(status, message) = self else { return nil }
        return "Daemon 配置命令失败（\(status)）：\(message)"
    }
}

struct ProcessDaemonCommandRunner: DaemonCommandRunning {
    let executableURL: URL
    let fileManager: FileManager
    let temporaryRootURL: URL?

    init(executableURL: URL, fileManager: FileManager = .default, temporaryRootURL: URL? = nil) {
        self.executableURL = executableURL
        self.fileManager = fileManager
        self.temporaryRootURL = temporaryRootURL
    }

    func run(arguments: [String], standardInput: Data?) throws -> DaemonCommandResult {
        let rootURL = temporaryRootURL ?? fileManager.temporaryDirectory
        let directoryURL = rootURL.appendingPathComponent(UUID().uuidString, isDirectory: true)
        try fileManager.createDirectory(at: directoryURL, withIntermediateDirectories: true, attributes: [.posixPermissions: 0o700])
        defer { try? fileManager.removeItem(at: directoryURL) }

        let inputURL = directoryURL.appendingPathComponent("input")
        let outputURL = directoryURL.appendingPathComponent("output")
        let errorURL = directoryURL.appendingPathComponent("error")
        try createPrivateFile(at: inputURL, contents: standardInput ?? Data())
        try createPrivateFile(at: outputURL, contents: Data())
        try createPrivateFile(at: errorURL, contents: Data())

        let process = Process()
        process.executableURL = executableURL
        process.arguments = arguments
        process.environment = DaemonLaunchEnvironment.inheritedSanitized()
        let inputHandle = try FileHandle(forReadingFrom: inputURL)
        let outputHandle = try FileHandle(forWritingTo: outputURL)
        let errorHandle = try FileHandle(forWritingTo: errorURL)
        process.standardInput = inputHandle
        process.standardOutput = outputHandle
        process.standardError = errorHandle
        try process.run()
        process.waitUntilExit()
        try inputHandle.close()
        try outputHandle.close()
        try errorHandle.close()

        return DaemonCommandResult(
            standardOutput: try Data(contentsOf: outputURL),
            standardError: try Data(contentsOf: errorURL),
            terminationStatus: process.terminationStatus
        )
    }

    private func createPrivateFile(at fileURL: URL, contents: Data) throws {
        guard fileManager.createFile(atPath: fileURL.path, contents: contents, attributes: [.posixPermissions: 0o600]) else {
            throw CocoaError(.fileWriteUnknown)
        }
    }
}
