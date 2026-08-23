import Foundation

enum ProductIdentifiers {
    static var bundleIdentifier: String {
        guard let bundleIdentifier = Bundle.main.bundleIdentifier,
              isResolved(bundleIdentifier) else {
            preconditionFailure("Missing resolved application bundle identifier")
        }
        return bundleIdentifier
    }

    static var displayName: String {
        requiredBundleString(forKey: "CFBundleDisplayName")
    }

    static var pairingScheme: String {
        requiredBundleString(forKey: "AnyAICLIRemoteProductPairingScheme")
    }

    static var authorizationHeader: String {
        requiredBundleString(forKey: "AnyAICLIRemoteProductAuthorizationHeader")
    }

    static var clientName: String {
        requiredBundleString(forKey: "AnyAICLIRemoteProductClientName")
    }

    static var clientVersion: String {
        requiredBundleString(forKey: "CFBundleShortVersionString")
    }

    static func authorize(_ request: inout URLRequest, key: String) {
        request.setValue(key, forHTTPHeaderField: authorizationHeader)
    }

    static func keychainAccessGroup(for bundleIdentifier: String) -> String? {
        guard let prefix = optionalBundleString(forKey: "AnyAICLIRemoteKeychainAccessGroupPrefix") else {
            return nil
        }
        return prefix + bundleIdentifier
    }

    fileprivate static func requiredBundleString(forKey key: String) -> String {
        guard let value = optionalBundleString(forKey: key) else {
            preconditionFailure("Missing resolved bundle value for \(key)")
        }
        return value
    }

    private static func optionalBundleString(forKey key: String) -> String? {
        guard let value = Bundle.main.object(forInfoDictionaryKey: key) as? String,
              isResolved(value) else {
            return nil
        }
        return value
    }

    private static func isResolved(_ value: String) -> Bool {
        !value.isEmpty && !value.contains("$(")
    }
}

enum LegacyCompatibility {
    static var pairingScheme: String {
        ProductIdentifiers.requiredBundleString(forKey: "AnyAICLIRemoteLegacyPairingScheme")
    }

    static var authorizationHeader: String {
        ProductIdentifiers.requiredBundleString(forKey: "AnyAICLIRemoteLegacyAuthorizationHeader")
    }

    static var bundleIdentifier: String {
        ProductIdentifiers.requiredBundleString(forKey: "AnyAICLIRemoteLegacyBundleIdentifier")
    }

    static func supportsPairingScheme(_ candidateScheme: String?) -> Bool {
        guard let normalizedScheme = candidateScheme?.lowercased() else { return false }
        return normalizedScheme == ProductIdentifiers.pairingScheme || normalizedScheme == pairingScheme
    }

}
