import Foundation

enum KeychainStore {
    static func save(_ value: String, account: String) throws {
        try save(value, account: account, namespace: currentLocation)
    }

    static func read(account: String) throws -> String? {
        for namespace in lookupLocations {
            guard let value = try read(account: account, namespace: namespace) else { continue }
            if namespace != currentLocation {
                // Copy first and retain the legacy item until an explicit delete. This makes
                // bundle/service migration recoverable if the app exits between operations.
                try save(value, account: account, namespace: currentLocation)
            }
            return value
        }
        return nil
    }

    static func delete(account: String) throws {
        for namespace in lookupLocations {
            try delete(account: account, namespace: namespace)
        }
    }

    private static func save(_ value: String, account: String, namespace: KeychainNamespace) throws {
        try GenericPasswordStore.save(
            Data(value.utf8),
            at: namespace.location(account: account)
        )
    }

    private static func read(account: String, namespace: KeychainNamespace) throws -> String? {
        guard let data = try GenericPasswordStore.read(at: namespace.location(account: account)) else {
            return nil
        }
        guard let value = String(data: data, encoding: .utf8) else {
            throw KeychainStoreError.invalidData
        }
        return value
    }

    private static func delete(account: String, namespace: KeychainNamespace) throws {
        try GenericPasswordStore.delete(at: namespace.location(account: account))
    }

    private static var currentLocation: KeychainNamespace {
        let bundleIdentifier = ProductIdentifiers.bundleIdentifier
        return KeychainNamespace(
            service: bundleIdentifier,
            accessGroup: ProductIdentifiers.keychainAccessGroup(for: bundleIdentifier)
        )
    }

    private static var lookupLocations: [KeychainNamespace] {
        let currentBundleIdentifier = ProductIdentifiers.bundleIdentifier
        let legacyBundleIdentifier = LegacyCompatibility.bundleIdentifier
        var locations = [
            currentLocation,
            KeychainNamespace(service: currentBundleIdentifier, accessGroup: nil),
            KeychainNamespace(
                service: legacyBundleIdentifier,
                accessGroup: ProductIdentifiers.keychainAccessGroup(for: legacyBundleIdentifier)
            ),
            KeychainNamespace(service: legacyBundleIdentifier, accessGroup: nil)
        ]
        var seen = Set<KeychainNamespace>()
        locations.removeAll { !seen.insert($0).inserted }
        return locations
    }
}

private struct KeychainNamespace: Hashable {
    let service: String
    let accessGroup: String?

    func location(account: String) -> GenericPasswordLocation {
        GenericPasswordLocation(
            service: service,
            account: account,
            accessGroup: accessGroup
        )
    }
}

private enum KeychainStoreError: LocalizedError {
    case invalidData

    var errorDescription: String? {
        "钥匙串中的配对 Key 格式无效"
    }
}
