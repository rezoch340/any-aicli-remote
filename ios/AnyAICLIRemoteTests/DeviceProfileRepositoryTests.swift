import XCTest
@testable import AnyAICLIRemoteCore

final class DeviceProfileRepositoryTests: XCTestCase {
  private let devicesKey = "savedDevices.v1"

  func testFreshDefaultsLoadNoDevicesWithoutKeychainAccess() {
    let defaults = makeDefaults()
    let keyStore = TrackingPairingKeyStore()
    let result = DeviceProfileRepository(defaults: defaults, keyStore: keyStore).loadDevices()

    XCTAssertEqual(result.devices, [])
    XCTAssertNil(result.errorMessage)
    XCTAssertEqual(keyStore.readCalls, 0)
    XCTAssertEqual(keyStore.saveCalls, 0)
    XCTAssertEqual(keyStore.deleteCalls, 0)
  }

  func testLoadDevicesReadsCurrentSavedDevices() throws {
    let defaults = makeDefaults()
    let expected = [SavedDevice(
      id: UUID(), name: "Office", baseURL: try XCTUnwrap(URL(string: "https://example.com")))]
    defaults.set(try JSONEncoder().encode(expected), forKey: devicesKey)

    let result = DeviceProfileRepository(defaults: defaults, keyStore: TrackingPairingKeyStore()).loadDevices()

    XCTAssertEqual(result.devices, expected)
    XCTAssertNil(result.errorMessage)
  }

  func testLoadDevicesKeepsCorruptCurrentData() {
    let defaults = makeDefaults()
    let corrupt = Data("not-json".utf8)
    defaults.set(corrupt, forKey: devicesKey)

    let result = DeviceProfileRepository(defaults: defaults, keyStore: TrackingPairingKeyStore()).loadDevices()

    XCTAssertEqual(result.devices, [])
    XCTAssertTrue(result.errorMessage?.contains("设备列表读取失败") == true)
    XCTAssertEqual(defaults.data(forKey: devicesKey), corrupt)
  }

  func testDeleteRestoresDevicesWhenKeyDeletionFails() throws {
    let defaults = makeDefaults()
    let device = SavedDevice(
      id: UUID(), name: "Office", baseURL: try XCTUnwrap(URL(string: "https://example.com")))
    let keyStore = TrackingPairingKeyStore(deleteError: DeviceStorageError.persistenceFailed)
    let repository = DeviceProfileRepository(defaults: defaults, keyStore: keyStore)

    XCTAssertThrowsError(try repository.delete(deviceID: device.id, devices: [device]))
    XCTAssertEqual(
      try JSONDecoder().decode([SavedDevice].self, from: try XCTUnwrap(defaults.data(forKey: devicesKey))),
      [device]
    )
  }

  private func makeDefaults() -> UserDefaults {
    let suiteName = "DeviceProfileRepositoryTests.\(UUID().uuidString)"
    let defaults = UserDefaults(suiteName: suiteName)!
    defaults.removePersistentDomain(forName: suiteName)
    return defaults
  }
}

private final class TrackingPairingKeyStore: PairingKeyStore {
  private let deleteError: Error?
  private(set) var readCalls = 0
  private(set) var saveCalls = 0
  private(set) var deleteCalls = 0

  init(deleteError: Error? = nil) {
    self.deleteError = deleteError
  }

  func read(account: String) throws -> String? {
    readCalls += 1
    return nil
  }

  func save(_ key: String, account: String) throws {
    saveCalls += 1
  }

  func delete(account: String) throws {
    deleteCalls += 1
    if let deleteError {
      throw deleteError
    }
  }
}
