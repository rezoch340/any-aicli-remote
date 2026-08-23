import Combine
import Foundation

@MainActor
final class LauncherSettings: ObservableObject {
  private let defaults: UserDefaults

  @Published var daemonPort: Int
  @Published var agentPort: Int
  @Published var bindAddress: String
  @Published var publicHost: String
  @Published private(set) var isConfigurationLoaded: Bool
  @Published var deviceName: String {
    didSet { defaults.set(deviceName, forKey: LauncherPreferenceKeys.deviceName) }
  }
  var lastLANAddress: String {
    didSet { defaults.set(lastLANAddress, forKey: LauncherPreferenceKeys.lastLANAddress) }
  }

  init(defaults: UserDefaults = .standard) {
    self.defaults = defaults
    daemonPort = 0
    agentPort = 0
    bindAddress = ""
    publicHost = ""
    isConfigurationLoaded = false
    deviceName =
      defaults.string(forKey: LauncherPreferenceKeys.deviceName)
      ?? Host.current().localizedName
      ?? ProcessInfo.processInfo.hostName
    lastLANAddress = defaults.string(forKey: LauncherPreferenceKeys.lastLANAddress) ?? ""
  }

  func migrateConfiguration(store: DaemonConfigurationStore, configurationURL: URL) throws
    -> CanonicalDaemonConfiguration
  {
    try DaemonConfigurationMigration.migrateIfNeeded(
      store: store,
      configurationURL: configurationURL,
      defaults: defaults,
      legacyDomainName: LegacyCompatibilityIdentifier.bundleIdentifier
    )
  }

  func loadDraft(from configuration: CanonicalDaemonConfiguration) throws {
    let editable = try configuration.editable
    daemonPort = editable.daemonPort
    agentPort = editable.agentPort
    bindAddress = editable.bindAddress
    publicHost = editable.publicHost
    isConfigurationLoaded = true
  }
}
