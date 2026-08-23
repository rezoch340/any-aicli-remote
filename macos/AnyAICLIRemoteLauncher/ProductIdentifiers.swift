enum ProductIdentifier {
  static let applicationSupportDirectoryName = "AnyAICLIRemoteLauncher"
  static let authenticationHeaderName = "X-Any-AI-CLI-Remote-Key"
  static let bundleIdentifier = "com.anyaicliremote.launcher"
  static let daemonExecutableName = "any-aicli-remote-daemon"
  static let deepLinkScheme = "anyaicliremote"
  static let deepLinkHost = "pair"
  static let pairingSecretAccountName = "pairing-secret"
  static let temporarySecretFilePrefix = "any-aicli-remote-daemon-secret-"
}

// Read-only compatibility identifiers. New data is always written with ProductIdentifier.
enum LegacyCompatibilityIdentifier {
  static let applicationSupportDirectoryName = "GrokRemoteLauncher"
  static let bundleIdentifier = "com.grokremote.launcher"
}
