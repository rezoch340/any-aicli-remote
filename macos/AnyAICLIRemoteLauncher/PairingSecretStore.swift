import Foundation

enum PairingSecretStore {
  private static let keychainLocation = GenericPasswordLocation(
    service: ProductIdentifier.bundleIdentifier,
    account: ProductIdentifier.pairingSecretAccountName
  )

  static func loadOrCreate() throws -> String {
    if let storedData = try GenericPasswordStore.read(
      at: keychainLocation,
      policy: .dataProtectionPreferred
    ) {
      guard let storedSecret = validSecret(in: storedData) else {
        throw PairingSecretStoreError.invalidKeychainSecret
      }
      try removePersistedSecretFiles()
      return storedSecret
    }

    if let migratedSecret = try migratedFileSecret() {
      try GenericPasswordStore.save(
        Data(migratedSecret.utf8),
        at: keychainLocation,
        policy: .dataProtectionPreferred
      )
      try removePersistedSecretFiles()
      return migratedSecret
    }

    let generatedSecret = generateSecret()
    try GenericPasswordStore.save(
      Data(generatedSecret.utf8),
      at: keychainLocation,
      policy: .dataProtectionPreferred
    )
    try removePersistedSecretFiles()
    return generatedSecret
  }

  private static func migratedFileSecret() throws -> String? {
    for fileURL in try persistedSecretFileURLs() {
      guard FileManager.default.fileExists(atPath: fileURL.path) else { continue }
      let resourceValues = try fileURL.resourceValues(forKeys: [
        .isRegularFileKey,
        .isSymbolicLinkKey,
      ])
      let attributes = try FileManager.default.attributesOfItem(atPath: fileURL.path)
      let permissions = (attributes[.posixPermissions] as? NSNumber)?.intValue ?? 0
      guard resourceValues.isRegularFile == true,
        resourceValues.isSymbolicLink != true,
        permissions & 0o777 == 0o600
      else {
        throw PairingSecretStoreError.insecureMigrationFile(fileURL)
      }
      let data = try Data(contentsOf: fileURL, options: .uncached)
      guard let secret = validSecret(in: data) else {
        throw PairingSecretStoreError.invalidMigrationFile(fileURL)
      }
      return secret
    }
    return nil
  }

  private static func persistedSecretFileURLs() throws -> [URL] {
    let baseApplicationSupportDirectory = try FileManager.default.url(
      for: .applicationSupportDirectory,
      in: .userDomainMask,
      appropriateFor: nil,
      create: true
    )
    return [
      ProductIdentifier.applicationSupportDirectoryName,
      LegacyCompatibilityIdentifier.applicationSupportDirectoryName,
    ].map { directoryName in
      baseApplicationSupportDirectory
        .appendingPathComponent(directoryName, isDirectory: true)
        .appendingPathComponent(ProductIdentifier.pairingSecretAccountName)
    }
  }

  private static func removePersistedSecretFiles() throws {
    for fileURL in try persistedSecretFileURLs() {
      do {
        try FileManager.default.removeItem(at: fileURL)
      } catch let cocoaError as CocoaError where cocoaError.code == .fileNoSuchFile {
        continue
      }
    }
  }

  private static func validSecret(in data: Data) -> String? {
    guard
      let value = String(data: data, encoding: .utf8)?
        .trimmingCharacters(in: .whitespacesAndNewlines),
      value.count >= 16
    else {
      return nil
    }
    return value
  }

  private static func generateSecret() -> String {
    var generator = SystemRandomNumberGenerator()
    return (0..<32).map { _ in
      String(format: "%02x", UInt8.random(in: .min ... .max, using: &generator))
    }.joined()
  }
}

enum EphemeralDaemonSecretFile {
  static func materialize(secret: String) throws -> URL {
    let fileURL = FileManager.default.temporaryDirectory.appendingPathComponent(
      ProductIdentifier.temporarySecretFilePrefix + UUID().uuidString
    )
    let attributes: [FileAttributeKey: Any] = [.posixPermissions: 0o600]
    guard
      FileManager.default.createFile(
        atPath: fileURL.path,
        contents: Data((secret + "\n").utf8),
        attributes: attributes
      )
    else {
      throw PairingSecretStoreError.cannotMaterializeTemporaryFile
    }
    do {
      try FileManager.default.setAttributes(attributes, ofItemAtPath: fileURL.path)
      let createdAttributes = try FileManager.default.attributesOfItem(atPath: fileURL.path)
      let permissions = (createdAttributes[.posixPermissions] as? NSNumber)?.intValue ?? 0
      guard permissions & 0o777 == 0o600 else {
        throw PairingSecretStoreError.insecureTemporaryFile
      }
      return fileURL
    } catch {
      try? FileManager.default.removeItem(at: fileURL)
      throw error
    }
  }

  static func remove(at fileURL: URL) throws {
    do {
      try FileManager.default.removeItem(at: fileURL)
    } catch let cocoaError as CocoaError where cocoaError.code == .fileNoSuchFile {
      return
    }
  }
}

private enum PairingSecretStoreError: LocalizedError {
  case cannotMaterializeTemporaryFile
  case insecureMigrationFile(URL)
  case insecureTemporaryFile
  case invalidKeychainSecret
  case invalidMigrationFile(URL)

  var errorDescription: String? {
    switch self {
    case .cannotMaterializeTemporaryFile:
      return "无法创建 daemon 临时密钥文件"
    case .insecureMigrationFile(let fileURL):
      return "拒绝迁移权限不是 0600 的密钥文件：\(fileURL.path)"
    case .insecureTemporaryFile:
      return "daemon 临时密钥文件权限不是 0600"
    case .invalidKeychainSecret:
      return "钥匙串中的配对密钥格式无效"
    case .invalidMigrationFile(let fileURL):
      return "旧配对密钥文件格式无效：\(fileURL.path)"
    }
  }
}
