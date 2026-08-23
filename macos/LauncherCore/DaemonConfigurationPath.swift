import Foundation

enum DaemonConfigurationPath {
    static let stateDirectoryName = ".any-aicli-remote"
    static let configurationFileName = "config.json"

    static func defaultURL(fileManager: FileManager = .default) -> URL {
        fileManager.homeDirectoryForCurrentUser
            .appendingPathComponent(stateDirectoryName, isDirectory: true)
            .appendingPathComponent(configurationFileName, isDirectory: false)
    }
}
