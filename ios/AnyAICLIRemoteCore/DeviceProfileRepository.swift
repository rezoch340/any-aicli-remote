import Foundation

public protocol PairingKeyStore {
  func read(account: String) throws -> String?
  func save(_ key: String, account: String) throws
  func delete(account: String) throws
}

public struct SystemPairingKeyStore: PairingKeyStore {
  public init() {}

  public func read(account: String) throws -> String? {
    try KeychainStore.read(account: account)
  }

  public func save(_ key: String, account: String) throws {
    try KeychainStore.save(key, account: account)
  }

  public func delete(account: String) throws {
    try KeychainStore.delete(account: account)
  }
}

public struct DeviceProfileRepository {
  private static let devicesDefaultsKey = "savedDevices.v1"
  private static let keychainAccountPrefix = "pairing-key."
  private let defaults: UserDefaults
  private let keyStore: PairingKeyStore

  public init(defaults: UserDefaults = .standard, keyStore: PairingKeyStore = SystemPairingKeyStore()) {
    self.defaults = defaults
    self.keyStore = keyStore
  }

  public func loadDevices() -> DeviceLoadResult {
    guard let data = defaults.data(forKey: Self.devicesDefaultsKey) else {
      return DeviceLoadResult(devices: [], errorMessage: nil)
    }
    do {
      return DeviceLoadResult(
        devices: try JSONDecoder().decode([SavedDevice].self, from: data),
        errorMessage: nil
      )
    } catch {
      return DeviceLoadResult(
        devices: [],
        errorMessage: "设备列表读取失败：\(error.localizedDescription)"
      )
    }
  }

  public func pairingKey(for deviceID: UUID) throws -> String {
    try keyStore.read(account: Self.keychainAccount(for: deviceID)) ?? ""
  }

  @discardableResult
  public func save(
    id: UUID? = nil,
    name: String,
    address: String,
    pairingKey: String,
    devices: [SavedDevice]
  ) throws -> DeviceSaveResult {
    let deviceID = id ?? UUID()
    let account = Self.keychainAccount(for: deviceID)
    let existingKey = id == nil ? "" : try keyStore.read(account: account) ?? ""
    let profile = try ServerProfile.parse(
      address: address, fallbackKey: pairingKey.isEmpty ? existingKey : pairingKey)
    let trimmedName = name.trimmingCharacters(in: .whitespacesAndNewlines)
    let resolvedName =
      trimmedName.isEmpty ? (profile.baseURL.host ?? ProductIdentifiers.displayName) : trimmedName
    let device = SavedDevice(id: deviceID, name: resolvedName, baseURL: profile.baseURL)
    var updatedDevices = devices
    if let index = updatedDevices.firstIndex(where: { $0.id == deviceID }) {
      updatedDevices[index] = device
    } else {
      updatedDevices.append(device)
    }
    try keyStore.save(profile.key, account: account)
    do {
      try persist(updatedDevices)
    } catch {
      if existingKey.isEmpty {
        try? keyStore.delete(account: account)
      } else {
        try? keyStore.save(existingKey, account: account)
      }
      throw error
    }
    return DeviceSaveResult(deviceID: deviceID, devices: updatedDevices, device: device)
  }

  public func delete(deviceID: UUID, devices: [SavedDevice]) throws -> [SavedDevice] {
    let updatedDevices = devices.filter { $0.id != deviceID }
    guard updatedDevices.count != devices.count else { return devices }
    try persist(updatedDevices)
    do {
      try keyStore.delete(account: Self.keychainAccount(for: deviceID))
    } catch {
      try? persist(devices)
      throw error
    }
    return updatedDevices
  }

  #if DEBUG
    public static func resetStorageForUITesting() {
      let defaults = UserDefaults.standard
      let deviceIDs = (defaults.data(forKey: devicesDefaultsKey)).flatMap {
        try? JSONDecoder().decode([SavedDevice].self, from: $0)
      } ?? []
      for deviceID in deviceIDs.map(\.id) {
        try? KeychainStore.delete(account: keychainAccount(for: deviceID))
      }
      defaults.removeObject(forKey: devicesDefaultsKey)
    }
  #endif

  private func persist(_ devices: [SavedDevice]) throws {
    let data = try JSONEncoder().encode(devices)
    defaults.set(data, forKey: Self.devicesDefaultsKey)
    guard defaults.data(forKey: Self.devicesDefaultsKey) == data else {
      throw DeviceStorageError.persistenceFailed
    }
  }

  private static func keychainAccount(for deviceID: UUID) -> String {
    keychainAccountPrefix + deviceID.uuidString.lowercased()
  }
}

public struct DeviceLoadResult {
  public let devices: [SavedDevice]
  public let errorMessage: String?
}

public struct DeviceSaveResult {
  public let deviceID: UUID
  public let devices: [SavedDevice]
  public let device: SavedDevice
}

public enum DeviceStorageError: LocalizedError {
  case persistenceFailed

  public var errorDescription: String? { "设备列表保存失败" }
}
