import Foundation
import Security

struct GenericPasswordLocation: Hashable {
    let service: String
    let account: String
    let accessGroup: String?

    init(service: String, account: String, accessGroup: String? = nil) {
        precondition(!service.isEmpty, "Keychain service must not be empty")
        precondition(!account.isEmpty, "Keychain account must not be empty")
        precondition(accessGroup?.isEmpty != true, "Keychain access group must not be empty")
        self.service = service
        self.account = account
        self.accessGroup = accessGroup
    }
}

enum GenericPasswordStoragePolicy {
    case dataProtectionRequired

    #if os(macOS)
    case dataProtectionPreferred
    case fileBased
    #endif
}

enum GenericPasswordStore {
    static func save(
        _ data: Data,
        at location: GenericPasswordLocation,
        policy: GenericPasswordStoragePolicy = .dataProtectionRequired
    ) throws {
        #if os(macOS)
        switch policy {
        case .dataProtectionRequired:
            try save(data, at: location, usesDataProtection: true)
        case .fileBased:
            try save(data, at: location, usesDataProtection: false)
        case .dataProtectionPreferred:
            do {
                try save(data, at: location, usesDataProtection: true)
                try delete(at: location, usesDataProtection: false)
            } catch let storeError as GenericPasswordStoreError where storeError.isMissingEntitlement {
                try save(data, at: location, usesDataProtection: false)
            }
        }
        #else
        try save(data, at: location, usesDataProtection: true)
        #endif
    }

    static func read(
        at location: GenericPasswordLocation,
        policy: GenericPasswordStoragePolicy = .dataProtectionRequired
    ) throws -> Data? {
        #if os(macOS)
        switch policy {
        case .dataProtectionRequired:
            return try read(at: location, usesDataProtection: true)
        case .fileBased:
            return try read(at: location, usesDataProtection: false)
        case .dataProtectionPreferred:
            do {
                if let protectedData = try read(at: location, usesDataProtection: true) {
                    return protectedData
                }
            } catch let storeError as GenericPasswordStoreError where storeError.isMissingEntitlement {
                return try read(at: location, usesDataProtection: false)
            }

            guard let fileBasedData = try read(at: location, usesDataProtection: false) else {
                return nil
            }
            do {
                try save(fileBasedData, at: location, usesDataProtection: true)
                try delete(at: location, usesDataProtection: false)
            } catch let storeError as GenericPasswordStoreError where storeError.isMissingEntitlement {
                return fileBasedData
            }
            return fileBasedData
        }
        #else
        return try read(at: location, usesDataProtection: true)
        #endif
    }

    static func delete(
        at location: GenericPasswordLocation,
        policy: GenericPasswordStoragePolicy = .dataProtectionRequired
    ) throws {
        #if os(macOS)
        switch policy {
        case .dataProtectionRequired:
            try delete(at: location, usesDataProtection: true)
        case .fileBased:
            try delete(at: location, usesDataProtection: false)
        case .dataProtectionPreferred:
            do {
                try delete(at: location, usesDataProtection: true)
            } catch let storeError as GenericPasswordStoreError where storeError.isMissingEntitlement {
                try delete(at: location, usesDataProtection: false)
                return
            }
            try delete(at: location, usesDataProtection: false)
        }
        #else
        try delete(at: location, usesDataProtection: true)
        #endif
    }

    private static func save(
        _ data: Data,
        at location: GenericPasswordLocation,
        usesDataProtection: Bool
    ) throws {
        let matchQuery = query(for: location, usesDataProtection: usesDataProtection)
        let updatedAttributes: [String: Any] = [
            kSecValueData as String: data,
            kSecAttrAccessible as String: kSecAttrAccessibleAfterFirstUnlockThisDeviceOnly,
        ]
        let updateStatus = SecItemUpdate(
            matchQuery as CFDictionary,
            updatedAttributes as CFDictionary
        )
        if updateStatus == errSecSuccess { return }
        guard updateStatus == errSecItemNotFound else {
            throw GenericPasswordStoreError(operation: "更新", status: updateStatus)
        }

        var addedAttributes = matchQuery
        addedAttributes[kSecValueData as String] = data
        addedAttributes[kSecAttrAccessible as String] = kSecAttrAccessibleAfterFirstUnlockThisDeviceOnly
        let addStatus = SecItemAdd(addedAttributes as CFDictionary, nil)
        if addStatus == errSecSuccess { return }

        // Another writer may have inserted the same generic-password primary key
        // between the update and add operations. Retrying the update preserves upsert semantics.
        if addStatus == errSecDuplicateItem {
            let retryStatus = SecItemUpdate(
                matchQuery as CFDictionary,
                updatedAttributes as CFDictionary
            )
            guard retryStatus == errSecSuccess else {
                throw GenericPasswordStoreError(operation: "更新", status: retryStatus)
            }
            return
        }
        throw GenericPasswordStoreError(operation: "保存", status: addStatus)
    }

    private static func read(
        at location: GenericPasswordLocation,
        usesDataProtection: Bool
    ) throws -> Data? {
        var matchQuery = query(for: location, usesDataProtection: usesDataProtection)
        matchQuery[kSecReturnData as String] = true
        matchQuery[kSecMatchLimit as String] = kSecMatchLimitOne
        var result: CFTypeRef?
        let copyStatus = SecItemCopyMatching(matchQuery as CFDictionary, &result)
        if copyStatus == errSecItemNotFound { return nil }
        guard copyStatus == errSecSuccess else {
            throw GenericPasswordStoreError(operation: "读取", status: copyStatus)
        }
        guard let data = result as? Data else {
            throw GenericPasswordStoreError.invalidData
        }
        return data
    }

    private static func delete(
        at location: GenericPasswordLocation,
        usesDataProtection: Bool
    ) throws {
        let deleteStatus = SecItemDelete(
            query(for: location, usesDataProtection: usesDataProtection) as CFDictionary
        )
        guard deleteStatus == errSecSuccess || deleteStatus == errSecItemNotFound else {
            throw GenericPasswordStoreError(operation: "删除", status: deleteStatus)
        }
    }

    private static func query(
        for location: GenericPasswordLocation,
        usesDataProtection: Bool
    ) -> [String: Any] {
        var query: [String: Any] = [
            kSecClass as String: kSecClassGenericPassword,
            kSecAttrService as String: location.service,
            kSecAttrAccount as String: location.account,
        ]
        if usesDataProtection {
            // Apple recommends the data-protection keychain for portable SecItem behavior.
            // Other platforms already use it and safely ignore this query key.
            query[kSecUseDataProtectionKeychain as String] = true
        }
        if let accessGroup = location.accessGroup {
            query[kSecAttrAccessGroup as String] = accessGroup
        }
        return query
    }
}

enum GenericPasswordStoreError: LocalizedError {
    case invalidData
    case status(operation: String, code: OSStatus)

    init(operation: String, status: OSStatus) {
        self = .status(operation: operation, code: status)
    }

    var isMissingEntitlement: Bool {
        guard case .status(_, let code) = self else { return false }
        return code == errSecMissingEntitlement
    }

    var errorDescription: String? {
        switch self {
        case .invalidData:
            return "钥匙串项目的数据格式无效"
        case .status(let operation, let code):
            let detail = SecCopyErrorMessageString(code, nil) as String? ?? "OSStatus \(code)"
            return "钥匙串\(operation)失败：\(detail)"
        }
    }
}
