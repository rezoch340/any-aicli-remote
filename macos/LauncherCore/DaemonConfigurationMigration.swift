import Foundation

enum DaemonConfigurationMigration {
    static let currentVersion = 1

    static func migrateIfNeeded(
        store: DaemonConfigurationStore,
        configurationURL: URL,
        defaults: UserDefaults,
        legacyDomainName: String,
        fileManager: FileManager = .default
    ) throws -> CanonicalDaemonConfiguration {
        let hasConfiguration = fileManager.fileExists(atPath: configurationURL.path)
        let hasCompletedMigration = defaults.integer(forKey: LauncherPreferenceKeys.migrationVersion) >= currentVersion
        let candidatePatch = hasConfiguration || hasCompletedMigration
            ? nil
            : patch(defaults: defaults, legacyDomainName: legacyDomainName)
        let configuration = try store.bootstrapIfMissing(
            migrating: candidatePatch,
            setAgentStopOnExit: !hasConfiguration
        )
        clearServicePreferences(defaults: defaults, legacyDomainName: legacyDomainName)
        defaults.set(currentVersion, forKey: LauncherPreferenceKeys.migrationVersion)
        return configuration
    }

    static func patch(defaults: UserDefaults, legacyDomainName: String) -> DaemonEditableConfigurationPatch? {
        let legacyValues = defaults.persistentDomain(forName: legacyDomainName) ?? [:]
        func value(for key: String) -> Any? {
            defaults.object(forKey: key) ?? legacyValues[key]
        }
        let candidatePatch = DaemonEditableConfigurationPatch(
            bindAddress: value(for: LauncherPreferenceKeys.bindAddress) as? String,
            daemonPort: value(for: LauncherPreferenceKeys.daemonPort) as? Int,
            publicHost: value(for: LauncherPreferenceKeys.publicHost) as? String,
            agentPort: value(for: LauncherPreferenceKeys.agentPort) as? Int
        )
        let hasPatch = candidatePatch.bindAddress != nil
            || candidatePatch.daemonPort != nil
            || candidatePatch.publicHost != nil
            || candidatePatch.agentPort != nil
        return hasPatch ? candidatePatch : nil
    }

    private static func clearServicePreferences(defaults: UserDefaults, legacyDomainName: String) {
        for key in LauncherPreferenceKeys.serviceKeys {
            defaults.removeObject(forKey: key)
        }
        guard var legacyValues = defaults.persistentDomain(forName: legacyDomainName) else { return }
        for key in LauncherPreferenceKeys.serviceKeys {
            legacyValues.removeValue(forKey: key)
        }
        defaults.setPersistentDomain(legacyValues, forName: legacyDomainName)
    }
}
