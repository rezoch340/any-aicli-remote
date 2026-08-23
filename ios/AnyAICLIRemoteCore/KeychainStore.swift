import Foundation

public enum KeychainStore {
  static func save(_ value: String, account: String) throws {
    try GenericPasswordStore.save(Data(value.utf8), at: currentLocation(account: account))
  }

  static func read(account: String) throws -> String? {
    guard let data = try GenericPasswordStore.read(at: currentLocation(account: account)) else {
      return nil
    }
    guard let value = String(data: data, encoding: .utf8) else {
      throw KeychainStoreError.invalidData
    }
    return value
  }

  static func delete(account: String) throws {
    try GenericPasswordStore.delete(at: currentLocation(account: account))
  }

  private static func currentLocation(account: String) -> GenericPasswordLocation {
    GenericPasswordLocation(service: ProductIdentifiers.bundleIdentifier, account: account)
  }
}

private enum KeychainStoreError: LocalizedError {
  case invalidData

  var errorDescription: String? {
    "钥匙串中的配对 Key 格式无效"
  }
}
