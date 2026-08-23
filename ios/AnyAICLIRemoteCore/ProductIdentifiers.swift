import Foundation

public enum ProductIdentifiers {
    static var bundleIdentifier: String {
        guard let bundleIdentifier = Bundle.main.bundleIdentifier,
              isResolved(bundleIdentifier) else {
            preconditionFailure("Missing resolved application bundle identifier")
        }
        return bundleIdentifier
    }

    public static var displayName: String {
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

    fileprivate static func requiredBundleString(forKey key: String) -> String {
        guard let value = optionalBundleString(forKey: key) else {
            preconditionFailure("Missing resolved bundle value for \(key)")
        }
        return value
    }

    private static func optionalBundleString(forKey key: String) -> String? {
        let bundles = [Bundle.main] + Bundle.allBundles.filter { $0 != Bundle.main }
        for bundle in bundles {
            guard let value = bundle.object(forInfoDictionaryKey: key) as? String,
                  isResolved(value) else {
                continue
            }
            return value
        }
        return nil
    }

    private static func isResolved(_ value: String) -> Bool {
        !value.isEmpty && !value.contains("$(")
    }
}
