import Foundation

enum LauncherPreferenceKeys {
    static let daemonPort = "daemonPort"
    static let agentPort = "agentPort"
    static let bindAddress = "bindAddress"
    static let publicHost = "publicHost"
    static let deviceName = "deviceName"
    static let lastLANAddress = "lastLANAddress"
    static let migrationVersion = "daemonConfigurationMigrationVersion"
    static let serviceKeys = [daemonPort, agentPort, bindAddress, publicHost]
}
