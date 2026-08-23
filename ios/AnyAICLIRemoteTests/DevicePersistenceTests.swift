import XCTest

@testable import AnyAICLIRemoteFeature
@testable import AnyAICLIRemoteCore

@MainActor
final class DevicePersistenceTests: XCTestCase {
  func testImportedDevicesPersistAcrossStoreReconstruction() throws {
    let fixtureID = UUID().uuidString.lowercased()
    let fixtures = [
      ("alpha-\(fixtureID)", "https://alpha-\(fixtureID).test:8443", "key-alpha-\(fixtureID)"),
      ("beta-\(fixtureID)", "http://beta-\(fixtureID).test:9555", "key-beta-\(fixtureID)")
    ]
    let defaults = UserDefaults(suiteName: "DevicePersistenceTests.\(fixtureID)")!
    defaults.removePersistentDomain(forName: "DevicePersistenceTests.\(fixtureID)")
    let keyStore = TestPairingKeyStore()
    let store = ChatStore(deviceRepository: DeviceProfileRepository(defaults: defaults, keyStore: keyStore))
    var importedIDs: [UUID] = []
    defer {
      for deviceID in importedIDs { try? store.deleteDevice(deviceID) }
    }

    for (name, address, pairingKey) in fixtures {
      var components = URLComponents()
      components.scheme = ProductIdentifiers.pairingScheme
      components.host = PairingDeepLink.pairingHost
      components.queryItems = [
        URLQueryItem(name: PairingDeepLink.serviceURLField, value: address),
        URLQueryItem(name: PairingDeepLink.pairingKeyField, value: pairingKey),
        URLQueryItem(name: PairingDeepLink.displayNameField, value: name)
      ]
      let parsed = try PairingDeepLink.parse(try XCTUnwrap(components.url))
      XCTAssertFalse(parsed.profile.baseURL.absoluteString.contains(pairingKey))
      XCTAssertTrue(store.importPairingDeepLink(try XCTUnwrap(components.url)))
      let device = try XCTUnwrap(store.devices.first { $0.baseURL == parsed.profile.baseURL })
      importedIDs.append(device.id)
    }

    let reconstructedStore = ChatStore(deviceRepository: DeviceProfileRepository(defaults: defaults, keyStore: keyStore))
    XCTAssertEqual(importedIDs.count, 2)
    for (index, fixture) in fixtures.enumerated() {
      let address = try XCTUnwrap(URL(string: fixture.1))
      let device = try XCTUnwrap(reconstructedStore.devices.first { $0.baseURL == address })
      XCTAssertEqual(try reconstructedStore.pairingKey(for: device.id), fixture.2)
      XCTAssertFalse(device.baseURL.absoluteString.contains(fixture.2))
      XCTAssertEqual(device.name, fixture.0)
      XCTAssertEqual(device.id, importedIDs[index])
    }
  }
}

private final class TestPairingKeyStore: PairingKeyStore {
  private var values: [String: String] = [:]

  func read(account: String) throws -> String? { values[account] }
  func save(_ key: String, account: String) throws { values[account] = key }
  func delete(account: String) throws { values.removeValue(forKey: account) }
}
